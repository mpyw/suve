// White-box tests for diff.go's argument parsing.
//declscope:namespace diff

package param

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
		wantSpec1  *version.NumericSpec
		wantSpec2  *version.NumericSpec
		wantErrMsg string
	}{
		{
			name: "one arg with version",
			args: []string{"/app/param#3"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: lo.ToPtr(int64(3))},
			},
			wantSpec2: &version.NumericSpec{
				Name: "/app/param",
			},
		},
		{
			name: "two args",
			args: []string{"/app/param#1", "#2"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: lo.ToPtr(int64(1))},
			},
			wantSpec2: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: lo.ToPtr(int64(2))},
			},
		},
		{
			name: "three args",
			args: []string{"/app/param", "#1", "#2"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: lo.ToPtr(int64(1))},
			},
			wantSpec2: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: lo.ToPtr(int64(2))},
			},
		},
		{
			name:       "no arguments",
			args:       []string{},
			wantErrMsg: "usage:",
		},
		{
			name:       "invalid spec",
			args:       []string{"/app/param#"},
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
