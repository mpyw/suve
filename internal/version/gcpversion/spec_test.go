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

		// Additional valid forms (integer versions, allowed name chars, shifts).
		{
			name:        "version zero accepted (positivity not enforced at parse time)",
			input:       "my-secret#0",
			wantName:    "my-secret",
			wantVersion: lo.ToPtr(int64(0)),
		},
		{
			name:     "name with @ allowed",
			input:    "user@example.com",
			wantName: "user@example.com",
		},
		{
			name:      "name with @ and shift",
			input:     "user@example.com~1",
			wantName:  "user@example.com",
			wantShift: 1,
		},
		{
			name:        "name with slashes and version",
			input:       "a/b/c#3",
			wantName:    "a/b/c",
			wantVersion: lo.ToPtr(int64(3)),
		},
		{
			name:     "name with dots and mixed case",
			input:    "MY-secret.v2",
			wantName: "MY-secret.v2",
		},
		{
			name:        "version with cumulative shifts",
			input:       "my-secret#5~1~2",
			wantName:    "my-secret",
			wantVersion: lo.ToPtr(int64(5)),
			wantShift:   3,
		},
		{
			name:      "large cumulative shift",
			input:     "my-secret~10~20",
			wantName:  "my-secret",
			wantShift: 30,
		},
		{
			name:      "numeric shift zero is a no-op",
			input:     "my-secret~0",
			wantName:  "my-secret",
			wantShift: 0,
		},
		{
			name:     "tilde followed by minus is part of the name",
			input:    "my-secret~-1",
			wantName: "my-secret~-1",
		},

		// Additional rejected forms.
		{
			name:    "multiple absolute specifiers rejected",
			input:   "my#3#4",
			wantErr: true,
		},
		{
			name:    "empty name (version at start)",
			input:   "#3",
			wantErr: true,
		},
		{
			name:    "empty name (shift at start)",
			input:   "~1",
			wantErr: true,
		},
		// NOTE: version-overflow ("#99999999999999999999999999") and
		// label-after-version ("#3:latest") are exercised by dedicated tests
		// (TestParse_VersionOverflow, TestParse_LabelAfterVersionRejected) that
		// assert the specific error, so they are omitted here.
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

// TestParse_LabelAfterVersionRejected exercises the ':' reject path reached
// AFTER a valid '#' specifier: parseAbsolute advances past "#3" and then hits
// ':', invoking the label parser's Apply (which returns ErrLabelUnsupported).
func TestParse_LabelAfterVersionRejected(t *testing.T) {
	t.Parallel()

	_, err := gcpversion.Parse("my-secret#3:latest")
	require.Error(t, err)
	require.ErrorIs(t, err, gcpversion.ErrLabelUnsupported)
}

// TestParse_VersionOverflow exercises the strconv.ParseInt failure branch when
// the integer version cannot fit in int64.
func TestParse_VersionOverflow(t *testing.T) {
	t.Parallel()

	_, err := gcpversion.Parse("my-secret#99999999999999999999999999")
	require.Error(t, err)
	require.ErrorContains(t, err, "out of range")
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

	t.Run("mixed format: full spec plus specifier-only", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := gcpversion.ParseDiffArgs([]string{"my-secret#1", "#2"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, lo.ToPtr(int64(2)), spec2.Absolute.Version)
	})

	t.Run("partial spec: name plus specifier-only is swapped", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := gcpversion.ParseDiffArgs([]string{"my-secret", "#3"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(3)), spec1.Absolute.Version)
		assert.Nil(t, spec2.Absolute.Version)
	})

	t.Run("three args: name plus two specifiers", func(t *testing.T) {
		t.Parallel()

		spec1, spec2, err := gcpversion.ParseDiffArgs([]string{"my-secret", "#1", "#2"})
		require.NoError(t, err)
		assert.Equal(t, lo.ToPtr(int64(1)), spec1.Absolute.Version)
		assert.Equal(t, lo.ToPtr(int64(2)), spec2.Absolute.Version)
	})

	t.Run("no args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := gcpversion.ParseDiffArgs([]string{})
		require.Error(t, err)
	})

	t.Run("too many args rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := gcpversion.ParseDiffArgs([]string{"a", "b", "c", "d"})
		require.Error(t, err)
	})

	t.Run("label rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := gcpversion.ParseDiffArgs([]string{"my-secret:latest"})
		require.Error(t, err)
	})
}
