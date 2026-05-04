// Package events lists known ASA ActiveEvent values and their typical
// calendar windows. The scheduler ships these as recurring_window
// presets; users can edit the dates per cluster.
package events

// ActiveEvent is a known ARK seasonal/holiday event.
type ActiveEvent struct {
	Name        string // value passed to -ActiveEvent=
	DisplayName string
	StartMonth  int // 1-12; zero means no default window
	StartDay    int
	EndMonth    int
	EndDay      int
}

// None is the sentinel that clears any active event.
var None = ActiveEvent{Name: "None", DisplayName: "None (clear)"}

// Known is the catalog of stock ASA ActiveEvent values with the
// approximate calendar windows Wildcard typically uses.
var Known = []ActiveEvent{
	{"LoveEvolved", "Love Evolved (Valentine's)", 2, 11, 2, 21},
	{"EggcellentAdventure", "Eggcellent Adventure (Easter)", 3, 28, 4, 11},
	{"ARKaeology", "ARKaeology", 6, 5, 6, 25},
	{"Summer", "Summer Bash", 6, 19, 7, 6},
	{"Birthday", "ARK Anniversary", 8, 28, 9, 11},
	{"FearEvolved", "Fear Evolved (Halloween)", 10, 25, 11, 5},
	{"TurkeyTrial", "Turkey Trial (Thanksgiving)", 11, 21, 11, 28},
	{"WinterWonderland", "Winter Wonderland", 12, 17, 1, 5},
}
