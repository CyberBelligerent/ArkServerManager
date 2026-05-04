package settings

const (
	CatDifficulty      = "Difficulty"
	CatTaming          = "Taming"
	CatXP              = "XP & Leveling"
	CatPlayerStats     = "Player Stats"
	CatLoot            = "Loot"
	CatHarvest         = "Harvest"
	CatDayNight        = "Day/Night Cycle"
	CatBreeding        = "Breeding"
	CatClusterTransfer = "Cluster Transfer"
	CatCryopodQoL      = "Cryopod QoL"
	CatGeneralQoL      = "General QoL"
	CatServerInfo      = "Server Info"
	CatTribes          = "Tribes & Limits"
	CatDecay           = "Decay & Spoilage"
	CatPerStatWild     = "Per-Level Stats — Wild Dinos"
	CatPerStatTamed    = "Per-Level Stats — Tamed Dinos"
	CatPerStatTamedAdd = "Per-Level Stats — Tamed Dinos (Add)"
	CatPerStatTamedAff = "Per-Level Stats — Tamed Dinos (Affinity)"
)

var statLabels = []string{
	"Health",
	"Stamina",
	"Torpidity",
	"Oxygen",
	"Food",
	"Water",
	"Temperature",
	"Weight",
	"MeleeDamage",
	"SpeedMultiplier",
	"Fortitude",
	"CraftingSpeedMultiplier",
}

func fp(v float64) *float64 { return &v }

