// Package log provides the Secrets Manager log command for viewing secret version history.
//
// The log command displays version history with optional patch/diff output,
// similar to git log. Use -p/--patch to show differences between consecutive versions.
package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// Runner executes the log command.
type Runner struct {
	UseCase *secret.LogUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// Options holds the options for the log command.
type Options struct {
	Name       string
	MaxResults int32
	ShowPatch  bool
	ParseJSON  bool
	Reverse    bool
	NoPager    bool
	Oneline    bool
	Since      *time.Time
	Until      *time.Time
	Output     output.Format
}

// JSONOutputItem represents a single version entry in JSON output.
type JSONOutputItem struct {
	VersionID string   `json:"versionId"`
	Stages    []string `json:"stages,omitempty"`
	Created   string   `json:"created,omitempty"`
	Value     *string  `json:"value,omitempty"` // nil when error, pointer to distinguish from empty string
	Error     string   `json:"error,omitempty"`
}

// Command returns the log command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "log",
		Aliases:   []string{"history"},
		Usage:     "Show secret version history",
		ArgsUsage: "<name>",
		Description: `Display the version history of a secret, showing each version's
UUID (truncated), staging labels, and creation date.

Output is sorted with the most recent version first (use --reverse to flip).
Version UUIDs are truncated to 8 characters for readability.

Use --patch to show the diff between consecutive versions (like git log -p).
Use --parse-json with --patch to format JSON values before diffing (keys are always sorted).
Use --oneline for a compact one-line-per-version format.
Use --since/--until to filter by creation date (RFC3339 format).

OUTPUT FORMAT:
   Use --output=json for structured JSON output.

EXAMPLES:
   suve secret log my-secret                             Show last 10 versions
   suve secret log --patch my-secret                     Show versions with diffs
   suve secret log --patch --parse-json my-secret        Show diffs with JSON formatting
   suve secret log --oneline my-secret                   Compact one-line format
   suve secret log --number 5 my-secret                  Show last 5 versions
   suve secret log --since 2024-01-01T00:00:00Z my-secret  Show versions since date
   suve secret log --output=json my-secret               Output as JSON`,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "number",
				Aliases: []string{"n"},
				Value:   10,
				Usage:   "Number of versions to show",
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
				Usage: "Show versions created after this date (RFC3339 format, e.g., '2024-01-01T00:00:00Z')",
			},
			&cli.StringFlag{
				Name:  "until",
				Usage: "Show versions created before this date (RFC3339 format, e.g., '2024-12-31T23:59:59Z')",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		Action: action,
	}
}

func action(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: suve secret log <name>")
	}

	opts := Options{
		Name:       cmd.Args().First(),
		MaxResults: int32(cmd.Int("number")),
		ShowPatch:  cmd.Bool("patch"),
		ParseJSON:  cmd.Bool("parse-json"),
		Reverse:    cmd.Bool("reverse"),
		NoPager:    cmd.Bool("no-pager"),
		Oneline:    cmd.Bool("oneline"),
		Output:     output.ParseFormat(cmd.String("output")),
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

	client, err := infra.NewSecretClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	// JSON output disables pager
	noPager := opts.NoPager || opts.Output == output.FormatJSON

	return pager.WithPagerWriter(cmd.Root().Writer, noPager, func(w io.Writer) error {
		r := &Runner{
			UseCase: &secret.LogUseCase{Client: client},
			Stdout:  w,
			Stderr:  cmd.Root().ErrWriter,
		}
		return r.Run(ctx, opts)
	})
}

// Run executes the log command.
//
//nolint:gocognit // Log output has multiple format branches that are clearer inline than extracted
func (r *Runner) Run(ctx context.Context, opts Options) error {
	result, err := r.UseCase.Execute(ctx, secret.LogInput{
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
		items := make([]JSONOutputItem, 0, len(entries))
		for _, entry := range entries {
			item := JSONOutputItem{
				VersionID: entry.VersionID,
			}
			if len(entry.VersionStage) > 0 {
				item.Stages = entry.VersionStage
			}
			if entry.CreatedDate != nil {
				item.Created = timeutil.FormatRFC3339(*entry.CreatedDate)
			}
			if entry.Error != nil {
				item.Error = entry.Error.Error()
			} else {
				item.Value = &entry.Value
			}
			items = append(items, item)
		}
		enc := json.NewEncoder(r.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Build secret values map for patch mode
	secretValues := make(map[string]string)
	for _, entry := range entries {
		if entry.Error == nil {
			secretValues[entry.VersionID] = entry.Value
		}
	}

	for i, entry := range entries {
		versionID := entry.VersionID

		if opts.Oneline && !opts.ShowPatch {
			// Compact one-line format: VERSION_ID  DATE  [LABELS]
			dateStr := ""
			if entry.CreatedDate != nil {
				dateStr = entry.CreatedDate.Format("2006-01-02")
			}
			labelsStr := ""
			if len(entry.VersionStage) > 0 {
				labelsStr = colors.Current(fmt.Sprintf(" %v", entry.VersionStage))
			}
			output.Printf(r.Stdout, "%s%s  %s%s\n",
				colors.Version(secretversion.TruncateVersionID(versionID)),
				labelsStr,
				colors.FieldLabel(dateStr),
				"",
			)
			continue
		}

		versionLabel := fmt.Sprintf("Version %s", secretversion.TruncateVersionID(versionID))
		if len(entry.VersionStage) > 0 {
			versionLabel += " " + colors.Current(fmt.Sprintf("%v", entry.VersionStage))
		}
		output.Println(r.Stdout, colors.Version(versionLabel))
		if entry.CreatedDate != nil {
			output.Printf(r.Stdout, "%s %s\n", colors.FieldLabel("Date:"), timeutil.FormatRFC3339(*entry.CreatedDate))
		}

		if opts.ShowPatch {
			// Determine old/new indices based on order
			var oldIdx, newIdx int
			if opts.Reverse {
				// In reverse mode: comparing with next version (newer)
				if i < len(entries)-1 {
					oldIdx = i
					newIdx = i + 1
				} else {
					oldIdx = -1 // No diff for the last (current) version
				}
			} else {
				// In normal mode: comparing with previous version (older)
				if i < len(entries)-1 {
					oldIdx = i + 1
					newIdx = i
				} else {
					oldIdx = -1 // No diff for the oldest version
				}
			}

			if oldIdx >= 0 {
				oldVersionID := entries[oldIdx].VersionID
				newVersionID := entries[newIdx].VersionID
				oldValue, oldOk := secretValues[oldVersionID]
				newValue, newOk := secretValues[newVersionID]
				if oldOk && newOk {
					if opts.ParseJSON {
						oldValue, newValue = jsonutil.TryFormatOrWarn2(oldValue, newValue, r.Stderr, "")
					}
					oldName := fmt.Sprintf("%s#%s", opts.Name, secretversion.TruncateVersionID(oldVersionID))
					newName := fmt.Sprintf("%s#%s", opts.Name, secretversion.TruncateVersionID(newVersionID))
					diff := output.Diff(oldName, newName, oldValue, newValue)
					if diff != "" {
						output.Println(r.Stdout, "")
						output.Print(r.Stdout, diff)
					}
				}
			}
		}

		if i < len(entries)-1 {
			output.Println(r.Stdout, "")
		}
	}

	return nil
}
