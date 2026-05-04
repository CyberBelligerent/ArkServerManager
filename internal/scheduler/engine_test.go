package scheduler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"asamanager/internal/preset"
)

func TestComputeNextFire_Cron(t *testing.T) {
	e := NewEngine(EngineDeps{Repo: NewRepo(newTestDB(t))})
	from := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	task := Task{TriggerKind: TriggerCron, TriggerCron: "0 4 * * *"} // 04:00 daily
	next, terminal, err := e.computeNextFire(task, from)
	if err != nil {
		t.Fatalf("computeNextFire: %v", err)
	}
	if terminal {
		t.Error("cron should not be terminal")
	}
	want := time.Date(2026, 5, 4, 4, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestComputeNextFire_Oneshot_Terminal(t *testing.T) {
	e := NewEngine(EngineDeps{Repo: NewRepo(newTestDB(t))})
	task := Task{TriggerKind: TriggerOneshot, StartAt: time.Now()}
	next, terminal, err := e.computeNextFire(task, time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !terminal {
		t.Error("oneshot should be terminal after firing")
	}
	if next != nil {
		t.Errorf("next = %v, want nil", next)
	}
}

func TestComputeNextFire_BadCron(t *testing.T) {
	e := NewEngine(EngineDeps{Repo: NewRepo(newTestDB(t))})
	task := Task{TriggerKind: TriggerCron, TriggerCron: "not a cron"}
	_, _, err := e.computeNextFire(task, time.Now())
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestExecuteAction_ApplyPreset_RejectsScopeMismatch(t *testing.T) {
	d := newTestDB(t)
	presetRepo := preset.NewRepo(d)
	clusterPreset, err := presetRepo.Create(context.Background(), preset.Preset{
		ScopeType: preset.ScopeCluster, ScopeID: 1, Name: "X",
	})
	if err != nil {
		t.Fatal(err)
	}
	e := NewEngine(EngineDeps{
		Repo: NewRepo(d), Presets: presetRepo,
		PresetManager: preset.NewManager(preset.ManagerDeps{Presets: presetRepo}),
	})
	payload, _ := json.Marshal(ApplyPresetPayload{PresetID: clusterPreset.ID})
	_, err = e.executeAction(context.Background(), Task{
		ScopeType: "server", ScopeID: 99,
		ActionKind: ActionApplyPreset, ActionPayload: payload,
	})
	if err == nil || !strings.Contains(err.Error(), "scope mismatch") {
		t.Errorf("expected scope mismatch error, got: %v", err)
	}
}
