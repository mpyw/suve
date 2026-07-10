package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
	usestaging "github.com/mpyw/suve/internal/usecase/staging"
)

// envelopeReadSource adapts file.ReadEnvelopeFile/DecodeState to the import use
// case's EnvelopeReader port. A missing per-service file yields an empty state
// with a nil error so a global dir holding only one file still imports cleanly.
type envelopeReadSource struct {
	passphrase string
	// pathFor resolves the per-service source path (the single file for a
	// service-specific import, or <dir>/<service>.json for a global import).
	pathFor func(staging.Service) string
}

// ReadState reads and decodes svc's envelope. A missing file is skipped (empty
// state); a present file is decoded (and decrypted when encrypted).
func (s *envelopeReadSource) ReadState(_ context.Context, svc staging.Service) (*staging.State, error) {
	path := s.pathFor(svc)

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return staging.NewEmptyState(), nil
	}

	env, err := file.ReadEnvelopeFile(path)
	if err != nil {
		return nil, err
	}

	return env.DecodeState(s.passphrase)
}

// importFlags returns the flags shared by the global and service-specific import
// commands. --merge / --overwrite are NOT here: they live in the mutually
// exclusive group (see importMutuallyExclusiveFlags).
func importFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  flagYes,
			Usage: usageSkipConfirm,
		},
		&cli.BoolFlag{
			Name:  flagPassphraseStdin,
			Usage: usagePassphraseStdin,
		},
		&cli.BoolFlag{
			Name:  flagForce,
			Usage: "Import even if the file's scope differs from the current scope",
		},
	}
}

// importMutuallyExclusiveFlags returns the mutually exclusive --merge /
// --overwrite constraint. The flags are declared ONLY here (never also in
// importFlags): urfave/cli v3 binds parsed values to the first flag instance of
// a name, so a duplicate copy in Flags would shadow these, leaving the group's
// instances never IsSet() and the exclusivity check dead. Declaring them solely
// in the group lets v3 fold them into the command and enforce the constraint.
func importMutuallyExclusiveFlags() []cli.MutuallyExclusiveFlags {
	return []cli.MutuallyExclusiveFlags{
		{
			Flags: [][]cli.Flag{
				{&cli.BoolFlag{Name: flagMerge, Usage: "Merge with the existing working staging area (default)"}},
				{&cli.BoolFlag{Name: flagOverwrite, Usage: "Overwrite the working staging area"}},
			},
		},
	}
}

// ImportModeInput holds the inputs to import-mode selection.
type ImportModeInput struct {
	MergeFlag     bool
	OverwriteFlag bool
	// SkipPrompt (--yes) accepts the default (Merge) without the interactive
	// Merge/Overwrite/Cancel prompt, for scripts/automation.
	SkipPrompt bool
	HasChanges bool
	ItemCount  int
	IsTTY      bool
}

// ImportModeResult holds the outcome of import-mode selection.
type ImportModeResult struct {
	Mode      usestaging.ImportMode
	Cancelled bool
}

// ImportModeChooser resolves the reconciliation mode for the working staging
// area, mirroring the former stash-pop chooser: an explicit flag wins; otherwise
// a Merge/Overwrite/Cancel prompt appears only when the working area already
// holds changes and a TTY is available; the default is Merge.
type ImportModeChooser struct {
	Prompter *confirm.Prompter
	Stderr   io.Writer
	Stdout   io.Writer
}

// ChooseMode determines the import mode, prompting interactively if needed.
func (c *ImportModeChooser) ChooseMode(input ImportModeInput) (ImportModeResult, error) {
	if input.OverwriteFlag {
		return ImportModeResult{Mode: usestaging.ImportModeOverwrite}, nil
	}

	if input.MergeFlag {
		return ImportModeResult{Mode: usestaging.ImportModeMerge}, nil
	}

	if input.HasChanges && input.IsTTY && !input.SkipPrompt {
		output.Warning(c.Stderr, "Working staging area already has %d staged change(s).", input.ItemCount)

		choice, err := c.Prompter.ConfirmChoice("How do you want to proceed?", []confirm.Choice{
			{Label: "Merge", Description: "combine imported changes with existing"},
			{Label: "Overwrite", Description: "replace existing with imported changes"},
			{Label: "Cancel", Description: "abort operation"},
		})
		if err != nil {
			return ImportModeResult{}, fmt.Errorf("failed to get confirmation: %w", err)
		}

		switch choice {
		case 0: // Merge
			return ImportModeResult{Mode: usestaging.ImportModeMerge}, nil
		case 1: // Overwrite
			return ImportModeResult{Mode: usestaging.ImportModeOverwrite}, nil
		default: // Cancel or error
			return ImportModeResult{Cancelled: true}, nil
		}
	}

	return ImportModeResult{Mode: usestaging.ImportModeMerge}, nil
}

