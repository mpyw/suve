package ssmversion_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version/ssmversion"
)

func TestParse(t *testing.T) {
	t.Parallel()
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
			wantVersion: lo.ToPtr(int64(3)),
		},
		{
			name:        "with version 0",
			input:       "/my/param#0",
			wantName:    "/my/param",
			wantVersion: lo.ToPtr(int64(0)),
		},
		{
			name:        "with version 1",
			input:       "/my/param#1",
			wantName:    "/my/param",
			wantVersion: lo.ToPtr(int64(1)),
		},
		{
			name:        "with large version",
			input:       "/my/param#999999",
			wantName:    "/my/param",
			wantVersion: lo.ToPtr(int64(999999)),
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
			wantVersion: lo.ToPtr(int64(5)),
			wantShift:   2,
		},
		{
			name:        "version and bare tilde",
			input:       "/my/param#3~",
			wantName:    "/my/param",
			wantVersion: lo.ToPtr(int64(3)),
			wantShift:   1,
		},
		{
			name:        "version and double tilde",
			input:       "/my/param#10~~",
			wantName:    "/my/param",
			wantVersion: lo.ToPtr(int64(10)),
			wantShift:   2,
		},

		// Tilde in name (not a shift specifier)
		{
			name:        "tilde followed by special char then version",
			input:       "/my/param~@test#3",
			wantName:    "/my/param~@test",
			wantVersion: lo.ToPtr(int64(3)),
		},

		// Dots in names
		{
			name:     "name with dots",
			input:    "/app.config/db.url",
			wantName: "/app.config/db.url",
		},
		{
			name:        "name with dots and version",
			input:       "/app.config/db.url#3",
			wantName:    "/app.config/db.url",
			wantVersion: lo.ToPtr(int64(3)),
		},
		{
			name:      "name with dots and shift",
			input:     "/app.config/db.url~1",
			wantName:  "/app.config/db.url",
			wantShift: 1,
		},
		{
			name:     "name ending with dot",
			input:    "/config/v1.0.",
			wantName: "/config/v1.0.",
		},
		{
			name:     "multiple consecutive dots",
			input:    "/app../config",
			wantName: "/app../config",
		},

		// Underscores and dashes
		{
			name:     "name with underscores",
			input:    "/app_config/db_url",
			wantName: "/app_config/db_url",
		},
		{
			name:        "name with dashes and version",
			input:       "/app-config/db-url#5",
			wantName:    "/app-config/db-url",
			wantVersion: lo.ToPtr(int64(5)),
		},
		{
			name:     "mixed special chars in name",
			input:    "/app.config-v1_2/db.url",
			wantName: "/app.config-v1_2/db.url",
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
			wantVersion: lo.ToPtr(int64(3)),
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
			t.Parallel()
			spec, err := ssmversion.Parse(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, spec.Name)
			assert.Equal(t, tt.wantVersion, spec.Absolute.Version)
			assert.Equal(t, tt.wantShift, spec.Shift)
		})
	}
}

func TestSpec_HasShift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec *ssmversion.Spec
		want bool
	}{
		{
			name: "no shift",
			spec: &ssmversion.Spec{Name: "/my/param", Shift: 0},
			want: false,
		},
		{
			name: "with shift 1",
			spec: &ssmversion.Spec{Name: "/my/param", Shift: 1},
			want: true,
		},
		{
			name: "with shift 5",
			spec: &ssmversion.Spec{Name: "/my/param", Shift: 5},
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