var allSettings = []Setting{

	// Difficulty

	{
		Key: "DifficultyOffset", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 0.2, Min: fp(0), Max: fp(1),
		Category: CatDifficulty,
		Tooltip:  "Scales wild dino max level. 1.0 with OverrideOfficialDifficulty=5.0 yields max wild level 150.",
	},
	{
		Key: "OverrideOfficialDifficulty", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 4.0, Min: fp(1), Max: fp(20),
		Category: CatDifficulty,
		Tooltip:  "Caps wild level multiplier. 5.0 = max wild level 150 when DifficultyOffset=1.0.",
	},

	// Taming

	{
		Key: "TamingSpeedMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatTaming,
		Tooltip:  "Higher = faster taming. 5.0 is a common solo value.",
	},
	{
		Key: "PassiveTameIntervalMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatTaming,
		Tooltip:  "Time between passive-tame feedings. Lower = feed more often.",
	},
	{
		Key: "DinoCharacterFoodDrainMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatTaming,
		Tooltip:  "How fast tamed dinos consume food.",
	},
	{
		Key: "WildDinoCharacterFoodDrainMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatTaming,
		Tooltip:  "How fast wild dinos consume food (affects passive-tame intervals).",
	},
	{
		Key: "WildDinoTorporDrainMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatTaming,
		Tooltip:  "How fast knocked-out wild dinos lose torpor during taming.",
	},

	// XP

	{
		Key: "XPMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatXP,
		Tooltip:  "Global XP gain rate.",
	},
	{
		Key: "OverrideMaxExperiencePointsPlayer", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeInt, Default: 0, Min: fp(0),
		Category: CatXP,
		Tooltip:  "Hard cap on player XP (0 = use game default).",
	},
	{
		Key: "OverrideMaxExperiencePointsDino", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeInt, Default: 0, Min: fp(0),
		Category: CatXP,
		Tooltip:  "Hard cap on dino XP (0 = use game default).",
	},

	// Player Stats

	{
		Key: "PlayerCharacterFoodDrainMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatPlayerStats,
		Tooltip:  "How fast players consume food.",
	},
	{
		Key: "PlayerCharacterWaterDrainMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatPlayerStats,
		Tooltip:  "How fast players consume water.",
	},
	{
		Key: "PlayerCharacterStaminaDrainMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatPlayerStats,
		Tooltip:  "How fast players burn stamina while sprinting/swinging.",
	},
	{
		Key: "PlayerCharacterHealthRecoveryMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatPlayerStats,
		Tooltip:  "How fast players regenerate health.",
	},
	{
		Key: "CraftingSkillBonusMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatPlayerStats,
		Tooltip:  "Scales the crafting-skill bonus on crafted items.",
	},

	// Loot

	{
		Key: "SupplyCrateLootQualityMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(1), Max: fp(5),
		Category: CatLoot,
		Tooltip:  "Quality of loot in supply crates and beacons.",
	},
	{
		Key: "FishingLootQualityMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(1), Max: fp(5),
		Category: CatLoot,
		Tooltip:  "Quality of loot from fishing.",
	},

	// Harvest

	{
		Key: "HarvestAmountMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatHarvest,
		Tooltip:  "Resources gained per harvest.",
	},
	{
		Key: "HarvestHealthMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatHarvest,
		Tooltip:  "How much HP harvestable nodes have. Higher = more swings to deplete.",
	},
	{
		Key: "ResourcesRespawnPeriodMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatHarvest,
		Tooltip:  "How long resources take to respawn. Lower = faster respawns.",
	},
	{
		Key: "DinoHarvestingDamageMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatHarvest,
		Tooltip:  "How much damage dinos do when harvesting.",
	},
	{
		Key: "PlayerHarvestingDamageMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatHarvest,
		Tooltip:  "How much damage players do when harvesting.",
	},

	// Day/Night Cycle

	{
		Key: "DayCycleSpeedScale", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatDayNight,
		Tooltip:  "Overall day/night cycle speed.",
	},
	{
		Key: "DayTimeSpeedScale", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatDayNight,
		Tooltip:  "Daytime length multiplier.",
	},
	{
		Key: "NightTimeSpeedScale", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatDayNight,
		Tooltip:  "Nighttime length multiplier.",
	},

	// Breeding

	{
		Key: "MatingIntervalMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatBreeding,
		Tooltip:  "Cooldown between matings. Lower = breed more often.",
	},
	{
		Key: "MatingSpeedMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatBreeding,
		Tooltip:  "How fast the mating bar fills.",
	},
	{
		Key: "EggHatchSpeedMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatBreeding,
		Tooltip:  "How fast eggs hatch / gestation completes.",
	},
	{
		Key: "BabyMatureSpeedMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatBreeding,
		Tooltip:  "How fast babies grow to juvenile/adolescent/adult.",
	},
	{
		Key: "BabyFoodConsumptionSpeedMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatBreeding,
		Tooltip:  "How fast babies eat. Lower = less feeding required.",
	},
	{
		Key: "BabyCuddleIntervalMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatBreeding,
		Tooltip:  "Time between imprint cuddle requests. Lower = more cuddles per maturation.",
	},
	{
		Key: "BabyCuddleGracePeriodMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatBreeding,
		Tooltip:  "How long you have to satisfy a cuddle request before imprint quality drops.",
	},
	{
		Key: "BabyCuddleLoseImprintQualitySpeedMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatBreeding,
		Tooltip:  "How fast imprint quality decays after a missed cuddle.",
	},
	{
		Key: "BabyImprintAmountMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatBreeding,
		Tooltip:  "Imprint percentage gained per cuddle. 2.0 = 100%% imprint in fewer cuddles.",
	},
	{
		Key: "BabyImprintingStatScaleMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatBreeding,
		Tooltip:  "Scales how much imprint quality buffs a tame's stats. ASB needs this for accurate stat extraction.",
	},
	{
		Key: "LayEggIntervalMultiplier", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatBreeding,
		Tooltip:  "Time between dino egg-laying. Lower = more eggs.",
	},

	// Cluster Transfer

	{
		Key: "PreventDownloadSurvivors", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatClusterTransfer,
		Tooltip:  "If true, players cannot download a survivor onto this server.",
	},
	{
		Key: "PreventUploadSurvivors", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatClusterTransfer,
		Tooltip:  "If true, players cannot upload their survivor from this server.",
	},
	{
		Key: "PreventDownloadItems", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatClusterTransfer,
		Tooltip:  "If true, items cannot be downloaded onto this server.",
	},
	{
		Key: "PreventUploadItems", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatClusterTransfer,
		Tooltip:  "If true, items cannot be uploaded from this server.",
	},
	{
		Key: "PreventDownloadDinos", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatClusterTransfer,
		Tooltip:  "If true, dinos cannot be downloaded onto this server.",
	},
	{
		Key: "PreventUploadDinos", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatClusterTransfer,
		Tooltip:  "If true, dinos cannot be uploaded from this server.",
	},
	{
		Key: "NoTributeDownloads", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatClusterTransfer,
		Tooltip:  "If true, all tribute (cluster) downloads are blocked.",
	},
	{
		Key: "CrossARKAllowForeignDinoDownloads", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatClusterTransfer,
		Tooltip:  "Allow downloading dinos from servers outside this cluster.",
	},

	// Cryopod QoL

	{
		Key: "DisableCryopodEnemyCheck", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatCryopodQoL,
		Tooltip:  "Allow cryopod use even when enemy structures/players are nearby.",
	},
	{
		Key: "AllowCryoFridgeOnSaddle", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatCryopodQoL,
		Tooltip:  "Allow placing the Cryofridge on platform-saddle structures.",
	},
	{
		Key: "DisableCryopodFridgeRequirement", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatCryopodQoL,
		Tooltip:  "Cryopods stay active without a Cryofridge nearby.",
	},

	// General QoL

	{
		Key: "AllowMultipleAttachedC4", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatGeneralQoL,
		Tooltip:  "Allow multiple C4 charges to be attached to a single dino/structure.",
	},
	{
		Key: "AllowAnyoneBabyImprintCuddle", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatGeneralQoL,
		Tooltip:  "Any tribe member can imprint, not just the original imprinter.",
	},
	{
		Key: "StructurePreventResourceRadiusMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0),
		Category: CatGeneralQoL,
		Tooltip:  "Radius around structures that suppresses resource respawns. Lower = resources respawn closer to bases.",
	},
	{
		Key: "AutoSavePeriodMinutes", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 15.0, Min: fp(1),
		Category: CatGeneralQoL,
		Tooltip:  "Minutes between automatic world saves.",
	},
	{
		Key: "bUseSingleplayerSettings", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeBool, Default: false,
		Category: CatGeneralQoL,
		Tooltip:  "Apply singleplayer rate boosts (taming, mating, maturation) on top of multipliers.",
	},

	// Server Info

	{
		Key: "ServerPVE", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeBool, Default: false,
		Category: CatServerInfo,
		Tooltip:  "Force PvE mode. Players cannot harm each other or each other's tames.",
	},
	{
		Key: "ServerHardcore", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeBool, Default: false,
		Category: CatServerInfo,
		Tooltip:  "On death, player character is wiped to level 1 (hardcore).",
	},
	{
		Key: "ServerCrosshair", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeBool, Default: true,
		Category: CatServerInfo,
		Tooltip:  "Show the player crosshair.",
	},
	{
		Key: "ShowMapPlayerLocation", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeBool, Default: true,
		Category: CatServerInfo,
		Tooltip:  "Show player location on the map screen.",
	},
	{
		Key: "AllowThirdPersonPlayer", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeBool, Default: true,
		Category: CatServerInfo,
		Tooltip:  "Allow third-person camera.",
	},
	{
		Key: "AllowFlyerCarryPvE", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeBool, Default: false,
		Category: CatServerInfo,
		Tooltip:  "Allow flyer dinos to carry players in PvE.",
	},

	// Tribes & Limits

	{
		Key: "MaxPersonalTamedDinos", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeInt, Default: 0, Min: fp(0),
		Category: CatTribes,
		Tooltip:  "Per-tribe tame cap. 0 = unlimited.",
	},
	{
		Key: "MaxTribeLogs", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeInt, Default: 100, Min: fp(0),
		Category: CatTribes,
		Tooltip:  "Maximum tribe log entries kept per tribe.",
	},
	{
		Key: "MaxPlayersInTribe", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeInt, Default: 0, Min: fp(0),
		Category: CatTribes,
		Tooltip:  "Maximum players per tribe. 0 = unlimited.",
	},

	// Decay & Spoilage

	{
		Key: "ItemStackSizeMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.1),
		Category: CatDecay,
		Tooltip:  "Multiplier on stack sizes for stackable items.",
	},
	{
		Key: "GlobalSpoilingTimeMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatDecay,
		Tooltip:  "Multiplier on perishable food spoilage time. Higher = food lasts longer.",
	},
	{
		Key: "GlobalItemDecompositionTimeMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatDecay,
		Tooltip:  "Multiplier on dropped-item despawn time.",
	},
	{
		Key: "GlobalCorpseDecompositionTimeMultiplier", Section: SectionServerSettings, File: FileGameUserSettings,
		Type: TypeFloat, Default: 1.0, Min: fp(0.01),
		Category: CatDecay,
		Tooltip:  "Multiplier on corpse despawn time.",
	},
	{
		Key: "DisableImprintDinoBuff", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatGeneralQoL,
		Tooltip:  "Imprinted dinos no longer get the imprinter-only stat buff.",
	},
	{
		Key: "bAutoUnlockAllEngrams", Section: SectionShooterGameMode, File: FileGame,
		Type: TypeBool, Default: false,
		Category: CatGeneralQoL,
		Tooltip:  "Players auto-unlock every engram on level-up (skip engram point spending).",
	},
}

