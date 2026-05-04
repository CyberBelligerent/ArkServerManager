package inifile

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type LineKind int

const (
	LineBlank LineKind = iota
	LineComment
	LineSection
	LineKeyValue
)

// Physical line in an ini file
type Line struct {
	Kind    LineKind
	Raw     string
	Section string
	Key     string
	Value   string
}

// In-memory section reader
type File struct {
	lines  []*Line
	eol    string
	hasBOM bool
}

func New() *File {
	return &File{eol: "\r\n"}
}

func Parse(r io.Reader) (*File, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	f := &File{eol: "\r\n"}
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		f.hasBOM = true
		raw = raw[3:]
	}
	if i := strings.IndexByte(string(raw), '\n'); i >= 0 {
		if !(i > 0 && raw[i-1] == '\r') {
			f.eol = "\n"
		}
	}

	currentSection := ""
	for _, line := range splitLines(raw) {
		ln := &Line{Raw: line}
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			ln.Kind = LineBlank
		case isCommentPrefix(trimmed):
			ln.Kind = LineComment
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			ln.Kind = LineSection
			ln.Section = trimmed[1 : len(trimmed)-1]
			currentSection = ln.Section
		default:
			eq := strings.IndexByte(line, '=')
			if eq < 0 {
				ln.Kind = LineComment
			} else {
				ln.Kind = LineKeyValue
				ln.Section = currentSection
				ln.Key = strings.TrimSpace(line[:eq])
				ln.Value = line[eq+1:]
			}
		}
		f.lines = append(f.lines, ln)
	}
	return f, nil
}

// WriteTo serializes the file to w. Lines that were never modified are
// emitted verbatim; key=value lines are rebuilt as `Key=Value`.
func (f *File) WriteTo(w io.Writer) (int64, error) {
	bw := bufio.NewWriter(w)
	var n int64
	if f.hasBOM {
		written, err := bw.Write([]byte{0xEF, 0xBB, 0xBF})
		n += int64(written)
		if err != nil {
			return n, err
		}
	}
	for _, ln := range f.lines {
		out := ln.Raw
		if ln.Kind == LineKeyValue {
			out = ln.Key + "=" + ln.Value
		}
		written, err := bw.WriteString(out)
		n += int64(written)
		if err != nil {
			return n, err
		}
		written, err = bw.WriteString(f.eol)
		n += int64(written)
		if err != nil {
			return n, err
		}
	}
	if err := bw.Flush(); err != nil {
		return n, err
	}
	return n, nil
}

func (f *File) Sections() []string {
	var out []string
	seen := map[string]bool{}
	for _, ln := range f.lines {
		if ln.Kind == LineSection && !seen[ln.Section] {
			seen[ln.Section] = true
			out = append(out, ln.Section)
		}
	}
	return out
}

func (f *File) HasSection(name string) bool {
	for _, ln := range f.lines {
		if ln.Kind == LineSection && ln.Section == name {
			return true
		}
	}
	return false
}

func (f *File) Get(section, key string) (string, bool) {
	var last string
	found := false
	for _, ln := range f.lines {
		if ln.Kind == LineKeyValue && ln.Section == section && ln.Key == key {
			last = ln.Value
			found = true
		}
	}
	return last, found
}

func (f *File) GetAll(section, key string) []string {
	var out []string
	for _, ln := range f.lines {
		if ln.Kind == LineKeyValue && ln.Section == section && ln.Key == key {
			out = append(out, ln.Value)
		}
	}
	return out
}

func (f *File) Set(section, key, value string) {
	first := -1
	kept := f.lines[:0]
	for _, ln := range f.lines {
		if ln.Kind == LineKeyValue && ln.Section == section && ln.Key == key {
			if first == -1 {
				first = len(kept)
				ln.Value = value
				kept = append(kept, ln)
			}
			continue
		}
		kept = append(kept, ln)
	}
	f.lines = kept
	if first >= 0 {
		return
	}
	f.Append(section, key, value)
}

func (f *File) SetAll(section, key string, values []string) {
	if len(values) == 0 {
		f.Delete(section, key)
		return
	}
	insertAt := -1
	kept := f.lines[:0]
	for _, ln := range f.lines {
		if ln.Kind == LineKeyValue && ln.Section == section && ln.Key == key {
			if insertAt == -1 {
				insertAt = len(kept)
			}
			continue
		}
		kept = append(kept, ln)
	}
	f.lines = kept
	if insertAt == -1 {
		for _, v := range values {
			f.Append(section, key, v)
		}
		return
	}
	newLines := make([]*Line, 0, len(values))
	for _, v := range values {
		newLines = append(newLines, &Line{
			Kind:    LineKeyValue,
			Section: section,
			Key:     key,
			Value:   v,
		})
	}
	tail := append([]*Line(nil), f.lines[insertAt:]...)
	f.lines = append(append(f.lines[:insertAt], newLines...), tail...)
}

func (f *File) Append(section, key, value string) {
	if !f.HasSection(section) {
		f.lines = append(f.lines,
			&Line{Kind: LineSection, Section: section, Raw: "[" + section + "]"},
			&Line{Kind: LineKeyValue, Section: section, Key: key, Value: value},
		)
		return
	}
	insertAt := f.sectionEnd(section)
	newLine := &Line{
		Kind:    LineKeyValue,
		Section: section,
		Key:     key,
		Value:   value,
	}
	f.lines = append(f.lines, nil)
	copy(f.lines[insertAt+1:], f.lines[insertAt:])
	f.lines[insertAt] = newLine
}

func (f *File) Delete(section, key string) {
	kept := f.lines[:0]
	for _, ln := range f.lines {
		if ln.Kind == LineKeyValue && ln.Section == section && ln.Key == key {
			continue
		}
		kept = append(kept, ln)
	}
	f.lines = kept
}

func (f *File) EnsureSection(name string) bool {
	if f.HasSection(name) {
		return false
	}
	f.lines = append(f.lines, &Line{
		Kind:    LineSection,
		Section: name,
		Raw:     "[" + name + "]",
	})
	return true
}

func (f *File) sectionEnd(section string) int {
	headerIdx := -1
	for i, ln := range f.lines {
		if ln.Kind == LineSection && ln.Section == section {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return len(f.lines)
	}
	end := len(f.lines)
	for j := headerIdx + 1; j < len(f.lines); j++ {
		if f.lines[j].Kind == LineSection {
			end = j
			break
		}
	}
	for end > headerIdx+1 && f.lines[end-1].Kind == LineBlank {
		end--
	}
	return end
}

func (f *File) Marshal() ([]byte, error) {
	var sb strings.Builder
	if _, err := f.WriteTo(&sb); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

func (f *File) String() string {
	b, err := f.Marshal()
	if err != nil {
		return fmt.Sprintf("<inifile error: %v>", err)
	}
	return string(b)
}

func isCommentPrefix(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c == ';' || c == '#'
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
