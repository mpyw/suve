package param

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/domain"
)

// runResolve builds a throwaway command carrying the value-type flags, runs it
// with the given args so the flags are populated, and returns what
// resolveValueType produced.
func runResolve(t *testing.T, args []string) (domain.ValueType, error) {
	t.Helper()

	var (
		got    domain.ValueType
		gotErr error
	)

	cmd := &cli.Command{
		Name:  "add",
		Flags: valueTypeFlags(),
		Action: func(_ context.Context, c *cli.Command) error {
			got, gotErr = resolveValueType(c)

			return nil
		},
	}

	require.NoError(t, cmd.Run(context.Background(), append([]string{"add"}, args...)))

	return got, gotErr
}

func TestResolveValueType(t *testing.T) {
	t.Parallel()

	t.Run("no flags is unset", func(t *testing.T) {
		t.Parallel()

		got, err := runResolve(t, nil)
		require.NoError(t, err)
		assert.Empty(t, string(got))
	})

	t.Run("--secure is SecureString", func(t *testing.T) {
		t.Parallel()

		got, err := runResolve(t, []string{"--secure"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValueTypeSecret, got)
	})

	t.Run("--type SecureString is secret", func(t *testing.T) {
		t.Parallel()

		got, err := runResolve(t, []string{"--type", "SecureString"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValueTypeSecret, got)
	})

	t.Run("--type StringList is list", func(t *testing.T) {
		t.Parallel()

		got, err := runResolve(t, []string{"--type", "StringList"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValueTypeList, got)
	})

	t.Run("--type String is plaintext", func(t *testing.T) {
		t.Parallel()

		got, err := runResolve(t, []string{"--type", "String"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValueTypePlaintext, got)
	})

	t.Run("--type with an invalid value errors", func(t *testing.T) {
		t.Parallel()

		_, err := runResolve(t, []string{"--type", "Sekret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --type")
	})

	t.Run("--secure with --type conflicts", func(t *testing.T) {
		t.Parallel()

		_, err := runResolve(t, []string{"--secure", "--type", "String"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use --secure with --type")
	})
}
