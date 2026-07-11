package passphrase_test

import (
	"bufio"
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/passphrase"
)

// TestSharedBufReader is the shared-reader regression for #471/#472: a
// confirmation prompt followed by a passphrase read over ONE piped stdin must
// read both lines correctly. A per-prompter bufio.Reader read-ahead buffers the
// whole pipe on the first read, stranding the second line and leaving the second
// reader at EOF.
func TestSharedBufReader(t *testing.T) {
	t.Parallel()

	t.Run("one shared reader reads confirm then passphrase", func(t *testing.T) {
		t.Parallel()

		r, w, err := os.Pipe()
		require.NoError(t, err)

		_, err = w.WriteString("y\nmypass\n")
		require.NoError(t, err)
		require.NoError(t, w.Close())

		shared := bufio.NewReader(r)

		cp := &confirm.Prompter{Stdin: r, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, BufReader: shared}
		ok, err := cp.Confirm("Continue?", false)
		require.NoError(t, err)
		assert.True(t, ok)

		pp := &passphrase.Prompter{Stdin: r, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
		pp.UseBufReader(shared)

		pass, err := pp.ReadFromStdin()
		require.NoError(t, err)
		assert.Equal(t, "mypass", pass)
	})

	// Control: two independent readers over the same fd drop the second line,
	// which is precisely the double-buffering bug the shared reader fixes.
	t.Run("independent readers drop the second line", func(t *testing.T) {
		t.Parallel()

		r, w, err := os.Pipe()
		require.NoError(t, err)

		_, err = w.WriteString("y\nmypass\n")
		require.NoError(t, err)
		require.NoError(t, w.Close())

		cp := &confirm.Prompter{Stdin: r, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
		ok, err := cp.Confirm("Continue?", false)
		require.NoError(t, err)
		assert.True(t, ok)

		pp := &passphrase.Prompter{Stdin: r, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}

		pass, err := pp.ReadFromStdin()
		require.NoError(t, err)
		assert.Empty(t, pass)
	})
}
