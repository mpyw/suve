package cli

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// TestStashMutuallyExclusiveFlags guards #322: --merge and --overwrite must be
// mutually exclusive, and each must still be readable (folded into the command)
// when declared only inside MutuallyExclusiveFlags.
func TestStashMutuallyExclusiveFlags(t *testing.T) {
	t.Parallel()

	const cmdName = "s" // arbitrary; avoids repeating a command-name literal

	cases := []struct {
		name  string
		flags func() []cli.Flag
		mux   func() []cli.MutuallyExclusiveFlags
	}{
		{"push-flags", stashPushFlags, stashPushMutuallyExclusiveFlags},
		{"pop-flags", stashPopFlags, stashPopMutuallyExclusiveFlags},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newCmd := func(gotMerge *bool) *cli.Command {
				return &cli.Command{
					Name:                   cmdName,
					Flags:                  tc.flags(),
					MutuallyExclusiveFlags: tc.mux(),
					Writer:                 io.Discard,
					ErrWriter:              io.Discard,
					Action: func(_ context.Context, cmd *cli.Command) error {
						if gotMerge != nil {
							*gotMerge = cmd.Bool(flagMerge)
						}

						return nil
					},
				}
			}

			// --merge --overwrite together must be rejected.
			err := newCmd(nil).Run(context.Background(), []string{cmdName, "--merge", "--overwrite"})
			require.Error(t, err)

			// Each alone is accepted, and its value is readable via cmd.Bool
			// (proving the group flags fold into the command's flag set).
			var gotMerge bool

			require.NoError(t, newCmd(&gotMerge).Run(context.Background(), []string{cmdName, "--merge"}))
			assert.True(t, gotMerge, "--merge must be readable via cmd.Bool")

			require.NoError(t, newCmd(nil).Run(context.Background(), []string{cmdName, "--overwrite"}))
		})
	}
}
