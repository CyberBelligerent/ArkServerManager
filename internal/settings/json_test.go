package settings

import "testing"

func TestEncodeDecodeValues_RoundTrip(t *testing.T) {
	in := map[string]Value{
		"DifficultyOffset":                  FloatVal(1.0),
		"OverrideMaxExperiencePointsPlayer": IntVal(1500000),
		"PreventDownloadSurvivors":          BoolVal(false),
		"DisableCryopodFridgeRequirement":   BoolVal(true),
	}
	encoded, err := EncodeValues(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := DecodeValues(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("len out=%d, want %d", len(out), len(in))
	}
	for k, want := range in {
		if got := out[k]; got != want {
			t.Errorf("%s: got %+v, want %+v", k, got, want)
		}
	}
}

func TestDecodeValues_EmptyAndNull(t *testing.T) {
	for _, in := range []string{"", "null"} {
		out, err := DecodeValues(in)
		if err != nil {
			t.Errorf("DecodeValues(%q): %v", in, err)
		}
		if len(out) != 0 {
			t.Errorf("DecodeValues(%q) returned %d entries, want 0", in, len(out))
		}
	}
}

func TestEncodeValues_DropsUnknownKeys(t *testing.T) {
	in := map[string]Value{
		"DifficultyOffset":   FloatVal(1.0),
		"NotARealSettingKey": StringVal("ignored"),
	}
	encoded, err := EncodeValues(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := DecodeValues(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := out["DifficultyOffset"]; !ok {
		t.Error("expected DifficultyOffset to round-trip")
	}
	if _, ok := out["NotARealSettingKey"]; ok {
		t.Error("unknown key should have been dropped")
	}
}

func TestDecodeValues_TolerantOfRenamedKeys(t *testing.T) {
	// Simulate stored data containing a key we no longer recognize.
	out, err := DecodeValues(`{"GhostKey": 1.5, "DifficultyOffset": 1.0}`)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := out["GhostKey"]; ok {
		t.Error("GhostKey should have been dropped")
	}
	if got := out["DifficultyOffset"]; got != FloatVal(1.0) {
		t.Errorf("DifficultyOffset = %+v, want 1.0", got)
	}
}
