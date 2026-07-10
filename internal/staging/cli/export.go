package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/passphrase"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
	usestaging "github.com/mpyw/suve/internal/usecase/staging"
)

// envelopeWriteTarget adapts file.WriteEnvelopeFile to the export use case's
// EnvelopeWriter port. It binds the destination path resolver, the scope (kept
// in the plaintext header), and the passphrase, so the use case only supplies
// the service and its state.
type envelopeWriteTarget struct {
	scope      provider.Scope
	passphrase string
	// pathFor resolves the per-service destination path. For a service-specific
	// export it always returns the single target file; for a global export it
	// returns <dir>/<service>.json.
	pathFor func(staging.Service) string
}

// WriteEnvelope writes svc's state to its resolved destination path.
func (t *envelopeWriteTarget) WriteEnvelope(_ context.Context, svc staging.Service, state *staging.State) error {
	return file.WriteEnvelopeFile(t.pathFor(svc), t.scope, svc, state, t.passphrase)
}

// exportFlags returns the flags shared by the global and service-specific export
// commands. Export always writes the working area out wholesale, so there is no
// --merge / --overwrite here.
func exportFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "keep",
			Usage: "Keep staged changes in the working staging area after exporting",
		},
		&cli.BoolFlag{
			Name:    flagYes,
			Aliases: []string{flagForce},
			Usage:   "Overwrite existing export file(s) without confirmation",
		},
		&cli.BoolFlag{
			Name:  flagPassphraseStdin,
			Usage: usagePassphraseStdin,
		},
	}
}

// exportPassphrase prompts for the encryption passphrase once per command.
// It returns the passphrase (empty means plaintext), whether the user cancelled,
// and any error. --passphrase-stdin reads from stdin; otherwise a TTY is prompted
// interactively and a non-TTY falls back to plaintext with a warning.
func exportPassphrase(cmd *cli.Command) (pass string, cancelled bool, err error) {
	prompter := &passphrase.Prompter{
		Stdin:  cmd.Root().Reader,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	switch {
	case cmd.Bool(flagPassphraseStdin):
		pass, err = prompter.ReadFromStdin()
		if err != nil {
			return "", false, fmt.Errorf("failed to read passphrase from stdin: %w", err)
		}

		return pass, false, nil
	case terminal.IsTerminalWriter(cmd.Root().ErrWriter):
		pass, err = prompter.PromptForEncrypt()
		if err != nil {
			if errors.Is(err, passphrase.ErrCancelled) {
				return "", true, nil
			}

			return "", false, fmt.Errorf("failed to get passphrase: %w", err)
		}

		return pass, false, nil
	default:
		prompter.WarnNonTTY()

		return "", false, nil
	}
}

// exportServices returns the services that will actually be written for this
// command: the single requested service (when non-empty and staged), otherwise
// every non-empty service in the peeked working state.
func exportServices(service staging.Service, state *staging.State) []staging.Service {
	services := []staging.Service{staging.ServiceParam, staging.ServiceSecret}
	if service != "" {
		services = []staging.Service{service}
	}

	var result []staging.Service

	for _, svc := range services {
		if !state.ExtractService(svc).IsEmpty() {
			result = append(result, svc)
		}
	}

	return result
}

// confirmExportOverwrite prompts (or refuses) before overwriting existing export
// files. It is a plain overwrite confirmation, NOT a merge: export always writes
// wholesale. --yes / --force skip it; a non-TTY without --yes is refused.
func confirmExportOverwrite(cmd *cli.Command, paths []string) (proceed bool, err error) {
	if cmd.Bool(flagYes) {
		return true, nil
	}

	var existing []string

	for _, p := range paths {
		if _, statErr := os.Stat(p); statErr == nil {
			existing = append(existing, p)
		}
	}

	if len(existing) == 0 {
		return true, nil
	}

	if !terminal.IsTerminalWriter(cmd.Root().ErrWriter) {
		return false, fmt.Errorf(
			"export file(s) already exist: %v; re-run with --yes to overwrite",
			existing,
		)
	}

	prompter := &confirm.Prompter{
		Stdin:  cmd.Root().Reader,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}

	output.Warning(cmd.Root().ErrWriter, "Export file(s) already exist and will be overwritten: %v", existing)

	return prompter.Confirm("Overwrite?", false)
}

// exportAction builds the action for the export commands. dest is the single
// target file for a service-specific export (service != "") or the destination
// directory for a global export (service == "").
func exportAction(service staging.Service, resolver staging.ScopeResolver) func(context.Context, *cli.Command) error {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() < 1 {
			if service != "" {
				return errors.New("usage: export <file>")
			}

			return errors.New("usage: export <dir>")
		}

		dest := cmd.Args().First()

		resolved, err := resolveScope(ctx, resolver)
		if err != nil {
			return err
		}

		scope := resolved.Scope

		working, err := file.NewWorkingStore(scope)
		if err != nil {
			return fmt.Errorf("failed to create staging store: %w", err)
		}

		// Peek the working area (keep=true) so we can (a) short-circuit an empty
		// export before prompting for a passphrase and (b) confirm overwriting only
		// the files we are actually going to write.
		peek, err := working.Drain(ctx, "", true)
		if err != nil {
			return fmt.Errorf("failed to read the working staging area: %w", err)
		}

		services := exportServices(service, peek)
		if len(services) == 0 {
			output.Info(cmd.Root().Writer, "No staged changes to export.")

			return nil
		}

		pathFor := func(svc staging.Service) string {
			if service != "" {
				return dest
			}

			return filepath.Join(dest, string(svc)+".json")
		}

		targetPaths := make([]string, 0, len(services))
		for _, svc := range services {
			targetPaths = append(targetPaths, pathFor(svc))
		}

		proceed, err := confirmExportOverwrite(cmd, targetPaths)
		if err != nil {
			return err
		}

		if !proceed {
			output.Info(cmd.Root().Writer, "Operation cancelled.")

			return nil
		}

		pass, cancelled, err := exportPassphrase(cmd)
		if err != nil {
			return err
		}

		if cancelled {
			return nil
		}

		uc := &usestaging.ExportUseCase{
			Working: working,
			Target: &envelopeWriteTarget{
				scope:      scope,
				passphrase: pass,
				pathFor:    pathFor,
			},
		}

		_, err = uc.Execute(ctx, usestaging.ExportInput{Service: service, Keep: cmd.Bool("keep")})
		if err != nil {
			if errors.Is(err, usestaging.ErrNothingToExport) {
				output.Info(cmd.Root().Writer, "No staged changes to export.")

				return nil
			}

			// A non-fatal error means the export files were written but clearing the
			// working area failed; warn and continue to the success message.
			var expErr *usestaging.ExportError
			if errors.As(err, &expErr) && expErr.NonFatal {
				output.Warning(cmd.Root().ErrWriter, "%v", err)
			} else {
				return err
			}
		}

		encrypted := pass != ""
		kept := cmd.Bool("keep")

		switch {
		case encrypted && kept:
			output.Success(cmd.Root().Writer, "Staged changes exported (encrypted, kept in the working staging area)")
		case encrypted:
			output.Success(cmd.Root().Writer, "Staged changes exported (encrypted) and cleared from the working staging area")
		case kept:
			output.Success(cmd.Root().Writer, "Staged changes exported (kept in the working staging area)")
		default:
			output.Success(cmd.Root().Writer, "Staged changes exported and cleared from the working staging area")
		}

		if !encrypted {
			output.Warn(cmd.Root().ErrWriter, "Secrets are stored as plain text.")
		}

		return nil
	}
}

