package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pelletier/go-toml/v2"
)

// Config holds the application's persisted settings. Paths are absolute.
type Config struct {
	DataDir          string         `toml:"data_dir"`
	SteamCMDPath     string         `toml:"steamcmd_path"`
	ServerInstallDir string         `toml:"server_install_dir"`
	BackupDir        string         `toml:"backup_dir"`
	ASBDir           string         `toml:"asb_dir"` 				// ARK Smart Breeding multipliers output dir
	CurseForgeAPIKey string         `toml:"curseforge_api_key"`
	Locale       string         `toml:"locale"`
	Discord      DiscordConfig  `toml:"discord"`
	Schedule     ScheduleConfig `toml:"schedule"`
	Backup       BackupConfig   `toml:"backup"`
	FirstRunDone bool           `toml:"first_run_done"`
}

type BackupConfig struct {
	KeepCount int `toml:"keep_count"`
}

type DiscordConfig struct {
	WebhookEnabled bool `toml:"webhook_enabled"`
	BotEnabled     bool `toml:"bot_enabled"`
}

type ScheduleConfig struct {
	CoalesceMinutes    int `toml:"coalesce_minutes"`
	ChurnWarningHours  int `toml:"churn_warning_hours"`
	StaggerSeconds     int `toml:"stagger_seconds"`
	PlayerPollSeconds  int `toml:"player_poll_seconds"`
}

func DefaultConfig() (Config, error) {
	dataDir, err := DefaultDataDir()
	if err != nil {
		return Config{}, err
	}
	return Config{
		DataDir:          dataDir,
		ServerInstallDir: filepath.Join(dataDir, "servers"),
		BackupDir:        filepath.Join(dataDir, "backups"),
		Schedule: ScheduleConfig{
			CoalesceMinutes:   5,
			ChurnWarningHours: 24,
			StaggerSeconds:    30,
			PlayerPollSeconds: 30,
		},
		Backup: BackupConfig{
			KeepCount: 10,
		},
	}, nil
}

func DefaultDataDir() (string, error) {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", errors.New("APPDATA environment variable is not set")
		}
		return filepath.Join(appData, "ASAManager"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "asamanager"), nil
}

func Path(dataDir string) string {
	return filepath.Join(dataDir, "config.toml")
}

func Load(dataDir string) (Config, error) {
	cfg, err := DefaultConfig()
	if err != nil {
		return Config{}, err
	}
	cfg.DataDir = dataDir

	data, err := os.ReadFile(Path(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.DataDir = dataDir
	return cfg, nil
}

func Save(cfg Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(Path(cfg.DataDir), data, 0o644)
}