// presentEnvelope pairs a validated source envelope with the service it holds.
type presentEnvelope struct {
	path string
	env  *file.Envelope
}

// collectImportEnvelopes reads and validates the source envelope headers WITHOUT
// decoding their payloads, so scope/service can be checked before a passphrase is
// prompted. For a service-specific import the single file must exist and its
// header service must match. For a global import it reads whichever of
// param.json / secret.json exist (missing ones are skipped); neither present is
// an error. Scope mismatches are refused unless --force.
func collectImportEnvelopes(
	cmd *cli.Command, service staging.Service, pathFor func(staging.Service) string, wantScope string,
) ([]presentEnvelope, error) {
	var services []staging.Service
	if service != "" {
		services = []staging.Service{service}
	} else {
		services = []staging.Service{staging.ServiceParam, staging.ServiceSecret}
	}

	var present []presentEnvelope

	for _, svc := range services {
		path := pathFor(svc)

		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if service != "" {
				return nil, fmt.Errorf("import file not found: %s", path)
			}

			continue
		}

		env, err := file.ReadEnvelopeFile(path)
		if err != nil {
			return nil, err
		}

		// The file's header service must match what is expected here: the command's
		// service for a service-specific import, or the per-service filename for a
		// global import. A mismatch is a hard error (no --force override): importing
		// another service's data under the wrong service would corrupt the working
		// area, and export always writes matching names/headers.
		if env.Service != string(svc) {
			return nil, fmt.Errorf(
				"export file %s holds %q data but %q was expected; use the matching command/file",
				path, env.Service, svc,
			)
		}

		if env.Scope != wantScope && !cmd.Bool(flagForce) {
			return nil, fmt.Errorf(
				"export file scope %q does not match the current scope %q; re-run with --force to import anyway",
				env.Scope, wantScope,
			)
		}

		present = append(present, presentEnvelope{path: path, env: env})
	}

	if len(present) == 0 {
		return nil, usestaging.ErrNothingToImport
	}

	return present, nil
}

// anyEncrypted reports whether any present envelope carries an encrypted payload.
func anyEncrypted(present []presentEnvelope) (bool, error) {
	for _, p := range present {
		enc, err := p.env.IsEncryptedPayload()
		if err != nil {
			return false, err
		}

		if enc {
			return true, nil
		}
	}

	return false, nil
}