// NewGlobalExportCommand creates the global `stage export <dir>` command. It
// writes <dir>/param.json and <dir>/secret.json, one file per service that has
// staged changes (empty services are skipped). The resolver determines the
// provider staging scope (nil defaults to AWS).
func NewGlobalExportCommand(resolver staging.ScopeResolver) *cli.Command {
	return &cli.Command{
		Name:      "export",
		Usage:     "Export staged changes to a directory (one file per service)",
		ArgsUsage: "<dir>",
		Description: `Export staged changes from the working staging area to a directory.

Writes one file per service that has staged changes:
   <dir>/param.json    SSM Parameter Store staged changes
   <dir>/secret.json   Secrets Manager staged changes

Only services with staged changes are written. The directory is created if
needed. Each file is a plaintext JSON envelope whose payload is encrypted when
a passphrase is supplied (empty passphrase = plaintext).

By default the working staging area is cleared after exporting; use --keep to
retain it.

EXAMPLES:
   suve stage export ./backup                       Export all staged changes to ./backup
   suve stage export ./backup --keep                Export but keep the working staging area
   echo "secret" | suve stage export ./backup --passphrase-stdin   Encrypt with passphrase from stdin`,
		Flags:  exportFlags(),
		Action: exportAction("", resolver),
	}
}

// NewExportCommand creates a service-specific `stage <svc> export <file>`
// command that writes the single service to <file>.
func NewExportCommand(cfg CommandConfig) *cli.Command {
	parser := cfg.ParserFactory()
	service := parser.Service()

	return &cli.Command{
		Name:      "export",
		Usage:     fmt.Sprintf("Export staged %s changes to a file", cfg.ItemName),
		ArgsUsage: "<file>",
		Description: fmt.Sprintf(`Export staged %s changes from the working staging area to a file.

The file is a plaintext JSON envelope whose payload is encrypted when a
passphrase is supplied (empty passphrase = plaintext). The parent directory is
created if needed.

By default the %s entries are cleared from the working staging area after
exporting; use --keep to retain them.

EXAMPLES:
   suve stage %s export ./%s.json                     Export staged %s changes
   suve stage %s export ./%s.json --keep              Export but keep the working staging area
   echo "secret" | suve stage %s export ./%s.json --passphrase-stdin   Encrypt with passphrase from stdin`,
			cfg.ItemName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.ItemName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName,
			cfg.CommandName),
		Flags:  exportFlags(),
		Action: exportAction(service, cfg.ScopeResolver),
	}
}
