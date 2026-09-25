package version_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version"
)

func TestKeyVaultParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantName string
		wantID   *string
		wantShif int
		wantErr  bool
	}{
		{
			name:     "simple name",
			input:    "my-secret",
			wantName: "my-secret",
		},
		{
			name:     "with opaque version id",
			input:    "my-secret#a1b2c3d4",
			wantName: "my-secret",
			wantID:   new("a1b2c3d4"),
		},
		{
			name:     "with single shift",
			input:    "my-secret~",
			wantName: "my-secret",
			wantShif: 1,
		},
		{
			name:     "with numeric shift",
			input:    "my-secret~2",
			wantName: "my-secret",
			wantShif: 2,
		},
		{
			name:     "with double tilde",
			input:    "my-secret~~",
			wantName: "my-secret",
			wantShif: 2,
		},
		{
			name:     "version id with shift",
			input:    "my-secret#abc123~2",
			wantName: "my-secret",
			wantID:   new("abc123"),
			wantShif: 2,
		},
		{
			name:     "whitespace trimmed",
			input:    "  my-secret  ",
			wantName: "my-secret",
		},

		// :LABEL is rejected (Key Vault has no staging labels).
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

		// Additional valid forms (opaque ids may contain letters, digits, dashes).
		{
			name:     "version id with dashes and uppercase",
			input:    "my-secret#ABC-123",
			wantName: "my-secret",
			wantID:   new("ABC-123"),
		},
		{
			name:     "name with @ allowed",
			input:    "user@example.com",
			wantName: "user@example.com",
		},
		{
			name:     "name with slashes and id",
			input:    "a/b/c#abc",
			wantName: "a/b/c",
			wantID:   new("abc"),
		},
		{
			name:     "id with cumulative shifts",
			input:    "my-secret#abc~1~2",
			wantName: "my-secret",
			wantID:   new("abc"),
			wantShif: 3,
		},
		{
			name:     "large numeric shift",
			input:    "my-secret~100",
			wantName: "my-secret",
			wantShif: 100,
		},
		{
			name:     "numeric shift zero is a no-op",
			input:    "my-secret~0",
			wantName: "my-secret",
			wantShif: 0,
		},
		{
			name:     "tilde followed by minus is part of the name",
			input:    "my-secret~-1",
			wantName: "my-secret~-1",
		},

		// Additional rejected forms.
		{
			name:    "id stops at underscore then shift parse fails",
			input:   "my-secret#a_b",
			wantErr: true,
		},
		{
			name:    "multiple absolute specifiers rejected",
			input:   "sec#id1#id2",
			wantErr: true,
		},
		{
			name:    "empty name (version at start)",
			input:   "#abc",
			wantErr: true,
		},
		{
			name:    "empty name (shift at start)",
			input:   "~1",
			wantErr: true,
		},
		{
			name:    "label after version rejected",
			input:   "my-secret#abc:latest",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec, err := version.AzureKeyVault.Parse(tt.input)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, spec.Name)
			assert.Equal(t, tt.wantID, spec.Absolute.ID)
			assert.Equal(t, tt.wantShif, spec.Shift)
		})
	}
}

func TestKeyVaultParse_LabelErrorMessage(t *testing.T) {
	t.Parallel()

	_, err := version.AzureKeyVault.Parse("my-secret:latest")
	require.Error(t, err)
	require.ErrorIs(t, err, version.ErrAzureKeyVaultLabelUnsupported)
}

// TestParse_LabelAfterVersionRejected exercises the ':' reject path reached
// AFTER a valid '#' specifier: parseAbsolute advances past "#abc" and then hits
// ':', invoking the label parser's Apply (which returns ErrLabelUnsupported).
func TestKeyVaultParse_LabelAfterVersionRejected(t *testing.T) {
	t.Parallel()

	_, err := version.AzureKeyVault.Parse("my-secret#abc:latest")
	require.Error(t, err)
	require.ErrorIs(t, err, version.ErrAzureKeyVaultLabelUnsupported)
}

// TestSuffix pins that Suffix rebuilds the part after the name, normalized, and
// that name+suffix re-parses to an equivalent spec.
func TestKeyVaultSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{input: "my-secret", want: ""},
		{input: "my-secret#abc123", want: "#abc123"},
		{input: "my-secret~~", want: "~2"},
		{input: "my-secret#abc123~1", want: "#abc123~1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			spec, err := version.AzureKeyVault.Parse(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, version.AzureKeyVault.Suffix(spec))

			reparsed, err := version.AzureKeyVault.Parse(spec.Name + version.AzureKeyVault.Suffix(spec))
			require.NoError(t, err)
			assert.Equal(t, spec, reparsed)
		})
	}
}
