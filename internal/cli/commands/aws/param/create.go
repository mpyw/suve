package param

import (
	"context"
	"errors"
	"io"

	"github.com/urfave/cli/v3"

	awsinternal "github.com/mpyw/suve/internal/cli/commands/aws/internal"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/provider/aws/paramtype"
	"github.com/mpyw/suve/internal/usecase/param"
)

// CreateRunner executes the create command.
type CreateRunner struct {
	UseCase *param.CreateUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// CreateOptions holds the options for the create command.
type CreateOptions struct {
	Name        string
	Value       string
	Type        string
	Description string
	// ParamOpts holds the raw AWS-specific option flag values (tier, data
	// type, allowed pattern, policies). Empty fields contribute no option.
	ParamOpts WriteOptionFlags
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
			cliinternal.ValueStdinFlag(),
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

	value, proceed, err := cliinternal.ResolveValue(ctx, cliinternal.ValueSource{
		FromStdin: cmd.Bool(cliinternal.FlagValueStdin),
		HasArg:    args.Len() >= 2, //nolint:mnd // arg 0 is the name, arg 1 is the optional value
		Arg:       args.Get(1),
		Stdin:     cliinternal.ValueStdin(cmd),
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

	r := &CreateRunner{
		UseCase: &param.CreateUseCase{Writer: store},
		Stdout:  cmd.Root().Writer,
		Stderr:  cmd.Root().ErrWriter,
	}

	return r.Run(ctx, CreateOptions{
		Name:        args.Get(0),
		Value:       value,
		Type:        paramType,
		Description: cmd.String("description"),
		ParamOpts: WriteOptionFlags{
			Tier:           cmd.String("tier"),
			DataType:       cmd.String("data-type"),
			AllowedPattern: cmd.String("allowed-pattern"),
			Policies:       cmd.String("policies"),
		},
	})
}

// Run executes the create command.
func (r *CreateRunner) Run(ctx context.Context, opts CreateOptions) error {
	result, err := r.UseCase.Execute(ctx, param.CreateInput{
		Name:        opts.Name,
		Value:       opts.Value,
		Type:        paramtype.Parse(opts.Type),
		Description: opts.Description,
		Options:     buildWriteOptions(opts.ParamOpts),
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Created parameter %s (version: %s)", result.Name, result.Version)

	return nil
}
