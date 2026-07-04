package gcpversion_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version/gcpversion"
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
		{
			name:     "simple name",
			input:    "my-secret",
			wantName: "my-secret",
		},
		{
			name:     "name with underscores",
			input:    "my_secret_key",
			wantName: "my_secret_key",
		},
		{
			name:        "with version",
			input:       "my-secret#3",
			wantName:    "my-secret",
			wantVersion: lo.ToPtr(int64(3)),
		},
		{
			name:        "with large version",
			input:       "my-secret#128",
			wantName:    "my-secret",
			wantVersion: lo.ToPtr(int64(128)),
		},
		{
			name:      "with single shift",
			input:     "my-secret~",
			wantName:  "my-secret",
			wantShift: 1,
		},
		{
			name:      "with numeric shift",
			input:     "my-secret~2",
			wantName:  "my-secret",
			wantShift: 2,
		},
		{
			name:      "with double tilde",
			input:     "my-secret~~",
			wantName:  "my-secret",
			wantShift: 2,
		},
		{
			name:        "version with shift",
			input:       "my-secret#5~2",
			wantName:    "my-secret",
			wantVersion: lo.ToPtr(int64(5)),
			wantShift:   2,
		},
		{
			name:     "whitespace trimmed",
			input:    "  my-secret  ",
			wantName: "my-secret",
		},

		// :LABEL is rejected (GCP has no staging labels).
		{
			name:    "label rejected",
			input:   "my-secret:latest",
			wantErr: true,
		},
		{
			name:    "label rejected uppercase",
			input:   "my-secret:AWSCURRENT",
			wantErr: true,
		},
		{
			name:    "colon at end rejected",
			input:   "my-secret:",
			wantErr: true,
		},
		{
			name:    "label with shift rejected",
			input:   "my-secret:latest~1",
			wantErr: true,
		},

		// Error cases mirroring the shared grammar.
		{
			name:    "hash at end without value",
			input:   "my-secret#",
			wantErr: true,
		},
		{
			name:    "hash followed by non-digit",
			input:   "my-secret#abc",
			wantErr: true,
		},
		{
			name:    "tilde followed by letter (ambiguous)",
			input:   "my-secret~backup",
			wantErr: true,
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
			t.Parallel()

			spec, err := gcpversion.Parse(tt.input)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, spec.Name)
			assert.Equal(t, tt.wantVersion, spec.Absolute.Version)
			assert.Equal(t, tt.wantShift, spec.Shift)
		})
	}
}

func TestParse_LabelErrorMessage(t *testing.T) {
	t.Parallel()

	_, err := gcpversion.Parse("my-secret:latest")
	require.Error(t, err)
	require.ErrorIs(t, err, gcpversion.ErrLabelUnsupported)
}

func TestParseDiffArgs(t *testing.T) {
	t.Parallel()

	t.Run("single spec compares against latest", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := gcpversion.ParseDiffArgs([]string{"my-secret#3"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(3)), spec1.Absolute.Version)
		assert.Nil(t, spec2.Absolute.Version)
	})

	t.Run("two specs", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := gcpversion.ParseDiffArgs([]string{"my-secret#1", "my-secret#2"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, lo.ToPtr(int64(2)), spec2.Absolute.Version)
	})

	t.Run("label rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := gcpversion.ParseDiffArgs([]string{"my-secret:latest"})
		require.Error(t, err)
	})
}
