package ssmversion

import (
	"testing"

	"github.com/mpyw/suve/internal/testutil"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion *int64
		wantShift   int
		wantErr     bool
	}{
		// Basic cases
		{
			name:     "simple name",
			input:    "/my/param",
			wantName: "/my/param",
		},
		{
			name:     "simple name without slash",
			input:    "my-param",
			wantName: "my-param",
		},

		// Version specifier
		{
			name:        "with version",
			input:       "/my/param#3",
			wantName:    "/my/param",
			wantVersion: testutil.Ptr(int64(3)),
		},
		{
			name:        "with version 0",
			input:       "/my/param#0",
			wantName:    "/my/param",
			wantVersion: testutil.Ptr(int64(0)),
		},
		{
			name:        "with version 1",
			input:       "/my/param#1",
			wantName:    "/my/param",
			wantVersion: testutil.Ptr(int64(1)),
		},
		{
			name:        "with large version",
			input:       "/my/param#999999",
			wantName:    "/my/param",
			wantVersion: testutil.Ptr(int64(999999)),
		},

		// Shift specifier
		{
			name:      "with shift ~1",
			input:     "/my/param~1",
			wantName:  "/my/param",
			wantShift: 1,
		},
		{
			name:      "with shift ~2",
			input:     "/my/param~2",
			wantName:  "/my/param",
			wantShift: 2,
		},
		{
			name:      "with bare tilde",
			input:     "/my/param~",
			wantName:  "/my/param",
			wantShift: 1,
		},
		{
			name:      "with double tilde",
			input:     "/my/param~~",
			wantName:  "/my/param",
			wantShift: 2,
		},
		{
			name:      "with triple tilde",
			input:     "/my/param~~~",
			wantName:  "/my/param",
			wantShift: 3,
		},
		{
			name:      "with cumulative shift",
			input:     "/my/param~1~2",
			wantName:  "/my/param",
			wantShift: 3,
		},
		{
			name:      "with shift ~0",
			input:     "/my/param~0",
			wantName:  "/my/param",
			wantShift: 0,
		},

		// Version + Shift
		{
			name:        "version and shift",
			input:       "/my/param#5~2",
			wantName:    "/my/param",
			wantVersion: testutil.Ptr(int64(5)),
			wantShift:   2,
		},
		{
			name:        "version and bare tilde",
			input:       "/my/param#3~",
			wantName:    "/my/param",
			wantVersion: testutil.Ptr(int64(3)),
			wantShift:   1,
		},
		{
			name:        "version and double tilde",
			input:       "/my/param#10~~",
			wantName:    "/my/param",
			wantVersion: testutil.Ptr(int64(10)),
			wantShift:   2,
		},

		// Whitespace handling
		{
			name:     "whitespace trimmed",
			input:    "  /my/param  ",
			wantName: "/my/param",
		},
		{
			name:        "whitespace with version",
			input:       "  /my/param#3  ",
			wantName:    "/my/param",
			wantVersion: testutil.Ptr(int64(3)),
		},

		// Error cases
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "hash at end",
			input:   "/my/param#",
			wantErr: true,
		},
		{
			name:    "hash followed by non-digit",
			input:   "/my/param#abc",
			wantErr: true,
		},
		{
			name:    "negative version syntax",
			input:   "/my/param#-1",
			wantErr: true,
		},
		{
			name:    "starts with #",
			input:   "#3",
			wantErr: true, // empty name
		},
		{
			name:    "starts with ~",
			input:   "~1",
			wantErr: true, // empty name
		},
		{
			name:    "version followed by unexpected chars",
			input:   "/param#123/value",
			wantErr: true,
		},
		{
			name:    "tilde followed by letter (ambiguous)",
			input:   "/my/param~backup",
			wantErr: true,
		},
		{
			name:    "tilde in middle of path",
			input:   "/home/user~old/file",
			wantErr: true,
		},
		{
			name:    "shift followed by special char",
			input:   "/my/param~1!",
			wantErr: true,
		},
		{
			name:    "shift followed by slash",
			input:   "/my/param~1/extra",
			wantErr: true,
		},
		{
			name:    "version then shift then extra",
			input:   "/my/param#3~1!",
			wantErr: true,
		},
		{
			name:    "version number overflow",
			input:   "/my/param#99999999999999999999",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := Parse(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Parse() unexpected error: %v", err)
				return
			}

			if spec.Name != tt.wantName {
				t.Errorf("Parse() Name = %q, want %q", spec.Name, tt.wantName)
			}

			if !testutil.PtrEqual(spec.Absolute.Version, tt.wantVersion) {
				t.Errorf("Parse() Version = %v, want %v", spec.Absolute.Version, tt.wantVersion)
			}

			if spec.Shift != tt.wantShift {
				t.Errorf("Parse() Shift = %d, want %d", spec.Shift, tt.wantShift)
			}
		})
	}
}

func TestSpec_HasShift(t *testing.T) {
	tests := []struct {
		name string
		spec *Spec
		want bool
	}{
		{
			name: "no shift",
			spec: &Spec{Name: "/my/param", Shift: 0},
			want: false,
		},
		{
			name: "with shift 1",
			spec: &Spec{Name: "/my/param", Shift: 1},
			want: true,
		},
		{
			name: "with shift 5",
			spec: &Spec{Name: "/my/param", Shift: 5},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.HasShift(); got != tt.want {
				t.Errorf("Spec.HasShift() = %v, want %v", got, tt.want)
			}
		})
	}
}
