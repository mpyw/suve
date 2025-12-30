package smversion

import (
	"testing"

	"github.com/mpyw/suve/internal/testutil"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantName  string
		wantID    *string
		wantLabel *string
		wantShift int
		wantErr   bool
	}{
		// Basic cases
		{
			name:     "simple name",
			input:    "my-secret",
			wantName: "my-secret",
		},
		{
			name:     "name with slashes",
			input:    "prod/db/password",
			wantName: "prod/db/password",
		},
		{
			name:     "name with dashes",
			input:    "my-app-secret-key",
			wantName: "my-app-secret-key",
		},

		// ID specifier (#uuid)
		{
			name:     "with ID",
			input:    "my-secret#abc123",
			wantName: "my-secret",
			wantID:   testutil.Ptr("abc123"),
		},
		{
			name:     "with UUID-like ID",
			input:    "my-secret#550e8400-e29b-41d4-a716-446655440000",
			wantName: "my-secret",
			wantID:   testutil.Ptr("550e8400-e29b-41d4-a716-446655440000"),
		},
		{
			name:     "with short ID",
			input:    "my-secret#a",
			wantName: "my-secret",
			wantID:   testutil.Ptr("a"),
		},
		{
			name:     "with numeric ID",
			input:    "my-secret#12345",
			wantName: "my-secret",
			wantID:   testutil.Ptr("12345"),
		},

		// Label specifier (:LABEL)
		{
			name:      "with AWSCURRENT label",
			input:     "my-secret:AWSCURRENT",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("AWSCURRENT"),
		},
		{
			name:      "with AWSPREVIOUS label",
			input:     "my-secret:AWSPREVIOUS",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("AWSPREVIOUS"),
		},
		{
			name:      "with AWSPENDING label",
			input:     "my-secret:AWSPENDING",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("AWSPENDING"),
		},
		{
			name:      "with custom label",
			input:     "my-secret:STAGING",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("STAGING"),
		},
		{
			name:      "with lowercase label",
			input:     "my-secret:production",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("production"),
		},
		{
			name:      "with label containing dash",
			input:     "my-secret:my-label",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("my-label"),
		},
		{
			name:      "with label containing underscore",
			input:     "my-secret:my_label",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("my_label"),
		},
		{
			name:      "with label containing digits",
			input:     "my-secret:v2",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("v2"),
		},

		// Shift specifier
		{
			name:      "with shift ~1",
			input:     "my-secret~1",
			wantName:  "my-secret",
			wantShift: 1,
		},
		{
			name:      "with bare tilde",
			input:     "my-secret~",
			wantName:  "my-secret",
			wantShift: 1,
		},
		{
			name:      "with double tilde",
			input:     "my-secret~~",
			wantName:  "my-secret",
			wantShift: 2,
		},
		{
			name:      "with cumulative shift",
			input:     "my-secret~1~2",
			wantName:  "my-secret",
			wantShift: 3,
		},
		{
			name:      "with shift ~0",
			input:     "my-secret~0",
			wantName:  "my-secret",
			wantShift: 0,
		},

		// ID + Shift
		{
			name:      "ID and shift",
			input:     "my-secret#abc123~1",
			wantName:  "my-secret",
			wantID:    testutil.Ptr("abc123"),
			wantShift: 1,
		},
		{
			name:      "ID and bare tilde",
			input:     "my-secret#abc~",
			wantName:  "my-secret",
			wantID:    testutil.Ptr("abc"),
			wantShift: 1,
		},

		// Label + Shift
		{
			name:      "label and shift",
			input:     "my-secret:AWSPREVIOUS~1",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("AWSPREVIOUS"),
			wantShift: 1,
		},
		{
			name:      "label and bare tilde",
			input:     "my-secret:STAGING~",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("STAGING"),
			wantShift: 1,
		},
		{
			name:      "label and double tilde",
			input:     "my-secret:AWSCURRENT~~",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("AWSCURRENT"),
			wantShift: 2,
		},

		// Whitespace handling
		{
			name:     "whitespace trimmed",
			input:    "  my-secret  ",
			wantName: "my-secret",
		},
		{
			name:      "whitespace with label",
			input:     "  my-secret:AWSCURRENT  ",
			wantName:  "my-secret",
			wantLabel: testutil.Ptr("AWSCURRENT"),
		},

		// @ in name (allowed in SM secret names)
		{
			name:     "at sign at end",
			input:    "my-secret@",
			wantName: "my-secret@",
		},
		{
			name:     "email-like name",
			input:    "user@example.com",
			wantName: "user@example.com",
		},
		{
			name:     "at sign in middle",
			input:    "config@prod",
			wantName: "config@prod",
		},
		{
			name:     "email-like name with ID",
			input:    "user@example.com#abc123",
			wantName: "user@example.com",
			wantID:   testutil.Ptr("abc123"),
		},
		{
			name:      "email-like name with label",
			input:     "user@example.com:AWSCURRENT",
			wantName:  "user@example.com",
			wantLabel: testutil.Ptr("AWSCURRENT"),
		},

		// # in name (not an ID specifier - not followed by valid ID char)
		{
			name:    "hash at end without value",
			input:   "my-secret#",
			wantErr: true,
		},

		// : in name (not a label specifier)
		{
			name:    "colon at end without value",
			input:   "my-secret:",
			wantErr: true,
		},

		// Tilde followed by letter (ambiguous - error)
		{
			name:    "tilde followed by letter (ambiguous)",
			input:   "my-secret~backup",
			wantErr: true,
		},
		{
			name:    "tilde followed by uppercase (ambiguous)",
			input:   "my-secret~OLD",
			wantErr: true,
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
			name:    "both ID and label",
			input:   "my-secret#abc123:AWSCURRENT",
			wantErr: true,
		},
		{
			name:    "starts with #",
			input:   "#abc123",
			wantErr: true, // empty name
		},
		{
			name:    "starts with :",
			input:   ":AWSCURRENT",
			wantErr: true, // empty name
		},
		{
			name:    "starts with ~",
			input:   "~1",
			wantErr: true, // empty name
		},
		{
			name:    "invalid label with special char",
			input:   "my-secret:LABEL!",
			wantErr: true,
		},
		{
			name:    "invalid label with space",
			input:   "my-secret:LABEL NAME",
			wantErr: true,
		},
		{
			name:    "shift followed by special char",
			input:   "my-secret~1!",
			wantErr: true,
		},
		{
			name:    "shift followed by slash",
			input:   "my-secret~1/extra",
			wantErr: true,
		},
		{
			name:    "ID then shift then extra",
			input:   "my-secret#abc~1!",
			wantErr: true,
		},
		{
			name:    "label then shift then extra",
			input:   "my-secret:AWSCURRENT~1!",
			wantErr: true,
		},
		{
			name:    "tilde in middle of name",
			input:   "my~secret/name",
			wantErr: true,
		},
		// ID followed by another # (triggers findNextSpecifier # branch)
		{
			name:    "ID with second hash",
			input:   "my-secret#user#domain",
			wantErr: true, // leftover after parsing first #id
		},
		// @ is allowed in secret names and doesn't conflict with #id
		{
			name:     "at signs in name with ID",
			input:    "user@host@domain#abc123",
			wantName: "user@host@domain",
			wantID:   testutil.Ptr("abc123"),
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

			if !testutil.PtrEqual(spec.ID, tt.wantID) {
				t.Errorf("Parse() ID = %v, want %v", spec.ID, tt.wantID)
			}

			if !testutil.PtrEqual(spec.Label, tt.wantLabel) {
				t.Errorf("Parse() Label = %v, want %v", spec.Label, tt.wantLabel)
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
			spec: &Spec{Name: "my-secret", Shift: 0},
			want: false,
		},
		{
			name: "with shift 1",
			spec: &Spec{Name: "my-secret", Shift: 1},
			want: true,
		},
		{
			name: "with shift 5",
			spec: &Spec{Name: "my-secret", Shift: 5},
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
