package cluster

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateClusterID_Syntax(t *testing.T) {
	cases := []struct {
		id      string
		wantErr error
	}{
		{"", ErrClusterIDEmpty},
		{"   ", ErrClusterIDEmpty},
		{"ab", ErrClusterIDFormat},                    // too short
		{"_abc", ErrClusterIDFormat},                  // can't start with _
		{"-abc", ErrClusterIDFormat},                  // can't start with -
		{"abc!", ErrClusterIDFormat},                  // bad char
		{"abc def", ErrClusterIDFormat},               // space
		{strings.Repeat("a", 65), ErrClusterIDFormat}, // too long
		{"abc", nil},
		{strings.Repeat("a", 64), nil}, // exactly max
		{"My-Cluster_01", nil},
		{"123abc", nil},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			err := ValidateClusterID(c.id, nil)
			if c.wantErr == nil {
				if err != nil {
					t.Errorf("got %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Errorf("got %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestValidateClusterID_Uniqueness(t *testing.T) {
	existing := []string{"alpha", "Bravo"}

	if err := ValidateClusterID("charlie", existing); err != nil {
		t.Fatalf("unique id rejected: %v", err)
	}
	if err := ValidateClusterID("alpha", existing); !errors.Is(err, ErrClusterIDDuplicate) {
		t.Errorf("expected duplicate error, got %v", err)
	}
	if err := ValidateClusterID("ALPHA", existing); !errors.Is(err, ErrClusterIDDuplicate) {
		t.Errorf("expected case-insensitive duplicate, got %v", err)
	}
	if err := ValidateClusterID("bravo", existing); !errors.Is(err, ErrClusterIDDuplicate) {
		t.Errorf("expected case-insensitive duplicate, got %v", err)
	}
}

func TestSuggestClusterID_Unique(t *testing.T) {
	a := SuggestClusterID("My Awesome Cluster!!")
	b := SuggestClusterID("My Awesome Cluster!!")
	if a == b {
		t.Errorf("expected distinct suggestions, got %q twice", a)
	}
	if err := ValidateClusterID(a, nil); err != nil {
		t.Errorf("suggested ID %q failed validation: %v", a, err)
	}
	if err := ValidateClusterID(b, nil); err != nil {
		t.Errorf("suggested ID %q failed validation: %v", b, err)
	}
}

func TestSuggestClusterID_FallsBackForGarbage(t *testing.T) {
	id := SuggestClusterID("!!!")
	if !strings.HasPrefix(id, "cluster-") {
		t.Errorf("expected cluster- prefix for garbage base, got %q", id)
	}
	if err := ValidateClusterID(id, nil); err != nil {
		t.Errorf("fallback ID %q failed validation: %v", id, err)
	}
}

func TestSuggestClusterID_TruncatesLongBase(t *testing.T) {
	long := strings.Repeat("abc", 30) // 90 chars; sanitizeBase trims to 32
	id := SuggestClusterID(long)
	if err := ValidateClusterID(id, nil); err != nil {
		t.Errorf("truncated ID %q failed validation: %v", id, err)
	}
	if len(id) > 64 {
		t.Errorf("ID length %d exceeds max 64", len(id))
	}
}
