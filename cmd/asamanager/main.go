package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"asamanager/internal/applog"
	"asamanager/internal/backup"
	"asamanager/internal/cluster"
	"asamanager/internal/config"
	"asamanager/internal/db"
	"asamanager/internal/discord/webhook"
	"asamanager/internal/events"
	"asamanager/internal/gui"
	"asamanager/internal/i18n"
	"asamanager/internal/players"
	"asamanager/internal/preset"
	"asamanager/internal/scheduler"
	"asamanager/internal/server"
	"asamanager/internal/steamcmd"
)

func main() {
	for _, a := range os.Args[1:] {
		if a == "-debug" || a == "--debug" {
			attachDebugConsole()
			break
		}
	}

	dataDir, err := config.DefaultDataDir()
	if err != nil {
		log.Fatalf("resolve data dir: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	logger, logCloser, err := applog.Setup(dataDir)
	if err != nil {
		log.Fatalf("setup logger: %v", err)
	}
	defer logCloser.Close()

	cfg, err := config.Load(dataDir)
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}
	if !fileExists(config.Path(dataDir)) {
		if err := config.Save(cfg); err != nil {
			logger.Error("save default config", "err", err)
		}
	}

	// Localization: read translator-supplied .toml files from a
	// "language" folder next to the executable. If the folder doesn't
	// exist or the configured locale isn't there, Bundle silently
	// falls back to the embedded en-us file so the app still runs.
	languageDir := languageDirNextToExe(dataDir)
	bundle, err := i18n.Load(languageDir, cfg.Locale)
	if err != nil {
		logger.Error("load language bundle", "err", err, "dir", languageDir, "locale", cfg.Locale)
		bundle = i18n.Default()
	}
	logger.Info("i18n bundle loaded",
		"requested", cfg.Locale, "active", bundle.Locale(),
		"language_dir", languageDir, "available", i18n.AvailableLocales(languageDir))

	dbPath := filepath.Join(dataDir, "asamanager.db")
	database, err := db.Open(dbPath)
	if err != nil {
		logger.Error("open database", "err", err, "path", dbPath)
		os.Exit(1)
	}
	defer database.Close()

	bus := events.NewBus(512)
	bus.Start()
	defer bus.Stop()

	clusterRepo := cluster.NewRepo(database)
	serverRepo := server.NewRepo(database)
	playersRepo := players.NewRepo(database)

	// Reset stale running/starting/stopping rows from a prior hard exit.
	if err := server.ReconcileOnStartup(context.Background(), serverRepo, logger); err != nil {
		logger.Error("reconcile on startup", "err", err)
	}

	// SteamCMD runner: prefer the path persisted in config; otherwise
	// detect and remember.
	steamRunner := steamcmd.New()
	if cfg.SteamCMDPath != "" {
		steamRunner.SetPath(cfg.SteamCMDPath)
	} else if p, _ := steamRunner.Detect(context.Background()); p != "" {
		cfg.SteamCMDPath = p
		_ = config.Save(cfg)
	}

	launchOpts := func(ctx context.Context, srv server.Server) (server.LaunchOptions, error) {
		// Standalone servers (no cluster) skip the cluster lookup. The
		// launch builder already conditionally emits -clusterid= and
		// -ClusterDirOverride=, so empty strings are the correct
		// "no flag" signal.
		if srv.ClusterID == 0 {
			return server.LaunchOptions{
				ServerPassword: srv.ServerPassword,
				MaxPlayers:     srv.MaxPlayers,
				Mods:           enabledModIDs(server.MergeMods(nil, srv.ActiveMods)),
				ActiveEvent:    server.ResolveActiveEvent(srv.ActiveEvent, ""),
			}, nil
		}
		c, err := clusterRepo.Get(ctx, srv.ClusterID)
		if err != nil {
			return server.LaunchOptions{}, err
		}
		return server.LaunchOptions{
			ClusterID:      c.ClusterID,
			ClusterDir:     c.ClusterDir,
			ServerPassword: srv.ServerPassword,
			MaxPlayers:     srv.MaxPlayers,
			Mods:           enabledModIDs(server.MergeMods(c.ActiveMods, srv.ActiveMods)),
			ActiveEvent:    server.ResolveActiveEvent(srv.ActiveEvent, c.ActiveEvent),
		}, nil
	}

	supervisor := server.NewSupervisor(server.SupervisorDeps{
		Repo:          serverRepo,
		Bus:           bus,
		Log:           logger,
		LaunchOptions: launchOpts,
		LogDir:        filepath.Join(dataDir, "logs"),
	})

	coordinator := &server.Coordinator{
		Sup:     supervisor,
		Repo:    serverRepo,
		Stagger: time.Duration(cfg.Schedule.StaggerSeconds) * time.Second,
		Log:     logger,
	}

	tracker := players.NewTracker(players.TrackerDeps{
		Repo:         playersRepo,
		Sup:          supervisor,
		Bus:          bus,
		Log:          logger,
		PollInterval: time.Duration(cfg.Schedule.PlayerPollSeconds) * time.Second,
	})
	trackerStop := tracker.Start()
	defer trackerStop()

	actions := players.NewActions(players.ActionsDeps{
		Repo: playersRepo,
		Sup:  supervisor,
		Bus:  bus,
		Log:  logger,
	})

	webhookRepo := webhook.NewRepo(database)
	webhookDispatcher := webhook.NewDispatcher(webhook.DispatcherDeps{
		Repo:   webhookRepo,
		Sender: webhook.NewHTTPSender(),
		Bus:    bus,
		DB:     database,
		Log:    logger,
	})
	webhookStop := webhookDispatcher.Start()
	defer webhookStop()

	backupDir := cfg.BackupDir
	if backupDir == "" {
		backupDir = filepath.Join(dataDir, "backups")
	}
	asbDir := cfg.ASBDir
	if asbDir == "" {
		asbDir = filepath.Join(dataDir, "asb-multipliers")
	}

	backupRepo := backup.NewRepo(database)
	backupManager := backup.NewManager(backup.ManagerDeps{
		Repo: backupRepo, Servers: serverRepo, Clusters: clusterRepo,
		Bus: bus, Log: logger,
		DestDir: backupDir, KeepCount: cfg.Backup.KeepCount,
	})

	presetRepo := preset.NewRepo(database)
	presetManager := preset.NewManager(preset.ManagerDeps{
		Presets: presetRepo, Servers: serverRepo, Clusters: clusterRepo,
		Bus: bus, Log: logger,
	})

	schedulerRepo := scheduler.NewRepo(database)
	schedulerEngine := scheduler.NewEngine(scheduler.EngineDeps{
		Repo:          schedulerRepo,
		Supervisor:    supervisor,
		Coordinator:   coordinator,
		Backups:       backupManager,
		Presets:       presetRepo,
		PresetManager: presetManager,
		ServerRepo:    serverRepo,
		ClusterRepo:   clusterRepo,
		Bus:           bus,
		Log:           logger,
		PollInterval:  10 * time.Second,
	})
	schedulerStop := schedulerEngine.Start(context.Background())
	defer schedulerStop()

	saveConfig := func(c config.Config) error {
		if c.DataDir == "" {
			c.DataDir = dataDir
		}
		return config.Save(c)
	}

	logger.Info("startup",
		"data_dir", dataDir,
		"db", dbPath,
		"first_run_done", cfg.FirstRunDone,
		"steamcmd_path", cfg.SteamCMDPath,
	)

	app := gui.New(gui.Deps{
		Config:            cfg,
		SaveConfig:        saveConfig,
		DataDir:           dataDir,
		DB:                database,
		Bus:               bus,
		Log:               logger,
		Clusters:          clusterRepo,
		Servers:           serverRepo,
		Players:           playersRepo,
		Supervisor:        supervisor,
		Coordinator:       coordinator,
		Tracker:           tracker,
		Actions:           actions,
		SteamCMD:          steamRunner,
		Webhooks:          webhookRepo,
		WebhookDispatcher: webhookDispatcher,
		BackupRepo:        backupRepo,
		BackupManager:     backupManager,
		SchedulerRepo:     schedulerRepo,
		SchedulerEngine:   schedulerEngine,
		Presets:           presetRepo,
		PresetManager:     presetManager,
		ASBDir:            asbDir,
		LogCloser:         logCloser,
		Bundle:            bundle,
		LanguageDir:       languageDir,
	})
	app.Run()

	// Graceful supervisor shutdown so any running servers don't get
	// orphaned. The bus + DB are torn down by the deferred closers.
	logger.Info("shutdown: stopping running servers")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	supervisor.Shutdown(shutdownCtx, true)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func languageDirNextToExe(dataDir string) string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "language")
	}
	return filepath.Join(dataDir, "language")
}

func enabledModIDs(refs []server.ModRef) []int {
	out := make([]int, 0, len(refs))
	for _, r := range refs {
		if r.Enabled {
			out = append(out, r.CurseForgeID)
		}
	}
	return out
}
