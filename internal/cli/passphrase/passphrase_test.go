package passphrase_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/passphrase"
)

func TestPrompter_PromptForEncrypt_WithPassphrase(t *testing.T) {
	t.Parallel()

	// Simulate user entering "secret\nsecret\n"
	stdin := strings.NewReader("secret\nsecret\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	p := &passphrase.Prompter{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	pass, err := p.PromptForEncrypt()
	require.NoError(t, err)
	assert.Equal(t, "secret", pass)
}

func TestPrompter_PromptForEncrypt_Mismatch(t *testing.T) {
	t.Parallel()

	// Simulate user entering different passwords
	stdin := strings.NewReader("secret1\nsecret2\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	p := &passphrase.Prompter{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	_, err := p.PromptForEncrypt()
	assert.ErrorIs(t, err, passphrase.ErrPassphraseMismatch)
}

func TestPrompter_PromptForEncrypt_EmptyConfirmed(t *testing.T) {
	t.Parallel()

	// Simulate user entering empty password and confirming
	stdin := strings.NewReader("\ny\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	p := &passphrase.Prompter{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	pass, err := p.PromptForEncrypt()
	require.NoError(t, err)
	assert.Empty(t, pass)
	assert.Contains(t, stderr.String(), "plain text")
}

func TestPrompter_PromptForEncrypt_EmptyCancelled(t *testing.T) {
	t.Parallel()

	// Simulate user entering empty password and declining
	stdin := strings.NewReader("\nn\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	p := &passphrase.Prompter{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	_, err := p.PromptForEncrypt()
	assert.ErrorIs(t, err, passphrase.ErrCancelled)
}

func TestPrompter_PromptForDecrypt(t *testing.T) {
	t.Parallel()

	stdin := strings.NewReader("mypassword\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	p := &passphrase.Prompter{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	pass, err := p.PromptForDecrypt()
	require.NoError(t, err)
	assert.Equal(t, "mypassword", pass)
}

func TestPrompter_PromptForDecrypt_CRLF(t *testing.T) {
	t.Parallel()

	// strings.NewReader is not a terminal.Fder, so readPassword takes the
	// non-TTY fallback. CRLF input must be normalized the same as ReadFromStdin.
	stdin := strings.NewReader("pass\r\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	p := &passphrase.Prompter{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	pass, err := p.PromptForDecrypt()
	require.NoError(t, err)
	assert.Equal(t, "pass", pass)
}

func TestPrompter_WarnNonTTY(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	p := &passphrase.Prompter{
		Stdin:  strings.NewReader(""),
		Stdout: stdout,
		Stderr: stderr,
	}

	p.WarnNonTTY()
	assert.Contains(t, stderr.String(), "Non-interactive mode")
	assert.Contains(t, stderr.String(), "plain text")
}

// errReader fails on the first Read, standing in for a stdin whose read errors
// (something other than EOF), so the non-TTY read paths surface the failure.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// dataThenErrReader yields data once, then fails on the next Read. It lets a
// prompt's first read succeed and its second read (the confirmation) error.
type dataThenErrReader struct {
	data []byte
	err  error
	done bool
}

func (r *dataThenErrReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}

	r.done = true

	return copy(p, r.data), nil
}

func TestPrompter_PromptForEncrypt_EmptyConfirmReadError(t *testing.T) {
	t.Parallel()

	// Empty passphrase reaches confirmPlainText; the reader is then at EOF, so
	// the confirmation ReadString errors and the prompt treats it as a decline.
	p := &passphrase.Prompter{
		Stdin:  strings.NewReader("\n"),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}

	_, err := p.PromptForEncrypt()
	assert.ErrorIs(t, err, passphrase.ErrCancelled)
}

func TestPrompter_PromptForEncrypt_ReadError(t *testing.T) {
	t.Parallel()

	p := &passphrase.Prompter{
		Stdin:  errReader{err: errors.New("boom")},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}

	_, err := p.PromptForEncrypt()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read passphrase")
}

func TestPrompter_PromptForEncrypt_ConfirmReadError(t *testing.T) {
	t.Parallel()

	// The first read yields a non-empty passphrase; the confirmation read then
	// errors, so the prompt reports a confirmation-read failure.
	p := &passphrase.Prompter{
		Stdin:  &dataThenErrReader{data: []byte("secret\n"), err: errors.New("boom")},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}

	_, err := p.PromptForEncrypt()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read confirmation")
}

func TestPrompter_PromptForDecrypt_ReadError(t *testing.T) {
	t.Parallel()

	p := &passphrase.Prompter{
		Stdin:  errReader{err: errors.New("boom")},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}

	_, err := p.PromptForDecrypt()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read passphrase")
}

func TestPrompter_ReadFromStdin_ReadError(t *testing.T) {
	t.Parallel()

	// A non-EOF read error must propagate rather than be swallowed as empty.
	sentinel := errors.New("boom")

	p := &passphrase.Prompter{
		Stdin:  errReader{err: sentinel},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}

	_, err := p.ReadFromStdin()
	assert.ErrorIs(t, err, sentinel)
}

func TestPrompter_ReadFromStdin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "mypassword\n", "mypassword"},
		{"with_crlf", "mypassword\r\n", "mypassword"},
		{"empty", "\n", ""},
		{"no_newline", "mypassword", "mypassword"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &passphrase.Prompter{
				Stdin:  strings.NewReader(tt.input),
				Stdout: &bytes.Buffer{},
				Stderr: &bytes.Buffer{},
			}

			pass, err := p.ReadFromStdin()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, pass)
		})
	}
}
