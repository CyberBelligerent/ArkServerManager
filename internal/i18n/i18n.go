// Package i18n provides string localization for the GUI. The runtime
// model is one Bundle per process, loaded at startup from a TOML file
// in ./language/{locale}.toml (relative to the executable). The English
// file is also embedded so the binary still runs if the language folder
// is missing or a translator's file is incomplete — missing keys fall
// back to en-us, then to a visible "[locale:key]" marker that's easy to
// spot in screenshots so translators know what's still untranslated.
//
// Substitution uses fmt.Sprintf — translators must keep "%s" / "%d" /
// "%q" tokens in the right spots in their language. Sprintf was picked
// over named placeholders to avoid pulling a custom template parser
// for what amounts to short UI strings.
package i18n

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

const DefaultLocale = "en-us"

//go:embed embedded/*.toml
var embeddedFS embed.FS

type Bundle struct {
	mu       sync.RWMutex
	locale   string
	strings  map[string]string
	fallback *Bundle 			// optional; chained for fallback lookup
}

func (b *Bundle) Locale() string { return b.locale }

// Default returns a Bundle backed only by the embedded en-us file. Use
// it as a fallback or when the language folder doesn't exist yet.
func Default() *Bundle {
	body, err := embeddedFS.ReadFile("embedded/" + DefaultLocale + ".toml")
	if err != nil {
		panic(fmt.Sprintf("i18n: embedded fallback %q missing: %v", DefaultLocale, err))
	}
	strings, err := parseTOML(body)
	if err != nil {
		panic(fmt.Sprintf("i18n: parse embedded fallback: %v", err))
	}
	return &Bundle{locale: DefaultLocale, strings: strings}
}

// Load reads ./{dir}/{locale}.toml and returns a Bundle. If the file
// is missing or the locale is empty, falls back to the embedded
// default
func Load(dir, locale string) (*Bundle, error) {
	fallback := Default()
	loc := strings.ToLower(strings.TrimSpace(locale))
	if loc == "" || loc == DefaultLocale {
		return fallback, nil
	}

	path := filepath.Join(dir, loc+".toml")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fallback, nil
		}
		return nil, fmt.Errorf("i18n: read %s: %w", path, err)
	}
	parsed, err := parseTOML(body)
	if err != nil {
		return nil, fmt.Errorf("i18n: parse %s: %w", path, err)
	}
	return &Bundle{locale: loc, strings: parsed, fallback: fallback}, nil
}

// T returns the translated string for key with sprintf args applied
func (b *Bundle) T(key string, args ...any) string {
	if b == nil {
		return "[nil:" + key + "]"
	}
	b.mu.RLock()
	s, ok := b.strings[key]
	b.mu.RUnlock()
	if !ok {
		if b.fallback != nil {
			return b.fallback.T(key, args...)
		}
		return "[" + b.locale + ":" + key + "]"
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// Has reports whether key is defined in this bundle
func (b *Bundle) Has(key string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.strings[key]
	return ok
}

// Keys returns every key defined in this bundle, sorted
func (b *Bundle) Keys() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.strings))
	for k := range b.strings {
		out = append(out, k)
	}
	return out
}

func AvailableLocales(dir string) []string {
	seen := map[string]bool{DefaultLocale: true}
	out := []string{DefaultLocale}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}
		loc := strings.ToLower(strings.TrimSuffix(name, ".toml"))
		if seen[loc] {
			continue
		}
		seen[loc] = true
		out = append(out, loc)
	}
	return out
}

func parseTOML(body []byte) (map[string]string, error) {
	tree := map[string]any{}
	if err := toml.Unmarshal(body, &tree); err != nil {
		return nil, err
	}
	out := map[string]string{}
	flatten("", tree, out)
	return out, nil
}

func flatten(prefix string, in map[string]any, out map[string]string) {
	for k, v := range in {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		switch x := v.(type) {
		case string:
			out[full] = x
		case map[string]any:
			flatten(full, x, out)
		default:
			out[full] = fmt.Sprintf("%v", v)
		}
	}
}
