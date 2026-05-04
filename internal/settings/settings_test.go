package settings

import (
	"strings"
	"testing"

	"asamanager/internal/inifile"
)

func TestApplyAndReadRoundTrip(t *testing.T) {
	reg := Default()
	values := map[string]Value{
		"DifficultyOffset":                  FloatVal(1.0),
		"OverrideOfficialDifficulty":        FloatVal(5.0),
		"TamingSpeedMultiplier":             FloatVal(5.0),
		"MatingIntervalMultiplier":          FloatVal(0.4),
		"PreventDownloadSurvivors":          BoolVal(false),
		"DisableCryopodFridgeRequirement":   BoolVal(true),
		"OverrideMaxExperiencePointsPlayer": IntVal(1500000),
	}

	game := inifile.New()
	gus := inifile.New()
	ApplyTo(reg, game, FilterByFile(reg, values, FileGame))
	ApplyTo(reg, gus, FilterByFile(reg, values, FileGameUserSettings))

	game2, err := inifile.Parse(strings.NewReader(game.String()))
	if err != nil {
		t.Fatalf("re-parse game: %v", err)
	}
	gus2, err := inifile.Parse(strings.NewReader(gus.String()))
	if err != nil {
		t.Fatalf("re-parse gus: %v", err)
	}

	gameBack, errs := ReadFrom(reg, game2)
	if len(errs) > 0 {
		t.Fatalf("read game errors: %v", errs)
	}
	gusBack, errs := ReadFrom(reg, gus2)
	if len(errs) > 0 {
		t.Fatalf("read gus errors: %v", errs)
	}

	for k, want := range values {
		s, _ := reg.Lookup(k)
		var got Value
		if s.File == FileGame {
			got = gameBack[k]
		} else {
			got = gusBack[k]
		}
		if got != want {
			t.Errorf("%s: got %+v, want %+v", k, got, want)
		}
	}
}

func TestSoloPresetIsValid(t *testing.T) {
	reg := Default()
	preset := SoloRecommendedPreset()
	if len(preset) == 0 {
		t.Fatal("SoloRecommendedPreset is empty")
	}
	for k, v := range preset {
		s, ok := reg.Lookup(k)
		if !ok {
			t.Errorf("preset key %q not in registry", k)
			continue
		}
		if err := s.Validate(v); err != nil {
			t.Errorf("preset %s invalid: %v", k, err)
		}
	}
}

func TestFormatAndParseRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		t         Type
		v         Value
		formatted string
	}{
		{"float-int", TypeFloat, FloatVal(2.0), "2.0"},
		{"float-decimal", TypeFloat, FloatVal(0.4), "0.4"},
		{"float-negative", TypeFloat, FloatVal(-1.5), "-1.5"},
		{"int", TypeInt, IntVal(1500000), "1500000"},
		{"bool-true", TypeBool, BoolVal(true), "True"},
		{"bool-false", TypeBool, BoolVal(false), "False"},
		{"string", TypeString, StringVal("hello"), "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.v.Format(c.t)
			if got != c.formatted {
				t.Errorf("Format = %q, want %q", got, c.formatted)
			}
			parsed, err := ParseValue(c.t, got)
			if err != nil {
				t.Fatalf("ParseValue: %v", err)
			}
			if parsed != c.v {
				t.Errorf("round-trip: got %+v, want %+v", parsed, c.v)
			}
		})
	}
}

func TestFilterByFile(t *testing.T) {
	reg := Default()
	values := map[string]Value{
		"DifficultyOffset":         FloatVal(1.0), // GameUserSettings
		"MatingIntervalMultiplier": FloatVal(0.4), // Game.ini
		"PreventDownloadSurvivors": BoolVal(false), // Game.ini
	}
	game := FilterByFile(reg, values, FileGame)
	gus := FilterByFile(reg, values, FileGameUserSettings)

	if _, ok := game["MatingIntervalMultiplier"]; !ok {
		t.Error("expected MatingIntervalMultiplier in Game.ini set")
	}
	if _, ok := game["PreventDownloadSurvivors"]; !ok {
		t.Error("expected PreventDownloadSurvivors in Game.ini set")
	}
	if _, ok := game["DifficultyOffset"]; ok {
		t.Error("DifficultyOffset should not be in Game.ini set")
	}
	if _, ok := gus["DifficultyOffset"]; !ok {
		t.Error("expected DifficultyOffset in GameUserSettings set")
	}
}

func TestRegistryLookup(t *testing.T) {
	reg := Default()

	for _, k := range []string{
		"DifficultyOffset",
		"XPMultiplier",
		"BabyImprintAmountMultiplier",
		"DisableCryopodFridgeRequirement",
		"AutoSavePeriodMinutes",
	} {
		if _, ok := reg.Lookup(k); !ok {
			t.Errorf("registry missing key %q", k)
		}
	}
	if _, ok := reg.Lookup("DefinitelyNotARealKey"); ok {
		t.Error("registry returned ok for unknown key")
	}
}

func TestValidateBoundsViolation(t *testing.T) {
	reg := Default()
	s, _ := reg.Lookup("DifficultyOffset") // Min 0, Max 1
	if err := s.Validate(FloatVal(0.5)); err != nil {
		t.Errorf("0.5 should pass: %v", err)
	}
	if err := s.Validate(FloatVal(-0.1)); err == nil {
		t.Error("-0.1 should fail min")
	}
	if err := s.Validate(FloatVal(1.5)); err == nil {
		t.Error("1.5 should fail max")
	}
}