// importPassphrase prompts for the decryption passphrase once. It is only called
// when at least one present envelope is encrypted.
func importPassphrase(cmd *cli.Command) (string, error) {
	prompter := &passphrase.Prompter{
		Stdin:  cmd.Root().Reader,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	switch {
	case cmd.Bool(flagPassphraseStdin):
		pass, err := prompter.ReadFromStdin()
		if err != nil {
			return "", fmt.Errorf("failed to read passphrase from stdin: %w", err)
		}

		return pass, nil
	case terminal.IsTerminalWriter(cmd.Root().ErrWriter):
		pass, err := prompter.PromptForDecrypt()
		if err != nil {
			return "", fmt.Errorf("failed to get passphrase: %w", err)
		}

		return pass, nil
	default:
		return "", errors.New("encrypted file cannot be decrypted in non-TTY environment; use --passphrase-stdin")
	}
}

// importAction builds the action for the import commands. dest is the single
// source file for a service-specific import (service != "") or the source
// directory for a global import (service == "").
func importAction(service staging.Service, resolver staging.ScopeResolver) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() < 1 {
			if service != "" {
				return errors.New("usage: import <file>")
			}

			return errors.New("usage: import <dir>")
		}

		dest := cmd.Args().First()

		resolved, err := resolveScope(ctx, resolver)
		if err != nil {
			return err
		}

		scope := resolved.Scope

		pathFor := func(svc staging.Service) string {
			if service != "" {
				return dest
			}

			return filepath.Join(dest, string(svc)+".json")
		}

		// Validate scope/service on the plaintext headers before prompting for a
		// passphrase.
		present, err := collectImportEnvelopes(cmd, service, pathFor, scope.Key())
		if err != nil {
			if errors.Is(err, usestaging.ErrNothingToImport) {
				output.Info(cmd.Root().Writer, "No staged changes to import.")

				return nil
			}

			return err
		}

		encrypted, err := anyEncrypted(present)
		if err != nil {
			return err
		}

		var pass string

		if encrypted {
			pass, err = importPassphrase(cmd)
			if err != nil {
				return err
			}
		}

		working, err := file.NewWorkingStore(scope)
		if err != nil {
			return fmt.Errorf("failed to create staging store: %w", err)
		}

		// Peek the working area to decide whether a merge/overwrite prompt is needed.
		existing, err := working.Drain(ctx, service, true)
		if err != nil {
			return fmt.Errorf("failed to check the working staging area: %w", err)
		}

		chooser := &ImportModeChooser{
			Prompter: &confirm.Prompter{
				Stdin:  cmd.Root().Reader,
				Stdout: cmd.Root().Writer,
				Stderr: cmd.Root().ErrWriter,
				Target: resolved.Target,
			},
			Stderr: cmd.Root().ErrWriter,
			Stdout: cmd.Root().Writer,
		}

		mode, err := chooser.ChooseMode(ImportModeInput{
			MergeFlag:     cmd.Bool(flagMerge),
			OverwriteFlag: cmd.Bool(flagOverwrite),
			SkipPrompt:    cmd.Bool(flagYes),
			HasChanges:    !existing.IsEmpty(),
			ItemCount:     existing.TotalCount(),
			IsTTY:         terminal.IsTerminalWriter(cmd.Root().ErrWriter),
		})
		if err != nil {
			return err
		}

		if mode.Cancelled {
			output.Info(cmd.Root().Writer, "Operation cancelled.")

			return nil
		}

		uc := &usestaging.ImportUseCase{
			Source: &envelopeReadSource{
				passphrase: pass,
				pathFor:    pathFor,
			},
			Working: working,
		}

		result, err := uc.Execute(ctx, usestaging.ImportInput{Service: service, Mode: mode.Mode})
		if err != nil {
			if errors.Is(err, usestaging.ErrNothingToImport) {
				output.Info(cmd.Root().Writer, "No staged changes to import.")

				return nil
			}

			return err
		}

		if result.Merged {
			output.Success(cmd.Root().Writer, "Staged changes imported and merged into the working staging area")
		} else {
			output.Success(cmd.Root().Writer, "Staged changes imported into the working staging area")
		}

		return nil
	}
}

// NewGlobalImportCommand creates the global `stage import <dir>` command. It
// reads <dir>/param.json and <dir>/secret.json (missing files are skipped; an
// error only when neither exists) into the working staging area. The resolver
// determines the provider staging scope (nil defaults to AWS).
func NewGlobalImportCommand(resolver staging.ScopeResolver) *cli.Command {
	return &cli.Command{
		Name:      "import",
		Usage:     "Import staged changes from a directory (one file per service)",
		ArgsUsage: "<dir>",
		Description: `Import staged changes from a directory into the working staging area.

Reads one file per service:
   <dir>/param.json    SSM Parameter Store staged changes
   <dir>/secret.json   Secrets Manager staged changes

Missing files are skipped; it is an error only when neither exists. Each file's
scope is validated against the current scope (override with --force). A
Merge / Overwrite prompt appears only when the working staging area already
holds changes; use --merge / --overwrite to choose non-interactively.

EXAMPLES:
   suve stage import ./backup                       Import staged changes from ./backup
   suve stage import ./backup --overwrite           Replace the working staging area
   echo "secret" | suve stage import ./backup --passphrase-stdin   Decrypt with passphrase from stdin`,
		Flags:                  importFlags(),
		MutuallyExclusiveFlags: importMutuallyExclusiveFlags(),
		Action:                 importAction("", resolver),
	}
}

// NewImportCommand creates a service-specific `stage <svc> import <file>`
// command that reads the single service from <file>. A service mismatch (the
// file holds another service) is a hard error.
func NewImportCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:      "import",
		Usage:     fmt.Sprintf("Import staged %s changes from a file", cfg.ItemName),
		ArgsUsage: "<file>",
		Description: fmt.Sprintf(`Import staged %s changes from a file into the working staging area.

The file's service must match (%s); importing another service's file is a hard
error. The file's scope is validated against the current scope (override with
--force). A Merge / Overwrite prompt appears only when the working staging area
already holds changes; use --merge / --overwrite to choose non-interactively.

EXAMPLES:
   suve stage %s import ./%s.json                     Import staged %s changes
   suve stage %s import ./%s.json --overwrite         Replace the working staging area
   echo "secret" | suve stage %s import ./%s.json --passphrase-stdin   Decrypt with passphrase from stdin`,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Flags:                  importFlags(),
		MutuallyExclusiveFlags: importMutuallyExclusiveFlags(),
		Action:                 importAction(service, cfg.ScopeResolver),
	}
}
