// Package diff provides the SSM diff command.
package diff

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/urfave/cli/v2"

	internalaws "github.com/mpyw/suve/internal/aws"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/ssm"
	"github.com/mpyw/suve/internal/version"
)

// Client is the interface for the diff command.
type Client interface {
	ssm.GetParameterAPI
	ssm.GetParameterHistoryAPI
}

// Command returns the diff command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show diff between two versions",
		ArgsUsage: "<name> <version1> [version2]",
		Action:    action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: suve ssm diff <name> <version1> [version2]")
	}

	name := c.Args().Get(0)
	version1 := c.Args().Get(1)
	version2 := c.Args().Get(2)

	if version2 == "" {
		version2 = version1
		version1 = ""
	}

	client, err := internalaws.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, name, version1, version2)
}

// Run executes the diff command.
func Run(ctx context.Context, client Client, w io.Writer, name, version1, version2 string) error {
	spec1, err := version.Parse(name + version1)
	if err != nil {
		return fmt.Errorf("invalid version1: %w", err)
	}

	spec2, err := version.Parse(name + version2)
	if err != nil {
		return fmt.Errorf("invalid version2: %w", err)
	}

	param1, err := ssm.GetParameterWithVersion(ctx, client, spec1, true)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version1, err)
	}

	param2, err := ssm.GetParameterWithVersion(ctx, client, spec2, true)
	if err != nil {
		return fmt.Errorf("failed to get version %s: %w", version2, err)
	}

	diff := output.Diff(
		fmt.Sprintf("%s@%d", name, param1.Version),
		fmt.Sprintf("%s@%d", name, param2.Version),
		aws.ToString(param1.Value),
		aws.ToString(param2.Value),
	)
	_, _ = fmt.Fprint(w, diff)

	return nil
}
