// Package maps lists known ASA maps and their internal level names.
// Update this list as new maps release; mod maps can be entered manually
// in the GUI.
package maps

// Map is a known ASA map.
type Map struct {
	DisplayName string
	LevelName   string
}

// Known is the catalog of stock ASA maps.
var Known = []Map{
	{"The Island", "TheIsland_WP"},
	{"Scorched Earth", "ScorchedEarth_WP"},
	{"Aberration", "Aberration_WP"},
	{"Extinction", "Extinction_WP"},
	{"The Center", "TheCenter_WP"},
	{"Ragnarok", "Ragnarok_WP"},
	{"Astraeos", "Astraeos_WP"},
	{"Lost Colony", "LostColony_WP"},
	{"Svartalfheim", "Svartalfheim_WP"},
}
