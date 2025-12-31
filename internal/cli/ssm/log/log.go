// Package log provides the SSM log command for viewing parameter version history.
//
// The log command displays version history with optional patch/diff output,
// similar to git log. Use -p/--patch to show differences between consecutive versions.
package log

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/output"
)

// Client is the interface for the log command.
type Client interface {
	ssmapi.GetParameterHistoryAPI
}

// Runner executes the log command.
type Runner struct {
	Client Client
	Stdout io.Writer
	Stderr io.Writer
}

// Options holds the options for the log command.
type Options struct {
	Name       string
	MaxResults int32
	ShowPatch  bool
	JSONFormat bool
	Reverse    bool
}

// Command returns the log command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "log",
		Usage:     "Show parameter version history",
		ArgsUsage: "<name>",
		Description: `Display the version history of a parameter, showing each version's
number, modification date, and a preview of the value.

Output is sorted with the most recent version first (use --reverse to flip).
Value previews are truncated at 50 characters.

Use -p/--patch to show the diff between consecutive versions (like git log -p).
Use -j/--json with -p to format JSON values before diffing (keys are always sorted).

EXAMPLES:
   suve ssm log /app/config/db-url           Show last 10 versions (default)
   suve ssm log -n 5 /app/config/db-url      Show last 5 versions
   suve ssm log -p /app/config/db-url        Show versions with diffs
   suve ssm log -p -j /app/config/db-url     Show diffs with JSON formatting
   suve ssm log --reverse /app/config/db-url Show oldest first`,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "number",
				Aliases: []string{"n"},
				Value:   10,
				Usage:   "Maximum number of versions to show",
			},
			&cli.BoolFlag{
				Name:    "patch",
				Aliases: []string{"p"},
				Value:   false,
				Usage:   "Show diff between consecutive versions",
			},
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (use with -p; keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:    "reverse",
				Aliases: []string{"r"},
				Usage:   "Show oldest versions first",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("parameter name required")
	}

	opts := Options{
		Name:       cmd.Args().First(),
		MaxResults: int32(cmd.Int("number")),
		ShowPatch:  cmd.Bool("patch"),
		JSONFormat: cmd.Bool("json"),
		Reverse:    cmd.Bool("reverse"),
	}

	// Warn if --json is used without -p
	if opts.JSONFormat && !opts.ShowPatch {
		output.Warning(cmd.Root().ErrWriter, "--json has no effect without -p/--patch")
	}

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	r := &Runner{
		Client: client,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
	}
	return r.Run(ctx, opts)
}

// Run executes the log command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.Client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           lo.ToPtr(opts.Name),
		MaxResults:     lo.ToPtr(opts.MaxResults),
		WithDecryption: lo.ToPtr(true),
	})
	if err != nil {
		return fmt.Errorf("failed to get parameter history: %w", err)
	}

	params := result.Parameters
	if len(params) == 0 {
		return nil
	}

	// AWS returns oldest first; reverse to show newest first (unless --reverse)
	if !opts.Reverse {
		slices.Reverse(params)
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Track if we've warned about invalid JSON (only warn once)
	jsonWarned := false

	// Find the current (latest) version index
	currentIdx := 0
	if opts.Reverse {
		currentIdx = len(params) - 1
	}

	for i, param := range params {
		versionLabel := fmt.Sprintf("Version %d", param.Version)
		if i == currentIdx {
			versionLabel += " " + green("(current)")
		}
		_, _ = fmt.Fprintln(r.Stdout, yellow(versionLabel))
		if param.LastModifiedDate != nil {
			_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", cyan("Date:"), param.LastModifiedDate.Format(time.RFC3339))
		}

		if opts.ShowPatch {
			// Determine old/new indices based on order
			var oldIdx, newIdx int
			if opts.Reverse {
				// In reverse mode: comparing with next version (newer)
				if i < len(params)-1 {
					oldIdx = i
					newIdx = i + 1
				} else {
					oldIdx = -1 // No diff for the last (current) version
				}
			} else {
				// In normal mode: comparing with previous version (older)
				if i < len(params)-1 {
					oldIdx = i + 1
					newIdx = i
				} else {
					oldIdx = -1 // No diff for the oldest version
				}
			}

			if oldIdx >= 0 {
				oldValue := lo.FromPtr(params[oldIdx].Value)
				newValue := lo.FromPtr(params[newIdx].Value)
				oldName := fmt.Sprintf("%s#%d", opts.Name, params[oldIdx].Version)
				newName := fmt.Sprintf("%s#%d", opts.Name, params[newIdx].Version)
				diff := output.DiffWithJSON(oldName, newName, oldValue, newValue, opts.JSONFormat, &jsonWarned, r.Stderr)
				if diff != "" {
					_, _ = fmt.Fprintln(r.Stdout)
					_, _ = fmt.Fprint(r.Stdout, diff)
				}
			}
		} else {
			// Show truncated value preview
			value := lo.FromPtr(param.Value)
			if len(value) > 50 {
				value = value[:50] + "..."
			}
			_, _ = fmt.Fprintf(r.Stdout, "%s\n", value)
		}

		if i < len(params)-1 {
			_, _ = fmt.Fprintln(r.Stdout)
		}
	}

	return nil
}
