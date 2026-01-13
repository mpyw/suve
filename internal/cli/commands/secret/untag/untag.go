// Package untag provides the Secrets Manager untag command.
package untag

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// Runner executes the untag command.
type Runner struct {
	UseCase *secret.TagUseCase
	Stdout  io.Writer
}

// Options holds the options for the untag command.
type Options struct {
	Name string
	Keys []string
}

// Command returns the untag command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "untag",
		Usage:     "Remove tags from a secret",
		ArgsUsage: "<name> <key>...",
		Description: `Remove one or more tags from an existing secret.

Specify the tag keys to remove. Non-existent keys are silently ignored.

EXAMPLES:
   suve secret untag my-api-key deprecated               Remove single tag
   suve secret untag my-api-key env team                 Remove multiple tags`,
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 2 {
		return fmt.Errorf("usage: suve secret untag <name> <key> [key]")
	}

	name := cmd.Args().Get(0)
	keys := cmd.Args().Slice()[1:]

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		UseCase: &secret.TagUseCase{Client: client},
		Stdout:  cmd.Root().Writer,
	}

	return r.Run(ctx, Options{
		Name: name,
		Keys: keys,
	})
}

// Run executes the untag command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	err := r.UseCase.Execute(ctx, secret.TagInput{
		Name:   opts.Name,
		Remove: opts.Keys,
	})
	if err != nil {
		return err
	}

	output.Success(r.Stdout, "Untagged secret %s (%d key(s))", opts.Name, len(opts.Keys))

	return nil
}
