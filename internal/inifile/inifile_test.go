package inifile

import (
	"strings"
	"testing"
)

func TestRoundTripPreservesFormat(t *testing.T) {
	src := "; top comment\r\n" +
		"\r\n" +
		"[ServerSettings]\r\n" +
		"DifficultyOffset=1.0\r\n" +
		"; mid comment\r\n" +
		"TamingSpeedMultiplier=5.0\r\n" +
		"\r\n" +
		"[/script/shootergame.shootergamemode]\r\n" +
		"MatingIntervalMultiplier=0.4\r\n"

	f, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := f.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != src {
		t.Errorf("round-trip mismatch:\nwant: %q\n got: %q", src, string(out))
	}
}

func TestRepeatedKeysPreserved(t *testing.T) {
	const sec = "/script/shootergame.shootergamemode"
	src := "[" + sec + "]\r\n" +
		"OverrideNamedEngramEntries=A\r\n" +
		"OverrideNamedEngramEntries=B\r\n" +
		"OverrideNamedEngramEntries=C\r\n"

	f, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := f.GetAll(sec, "OverrideNamedEngramEntries")
	want := []string{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("GetAll length %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GetAll[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if last, _ := f.Get(sec, "OverrideNamedEngramEntries"); last != "C" {
		t.Errorf("Get last = %q, want C", last)
	}
	// Round-trip preserves all three.
	out, _ := f.Marshal()
	if string(out) != src {
		t.Errorf("round-trip mismatch:\nwant: %q\n got: %q", src, string(out))
	}
}

func TestSetUpdatesInPlace(t *testing.T) {
	src := "[ServerSettings]\r\nDifficultyOffset=1.0\r\nTamingSpeedMultiplier=5.0\r\n"
	f, _ := Parse(strings.NewReader(src))
	f.Set("ServerSettings", "DifficultyOffset", "2.0")
	got := f.String()
	want := "[ServerSettings]\r\nDifficultyOffset=2.0\r\nTamingSpeedMultiplier=5.0\r\n"
	if got != want {
		t.Errorf("Set in place\n got: %q\nwant: %q", got, want)
	}
}

func TestSetCreatesSectionAndKey(t *testing.T) {
	f := New()
	f.Set("ServerSettings", "XPMultiplier", "2.0")
	got := f.String()
	want := "[ServerSettings]\r\nXPMultiplier=2.0\r\n"
	if got != want {
		t.Errorf("Set new\n got: %q\nwant: %q", got, want)
	}
}

func TestSetAllReplacesRepeatedKeys(t *testing.T) {
	const sec = "/script/shootergame.shootergamemode"
	src := "[" + sec + "]\r\n" +
		"OverrideNamedEngramEntries=A\r\n" +
		"OverrideNamedEngramEntries=B\r\n"
	f, _ := Parse(strings.NewReader(src))
	f.SetAll(sec, "OverrideNamedEngramEntries", []string{"X", "Y", "Z"})
	got := f.String()
	want := "[" + sec + "]\r\n" +
		"OverrideNamedEngramEntries=X\r\n" +
		"OverrideNamedEngramEntries=Y\r\n" +
		"OverrideNamedEngramEntries=Z\r\n"
	if got != want {
		t.Errorf("SetAll\n got: %q\nwant: %q", got, want)
	}
}

func TestDeleteRemovesAllOccurrences(t *testing.T) {
	src := "[s]\r\nA=1\r\nB=2\r\nA=3\r\n"
	f, _ := Parse(strings.NewReader(src))
	f.Delete("s", "A")
	got := f.String()
	want := "[s]\r\nB=2\r\n"
	if got != want {
		t.Errorf("Delete\n got: %q\nwant: %q", got, want)
	}
}

func TestAppendInsertsBeforeNextSection(t *testing.T) {
	src := "[s]\r\nA=1\r\n\r\n[t]\r\nC=3\r\n"
	f, _ := Parse(strings.NewReader(src))
	f.Append("s", "B", "2")
	got := f.String()
	// B=2 lands inside [s] (before the blank that separates the
	// sections), so [t] stays intact.
	want := "[s]\r\nA=1\r\nB=2\r\n\r\n[t]\r\nC=3\r\n"
	if got != want {
		t.Errorf("Append\n got: %q\nwant: %q", got, want)
	}
}

func TestParseLFOnly(t *testing.T) {
	src := "[s]\nA=1\n"
	f, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, _ := f.Get("s", "A"); got != "1" {
		t.Errorf("Get = %q, want 1", got)
	}
	out, _ := f.Marshal()
	if string(out) != src {
		t.Errorf("round-trip LF\n got: %q\nwant: %q", string(out), src)
	}
}

func TestSectionsInOrder(t *testing.T) {
	src := "[a]\nA=1\n[b]\nB=2\n[c]\nC=3\n"
	f, _ := Parse(strings.NewReader(src))
	secs := f.Sections()
	want := []string{"a", "b", "c"}
	if len(secs) != len(want) {
		t.Fatalf("sections len = %d, want %d", len(secs), len(want))
	}
	for i := range want {
		if secs[i] != want[i] {
			t.Errorf("sections[%d] = %q, want %q", i, secs[i], want[i])
		}
	}
}