// init appends the auto-generated PerLevelStatsMultiplier_DinoTamed*
// entries to allSettings, then rebuilds the default registry
func init() {
	allSettings = append(allSettings, perLevelStatSettings()...)
	defaultRegistry = buildRegistry(allSettings)
}

// PerStatGroup describes one of the four PerLevelStatsMultiplier_* tables
type PerStatGroup struct {
	Suffix   string // PerLevelStatsMultiplier_<Suffix>[idx]
	Display  string // column header in the GUI grid
	Category string // matching CatPerStat* constant
}

// perLevelStatGroups lists every PerLevelStatsMultiplier_* table
var perLevelStatGroups = []PerStatGroup{
	{Suffix: "DinoWild", Display: "WildLevel", Category: CatPerStatWild},
	{Suffix: "DinoTamed", Display: "TameLevel", Category: CatPerStatTamed},
	{Suffix: "DinoTamed_Add", Display: "TameAdd", Category: CatPerStatTamedAdd},
	{Suffix: "DinoTamed_Affinity", Display: "TameAff", Category: CatPerStatTamedAff},
}

// PerStatGroupCatalog returns a copy of the per-stat groups
func PerStatGroupCatalog() []PerStatGroup {
	out := make([]PerStatGroup, len(perLevelStatGroups))
	copy(out, perLevelStatGroups)
	return out
}

