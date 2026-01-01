package smversion_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version/smversion"
)

func TestParse(t *testing.T) {
	t.Parallel()
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
			wantID:   lo.ToPtr("abc123"),
		},
		{
			name:     "with UUID-like ID",
			input:    "my-secret#550e8400-e29b-41d4-a716-446655440000",
			wantName: "my-secret",
			wantID:   lo.ToPtr("550e8400-e29b-41d4-a716-446655440000"),
		},
		{
			name:     "with short ID",
			input:    "my-secret#a",
			wantName: "my-secret",
			wantID:   lo.ToPtr("a"),
		},
		{
			name:     "with numeric ID",
			input:    "my-secret#12345",
			wantName: "my-secret",
			wantID:   lo.ToPtr("12345"),
		},

		// Label specifier (:LABEL)
		{
			name:      "with AWSCURRENT label",
			input:     "my-secret:AWSCURRENT",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("AWSCURRENT"),
		},
		{
			name:      "with AWSPREVIOUS label",
			input:     "my-secret:AWSPREVIOUS",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("AWSPREVIOUS"),
		},
		{
			name:      "with AWSPENDING label",
			input:     "my-secret:AWSPENDING",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("AWSPENDING"),
		},
		{
			name:      "with custom label",
			input:     "my-secret:STAGING",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("STAGING"),
		},
		{
			name:      "with lowercase label",
			input:     "my-secret:production",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("production"),
		},
		{
			name:      "with label containing dash",
			input:     "my-secret:my-label",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("my-label"),
		},
		{
			name:      "with label containing underscore",
			input:     "my-secret:my_label",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("my_label"),
		},
		{
			name:      "with label containing digits",
			input:     "my-secret:v2",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("v2"),
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
			wantID:    lo.ToPtr("abc123"),
			wantShift: 1,
		},
		{
			name:      "ID and bare tilde",
			input:     "my-secret#abc~",
			wantName:  "my-secret",
			wantID:    lo.ToPtr("abc"),
			wantShift: 1,
		},

		// Label + Shift
		{
			name:      "label and shift",
			input:     "my-secret:AWSPREVIOUS~1",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("AWSPREVIOUS"),
			wantShift: 1,
		},
		{
			name:      "label and bare tilde",
			input:     "my-secret:STAGING~",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("STAGING"),
			wantShift: 1,
		},
		{
			name:      "label and double tilde",
			input:     "my-secret:AWSCURRENT~~",
			wantName:  "my-secret",
			wantLabel: lo.ToPtr("AWSCURRENT"),
			wantShift: 2,
		},

		// Dots in names
		{
			name:     "name with dots",
			input:    "app.config.db",
			wantName: "app.config.db",
		},
		{
			name:     "name with dots and ID",
			input:    "app.config.db#abc123",
			wantName: "app.config.db",
			wantID:   lo.ToPtr("abc123"),
		},
		{
			name:      "name with dots and label",
			input:     "app.config.db:AWSCURRENT",
			wantName:  "app.config.db",
			wantLabel: lo.ToPtr("AWSCURRENT"),
		},
		{
			name:      "name with dots and shift",
			input:     "app.config.db~1",
			wantName:  "app.config.db",
			wantShift: 1,
		},
		{
			name:     "name ending with dot",
			input:    "config.v1.0.",
			wantName: "config.v1.0.",
		},
		{
			name:     "multiple consecutive dots",
			input:    "app..config",
			wantName: "app..config",
		},

		// Underscores mixed
		{
			name:     "name with underscores",
			input:    "app_config_db",
			wantName: "app_config_db",
		},
		{
			name:     "mixed special chars in name",
			input:    "app.config-v1_2",
			wantName: "app.config-v1_2",
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
			wantLabel: lo.ToPtr("AWSCURRENT"),
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
			wantID:   lo.ToPtr("abc123"),
		},
		{
			name:      "email-like name with label",
			input:     "user@example.com:AWSCURRENT",
			wantName:  "user@example.com",
			wantLabel: lo.ToPtr("AWSCURRENT"),
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
			name:    "both label and ID (reverse order)",
			input:   "my-secret:AWSCURRENT#abc123",
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
			wantID:   lo.ToPtr("abc123"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec, err := smversion.Parse(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, spec.Name)
			assert.Equal(t, tt.wantID, spec.Absolute.ID)
			assert.Equal(t, tt.wantLabel, spec.Absolute.Label)
			assert.Equal(t, tt.wantShift, spec.Shift)
		})
	}
}

func TestSpec_HasShift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec *smversion.Spec
		want bool
	}{
		{
			name: "no shift",
			spec: &smversion.Spec{Name: "my-secret", Shift: 0},
			want: false,
		},
		{
			name: "with shift 1",
			spec: &smversion.Spec{Name: "my-secret", Shift: 1},
			want: true,
		},
		{
			name: "with shift 5",
			spec: &smversion.Spec{Name: "my-secret", Shift: 5},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.spec.HasShift())
		})
	}
}

func TestParseDiffArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *smversion.Spec
		wantSpec2  *smversion.Spec
		wantErrMsg string
	}{
		{
			name: "one arg with label",
			args: []string{"my-secret:AWSPREVIOUS"},
			wantSpec1: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")},
			},
			wantSpec2: &smversion.Spec{
				Name: "my-secret",
			},
		},
		{
			name: "two args with version ID",
			args: []string{"my-secret#abc123", "#def456"},
			wantSpec1: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{ID: lo.ToPtr("abc123")},
			},
			wantSpec2: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{ID: lo.ToPtr("def456")},
			},
		},
		{
			name: "three args with labels",
			args: []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")},
			},
			wantSpec2: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")},
			},
		},
		{
			name:       "no arguments",
			args:       []string{},
			wantErrMsg: "usage:",
		},
		{
			name:       "invalid spec",
			args:       []string{"my-secret#"},
			wantErrMsg: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec1, spec2, err := smversion.ParseDiffArgs(tt.args)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSpec1, spec1)
			assert.Equal(t, tt.wantSpec2, spec2)
		})
	}
}
