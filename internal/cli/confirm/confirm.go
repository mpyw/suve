// Package confirm provides confirmation prompts for destructive operations.
package confirm

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/mpyw/suve/internal/cli/colors"
)

// Prompter handles confirmation prompts.
type Prompter struct {
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	AccountID string
	Region    string
	Profile   string
}

// printTargetInfo prints AWS profile, account, and region information if available.
func (p *Prompter) printTargetInfo() {
	if p.AccountID == "" || p.Region == "" {
		return
	}
	if p.Profile != "" {
		_, _ = fmt.Fprintf(p.Stderr, "%s Target: %s (%s / %s)\n", colors.Info("i"), p.Profile, p.AccountID, p.Region)
	} else {
		_, _ = fmt.Fprintf(p.Stderr, "%s Target: %s / %s\n", colors.Info("i"), p.AccountID, p.Region)
	}
}

// Confirm displays a confirmation prompt and returns true if the user confirms.
// If skipConfirm is true, returns true without prompting.
func (p *Prompter) Confirm(message string, skipConfirm bool) (bool, error) {
	if skipConfirm {
		return true, nil
	}

	p.printTargetInfo()
	_, _ = fmt.Fprintf(p.Stderr, "%s %s [y/N]: ", colors.Warning("?"), message)

	reader := bufio.NewReader(p.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}

// ConfirmAction is a convenience function for confirming an action.
func (p *Prompter) ConfirmAction(action, target string, skipConfirm bool) (bool, error) {
	message := fmt.Sprintf("%s %s?", action, target)
	return p.Confirm(message, skipConfirm)
}

// ConfirmDelete confirms a delete operation with a warning.
func (p *Prompter) ConfirmDelete(target string, skipConfirm bool) (bool, error) {
	if skipConfirm {
		return true, nil
	}

	p.printTargetInfo()
	_, _ = fmt.Fprintf(p.Stderr, "%s This will permanently delete: %s\n", colors.Error("!"), target)
	_, _ = fmt.Fprintf(p.Stderr, "%s Continue? [y/N]: ", colors.Warning("?"))

	reader := bufio.NewReader(p.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}
