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
			name:     "name with dots",
			input:    "app.timeout",
			wantName: "app.timeout",
		},
		{
			name:     "whitespace trimmed",
			input:    "  my-key  ",
			wantName: "my-key",
		},

		// Characters that look like version specifiers are legal key characters
		// in App Configuration and are preserved verbatim (the #353 regression).
		{
			name:     "colon-separated ASP.NET-style key",
			input:    "Logging:LogLevel:Default",
			wantName: "Logging:LogLevel:Default",
		},
		{
			name:     "hash in key preserved",
			input:    "my-key#3",
			wantName: "my-key#3",
		},
		{
			name:     "hash id in key preserved",
			input:    "my-key#abc123",
			wantName: "my-key#abc123",
		},
		{
			name:     "trailing tilde preserved",
			input:    "my-key~",
			wantName: "my-key~",
		},
		{
			name:     "tilde with number preserved",
			input:    "my-key~2",
			wantName: "my-key~2",
		},
		{
			name:     "double tilde preserved",
			input:    "my-key~~",
			wantName: "my-key~~",
		},
		{
			name:     "single label-like colon preserved",
			input:    "my-key:prod",
			wantName: "my-key:prod",
		},
		{
			name:     "trailing hash preserved",
			input:    "my-key#",
			wantName: "my-key#",
		},
		{
			name:     "trailing colon preserved",
			input:    "my-key:",
			wantName: "my-key:",
		},
		{
			name:     "name with @ allowed",
			input:    "user@example.com",
			wantName: "user@example.com",
		},
		{
			name:     "tilde-zero preserved verbatim",
			input:    "my-key~0",
			wantName: "my-key~0",
		},
		{
			name:     "cumulative-shift-like key preserved",
			input:    "my-key~1~2",
			wantName: "my-key~1~2",
		},
		{
			name:     "hash-and-tilde key preserved",
			input:    "my-key#abc~1",
			wantName: "my-key#abc~1",
		},
		{
			name:     "multiple colons preserved",
			input:    "a:b:c",
			wantName: "a:b:c",
		},

		// Empty-input errors are surfaced.
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

// TestParse_SpecifierLikeKeysAccepted verifies that keys containing what would
// be version specifiers in versioned stores are accepted verbatim (#353).
func TestParse_SpecifierLikeKeysAccepted(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"my-key#3", "my-key~1", "my-key:prod", "Logging:LogLevel:Default"} {
		spec, err := azureappconfigversion.Parse(input)
		require.NoError(t, err, "input=%q", input)
		assert.Equal(t, input, spec.Name)
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

	t.Run("single key containing hash compares against itself", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := azureappconfigversion.ParseDiffArgs([]string{"my-key#1"})
		require.NoError(t, err)
		assert.Equal(t, "my-key#1", spec1.Name)
		assert.Equal(t, "my-key#1", spec2.Name)
	})

	t.Run("two keys where the second contains a hash", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := azureappconfigversion.ParseDiffArgs([]string{"my-key", "#1"})
		require.NoError(t, err)
		assert.Equal(t, "my-key", spec1.Name)
		assert.Equal(t, "#1", spec2.Name)
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
