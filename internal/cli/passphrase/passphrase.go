// Package passphrase provides passphrase input handling for encryption.
package passphrase

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/term"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/terminal"
)

var (
	// ErrPassphraseMismatch is returned when confirmation doesn't match.
	ErrPassphraseMismatch = errors.New("passphrases do not match")
	// ErrCancelled is returned when user cancels the operation.
	ErrCancelled = errors.New("operation cancelled")
)

// Prompter handles passphrase input prompts.
type Prompter struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// bufReader is a buffered reader for non-TTY input
	bufReader *bufio.Reader
}

// PromptForEncrypt prompts for passphrase with confirmation for encryption.
// Returns empty string if user chooses to continue without encryption after warning.
// Returns ErrCancelled if user declines to continue without encryption.
func (p *Prompter) PromptForEncrypt() (string, error) {
	_, _ = fmt.Fprintf(p.Stderr, "Enter passphrase (empty for plain text): ")

	pass, err := p.readPassword()
	if err != nil {
		return "", fmt.Errorf("failed to read passphrase: %w", err)
	}
	_, _ = fmt.Fprintln(p.Stderr) // newline after password input

	// If empty, warn and confirm
	if pass == "" {
		if !p.confirmPlainText() {
			return "", ErrCancelled
		}
		return "", nil
	}

	// Confirm passphrase
	_, _ = fmt.Fprintf(p.Stderr, "Confirm passphrase: ")
	confirm, err := p.readPassword()
	if err != nil {
		return "", fmt.Errorf("failed to read confirmation: %w", err)
	}
	_, _ = fmt.Fprintln(p.Stderr) // newline after password input

	if pass != confirm {
		return "", ErrPassphraseMismatch
	}

	return pass, nil
}

// PromptForDecrypt prompts for passphrase for decryption (no confirmation).
func (p *Prompter) PromptForDecrypt() (string, error) {
	_, _ = fmt.Fprintf(p.Stderr, "Enter passphrase: ")

	pass, err := p.readPassword()
	if err != nil {
		return "", fmt.Errorf("failed to read passphrase: %w", err)
	}
	_, _ = fmt.Fprintln(p.Stderr) // newline after password input

	return pass, nil
}

// WarnNonTTY prints warning for non-TTY environment.
func (p *Prompter) WarnNonTTY() {
	output.Warn(p.Stderr, "Non-interactive mode. Storing secrets as plain text.")
}

// ReadFromStdin reads a single line from stdin without prompting.
// Used with --passphrase-stdin flag for non-interactive input.
func (p *Prompter) ReadFromStdin() (string, error) {
	if p.bufReader == nil {
		p.bufReader = bufio.NewReader(p.Stdin)
	}
	line, err := p.bufReader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

// confirmPlainText asks user to confirm storing as plain text.
func (p *Prompter) confirmPlainText() bool {
	output.Warn(p.Stderr, "Storing secrets as plain text on disk.")
	_, _ = fmt.Fprintf(p.Stderr, "%s Continue without encryption? [y/N]: ", colors.Warning("?"))

	// Use buffered reader to preserve stream position
	if p.bufReader == nil {
		p.bufReader = bufio.NewReader(p.Stdin)
	}
	response, err := p.bufReader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// readPassword reads password from terminal without echoing.
// Falls back to regular read if not a terminal.
func (p *Prompter) readPassword() (string, error) {
	// Try to get file descriptor for secure password reading
	if f, ok := p.Stdin.(terminal.Fder); ok && terminal.IsTTY(f.Fd()) {
		pass, err := term.ReadPassword(int(f.Fd()))
		if err != nil {
			return "", err
		}
		return string(pass), nil
	}

	// Fallback: read line (for testing or non-TTY)
	// Use buffered reader to preserve stream position across multiple calls
	if p.bufReader == nil {
		p.bufReader = bufio.NewReader(p.Stdin)
	}
	line, err := p.bufReader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSuffix(line, "\n"), nil
}
