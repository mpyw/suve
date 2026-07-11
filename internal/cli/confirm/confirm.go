// Package confirm provides confirmation prompts for destructive operations.
package confirm

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
)

// Prompter handles confirmation prompts.
type Prompter struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// BufReader, when set, is a shared buffered stdin reader used instead of
	// allocating a per-call one. Sharing one reader across prompters (e.g. a
	// passphrase prompt then a confirm prompt) prevents double-buffering: a
	// fresh bufio.Reader over piped stdin would find the bytes already
	// read-ahead into the first reader's buffer and hit EOF.
	BufReader *bufio.Reader

	// Target is a provider-neutral, human-readable description of where the
	// operation applies (e.g. "my-profile (123456789012 / us-east-1)" for AWS or
	// "project my-gcloud-project" for Google Cloud). When set, it is shown before
	// the prompt. It takes precedence over the AWS-specific fields below.
	Target string

	// AccountID/Region/Profile describe the AWS target. They are used only when
	// Target is empty, preserving the original AWS confirmation output.
	AccountID string
	Region    string
	Profile   string
}

// printTargetInfo prints the target information before a prompt, if available.
func (p *Prompter) printTargetInfo() {
	if p.Target != "" {
		output.Printf(p.Stderr, "%s Target: %s\n", colors.For(p.Stderr).Info("i"), p.Target)

		return
	}

	if p.AccountID == "" || p.Region == "" {
		return
	}

	if p.Profile != "" {
		output.Printf(p.Stderr, "%s Target: %s (%s / %s)\n", colors.For(p.Stderr).Info("i"), p.Profile, p.AccountID, p.Region)
	} else {
		output.Printf(p.Stderr, "%s Target: %s / %s\n", colors.For(p.Stderr).Info("i"), p.AccountID, p.Region)
	}
}

// reader returns the shared buffered reader when one was injected, otherwise a
// per-call reader over Stdin.
func (p *Prompter) reader() *bufio.Reader {
	if p.BufReader != nil {
		return p.BufReader
	}

	return bufio.NewReader(p.Stdin)
}

// readYesNo reads a yes/no response from stdin.
func (p *Prompter) readYesNo() (bool, error) {
	response, err := p.reader().ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))

	return response == "y" || response == "yes", nil
}

// Confirm displays a confirmation prompt and returns true if the user confirms.
// If skipConfirm is true, returns true without prompting.
func (p *Prompter) Confirm(message string, skipConfirm bool) (bool, error) {
	if skipConfirm {
		return true, nil
	}

	p.printTargetInfo()
	output.Printf(p.Stderr, "%s %s [y/N]: ", colors.For(p.Stderr).Warning("?"), message)

	return p.readYesNo()
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
	output.Printf(p.Stderr, "%s This will permanently delete: %s\n", colors.For(p.Stderr).Error("!"), target)
	output.Printf(p.Stderr, "%s Continue? [y/N]: ", colors.For(p.Stderr).Warning("?"))

	return p.readYesNo()
}

// Choice represents an option in a multiple choice prompt.
type Choice struct {
	Label       string
	Description string
}

// ChoiceResult represents the result of a choice prompt.
type ChoiceResult int

const (
	// ChoiceCancelled indicates the user cancelled the prompt.
	ChoiceCancelled ChoiceResult = -1
)

// ConfirmChoice displays a multiple choice prompt and returns the selected index.
// Returns ChoiceCancelled (-1) if the user cancels or selects cancel option.
// The first choice (index 0) is the default when user just presses Enter.
func (p *Prompter) ConfirmChoice(message string, choices []Choice) (ChoiceResult, error) {
	p.printTargetInfo()
	output.Printf(p.Stderr, "%s %s\n", colors.For(p.Stderr).Warning("?"), message)

	for i, choice := range choices {
		if choice.Description != "" {
			output.Printf(p.Stderr, "  %d. %s (%s)\n", i+1, choice.Label, choice.Description)
		} else {
			output.Printf(p.Stderr, "  %d. %s\n", i+1, choice.Label)
		}
	}

	output.Printf(p.Stderr, "Enter choice [1]: ")

	response, err := p.reader().ReadString('\n')
	if err != nil {
		return ChoiceCancelled, fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(response)

	// Default to first choice if empty
	if response == "" {
		return 0, nil
	}

	// Parse as number
	var choice int
	if _, err := fmt.Sscanf(response, "%d", &choice); err != nil {
		return ChoiceCancelled, nil //nolint:nilerr // Invalid input is intentionally treated as cancel, not error
	}

	// Validate range
	if choice < 1 || choice > len(choices) {
		return ChoiceCancelled, nil // Out of range treated as cancel
	}

	return ChoiceResult(choice - 1), nil
}
