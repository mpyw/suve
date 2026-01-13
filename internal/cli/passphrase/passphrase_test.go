package passphrase_test

import (
	"bytes"
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
