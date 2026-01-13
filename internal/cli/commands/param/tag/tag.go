// Package tag provides the SSM Parameter Store tag command.
package tag

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/param"
)

// Runner executes the tag command.
type Runner struct {
	UseCase *param.TagUseCase
	Stdout  io.Writer
}

// Options holds the options for the tag command.
type Options struct {
	Name string
	Tags map[string]string
}

// Command returns the tag command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "tag",
		Usage:     "Add or update tags on a parameter",
		ArgsUsage: "<name> <key=value>...",
		Description: `Add or update one or more tags on an existing parameter.

Tags are key=value pairs. If a tag key already exists, its value will be updated.
You can specify multiple tags in a single command.

EXAMPLES:
   suve param tag /app/config env=prod                     Add single tag
   suve param tag /app/config env=prod team=backend        Add multiple tags
   suve param tag /app/config env=staging                  Update existing tag`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve param tag <name> <key=value> [key=value]")
	}

	name := cmd.Args().Get(0)

	tags, err := parseTags(cmd.Args().Slice()[1:])
	if err != nil {
		return err
	}

	client, err := infra.NewParamClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		UseCase: &param.TagUseCase{Client: client},
		Stdout:  cmd.Root().Writer,
	}

	return r.Run(ctx, Options{
		Name: name,
		Tags: tags,
	})
}

// Run executes the tag command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	err := r.UseCase.Execute(ctx, param.TagInput{
		Name: opts.Name,
		Add:  opts.Tags,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Tagged parameter %s (%d tag(s))", opts.Name, len(opts.Tags))

	return nil
}

func parseTags(args []string) (map[string]string, error) {
	tags := make(map[string]string)

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag format %q: expected key=value", arg)
		}

		key, value := parts[0], parts[1]
		if key == "" {
			return nil, fmt.Errorf("invalid tag format %q: key cannot be empty", arg)
		}

		tags[key] = value
	}

	return tags, nil
}
