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
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/tagging"
)

// Client is the interface for the set command.
type Client interface {
	paramapi.GetParameterAPI
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
   suve param set --yes /app/config/db-url "postgres://..."        Update without confirmation`,
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
				Usage: "Skip confirmation prompt (only applies when updating existing parameter)",
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

	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	newValue := cmd.Args().Get(1)

	// Check if parameter exists and get current value for diff
	currentValue, exists := getCurrentValue(ctx, client, name)

	// Only confirm for updates, not creates
	if exists && !skipConfirm {
		// Show diff
		diff := output.Diff(name+" (AWS)", name+" (new)", currentValue, newValue)
		if diff != "" {
			_, _ = fmt.Fprintln(cmd.Root().ErrWriter, diff)
		}

		// Confirm operation
		prompter := &confirm.Prompter{
			Stdin:  os.Stdin,
			Stdout: cmd.Root().Writer,
			Stderr: cmd.Root().ErrWriter,
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
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, Options{
		Name:        name,
		Value:       newValue,
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

// getCurrentValue fetches the current parameter value.
// Returns the value and true if exists, empty string and false if not found.
func getCurrentValue(ctx context.Context, client paramapi.GetParameterAPI, name string) (string, bool) {
	result, err := client.GetParameter(ctx, &paramapi.GetParameterInput{
		Name:           lo.ToPtr(name),
		WithDecryption: lo.ToPtr(true),
	})
	if err != nil {
		return "", false
	}
	if result.Parameter == nil || result.Parameter.Value == nil {
		return "", false
	}
	return *result.Parameter.Value, true
}