// PerStatLabels returns the 12 ARK stat names indexed by ARK stat
func PerStatLabels() []string {
	out := make([]string, len(statLabels))
	copy(out, statLabels)
	return out
}

// PerStatKey returns the INI key for one cell
func PerStatKey(suffix string, idx int) string {
	return formatPerLevelKey(suffix, idx)
}

// IsPerStatCategory reports whether a category name belongs to one of
// the four per-stat tables
func IsPerStatCategory(name string) bool {
	for _, g := range perLevelStatGroups {
		if g.Category == name {
			return true
		}
	}
	return false
}

// perLevelStatSettings generates the 48 PerLevelStatsMultiplier_* entries
func perLevelStatSettings() []Setting {
	out := make([]Setting, 0, len(perLevelStatGroups)*len(statLabels))
	for _, g := range perLevelStatGroups {
		for i, label := range statLabels {
			out = append(out, Setting{
				Key:      formatPerLevelKey(g.Suffix, i),
				Section:  SectionShooterGameMode,
				File:     FileGame,
				Type:     TypeFloat,
				Default:  1.0,
				Min:      fp(0),
				Category: g.Category,
				Tooltip:  formatPerLevelTooltip(g.Suffix, i, label),
			})
		}
	}
	return out
}

func formatPerLevelKey(group string, idx int) string {
	return "PerLevelStatsMultiplier_" + group + "[" + itoaSmall(idx) + "]"
}

func formatPerLevelTooltip(group string, idx int, label string) string {
	return "Per-level " + label + " gain for " + group + " (index " + itoaSmall(idx) + "). Default 1.0."
}

// itoaSmall avoids importing strconv in the static catalog file.
func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	var b [3]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// soloPerLevelOverrides is the per-stat multiplier preset for the Solo
// Cluster template. Stats are indexed by:
//
//   0 Health     1 Stamina  2 Torpidity  3 Oxygen   4 Food    5 Water
//   6 Temperature 7 Weight  8 MeleeDamage 9 Speed   10 Fortitude  11 CraftingSpeed
var soloPerLevelOverrides = map[string]map[int]float64{
	"DinoWild":           {},
	"DinoTamed":          {0: 0.2, 8: 0.17},
	"DinoTamed_Add":      {0: 0.14, 8: 0.14},
	"DinoTamed_Affinity": {0: 0.44, 8: 0.44},
}

func NormalPreset() map[string]Value {
	reg := Default()
	out := map[string]Value{}
	for k := range SoloRecommendedPreset() {
		if s, ok := reg.Lookup(k); ok {
			out[k] = s.DefaultValue()
		}
	}
	return out
}

func SoloRecommendedPreset() map[string]Value {
	out := map[string]Value{
		"DifficultyOffset":                FloatVal(1.0),
		"OverrideOfficialDifficulty":      FloatVal(5.0),
		"TamingSpeedMultiplier":           FloatVal(5.0),
		"XPMultiplier":                    FloatVal(2.0),
		"HarvestAmountMultiplier":         FloatVal(2.0),
		"MatingIntervalMultiplier":        FloatVal(0.4),
		"EggHatchSpeedMultiplier":         FloatVal(15.0),
		"BabyMatureSpeedMultiplier":       FloatVal(20.0),
		"BabyCuddleIntervalMultiplier":    FloatVal(0.25),
		"BabyImprintAmountMultiplier":     FloatVal(2.0),
		"PreventDownloadSurvivors":        BoolVal(false),
		"PreventUploadSurvivors":          BoolVal(false),
		"PreventDownloadItems":            BoolVal(false),
		"PreventUploadItems":              BoolVal(false),
		"PreventDownloadDinos":            BoolVal(false),
		"PreventUploadDinos":              BoolVal(false),
		"NoTributeDownloads":              BoolVal(false),
		"DisableCryopodFridgeRequirement": BoolVal(true),
	}

	for _, g := range perLevelStatGroups {
		tuned := soloPerLevelOverrides[g.Suffix]
		for i := range statLabels {
			v := 1.0
			if t, ok := tuned[i]; ok {
				v = t
			}
			out[formatPerLevelKey(g.Suffix, i)] = FloatVal(v)
		}
	}
	return out
}
