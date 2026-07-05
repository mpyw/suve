package azureappconfigversion_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version/azureappconfigversion"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantName string
		wantErr  bool
	}{
		{
			name:     "simple name",
			input:    "my-key",
			wantName: "my-key",
		},
		{
			name:     "path-like name",
			input:    "app/config/timeout",
			wantName: "app/config/timeout",
		},
		{
			name:     "name with dots and colons in the middle are fine only if not a specifier",
			input:    "app.timeout",
			wantName: "app.timeout",
		},
		{
			name:     "whitespace trimmed",
			input:    "  my-key  ",
			wantName: "my-key",
		},

		// ANY version specifier is rejected (App Configuration is unversioned).
		{
			name:    "version number rejected",
			input:   "my-key#3",
			wantErr: true,
		},
		{
			name:    "version id rejected",
			input:   "my-key#abc123",
			wantErr: true,
		},
		{
			name:    "single shift rejected",
			input:   "my-key~",
			wantErr: true,
		},
		{
			name:    "numeric shift rejected",
			input:   "my-key~2",
			wantErr: true,
		},
		{
			name:    "double tilde rejected",
			input:   "my-key~~",
			wantErr: true,
		},
		{
			name:    "label rejected",
			input:   "my-key:prod",
			wantErr: true,
		},
		{
			name:    "hash at end rejected",
			input:   "my-key#",
			wantErr: true,
		},
		{
			name:    "colon at end rejected",
			input:   "my-key:",
			wantErr: true,
		},

		// Additional accepted bare names (no version specifier present).
		{
			name:     "name with @ allowed",
			input:    "user@example.com",
			wantName: "user@example.com",
		},
		{
			name:     "shift zero collapses to a bare name",
			input:    "my-key~0",
			wantName: "my-key",
		},

		// Additional rejected version specifiers.
		{
			name:    "cumulative shift rejected",
			input:   "my-key~1~2",
			wantErr: true,
		},
		{
			name:    "version id with shift rejected",
			input:   "my-key#abc~1",
			wantErr: true,
		},
		{
			name:    "multiple colons rejected",
			input:   "a:b:c",
			wantErr: true,
		},

		// Empty-input errors are surfaced (not normalized to unsupported).
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

			spec, err := azureappconfigversion.Parse(tt.input)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, spec.Name)
		})
	}
}

func TestParse_VersionSpecErrorIsUnsupported(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"my-key#3", "my-key~1", "my-key:prod", "my-key#abc"} {
		_, err := azureappconfigversion.Parse(input)
		require.Error(t, err)
		require.ErrorIs(t, err, azureappconfigversion.ErrVersioningUnsupported, "input=%q", input)
	}
}

// TestParse_EmptyErrorsNotNormalized verifies that empty-input and empty-name
// errors from the shared grammar are surfaced unchanged rather than being
// collapsed into ErrVersioningUnsupported.
func TestParse_EmptyErrorsNotNormalized(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "   "} {
		_, err := azureappconfigversion.Parse(input)
		require.Error(t, err)
		require.NotErrorIs(t, err, azureappconfigversion.ErrVersioningUnsupported, "input=%q", input)
	}
}

func TestParseDiffArgs(t *testing.T) {
	t.Parallel()

	t.Run("single bare key compares against itself", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := azureappconfigversion.ParseDiffArgs([]string{"key-a"})
		require.NoError(t, err)
		assert.Equal(t, "key-a", spec1.Name)
		assert.Equal(t, "key-a", spec2.Name)
	})

	t.Run("two bare keys compared", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := azureappconfigversion.ParseDiffArgs([]string{"key-a", "key-b"})
		require.NoError(t, err)
		assert.Equal(t, "key-a", spec1.Name)
		assert.Equal(t, "key-b", spec2.Name)
	})

	t.Run("version spec rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := azureappconfigversion.ParseDiffArgs([]string{"my-key#1"})
		require.Error(t, err)
	})

	t.Run("name plus specifier-only arg rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := azureappconfigversion.ParseDiffArgs([]string{"my-key", "#1"})
		require.Error(t, err)
	})

	t.Run("three args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := azureappconfigversion.ParseDiffArgs([]string{"my-key", "#1", "#2"})
		require.Error(t, err)
	})

	t.Run("no args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := azureappconfigversion.ParseDiffArgs([]string{})
		require.Error(t, err)
	})

	t.Run("too many args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := azureappconfigversion.ParseDiffArgs([]string{"a", "b", "c", "d"})
		require.Error(t, err)
	})
}
