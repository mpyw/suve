// White-box tests for diff.go's argument parsing.
//declscope:namespace diff

package secret

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version"
)

func TestParseDiffArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *version.OpaqueSpec
		wantSpec2  *version.OpaqueSpec
		wantErrMsg string
	}{
		{
			name: "one arg with label",
			args: []string{"my-secret:AWSPREVIOUS"},
			wantSpec1: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{Label: lo.ToPtr("AWSPREVIOUS")},
			},
			wantSpec2: &version.OpaqueSpec{
				Name: "my-secret",
			},
		},
		{
			name: "two args with version ID",
			args: []string{"my-secret#abc123", "#def456"},
			wantSpec1: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{ID: lo.ToPtr("abc123")},
			},
			wantSpec2: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{ID: lo.ToPtr("def456")},
			},
		},
		{
			name: "three args with labels",
			args: []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{Label: lo.ToPtr("AWSPREVIOUS")},
			},
			wantSpec2: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{Label: lo.ToPtr("AWSCURRENT")},
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

			spec1, spec2, err := parseDiffArgs(tt.args)

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
