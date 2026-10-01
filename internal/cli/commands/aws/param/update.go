package param

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/valueinput"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/usecase/param"
)

// updateRunner executes the update command.
type updateRunner struct {
	useCase *param.UpdateUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// updateOptions holds the options for the update command.
type updateOptions struct {
	name        string
	value       string
	paramType   string
	description string
	// preserveType keeps the parameter's existing type when neither --type nor
	// --secure was given, so a value-only update never downgrades a
	// SecureString/StringList to String. When true, paramType is ignored.
	preserveType bool
	// paramOpts holds the raw AWS-specific option flag values (tier, data
	// type, allowed pattern, policies). Empty fields contribute no option.
	paramOpts writeOptionFlags
}

// UpdateCommand returns the update command.
func UpdateCommand() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a parameter value",
		ArgsUsage: "<name> [<value>]",
		Description: `Update the value of an existing parameter.

This creates a new version of the parameter in AWS Systems Manager Parameter Store.

Use 'suve aws param create' to create a new parameter.
To manage tags, use 'suve aws param tag' and 'suve aws param untag' commands.

PARAMETER TYPES:
   String        Plain text value
   StringList    Comma-separated list of values
   SecureString  Encrypted value using AWS KMS

When neither --type nor --secure is given, the parameter's existing type is
preserved, so a value-only update never downgrades a SecureString or StringList
to String. Pass --type/--secure to change the type intentionally.

The --secure flag is a shorthand for --type SecureString.
You cannot use both --secure and --type together.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

EXAMPLES:
   suve aws param update /app/config/db-url "postgres://..."       Update parameter
   suve aws param update --secure /app/config/api-key "secret123"  Update as SecureString
   suve aws param update --yes /app/config/db-url "postgres://..." Update without confirmation
   printf '%s' "$V" | suve aws param update --yes --secure /app/key --value-stdin  Read value from stdin
   suve aws param update --secure /app/key                         Type value into $EDITOR`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "type",
				Usage: "Parameter type (String, StringList, SecureString); omit to preserve the existing type",
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
			valueinput.ValueStdinFlag(),
		},
		Action: updateAction,
	}
}

func updateAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve aws param update <name> [<value>]")
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

	// With neither --type nor --secure, preserve the parameter's existing type
	// instead of defaulting to String, so a value-only update never downgrades a
	// SecureString (dropping KMS encryption) or StringList.
	preserveType := !secure && !cmd.IsSet("type")

	if err := paramtype.Validate(paramType); err != nil {
		return err
	}

	if err := validateWriteOptionTier(cmd.String("tier")); err != nil {
		return err
	}

	name := args.Get(0)
	skipConfirm := cmd.Bool("yes")

	newValue, proceed, err := valueinput.ResolveValue(ctx, valueinput.ValueSource{
		FromStdin: cmd.Bool(valueinput.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the name, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     valueinput.ValueStdin(cmd),
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

	store, err := awsinternal.ParamStore(ctx)
	if err != nil {
		return err
	}

	uc := &param.UpdateUseCase{Store: store, ItemNoun: itemNoun()}

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
			Stdin:  valueinput.ValueStdin(cmd),
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
			Target: awsinternal.ConfirmTarget(ctx),
		}

		confirmed, err := prompter.ConfirmAction("Update parameter", name, false)
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	r := &updateRunner{
		useCase: uc,
		stdout:  cmd.Root().Writer,
		stderr:  cmd.Root().ErrWriter,
	}

	return r.run(ctx, updateOptions{
		name:         name,
		value:        newValue,
		paramType:    paramType,
		preserveType: preserveType,
		description:  cmd.String("description"),
		paramOpts: writeOptionFlags{
			tier:           cmd.String("tier"),
			dataType:       cmd.String("data-type"),
			allowedPattern: cmd.String("allowed-pattern"),
			policies:       cmd.String("policies"),
		},
	})
}

// run executes the update command.
func (r *updateRunner) run(ctx context.Context, opts updateOptions) error {
	result, err := r.useCase.Execute(ctx, param.UpdateInput{
		Name:         opts.name,
		Value:        opts.value,
		Type:         paramtype.Parse(opts.paramType),
		PreserveType: opts.preserveType,
		Description:  opts.description,
		Options:      buildWriteOptions(opts.paramOpts),
	})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Updated parameter %s (version: %s)", result.Name, result.Version)

	return nil
}
