package webhook

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"asamanager/internal/events"
)

type DispatcherDeps struct {
	Repo   *Repo
	Sender Sender
	Bus    *events.Bus
	DB     *sql.DB
	Log    *slog.Logger
}

// Dispatcher subscribes to the events bus and routes each event to
// every webhook whose scope and event mask match
type Dispatcher struct {
	deps DispatcherDeps

	workersMu sync.Mutex
	workers   map[string]*urlWorker
}

type urlWorker struct {
	queue  chan deliveryJob
	bucket *tokenBucket
}

type deliveryJob struct {
	wh  Webhook
	msg Message
}

func NewDispatcher(deps DispatcherDeps) *Dispatcher {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Dispatcher{deps: deps, workers: map[string]*urlWorker{}}
}

func (d *Dispatcher) Start() func() {
	return d.deps.Bus.SubscribeAll(d.handle)
}

func (d *Dispatcher) SendTest(ctx context.Context, w Webhook) error {
	return d.deps.Sender.Send(ctx, w.URL, Message{Embeds: []Embed{testEmbed(w.Name)}})
}

func (d *Dispatcher) handle(e events.Event) {
	name := e.EventName()
	es := d.eventScopeOf(e)

	webhooks, err := d.deps.Repo.List(context.Background())
	if err != nil {
		d.deps.Log.Error("list webhooks", "err", err)
		return
	}

	for _, wh := range webhooks {
		if !wh.Enabled {
			continue
		}
		if !SubscribesTo(wh.EventMask, name) {
			continue
		}
		if !scopeMatches(wh.Scope, es) {
			continue
		}
		msg, err := Render(name, e, wh.Templates)
		if err != nil {
			d.deps.Log.Warn("render webhook", "webhook", wh.Name, "err", err)
			continue
		}
		if len(msg.Embeds) == 0 && msg.Content == "" {
			continue
		}
		d.enqueue(wh, msg)
	}
}

func (d *Dispatcher) enqueue(wh Webhook, msg Message) {
	w := d.workerFor(wh.URL)
	select {
	case w.queue <- deliveryJob{wh: wh, msg: msg}:
	default:
		d.deps.Log.Warn("webhook queue full, dropping",
			"webhook", wh.Name, "url_hash", urlFingerprint(wh.URL))
	}
}

func (d *Dispatcher) workerFor(url string) *urlWorker {
	d.workersMu.Lock()
	defer d.workersMu.Unlock()
	w, ok := d.workers[url]
	if !ok {
		w = &urlWorker{
			queue: make(chan deliveryJob, 64),
			// Discord per-webhook limit 5 messages per 2s.
			bucket: newTokenBucket(5, 2.5),
		}
		d.workers[url] = w
		go d.runWorker(w)
	}
	return w
}

func (d *Dispatcher) runWorker(w *urlWorker) {
	for job := range w.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		if err := w.bucket.Wait(ctx); err != nil {
			d.deps.Log.Warn("webhook wait", "webhook", job.wh.Name, "err", err)
			cancel()
			continue
		}
		if err := d.deps.Sender.Send(ctx, job.wh.URL, job.msg); err != nil {
			d.deps.Log.Warn("webhook send", "webhook", job.wh.Name, "err", err)
		}
		cancel()
	}
}

func urlFingerprint(url string) string {
	if len(url) <= 12 {
		return url
	}
	return url[:8] + "..." + url[len(url)-4:]
}

type eventScope struct {
	serverID  int64
	clusterID int64
}

func (d *Dispatcher) eventScopeOf(e events.Event) eventScope {
	es := eventScope{}
	switch v := e.(type) {
	// Server supervisor events
	case events.ServerStarting:
		es.serverID = v.ServerID
	case events.ServerStarted:
		es.serverID = v.ServerID
	case events.ServerStopped:
		es.serverID = v.ServerID
	case events.ServerCrashed:
		es.serverID = v.ServerID
	case events.ServerSaved:
		es.serverID = v.ServerID
	case events.PlayerJoined:
		es.serverID = v.ServerID
	case events.PlayerLeft:
		es.serverID = v.ServerID
	case events.PlayerBanned:
		es.serverID = v.ServerID
	case events.RestartChurnWarning:
		es.serverID = v.ServerID
	case events.ModUpdateAvailable:
		es.serverID = v.ServerID
	case events.ServerDeleted:
		es.serverID = v.ServerID
	case events.ServerBackupRestored:
		es.serverID = v.ServerID
		es.clusterID = v.ClusterID
	case events.ClusterBackupRestored:
		es.clusterID = v.ClusterID

	// CRUD events
	case events.ServerCreated:
		es.serverID = v.ServerID
		es.clusterID = v.ClusterID
	case events.ServerInstallUpdate:
		es.serverID = v.ServerID
		es.clusterID = v.ClusterID
	case events.ServerInstallUpdateFinished:
		es.serverID = v.ServerID
		es.clusterID = v.ClusterID
	case events.ServerSettingsChanged:
		es.serverID = v.ServerID
		es.clusterID = v.ClusterID
	case events.ServerSettingsSaved:
		es.serverID = v.ServerID
		es.clusterID = v.ClusterID

	// Cluster events.
	case events.ClusterCreated:
		es.clusterID = v.ClusterID
	case events.ClusterDeleted:
		es.clusterID = v.ClusterID
	case events.ClusterSettingsChanged:
		es.clusterID = v.ClusterID
	case events.ClusterSettingsSaved:
		es.clusterID = v.ClusterID
	case events.ClusterSettingsApplied:
		es.clusterID = v.ClusterID
	case events.ClusterInstallUpdateAll:
		es.clusterID = v.ClusterID
	case events.ClusterInstallUpdateAllFinished:
		es.clusterID = v.ClusterID
	}
	
	// Only hit the DB when we have a server but not its cluster.
	if es.serverID > 0 && es.clusterID == 0 && d.deps.DB != nil {
		var cid int64
		err := d.deps.DB.QueryRow(`SELECT cluster_id FROM servers WHERE id = ?`, es.serverID).Scan(&cid)
		if err == nil {
			es.clusterID = cid
		}
	}
	return es
}

func scopeMatches(webhookScope Scope, es eventScope) bool {
	switch webhookScope.Type {
	case "global":
		return true
	case "server":
		return webhookScope.ID == es.serverID
	case "cluster":
		return webhookScope.ID == es.clusterID
	}
	return false
}
