// Package set provides the SSM set command.
package set

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/confirm"
)

// Client is the interface for the set command.
type Client interface {
	ssmapi.PutParameterAPI
}

// Runner executes the set command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the set command.
type Options struct {
	Name        string
	Value       string
	Type        string
	Description string
	Tags        map[string]string
}

// Command returns the set command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Set parameter value",
		ArgsUsage: "<name> <value>",
		Description: `Create a new parameter or update an existing one.

PARAMETER TYPES:
   String        Plain text value (default)
   StringList    Comma-separated list of values
   SecureString  Encrypted value using AWS KMS

The --secure flag is a shorthand for --type SecureString.
You cannot use both --secure and --type together.

EXAMPLES:
   suve ssm set /app/config/db-url "postgres://..."              Create String parameter
   suve ssm set --secure /app/config/api-key "secret123"         Create SecureString
   suve ssm set --type StringList /app/hosts "a.com,b.com"       Create StringList
   suve ssm set -y /app/config/db-url "postgres://..."           Set without confirmation`,
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
			&cli.StringSliceFlag{
				Name:  "tag",
				Usage: "Tag in key=value format (can be specified multiple times)",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve ssm set <name> <value>")
	}

	secure := cmd.Bool("secure")
	paramType := cmd.String("type")

	// Check for conflicting flags
	if secure && cmd.IsSet("type") {
		return fmt.Errorf("cannot use --secure with --type; use one or the other")
	}
	if secure {
		paramType = "SecureString"
	}

	name := cmd.Args().Get(0)
	skipConfirm := cmd.Bool("yes")

	// Confirm operation
	prompter := &confirm.Prompter{
		Stdin:  os.Stdin,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	confirmed, err := prompter.ConfirmAction("Set parameter", name, skipConfirm)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	tags, err := parseTags(cmd.StringSlice("tag"))
	if err != nil {
		return err
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name:        name,
		Value:       cmd.Args().Get(1),
		Type:        paramType,
		Description: cmd.String("description"),
		Tags:        tags,
	})
}

func parseTags(tagSlice []string) (map[string]string, error) {
	if len(tagSlice) == 0 {
		return nil, nil
	}
	tags := make(map[string]string)
	for _, t := range tagSlice {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format %q: expected key=value", t)
		}
		tags[parts[0]] = parts[1]
	}
	return tags, nil
}

// Run executes the set command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	input := &ssm.PutParameterInput{
		Name:      lo.ToPtr(opts.Name),
		Value:     lo.ToPtr(opts.Value),
		Type:      types.ParameterType(opts.Type),
		Overwrite: lo.ToPtr(true),
	}
	if opts.Description != "" {
		input.Description = lo.ToPtr(opts.Description)
	}
	if len(opts.Tags) > 0 {
		input.Tags = make([]types.Tag, 0, len(opts.Tags))
		for k, v := range opts.Tags {
			input.Tags = append(input.Tags, types.Tag{
				Key:   lo.ToPtr(k),
				Value: lo.ToPtr(v),
			})
		}
	}

	result, err := r.Client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}

	green := color.New(color.FgGreen).SprintFunc()
	_, _ = fmt.Fprintf(r.Stdout, "%s Set parameter %s (version: %d)\n",
		green("✓"),
		opts.Name,
		result.Version,
	)

	return nil
}
