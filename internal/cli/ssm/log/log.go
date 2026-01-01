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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/pager"
	"github.com/mpyw/suve/internal/version/ssmversion"
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
	Name        string
	MaxResults  int32
	ShowPatch   bool
	JSONFormat  bool
	Reverse     bool
	NoPager     bool
	Oneline     bool
	FromVersion *int64
	ToVersion   *int64
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
Use --oneline for a compact one-line-per-version format.
Use --from/--to to filter by version range (accepts version specs like '#3', '~1').

EXAMPLES:
   suve ssm log /app/config/db-url              Show last 10 versions (default)
   suve ssm log -n 5 /app/config/db-url         Show last 5 versions
   suve ssm log -p /app/config/db-url           Show versions with diffs
   suve ssm log -p -j /app/config/db-url        Show diffs with JSON formatting
   suve ssm log --oneline /app/config/db-url    Compact one-line format
   suve ssm log --reverse /app/config/db-url    Show oldest first
   suve ssm log --from '#3' --to '#5' /app/...  Show versions 3 to 5`,
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
				Name:  "oneline",
				Usage: "Compact one-line-per-version format",
			},
			&cli.BoolFlag{
				Name:  "reverse",
				Usage: "Show oldest versions first",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.StringFlag{
				Name:  "from",
				Usage: "Start version (e.g., '#3', '~2')",
			},
			&cli.StringFlag{
				Name:  "to",
				Usage: "End version (e.g., '#5', '~0')",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve ssm log <name>")
	}

	name := cmd.Args().First()

	opts := Options{
		Name:       name,
		MaxResults: int32(cmd.Int("number")),
		ShowPatch:  cmd.Bool("patch"),
		JSONFormat: cmd.Bool("json"),
		Reverse:    cmd.Bool("reverse"),
		NoPager:    cmd.Bool("no-pager"),
		Oneline:    cmd.Bool("oneline"),
	}

	// Parse --from version spec
	if fromArg := cmd.String("from"); fromArg != "" {
		fromVersion, err := parseVersionSpec(name, fromArg)
		if err != nil {
			return fmt.Errorf("invalid --from value: %w", err)
		}
		opts.FromVersion = fromVersion
	}

	// Parse --to version spec
	if toArg := cmd.String("to"); toArg != "" {
		toVersion, err := parseVersionSpec(name, toArg)
		if err != nil {
			return fmt.Errorf("invalid --to value: %w", err)
		}
		opts.ToVersion = toVersion
	}

	// Warn if --json is used without -p
	if opts.JSONFormat && !opts.ShowPatch {
		output.Warning(cmd.Root().ErrWriter, "--json has no effect without -p/--patch")
	}

	// Warn if --oneline is used with -p
	if opts.Oneline && opts.ShowPatch {
		output.Warning(cmd.Root().ErrWriter, "--oneline has no effect with -p/--patch")
	}

	client, err := awsutil.NewSSMClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
		r := &Runner{
			Client: client,
			Stdout: w,
			Stderr: cmd.Root().ErrWriter,
		}
		return r.Run(ctx, opts)
	})
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

	// Filter by version range if specified
	if opts.FromVersion != nil || opts.ToVersion != nil {
		params = filterVersionRange(params, opts.FromVersion, opts.ToVersion)
		if len(params) == 0 {
			return nil
		}
	}

	// AWS returns oldest first; reverse to show newest first (unless --reverse)
	if !opts.Reverse {
		slices.Reverse(params)
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Find the current (latest) version index
	currentIdx := 0
	if opts.Reverse {
		currentIdx = len(params) - 1
	}

	for i, param := range params {
		if opts.Oneline && !opts.ShowPatch {
			// Compact one-line format: VERSION  DATE  VALUE_PREVIEW
			dateStr := ""
			if param.LastModifiedDate != nil {
				dateStr = param.LastModifiedDate.Format("2006-01-02")
			}
			value := lo.FromPtr(param.Value)
			if len(value) > 40 {
				value = value[:40] + "..."
			}
			currentMark := ""
			if i == currentIdx {
				currentMark = green(" (current)")
			}
			_, _ = fmt.Fprintf(r.Stdout, "%s%d%s  %s  %s\n",
				yellow(""),
				param.Version,
				currentMark,
				cyan(dateStr),
				value,
			)
			continue
		}

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
				if opts.JSONFormat {
					oldValue, newValue = jsonutil.TryFormatOrWarn2(oldValue, newValue, r.Stderr, "")
				}
				oldName := fmt.Sprintf("%s#%d", opts.Name, params[oldIdx].Version)
				newName := fmt.Sprintf("%s#%d", opts.Name, params[newIdx].Version)
				diff := output.Diff(oldName, newName, oldValue, newValue)
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

// parseVersionSpec parses a version specifier like "#3" or "~1" and returns the resolved version.
// If the spec doesn't start with a specifier character, it's treated as a full spec.
func parseVersionSpec(name, spec string) (*int64, error) {
	// If spec starts with #, ~, prepend the name
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "#") || strings.HasPrefix(spec, "~") {
		spec = name + spec
	}

	parsed, err := ssmversion.Parse(spec)
	if err != nil {
		return nil, err
	}

	// For now, we only support absolute version or shift from latest
	// If there's a shift, we can't resolve it here (need history data)
	if parsed.HasShift() {
		return nil, fmt.Errorf("shift syntax (~) not supported in --from/--to; use absolute version (#N)")
	}

	if parsed.Absolute.Version == nil {
		return nil, fmt.Errorf("version specifier required (e.g., '#3')")
	}

	return parsed.Absolute.Version, nil
}

// filterVersionRange filters parameters to only include versions in the specified range.
// Parameters are expected in oldest-first order (as returned by AWS).
func filterVersionRange(params []types.ParameterHistory, from, to *int64) []types.ParameterHistory {
	var filtered []types.ParameterHistory
	for _, p := range params {
		if from != nil && p.Version < *from {
			continue
		}
		if to != nil && p.Version > *to {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}
