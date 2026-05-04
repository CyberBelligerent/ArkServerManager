package server

import "testing"

func TestSuggestPorts_Empty(t *testing.T) {
	got := SuggestPorts(nil)
	if got != DefaultBase {
		t.Errorf("got %+v, want %+v", got, DefaultBase)
	}
}

func TestSuggestPorts_IncrementByStep(t *testing.T) {
	existing := []PortTriple{DefaultBase}
	got := SuggestPorts(existing)
	want := PortTriple{Game: 7787, Query: 27025, RCON: 27030}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSuggestPorts_PicksHighestPlusStep(t *testing.T) {
	existing := []PortTriple{
		{Game: 7777, Query: 27015, RCON: 27020},
		{Game: 7787, Query: 27025, RCON: 27030},
		{Game: 7797, Query: 27035, RCON: 27040},
	}
	got := SuggestPorts(existing)
	want := PortTriple{Game: 7807, Query: 27045, RCON: 27050}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSuggestPorts_HandlesGapInExisting(t *testing.T) {
	existing := []PortTriple{
		{Game: 7777, Query: 27015, RCON: 27020},
		{Game: 7900, Query: 27200, RCON: 27500}, // big jump
	}
	got := SuggestPorts(existing)
	want := PortTriple{Game: 7910, Query: 27210, RCON: 27510}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestCheckPortConflict_GamePeerCollision(t *testing.T) {
	existing := []PortTriple{{Game: 7777, Query: 27015, RCON: 27020}}
	// Game+1 = 7778; candidate.Game = 7778 collides.
	candidate := PortTriple{Game: 7778, Query: 27050, RCON: 27060}
	if err := CheckPortConflict(candidate, existing); err == nil {
		t.Fatal("expected collision on game+1=7778")
	}
}

func TestCheckPortConflict_QueryCollision(t *testing.T) {
	existing := []PortTriple{{Game: 7777, Query: 27015, RCON: 27020}}
	candidate := PortTriple{Game: 8000, Query: 27015, RCON: 27050}
	if err := CheckPortConflict(candidate, existing); err == nil {
		t.Fatal("expected query collision")
	}
}

func TestCheckPortConflict_RCONCollision(t *testing.T) {
	existing := []PortTriple{{Game: 7777, Query: 27015, RCON: 27020}}
	candidate := PortTriple{Game: 8000, Query: 27050, RCON: 27020}
	if err := CheckPortConflict(candidate, existing); err == nil {
		t.Fatal("expected RCON collision")
	}
}

func TestCheckPortConflict_NoConflict(t *testing.T) {
	existing := []PortTriple{{Game: 7777, Query: 27015, RCON: 27020}}
	candidate := PortTriple{Game: 7787, Query: 27025, RCON: 27030}
	if err := CheckPortConflict(candidate, existing); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

func TestPortTripleValidate(t *testing.T) {
	cases := []struct {
		name    string
		p       PortTriple
		wantErr bool
	}{
		{"ok", DefaultBase, false},
		{"port-zero", PortTriple{Game: 0, Query: 27015, RCON: 27020}, true},
		{"port-too-high", PortTriple{Game: 70000, Query: 27015, RCON: 27020}, true},
		{"internal-dup", PortTriple{Game: 7777, Query: 7778, RCON: 27020}, true}, // game+1 == query
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.p.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestPortTripleAllIncludesPeer(t *testing.T) {
	p := PortTriple{Game: 7777, Query: 27015, RCON: 27020}
	all := p.All()
	want := []int{7777, 7778, 27015, 27020}
	if len(all) != len(want) {
		t.Fatalf("len(all)=%d, want %d", len(all), len(want))
	}
	for i := range want {
		if all[i] != want[i] {
			t.Errorf("all[%d]=%d, want %d", i, all[i], want[i])
		}
	}
}
