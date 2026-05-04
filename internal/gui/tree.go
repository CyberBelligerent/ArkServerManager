package gui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/cluster"
	"asamanager/internal/server"
)

type clusterTree struct {
	app  *App
	tree *widget.Tree

	mu          sync.Mutex
	clusters    []cluster.Cluster
	standalone  []server.Server
	byCluster   map[int64][]server.Server
	clusterByID map[int64]cluster.Cluster
	serverByID  map[int64]server.Server
}

func newClusterTree(app *App) *clusterTree {
	t := &clusterTree{
		app:         app,
		byCluster:   map[int64][]server.Server{},
		clusterByID: map[int64]cluster.Cluster{},
		serverByID:  map[int64]server.Server{},
	}
	t.tree = widget.NewTree(t.childUIDs, t.isBranch, t.createNode, t.updateNode)
	t.tree.OnSelected = t.onSelected
	return t
}

func (t *clusterTree) Container() fyne.CanvasObject {
	return container.NewStack(t.tree)
}

func (t *clusterTree) Refresh() {
	t.mu.Lock()
	clusters, err := t.app.deps.Clusters.List(t.app.ctx())
	if err != nil {
		t.app.deps.Log.Error("list clusters", "err", err)
		t.mu.Unlock()
		return
	}
	t.clusters = clusters
	t.byCluster = map[int64][]server.Server{}
	t.clusterByID = map[int64]cluster.Cluster{}
	t.serverByID = map[int64]server.Server{}
	for _, c := range clusters {
		t.clusterByID[c.ID] = c
		servers, err := t.app.deps.Servers.ListByCluster(t.app.ctx(), c.ID)
		if err != nil {
			t.app.deps.Log.Error("list servers", "cluster_id", c.ID, "err", err)
			continue
		}
		t.byCluster[c.ID] = servers
		for _, s := range servers {
			t.serverByID[s.ID] = s
		}
	}
	standalone, err := t.app.deps.Servers.ListStandalone(t.app.ctx())
	if err != nil {
		t.app.deps.Log.Error("list standalone servers", "err", err)
	} else {
		t.standalone = standalone
		for _, s := range standalone {
			t.serverByID[s.ID] = s
		}
	}
	t.mu.Unlock()
	t.tree.Refresh()
	t.tree.OpenAllBranches()
}

func (t *clusterTree) ClusterIDs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.clusters))
	for _, c := range t.clusters {
		out = append(out, c.ClusterID)
	}
	return out
}

func (t *clusterTree) ClusterByClusterID(clusterID string) (cluster.Cluster, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.clusters {
		if c.ClusterID == clusterID {
			return c, true
		}
	}
	return cluster.Cluster{}, false
}

func (t *clusterTree) childUIDs(uid widget.TreeNodeID) []widget.TreeNodeID {
	t.mu.Lock()
	defer t.mu.Unlock()
	if uid == "" {
		out := make([]widget.TreeNodeID, 0, len(t.clusters)+len(t.standalone))
		for _, c := range t.clusters {
			out = append(out, fmt.Sprintf("c-%d", c.ID))
		}
		// Standalone servers appear as siblings to clusters at root
		for _, s := range t.standalone {
			out = append(out, fmt.Sprintf("s-%d", s.ID))
		}
		return out
	}
	if cid, ok := parseClusterUID(uid); ok {
		servers := t.byCluster[cid]
		out := make([]widget.TreeNodeID, 0, len(servers))
		for _, s := range servers {
			out = append(out, fmt.Sprintf("s-%d", s.ID))
		}
		return out
	}
	return nil
}

func (t *clusterTree) isBranch(uid widget.TreeNodeID) bool {
	if uid == "" {
		return true
	}
	_, isCluster := parseClusterUID(uid)
	return isCluster
}

func (t *clusterTree) createNode(_ bool) fyne.CanvasObject {
	return widget.NewLabel("...")
}

func (t *clusterTree) updateNode(uid widget.TreeNodeID, _ bool, obj fyne.CanvasObject) {
	label := obj.(*widget.Label)
	t.mu.Lock()
	defer t.mu.Unlock()
	if cid, ok := parseClusterUID(uid); ok {
		if c, found := t.clusterByID[cid]; found {
			label.SetText(c.Name + "  (" + c.ClusterID + ")")
		}
		return
	}
	if sid, ok := parseServerUID(uid); ok {
		if s, found := t.serverByID[sid]; found {
			text := statusBadge(t.app, s.Status) + "  " + s.Name
			if s.ClusterID == 0 {
				text += "  " + t.app.T("tree.standalone_suffix")
			}
			label.SetText(text)
		}
	}
}

func (t *clusterTree) onSelected(uid widget.TreeNodeID) {
	if cid, ok := parseClusterUID(uid); ok {
		t.mu.Lock()
		c := t.clusterByID[cid]
		t.mu.Unlock()
		t.app.tabs.ShowCluster(c)
		return
	}
	if sid, ok := parseServerUID(uid); ok {
		t.mu.Lock()
		s := t.serverByID[sid]
		c := t.clusterByID[s.ClusterID]
		t.mu.Unlock()
		t.app.tabs.ShowServer(c, s)
	}
}

func parseClusterUID(uid widget.TreeNodeID) (int64, bool) {
	if !strings.HasPrefix(uid, "c-") {
		return 0, false
	}
	n, err := strconv.ParseInt(uid[2:], 10, 64)
	return n, err == nil
}

func parseServerUID(uid widget.TreeNodeID) (int64, bool) {
	if !strings.HasPrefix(uid, "s-") {
		return 0, false
	}
	n, err := strconv.ParseInt(uid[2:], 10, 64)
	return n, err == nil
}

func statusBadge(a *App, s server.Status) string {
	switch s {
	case server.StatusRunning:
		return a.T("tree.status.running")
	case server.StatusStarting:
		return a.T("tree.status.starting")
	case server.StatusStopping:
		return a.T("tree.status.stopping")
	case server.StatusCrashed:
		return a.T("tree.status.crashed")
	default:
		return a.T("tree.status.off")
	}
}
