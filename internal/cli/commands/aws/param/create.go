package param

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/valueinput"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/usecase/param"
)

// createRunner executes the create command.
type createRunner struct {
	useCase *param.CreateUseCase
	stdout  io.Writer
	stderr  io.Writer
}

// createOptions holds the options for the create command.
type createOptions struct {
	name        string
	value       string
	paramType   string
	description string
	// paramOpts holds the raw AWS-specific option flag values (tier, data
	// type, allowed pattern, policies). Empty fields contribute no option.
	paramOpts writeOptionFlags
}

// CreateCommand returns the create command.
func CreateCommand() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new parameter",
		ArgsUsage: "<name> [<value>]",
		Description: `Create a new parameter in AWS Systems Manager Parameter Store.

Use this command for new parameters only. To update an existing parameter,
use 'suve aws param update' instead.

PARAMETER TYPES:
   String        Plain text value (default)
   StringList    Comma-separated list of values
   SecureString  Encrypted value using AWS KMS

The --secure flag is a shorthand for --type SecureString.
You cannot use both --secure and --type together.

The value may be given as a positional argument, read from stdin with
--value-stdin (so it never appears in argv/ps or shell history), or, when
omitted, typed into $EDITOR.

To add tags after creation, use 'suve aws param tag' command.

EXAMPLES:
   suve aws param create /app/config/db-url "postgres://..."       Create String parameter
   suve aws param create --secure /app/config/api-key "secret123"  Create SecureString
   suve aws param create --type StringList /app/hosts "a.com,b.com" Create StringList
   suve aws param create --description "DB URL" /app/db-url "..."  With description
   printf '%s' "$V" | suve aws param create --secure /app/key --value-stdin  Read value from stdin
   suve aws param create --secure /app/key                         Type value into $EDITOR`,
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
			valueinput.ValueStdinFlag(),
		},
		Action: createAction,
	}
}

func createAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 1 {
		return errors.New("usage: suve aws param create <name> [<value>]")
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

	if err := validateWriteOptionTier(cmd.String("tier")); err != nil {
		return err
	}

	value, proceed, err := valueinput.ResolveValue(ctx, valueinput.ValueSource{
		FromStdin: cmd.Bool(valueinput.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the name, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     valueinput.ValueStdin(cmd),
	})
	if err != nil {
		return err
	}

	if !proceed {
		output.Info(cmd.Root().Writer, "Empty value, nothing to create.")

		return nil
	}

	store, err := awsinternal.ParamStore(ctx)
	if err != nil {
		return err
	}

	r := &createRunner{
		useCase: &param.CreateUseCase{Writer: store, ItemNoun: itemNoun()},
		stdout:  cmd.Root().Writer,
		stderr:  cmd.Root().ErrWriter,
	}

	return r.run(ctx, createOptions{
		name:        args.Get(0),
		value:       value,
		paramType:   paramType,
		description: cmd.String("description"),
		paramOpts: writeOptionFlags{
			tier:           cmd.String("tier"),
			dataType:       cmd.String("data-type"),
			allowedPattern: cmd.String("allowed-pattern"),
			policies:       cmd.String("policies"),
		},
	})
}

// run executes the create command.
func (r *createRunner) run(ctx context.Context, opts createOptions) error {
	result, err := r.useCase.Execute(ctx, param.CreateInput{
		Name:        opts.name,
		Value:       opts.value,
		Type:        paramtype.Parse(opts.paramType),
		Description: opts.description,
		Options:     buildWriteOptions(opts.paramOpts),
	})
	if err != nil {
		return err
	}

	output.Success(r.stdout, "Created parameter %s (version: %s)", result.Name, result.Version)

	return nil
}
