// Package log provides the SSM Parameter Store log command for viewing parameter version history.
//
// The log command displays version history with optional patch/diff output,
// similar to git log. Use -p/--patch to show differences between consecutive versions.
package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/jsonutil"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/param"
)

// Runner executes the log command.
type Runner struct {
	UseCase *param.LogUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the log command.
type Options struct {
	Name           string
	MaxResults     int32
	ShowPatch      bool
	ParseJSON      bool
	Reverse        bool
	NoPager        bool
	Oneline        bool
	Since          *time.Time
	Until          *time.Time
	Output         output.Format
	MaxValueLength int
}

// JSONOutputItem represents a single version entry in JSON output.
type JSONOutputItem struct {
	Version  int64  `json:"version"`
	Type     string `json:"type"`
	Modified string `json:"modified,omitempty"`
	Value    string `json:"value"`
}

// Command returns the log command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "log",
		Aliases:   []string{"history"},
		Usage:     "Show parameter version history",
		ArgsUsage: "<name>",
		Description: `Display the version history of a parameter, showing each version's
number, modification date, and a preview of the value.

Output is sorted with the most recent version first (use --reverse to flip).
Value preview truncation depends on the mode:
  - Normal mode: Full value is shown (no truncation)
  - Oneline mode: Truncated to fit terminal width (default 50 if width unavailable)

Use --max-value-length to override the automatic truncation.
Use --patch to show the diff between consecutive versions (like git log -p).
Use --parse-json with --patch to format JSON values before diffing (keys are always sorted).
Use --oneline for a compact one-line-per-version format.
Use --since/--until to filter by modification date (RFC3339 format).

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
   suve param log /app/config                             Show last 10 versions
   suve param log --patch /app/config                     Show versions with diffs
   suve param log --patch --parse-json /app/config        Show diffs with JSON formatting
   suve param log --oneline /app/config                   Compact one-line format
   suve param log --oneline --max-value-length 80 /app/config  Custom truncation length
   suve param log --number 5 /app/config                  Show last 5 versions
   suve param log --since 2024-01-01T00:00:00Z /app/config  Show versions since date
   suve param log --output=json /app/config               Output as JSON`,
		Flags: []cli.Flag{
			&cli.Int32Flag{
				Name:    "number",
				Aliases: []string{"n"},
				Value:   10, //nolint:mnd // default number of versions to display
				Usage:   "Maximum number of versions to show",
			},
			&cli.BoolFlag{
				Name:    "patch",
				Aliases: []string{"p"},
				Value:   false,
				Usage:   "Show diff between consecutive versions",
			},
			&cli.BoolFlag{
				Name:    "parse-json",
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
				Name:  "since",
				Usage: "Show versions modified after this date (RFC3339 format, e.g., '2024-01-01T00:00:00Z')",
			},
			&cli.StringFlag{
				Name:  "until",
				Usage: "Show versions modified before this date (RFC3339 format, e.g., '2024-12-31T23:59:59Z')",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
			&cli.Int32Flag{
				Name:  "max-value-length",
				Value: 0,
				Usage: "Maximum value preview length (0 = auto: unlimited for normal, terminal width for oneline)",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve param log <name>")
	}

	name := cmd.Args().First()

	opts := Options{
		Name:           name,
		MaxResults:     cmd.Int32("number"),
		ShowPatch:      cmd.Bool("patch"),
		ParseJSON:      cmd.Bool("parse-json"),
		Reverse:        cmd.Bool("reverse"),
		NoPager:        cmd.Bool("no-pager"),
		Oneline:        cmd.Bool("oneline"),
		Output:         output.ParseFormat(cmd.String("output")),
		MaxValueLength: cmd.Int("max-value-length"),
	}

	// Parse --since timestamp
	if sinceArg := cmd.String("since"); sinceArg != "" {
		since, err := time.Parse(time.RFC3339, sinceArg)
		if err != nil {
			return fmt.Errorf("invalid --since value: must be RFC3339 format (e.g., '2024-01-01T00:00:00Z')")
		}

		opts.Since = &since
	}

	// Parse --until timestamp
	if untilArg := cmd.String("until"); untilArg != "" {
		until, err := time.Parse(time.RFC3339, untilArg)
		if err != nil {
			return fmt.Errorf("invalid --until value: must be RFC3339 format (e.g., '2024-12-31T23:59:59Z')")
		}

		opts.Until = &until
	}

	// Warn if --parse-json is used without -p
	if opts.ParseJSON && !opts.ShowPatch {
		output.Warning(cmd.Root().ErrWriter, "--parse-json has no effect without -p/--patch")
	}

	// Warn if --oneline is used with -p
	if opts.Oneline && opts.ShowPatch {
		output.Warning(cmd.Root().ErrWriter, "--oneline has no effect with -p/--patch")
	}

	// Warn if --output=json is used with incompatible options
	if opts.Output == output.FormatJSON {
		if opts.ShowPatch {
			output.Warning(cmd.Root().ErrWriter, "-p/--patch has no effect with --output=json")
		}

		if opts.Oneline {
			output.Warning(cmd.Root().ErrWriter, "--oneline has no effect with --output=json")
		}
	}

	adapter, err := awsparam.NewAdapter(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	// JSON output disables pager
	noPager := opts.NoPager || opts.Output == output.FormatJSON

	return pager.WithPagerWriter(cmd.Root().Writer, noPager, func(w io.Writer) error {
		r := &Runner{
			UseCase: &param.LogUseCase{Client: adapter},
			Stdout:  w,
			Stderr:  cmd.Root().ErrWriter,
		}

		return r.Run(ctx, opts)
	})
}

// Run executes the log command.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, param.LogInput{
		Name:       opts.Name,
		MaxResults: opts.MaxResults,
		Since:      opts.Since,
		Until:      opts.Until,
		Reverse:    opts.Reverse,
	})
	if err != nil {
		return err
	}

	entries := result.Entries
	if len(entries) == 0 {
		return nil
	}

	// JSON output mode
	if opts.Output == output.FormatJSON {
		items := make([]JSONOutputItem, len(entries))
		for i, entry := range entries {
			version, _ := strconv.ParseInt(entry.Version, 10, 64)

			items[i] = JSONOutputItem{
				Version: version,
				Type:    entry.Type,
				Value:   entry.Value,
			}
			if entry.UpdatedAt != nil {
				items[i].Modified = timeutil.FormatRFC3339(*entry.UpdatedAt)
			}
		}

		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")

		return enc.Encode(items)
	}

	for i, entry := range entries {
		if opts.Oneline && !opts.ShowPatch {
			// Compact one-line format: VERSION  DATE  VALUE_PREVIEW
			dateStr := ""
			if entry.UpdatedAt != nil {
				dateStr = entry.UpdatedAt.Format("2006-01-02")
			}

			value := entry.Value
			// Determine max length for oneline mode
			maxLen := opts.MaxValueLength
			if maxLen == 0 {
				// Auto: use terminal width minus overhead for metadata
				// Reserve ~30 chars for: version (6) + current mark (10) + date (10) + separators (4)
				termWidth := terminal.GetWidthFromWriter(r.Stdout)

				const metadataOverhead = 30 // version + current mark + date + separators

				const minValueLength = 10

				maxLen = max(termWidth-metadataOverhead, minValueLength)
			}

			if maxLen > 0 && len(value) > maxLen {
				value = value[:maxLen] + "..."
			}

			currentMark := ""
			if entry.IsCurrent {
				currentMark = colors.Current(" (current)")
			}

			output.Printf(r.Stdout, "%s%s%s  %s  %s\n",
				colors.Version(""),
				entry.Version,
				currentMark,
				colors.FieldLabel(dateStr),
				value,
			)

			continue
		}

		versionLabel := fmt.Sprintf("Version %s", entry.Version)
		if entry.IsCurrent {
			versionLabel += " " + colors.Current("(current)")
		}

		output.Println(r.Stdout, colors.Version(versionLabel))

		if entry.UpdatedAt != nil {
			output.Printf(r.Stdout, "%s %s\n", colors.FieldLabel("Date:"), timeutil.FormatRFC3339(*entry.UpdatedAt))
		}

		if opts.ShowPatch {
			// For patch mode, we need to compare with the next entry in the list
			// The list is ordered by the usecase based on opts.Reverse
			if i < len(entries)-1 {
				var oldEntry, newEntry param.LogEntry
				if opts.Reverse {
					// In reverse mode (oldest first): current is old, next is new
					oldEntry = entry
					newEntry = entries[i+1]
				} else {
					// In normal mode (newest first): next is old, current is new
					oldEntry = entries[i+1]
					newEntry = entry
				}

				oldValue := oldEntry.Value

				newValue := newEntry.Value
				if opts.ParseJSON {
					oldValue, newValue = jsonutil.TryFormatOrWarn2(oldValue, newValue, r.Stderr, "")
				}

				oldName := fmt.Sprintf("%s#%s", result.Name, oldEntry.Version)
				newName := fmt.Sprintf("%s#%s", result.Name, newEntry.Version)

				diff := output.Diff(oldName, newName, oldValue, newValue)
				if diff != "" {
					output.Println(r.Stdout, "")
					output.Print(r.Stdout, diff)
				}
			}
		} else {
			// Show value preview (unlimited unless --max-value-length specified)
			value := entry.Value
			if opts.MaxValueLength > 0 && len(value) > opts.MaxValueLength {
				value = value[:opts.MaxValueLength] + "..."
			}

			output.Printf(r.Stdout, "%s\n", value)
		}

		if i < len(entries)-1 {
			output.Println(r.Stdout, "")
		}
	}

	return nil
}
