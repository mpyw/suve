// Package update provides the SSM Parameter Store update command.
package update

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/param/paramopts"
	"github.com/mpyw/suve/internal/cli/commands/param/paramtype"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/param"
)

// Runner executes the update command.
type Runner struct {
	UseCase *param.UpdateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the update command.
type Options struct {
	Name        string
	Value       string
	Type        string
	Description string
	// ParamOpts holds the raw AWS-specific option flag values (tier, data
	// type, allowed pattern, policies). Empty fields contribute no option.
	ParamOpts paramopts.Values
}

// Command returns the update command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a parameter value",
		ArgsUsage: "<name> [<value>]",
		Description: `Update the value of an existing parameter.

This creates a new version of the parameter in AWS Systems Manager Parameter Store.

Use 'suve param create' to create a new parameter.
To manage tags, use 'suve param tag' and 'suve param untag' commands.

PARAMETER TYPES:
   String        Plain text value (default)
   StringList    Comma-separated list of values
   SecureString  Encrypted value using AWS KMS

The --secure flag is a shorthand for --type SecureString.
You cannot use both --secure and --type together.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

EXAMPLES:
   suve param update /app/config/db-url "postgres://..."       Update parameter
   suve param update --secure /app/config/api-key "secret123"  Update as SecureString
   suve param update --yes /app/config/db-url "postgres://..." Update without confirmation
   printf '%s' "$V" | suve param update --yes --secure /app/key --value-stdin  Read value from stdin
   suve param update --secure /app/key                         Type value into $EDITOR`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "type",
				Value: "String",
				Usage: "Parameter type (String, StringList, SecureString)",
			},
			&cli.BoolFlag{
				Name:  "secure",
				Usage: "Shorthand for --type SecureString",
			},
			&cli.StringFlag{
				Name:  "description",
				Usage: "Parameter description",
			},
			&cli.StringFlag{
				Name:  "tier",
				Usage: "Parameter tier (Standard, Advanced, Intelligent-Tiering)",
			},
			&cli.StringFlag{
				Name:  "data-type",
				Usage: "Parameter data type (e.g. text, aws:ec2:image)",
			},
			&cli.StringFlag{
				Name:  "allowed-pattern",
				Usage: "Regular expression the value must match",
			},
			&cli.StringFlag{
				Name:  "policies",
				Usage: "Parameter policies as a JSON document",
			},
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
			internal.ValueStdinFlag(),
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve param update <name> [<value>]")
	}

	secure := cmd.Bool("secure")
	paramType := cmd.String("type")

	// Check for conflicting flags
	if secure && cmd.IsSet("type") {
		return errors.New("cannot use --secure with --type; use one or the other")
	}

	if secure {
		paramType = "SecureString"
	}

	if err := paramtype.Validate(paramType); err != nil {
		return err
	}

	if err := paramopts.ValidateTier(cmd.String("tier")); err != nil {
		return err
	}

	name := args.Get(0)
	skipConfirm := cmd.Bool("yes")

	newValue, proceed, err := internal.ResolveValue(ctx, internal.ValueSource{
		FromStdin: cmd.Bool(internal.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the name, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     internal.Stdin(cmd),
		// Without --yes we prompt for confirmation on the same stdin below;
		// reading the value from stdin would leave nothing for that prompt.
		ConfirmRequired: !skipConfirm,
	})
	if err != nil {
		return err
	}

	if !proceed {
		output.Info(cmd.Root().Writer, "Empty value, nothing to update.")

		return nil
	}

	store, err := internal.ParamStore(ctx)
	if err != nil {
		return err
	}

	uc := &param.UpdateUseCase{Store: store}

	// Fetch current value and show diff before confirming
	if !skipConfirm {
		currentValue, _ := uc.GetCurrentValue(ctx, name)
		if currentValue != "" {
			diff := output.Diff(cmd.Root().ErrWriter, name+" (AWS)", name+" (new)", currentValue, newValue)
			if diff != "" {
				output.Println(cmd.Root().ErrWriter, diff)
			}
		}

		// Confirm operation
		prompter := &confirm.Prompter{
			Stdin:  internal.Stdin(cmd),
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
		}
		if identity, _ := infra.GetAWSIdentity(ctx); identity != nil {
			prompter.AccountID = identity.AccountID
			prompter.Region = identity.Region
			prompter.Profile = identity.Profile
		}

		confirmed, err := prompter.ConfirmAction("Update parameter", name, false)
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	r := &Runner{
		UseCase: uc,
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, Options{
		Name:        name,
		Value:       newValue,
		Type:        paramType,
		Description: cmd.String("description"),
		ParamOpts: paramopts.Values{
			Tier:           cmd.String("tier"),
			DataType:       cmd.String("data-type"),
			AllowedPattern: cmd.String("allowed-pattern"),
			Policies:       cmd.String("policies"),
		},
	})
}

// Run executes the update command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, param.UpdateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Type:        paramtype.Parse(opts.Type),
		Description: opts.Description,
		Options:     paramopts.Build(opts.ParamOpts),
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Updated parameter %s (version: %d)", result.Name, result.Version)

	return nil
}
