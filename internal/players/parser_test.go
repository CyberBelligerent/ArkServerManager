package players

import "testing"

func TestParseListPlayers_NoPlayers(t *testing.T) {
	for _, raw := range []string{
		"",
		"  \r\n",
		"No Players Connected",
		"No Players Connected\r\n",
	} {
		got := ParseListPlayers(raw)
		if len(got) != 0 {
			t.Errorf("ParseListPlayers(%q) = %v, want empty", raw, got)
		}
	}
}

func TestParseListPlayers_StandardFormat(t *testing.T) {
	raw := "0. Alice, 76561198000000001\r\n" +
		"1. Bob Smith, 76561198000000002\r\n" +
		"2. Carol, 76561198000000003\r\n"
	got := ParseListPlayers(raw)
	if len(got) != 3 {
		t.Fatalf("got %d players, want 3", len(got))
	}
	want := []OnlinePlayer{
		{Name: "Alice", SteamID: "76561198000000001"},
		{Name: "Bob Smith", SteamID: "76561198000000002"},
		{Name: "Carol", SteamID: "76561198000000003"},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] got %+v, want %+v", i, got[i], w)
		}
	}
}

func TestParseListPlayers_ToleratesWhitespaceAndJunk(t *testing.T) {
	raw := "  0.  Alice  ,  76561198000000001  \n" +
		"some unrelated header line\n" +
		"\n" +
		"99. Eve, 76561198000000099\n"
	got := ParseListPlayers(raw)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0] != (OnlinePlayer{Name: "Alice", SteamID: "76561198000000001"}) {
		t.Errorf("[0] = %+v", got[0])
	}
	if got[1] != (OnlinePlayer{Name: "Eve", SteamID: "76561198000000099"}) {
		t.Errorf("[1] = %+v", got[1])
	}
}

func TestParseListPlayers_AcceptsMissingIndex(t *testing.T) {
	got := ParseListPlayers("Alice, 76561198000000001")
	if len(got) != 1 || got[0].Name != "Alice" {
		t.Errorf("got %+v", got)
	}
}
