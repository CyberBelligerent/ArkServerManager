package cluster

import (
	"testing"

	"asamanager/internal/settings"
)

func TestMergeSettings_OverridesWin(t *testing.T) {
	base := map[string]settings.Value{
		"DifficultyOffset":         settings.FloatVal(1.0),
		"PreventDownloadSurvivors": settings.BoolVal(false),
	}
	overrides := map[string]settings.Value{
		"DifficultyOffset": settings.FloatVal(0.2),
		"XPMultiplier":     settings.FloatVal(2.0),
	}
	got := MergeSettings(base, overrides)
	if got["DifficultyOffset"] != settings.FloatVal(0.2) {
		t.Errorf("override should win for DifficultyOffset: %+v", got["DifficultyOffset"])
	}
	if got["PreventDownloadSurvivors"] != settings.BoolVal(false) {
		t.Errorf("base value should survive when not overridden")
	}
	if got["XPMultiplier"] != settings.FloatVal(2.0) {
		t.Errorf("override-only key missing")
	}
}

func TestMergeSettings_DoesNotMutateInputs(t *testing.T) {
	base := map[string]settings.Value{"DifficultyOffset": settings.FloatVal(1.0)}
	overrides := map[string]settings.Value{"DifficultyOffset": settings.FloatVal(0.5)}
	_ = MergeSettings(base, overrides)
	if base["DifficultyOffset"] != settings.FloatVal(1.0) {
		t.Error("base map mutated")
	}
	if overrides["DifficultyOffset"] != settings.FloatVal(0.5) {
		t.Error("overrides map mutated")
	}
}

func TestMergeSettings_NilInputs(t *testing.T) {
	got := MergeSettings(nil, nil)
	if got == nil {
		t.Fatal("expected empty map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d entries", len(got))
	}
}
