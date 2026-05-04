// Creates an Ark Smart Breeder Configuration
package asb

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"asamanager/internal/cluster"
	"asamanager/internal/server"
	"asamanager/internal/settings"
)

// Get names for either Cluster or Server
func FilenameForCluster(c cluster.Cluster) string {
	return "cluster-" + sanitizeFilename(c.Name) + ".ini"
}

func FilenameForServer(s server.Server) string {
	return "server-" + sanitizeFilename(s.Name) + ".ini"
}

// Export and show file path to configuration
func ExportCluster(c cluster.Cluster, destDir string) (string, error) {
	path := filepath.Join(destDir, FilenameForCluster(c))
	if err := write(c.Settings, path); err != nil {
		return path, err
	}
	return path, nil
}

func ExportServer(c cluster.Cluster, s server.Server, destDir string) (string, error) {
	path := filepath.Join(destDir, FilenameForServer(s))
	effective := cluster.MergeSettings(c.Settings, s.SettingOverrides)
	if err := write(effective, path); err != nil {
		return path, err
	}
	return path, nil
}

// Remove for cluster or server
func DeleteForCluster(c cluster.Cluster, destDir string) error {
	return removeIfExists(filepath.Join(destDir, FilenameForCluster(c)))
}

func DeleteForServer(s server.Server, destDir string) error {
	return removeIfExists(filepath.Join(destDir, FilenameForServer(s)))
}

func write(values map[string]settings.Value, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create asb dir: %w", err)
	}
	body := buildBody(values)
	return os.WriteFile(path, []byte(body), 0o644)
}

func buildBody(values map[string]settings.Value) string {
	getF := func(key string, def float64) float64 {
		if v, ok := values[key]; ok {
			return v.Float
		}
		return def
	}
	getB := func(key string, def bool) bool {
		if v, ok := values[key]; ok {
			return v.Bool
		}
		return def
	}

	var b strings.Builder

	// Order within each index matches the sample file:
	//   _Add, _Affinity, (base) DinoTamed, DinoWild
	suffixes := []string{"DinoTamed_Add", "DinoTamed_Affinity", "DinoTamed", "DinoWild"}
	for i := 0; i < 12; i++ {
		for _, suf := range suffixes {
			key := settings.PerStatKey(suf, i)
			fmt.Fprintf(&b, "PerLevelStatsMultiplier_%s[%d] = %s\n",
				suf, i, formatNum(getF(key, 1.0)))
		}
	}

	// Breeding multipliers
	writeFloat(&b, "MatingIntervalMultiplier", getF("MatingIntervalMultiplier", 1))
	writeFloat(&b, "EggHatchSpeedMultiplier", getF("EggHatchSpeedMultiplier", 1))
	writeFloat(&b, "MatingSpeedMultiplier", getF("MatingSpeedMultiplier", 1))
	writeFloat(&b, "BabyMatureSpeedMultiplier", getF("BabyMatureSpeedMultiplier", 1))
	writeFloat(&b, "BabyImprintingStatScaleMultiplier", getF("BabyImprintingStatScaleMultiplier", 1))
	writeFloat(&b, "BabyImprintAmountMultiplier", getF("BabyImprintAmountMultiplier", 1))
	writeFloat(&b, "BabyCuddleIntervalMultiplier", getF("BabyCuddleIntervalMultiplier", 1))
	writeFloat(&b, "BabyFoodConsumptionSpeedMultiplier", getF("BabyFoodConsumptionSpeedMultiplier", 1))

	// Taming and draining
	writeFloat(&b, "TamingSpeedMultiplier", getF("TamingSpeedMultiplier", 1))
	writeFloat(&b, "DinoCharacterFoodDrainMultiplier", getF("DinoCharacterFoodDrainMultiplier", 1))
	writeFloat(&b, "WildDinoCharacterFoodDrainMultiplier", getF("WildDinoCharacterFoodDrainMultiplier", 1))
	writeFloat(&b, "WildDinoTorporDrainMultiplier", getF("WildDinoTorporDrainMultiplier", 1))

	// Smart Breeder Stuff
	maxWild := int(getF("OverrideOfficialDifficulty", 4.0) * 30)
	fmt.Fprintf(&b, "ASBMaxWildLevels_Dinos = %d\n", maxWild)
	fmt.Fprintf(&b, "ASBMaxDomLevels_Dinos = %d\n", 88)
	fmt.Fprintf(&b, "ASBMaxGraphLevels = %d\n", 50)
	fmt.Fprintf(&b, "ASBExtractorWildLevelSteps = %d\n", 1)
	fmt.Fprintf(&b, "ASBAllowHyperImprinting = %s\n", formatBool(false))
	fmt.Fprintf(&b, "bAllowSpeedLeveling = %s\n", formatBool(false))
	fmt.Fprintf(&b, "bAllowFlyerSpeedLeveling = %s\n", formatBool(false))

	// All ArkSmart Breeder Stuff
	
	// Unused right now, will fix later
	fmt.Fprintf(&b, "DestroyTamesOverLevelClamp = %d\n", 450)
	
	fmt.Fprintf(&b, "bUseSingleplayerSettings = %s\n", formatBool(getB("bUseSingleplayerSettings", false)))
	for _, name := range []string{
		"ASBEvent_MatingIntervalMultiplier",
		"ASBEvent_EggHatchSpeedMultiplier",
		"ASBEvent_BabyMatureSpeedMultiplier",
		"ASBEvent_BabyCuddleIntervalMultiplier",
		"ASBEvent_BabyFoodConsumptionSpeedMultiplier",
		"ASBEvent_TamingSpeedMultiplier",
		"ASBEvent_DinoCharacterFoodDrainMultiplier",
	} {
		fmt.Fprintf(&b, "%s = %s\n", name, formatNum(1))
	}

	return b.String()
}

func writeFloat(b *strings.Builder, key string, v float64) {
	fmt.Fprintf(b, "%s = %s\n", key, formatNum(v))
}

func formatNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func formatBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	var out strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			out.WriteRune(r)
		default:
			out.WriteRune('_')
		}
	}
	return out.String()
}

func removeIfExists(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
