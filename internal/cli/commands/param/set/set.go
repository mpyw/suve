// Package set provides the SSM Parameter Store set command.
package set

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/tagging"
)

// Client is the interface for the set command.
type Client interface {
	paramapi.PutParameterAPI
	paramapi.AddTagsToResourceAPI
	paramapi.RemoveTagsFromResourceAPI
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
	TagChange   *tagging.Change
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

TAGGING:
   --tag adds or updates tags (additive, does not remove existing tags)
   --untag removes specific tags by key
   If the same key appears in both, the later flag wins with a warning.

EXAMPLES:
   suve param set /app/config/db-url "postgres://..."              Create String parameter
   suve param set --secure /app/config/api-key "secret123"         Create SecureString
   suve param set --type StringList /app/hosts "a.com,b.com"       Create StringList
   suve param set --tag env=prod --tag team=platform /app/key val  Set with tags
   suve param set --untag deprecated /app/key val                  Remove a tag
   suve param set --yes /app/config/db-url "postgres://..."        Set without confirmation`,
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
				Usage: "Tag in key=value format (can be specified multiple times, additive)",
			},
			&cli.StringSliceFlag{
				Name:  "untag",
				Usage: "Tag key to remove (can be specified multiple times)",
			},
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve param set <name> <value>")
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

	// Parse tags
	tagResult, err := tagging.ParseFlags(cmd.StringSlice("tag"), cmd.StringSlice("untag"))
	if err != nil {
		return err
	}

	// Output warnings
	for _, w := range tagResult.Warnings {
		output.Warning(cmd.Root().ErrWriter, "%s", w)
	}

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

	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
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
		TagChange:   tagResult.Change,
	})
}

// Run executes the set command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	input := &paramapi.PutParameterInput{
		Name:      lo.ToPtr(opts.Name),
		Value:     lo.ToPtr(opts.Value),
		Type:      paramapi.ParameterType(opts.Type),
		Overwrite: lo.ToPtr(true),
	}
	if opts.Description != "" {
		input.Description = lo.ToPtr(opts.Description)
	}

	result, err := r.Client.PutParameter(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set parameter: %w", err)
	}

	// Apply tag changes (additive)
	if opts.TagChange != nil && !opts.TagChange.IsEmpty() {
		if err := tagging.ApplyParam(ctx, r.Client, opts.Name, opts.TagChange); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(r.Stdout, "%s Set parameter %s (version: %d)\n",
		colors.Success("✓"),
		opts.Name,
		result.Version,
	)

	return nil
}
