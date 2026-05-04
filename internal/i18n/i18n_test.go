package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault_LoadsEmbedded(t *testing.T) {
	b := Default()
	if b.Locale() != DefaultLocale {
		t.Errorf("locale = %q, want %q", b.Locale(), DefaultLocale)
	}
	// Sanity-check that a few known keys are present.
	for _, k := range []string{"common.save", "scheduler.button_add_task", "scheduler.dialog.title_create"} {
		if !b.Has(k) {
			t.Errorf("missing embedded key %q", k)
		}
	}
}

func TestT_SubstitutesArgs(t *testing.T) {
	b := Default()
	got := b.T("scheduler.row_meta", "[ON]", "x", "backup", "cron `0 4 * * *`", "next: —")
	if !strings.HasPrefix(got, "[ON]  x") {
		t.Errorf("unexpected substitution: %q", got)
	}
}

func TestT_MissingKeyReturnsMarker(t *testing.T) {
	b := Default()
	got := b.T("totally.not.real.key")
	if !strings.HasPrefix(got, "[") || !strings.Contains(got, "totally.not.real.key") {
		t.Errorf("missing-key fallback should be a visible marker, got %q", got)
	}
}

func TestT_NilBundleSafe(t *testing.T) {
	var b *Bundle
	got := b.T("anything")
	if got != "[nil:anything]" {
		t.Errorf("nil bundle should return marker, got %q", got)
	}
}

func TestLoad_FallsBackToEmbeddedWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	b, err := Load(dir, "fr-fr")
	if err != nil {
		t.Fatalf("Load returned error for missing file: %v", err)
	}
	// File doesn't exist yet. The caller gets the embedded fallback so the app stays usable
	if b.Locale() != DefaultLocale {
		t.Errorf("locale = %q, want %q (silent fallback)", b.Locale(), DefaultLocale)
	}
}

func TestLoad_OverridesPartialAndFallsBackToEmbedded(t *testing.T) {
	dir := t.TempDir()
	body := `
[common]
save = "Sauvegarder"
`
	if err := os.WriteFile(filepath.Join(dir, "fr-fr.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(dir, "fr-fr")
	if err != nil {
		t.Fatal(err)
	}
	if b.Locale() != "fr-fr" {
		t.Errorf("locale = %q, want fr-fr", b.Locale())
	}
	if got := b.T("common.save"); got != "Sauvegarder" {
		t.Errorf("translated key drifted: got %q", got)
	}
	// Untranslated key falls back to English
	if got := b.T("common.cancel"); got != "Cancel" {
		t.Errorf("fallback to en-us failed: got %q", got)
	}
}

func TestAvailableLocales_AlwaysIncludesDefault(t *testing.T) {
	dir := t.TempDir()
	got := AvailableLocales(dir)
	if len(got) != 1 || got[0] != DefaultLocale {
		t.Errorf("got %v, want [%q]", got, DefaultLocale)
	}
}

func TestAvailableLocales_FindsFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"de-de.toml", "pt-br.toml", "README.md"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644)
	}
	got := AvailableLocales(dir)
	want := map[string]bool{DefaultLocale: true, "de-de": true, "pt-br": true}
	for _, loc := range got {
		if !want[loc] {
			t.Errorf("unexpected locale %q in %v", loc, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %d locales, want 3 (en-us + 2 from disk): %v", len(got), got)
	}
}
