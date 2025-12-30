package version

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion *int64
		wantShift   int
		wantLabel   *string
		wantErr     bool
	}{
		{
			name:     "simple name",
			input:    "/my/param",
			wantName: "/my/param",
		},
		{
			name:        "with version",
			input:       "/my/param@3",
			wantName:    "/my/param",
			wantVersion: ptr(int64(3)),
		},
		{
			name:      "with shift",
			input:     "/my/param~1",
			wantName:  "/my/param",
			wantShift: 1,
		},
		{
			name:        "with version and shift",
			input:       "/my/param@5~2",
			wantName:    "/my/param",
			wantVersion: ptr(int64(5)),
			wantShift:   2,
		},
		{
			name:      "with label",
			input:     "my-secret:AWSCURRENT",
			wantName:  "my-secret",
			wantLabel: strPtr("AWSCURRENT"),
		},
		{
			name:      "with label AWSPREVIOUS",
			input:     "my-secret:AWSPREVIOUS",
			wantName:  "my-secret",
			wantLabel: strPtr("AWSPREVIOUS"),
		},
		{
			name:        "full spec",
			input:       "/app/secret@2~1:STAGING",
			wantName:    "/app/secret",
			wantVersion: ptr(int64(2)),
			wantShift:   1,
			wantLabel:   strPtr("STAGING"),
		},
		{
			name:     "whitespace trimmed",
			input:    "  /my/param  ",
			wantName: "/my/param",
		},
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
				t.Errorf("Parse() Name = %v, want %v", spec.Name, tt.wantName)
			}

			if !int64PtrEqual(spec.Version, tt.wantVersion) {
				t.Errorf("Parse() Version = %v, want %v", derefInt64(spec.Version), derefInt64(tt.wantVersion))
			}

			if spec.Shift != tt.wantShift {
				t.Errorf("Parse() Shift = %v, want %v", spec.Shift, tt.wantShift)
			}

			if !strPtrEqual(spec.Label, tt.wantLabel) {
				t.Errorf("Parse() Label = %v, want %v", derefStr(spec.Label), derefStr(tt.wantLabel))
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
			name: "with shift",
			spec: &Spec{Name: "/my/param", Shift: 1},
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

// Helper functions
func ptr(v int64) *int64 {
	return &v
}

func strPtr(s string) *string {
	return &s
}

func int64PtrEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func strPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func derefInt64(p *int64) string {
	if p == nil {
		return "<nil>"
	}
	return string(rune(*p))
}

func derefStr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
