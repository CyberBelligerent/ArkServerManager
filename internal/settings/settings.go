package settings

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"asamanager/internal/inifile"
)

const (
	SectionShooterGameMode = "/script/shootergame.shootergamemode" // Game.ini
	SectionServerSettings  = "ServerSettings"                      // GameUserSettings.ini
	SectionSessionSettings = "SessionSettings"                     // GameUserSettings.ini
)

type File int

const (
	FileGame File = iota
	FileGameUserSettings
)

func (f File) String() string {
	switch f {
	case FileGame:
		return "Game.ini"
	case FileGameUserSettings:
		return "GameUserSettings.ini"
	default:
		return "unknown"
	}
}

type Type int

const (
	TypeFloat Type = iota
	TypeInt
	TypeBool
	TypeString
)

type Setting struct {
	Key      string
	Section  string
	File     File
	Type     Type
	Default  any
	Min      *float64
	Max      *float64
	Category string
	Tooltip  string
}

type Value struct {
	Float  float64
	Int    int
	Bool   bool
	String string
}

func FloatVal(f float64) Value  { return Value{Float: f} }
func IntVal(n int) Value        { return Value{Int: n} }
func BoolVal(b bool) Value      { return Value{Bool: b} }
func StringVal(s string) Value  { return Value{String: s} }

func (v Value) Format(t Type) string {
	switch t {
	case TypeFloat:
		return formatFloat(v.Float)
	case TypeInt:
		return strconv.Itoa(v.Int)
	case TypeBool:
		if v.Bool {
			return "True"
		}
		return "False"
	case TypeString:
		return v.String
	}
	return ""
}

func ParseValue(t Type, raw string) (Value, error) {
	raw = strings.TrimSpace(raw)
	switch t {
	case TypeFloat:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return Value{}, fmt.Errorf("parse float %q: %w", raw, err)
		}
		return Value{Float: f}, nil
	case TypeInt:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Value{}, fmt.Errorf("parse int %q: %w", raw, err)
		}
		return Value{Int: n}, nil
	case TypeBool:
		switch strings.ToLower(raw) {
		case "true", "1", "yes":
			return Value{Bool: true}, nil
		case "false", "0", "no":
			return Value{Bool: false}, nil
		default:
			return Value{}, fmt.Errorf("parse bool %q: not a boolean", raw)
		}
	case TypeString:
		return Value{String: raw}, nil
	}
	return Value{}, fmt.Errorf("unknown type %d", t)
}

func (s Setting) DefaultValue() Value {
	switch s.Type {
	case TypeFloat:
		return Value{Float: toFloat(s.Default)}
	case TypeInt:
		return Value{Int: toInt(s.Default)}
	case TypeBool:
		return Value{Bool: toBool(s.Default)}
	case TypeString:
		return Value{String: toString(s.Default)}
	}
	return Value{}
}

func (s Setting) Validate(v Value) error {
	if s.Type != TypeFloat && s.Type != TypeInt {
		return nil
	}
	var x float64
	if s.Type == TypeFloat {
		x = v.Float
	} else {
		x = float64(v.Int)
	}
	if s.Min != nil && x < *s.Min {
		return fmt.Errorf("%s=%v below minimum %v", s.Key, x, *s.Min)
	}
	if s.Max != nil && x > *s.Max {
		return fmt.Errorf("%s=%v above maximum %v", s.Key, x, *s.Max)
	}
	return nil
}

func toFloat(x any) float64 {
	switch v := x.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	}
	return 0
}

func toInt(x any) int {
	switch v := x.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

func toBool(x any) bool {
	if b, ok := x.(bool); ok {
		return b
	}
	return false
}

func toString(x any) string {
	if s, ok := x.(string); ok {
		return s
	}
	return ""
}

func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

type Registry struct {
	byKey map[string]Setting
	order []string
}

func (r *Registry) All() []Setting {
	out := make([]Setting, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.byKey[k])
	}
	return out
}

func (r *Registry) Lookup(key string) (Setting, bool) {
	s, ok := r.byKey[key]
	return s, ok
}

type CategoryGroup struct {
	Name     string
	Settings []Setting
}

func (r *Registry) ByCategory() []CategoryGroup {
	var groups []CategoryGroup
	idx := map[string]int{}
	for _, k := range r.order {
		s := r.byKey[k]
		i, ok := idx[s.Category]
		if !ok {
			idx[s.Category] = len(groups)
			groups = append(groups, CategoryGroup{Name: s.Category})
			i = len(groups) - 1
		}
		groups[i].Settings = append(groups[i].Settings, s)
	}
	return groups
}

func Default() *Registry { return defaultRegistry }

var defaultRegistry = buildRegistry(allSettings)

func buildRegistry(specs []Setting) *Registry {
	r := &Registry{byKey: make(map[string]Setting, len(specs))}
	for _, s := range specs {
		if _, dup := r.byKey[s.Key]; dup {
			panic("duplicate setting key: " + s.Key)
		}
		r.byKey[s.Key] = s
		r.order = append(r.order, s.Key)
	}
	return r
}

func ApplyTo(r *Registry, f *inifile.File, values map[string]Value) {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s, ok := r.Lookup(k)
		if !ok {
			continue
		}
		f.Set(s.Section, s.Key, values[k].Format(s.Type))
	}
}

func ReadFrom(r *Registry, f *inifile.File) (map[string]Value, []error) {
	out := make(map[string]Value, len(r.order))
	var errs []error
	for _, k := range r.order {
		s := r.byKey[k]
		raw, ok := f.Get(s.Section, s.Key)
		if !ok {
			out[k] = s.DefaultValue()
			continue
		}
		v, err := ParseValue(s.Type, raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", k, err))
			out[k] = s.DefaultValue()
			continue
		}
		out[k] = v
	}
	return out, errs
}

func FilterByFile(r *Registry, values map[string]Value, file File) map[string]Value {
	out := make(map[string]Value, len(values))
	for k, v := range values {
		s, ok := r.Lookup(k)
		if !ok || s.File != file {
			continue
		}
		out[k] = v
	}
	return out
}
