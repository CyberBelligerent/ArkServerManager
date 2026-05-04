package settings

import "testing"

func TestSoloPreset_IncludesAllPerLevelStats(t *testing.T) {
	preset := SoloRecommendedPreset()
	expected := len(perLevelStatGroups) * len(statLabels) // 4 × 12 = 48
	count := 0
	for k := range preset {
		if len(k) > len("PerLevelStatsMultiplier_") &&
			k[:len("PerLevelStatsMultiplier_")] == "PerLevelStatsMultiplier_" {
			count++
		}
	}
	if count != expected {
		t.Errorf("preset has %d PerLevelStatsMultiplier_* entries, want %d", count, expected)
	}
}

func TestSoloPreset_HealthAndDamageOverrides(t *testing.T) {
	preset := SoloRecommendedPreset()
	checks := map[string]float64{
		"PerLevelStatsMultiplier_DinoTamed[0]":          0.2,
		"PerLevelStatsMultiplier_DinoTamed[8]":          0.17,
		"PerLevelStatsMultiplier_DinoTamed_Add[0]":      0.14,
		"PerLevelStatsMultiplier_DinoTamed_Add[8]":      0.14,
		"PerLevelStatsMultiplier_DinoTamed_Affinity[0]": 0.44,
		"PerLevelStatsMultiplier_DinoTamed_Affinity[8]": 0.44,
		"PerLevelStatsMultiplier_DinoWild[0]":           1.0,
		"PerLevelStatsMultiplier_DinoWild[8]":           1.0,
	}
	for key, want := range checks {
		got, ok := preset[key]
		if !ok {
			t.Errorf("preset missing %s", key)
			continue
		}
		if got.Float != want {
			t.Errorf("%s = %v, want %v", key, got.Float, want)
		}
	}
}

func TestNormalPreset_MatchesSoloKeysetWithDefaults(t *testing.T) {
	solo := SoloRecommendedPreset()
	normal := NormalPreset()
	if len(solo) != len(normal) {
		t.Fatalf("solo has %d keys, normal has %d — keysets must match", len(solo), len(normal))
	}
	reg := Default()
	for k := range solo {
		got, ok := normal[k]
		if !ok {
			t.Errorf("normal missing key %q present in solo", k)
			continue
		}
		s, _ := reg.Lookup(k)
		want := s.DefaultValue()
		if got != want {
			t.Errorf("normal[%q] = %+v, want default %+v", k, got, want)
		}
	}
}

func TestRegistry_HasNewServerInfoKeys(t *testing.T) {
	reg := Default()
	for _, k := range []string{"ServerPVE", "MaxPersonalTamedDinos", "GlobalSpoilingTimeMultiplier"} {
		if _, ok := reg.Lookup(k); !ok {
			t.Errorf("registry missing %s", k)
		}
	}
}
