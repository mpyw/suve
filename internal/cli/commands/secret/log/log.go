// Package log provides the SM log command for viewing secret version history.
//
// The log command displays version history with optional patch/diff output,
// similar to git log. Use -p/--patch to show differences between consecutive versions.
package log

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/pager"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// Client is the interface for the log command.
type Client interface {
	secretapi.ListSecretVersionIdsAPI
	secretapi.GetSecretValueAPI
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
	NoPager    bool
	Oneline    bool
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

Use -p/--patch to show the diff between consecutive versions (like git log -p).
Use -j/--json with -p to format JSON values before diffing (keys are always sorted).
Use --oneline for a compact one-line-per-version format.

EXAMPLES:
   suve secret log my-secret           Show last 10 versions (default)
   suve secret log -n 5 my-secret      Show last 5 versions
   suve secret log -p my-secret        Show versions with diffs
   suve secret log -p -j my-secret     Show diffs with JSON formatting
   suve secret log --oneline my-secret Compact one-line format
   suve secret log --reverse my-secret Show oldest first`,
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
		JSONFormat: cmd.Bool("json"),
		Reverse:    cmd.Bool("reverse"),
		NoPager:    cmd.Bool("no-pager"),
		Oneline:    cmd.Bool("oneline"),
	}

	// Warn if --json is used without -p
	if opts.JSONFormat && !opts.ShowPatch {
		output.Warning(cmd.Root().ErrWriter, "--json has no effect without -p/--patch")
	}

	// Warn if --oneline is used with -p
	if opts.Oneline && opts.ShowPatch {
		output.Warning(cmd.Root().ErrWriter, "--oneline has no effect with -p/--patch")
	}

	client, err := infra.NewSecretClient(ctx)
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
	result, err := r.Client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId:   lo.ToPtr(opts.Name),
		MaxResults: lo.ToPtr(opts.MaxResults),
	})
	if err != nil {
		return fmt.Errorf("failed to list secret versions: %w", err)
	}

	versions := result.Versions
	if len(versions) == 0 {
		return nil
	}

	sort.Slice(versions, func(i, j int) bool {
		if versions[i].CreatedDate == nil {
			return false
		}
		if versions[j].CreatedDate == nil {
			return true
		}
		if opts.Reverse {
			// Oldest first
			return versions[i].CreatedDate.Before(*versions[j].CreatedDate)
		}
		// Newest first (default)
		return versions[i].CreatedDate.After(*versions[j].CreatedDate)
	})

	// If showing patches, fetch all secret values upfront
	var secretValues map[string]string
	if opts.ShowPatch {
		secretValues = make(map[string]string)
		for _, v := range versions {
			versionID := lo.FromPtr(v.VersionId)
			secretResult, err := r.Client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
				SecretId:  lo.ToPtr(opts.Name),
				VersionId: lo.ToPtr(versionID),
			})
			if err != nil {
				// Skip versions that can't be retrieved
				continue
			}
			secretValues[versionID] = lo.FromPtr(secretResult.SecretString)
		}
	}

	for i, v := range versions {
		versionID := lo.FromPtr(v.VersionId)

		if opts.Oneline && !opts.ShowPatch {
			// Compact one-line format: VERSION_ID  DATE  [LABELS]
			dateStr := ""
			if v.CreatedDate != nil {
				dateStr = v.CreatedDate.Format("2006-01-02")
			}
			labelsStr := ""
			if len(v.VersionStages) > 0 {
				labelsStr = colors.Current(fmt.Sprintf(" %v", v.VersionStages))
			}
			_, _ = fmt.Fprintf(r.Stdout, "%s%s  %s%s\n",
				colors.Version(secretversion.TruncateVersionID(versionID)),
				labelsStr,
				colors.FieldLabel(dateStr),
				"",
			)
			continue
		}

		versionLabel := fmt.Sprintf("Version %s", secretversion.TruncateVersionID(versionID))
		if len(v.VersionStages) > 0 {
			versionLabel += " " + colors.Current(fmt.Sprintf("%v", v.VersionStages))
		}
		_, _ = fmt.Fprintln(r.Stdout, colors.Version(versionLabel))
		if v.CreatedDate != nil {
			_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", colors.FieldLabel("Date:"), v.CreatedDate.Format(time.RFC3339))
		}

		if opts.ShowPatch {
			// Determine old/new indices based on order
			var oldIdx, newIdx int
			if opts.Reverse {
				// In reverse mode: comparing with next version (newer)
				if i < len(versions)-1 {
					oldIdx = i
					newIdx = i + 1
				} else {
					oldIdx = -1 // No diff for the last (current) version
				}
			} else {
				// In normal mode: comparing with previous version (older)
				if i < len(versions)-1 {
					oldIdx = i + 1
					newIdx = i
				} else {
					oldIdx = -1 // No diff for the oldest version
				}
			}

			if oldIdx >= 0 {
				oldVersionID := lo.FromPtr(versions[oldIdx].VersionId)
				newVersionID := lo.FromPtr(versions[newIdx].VersionId)
				oldValue, oldOk := secretValues[oldVersionID]
				newValue, newOk := secretValues[newVersionID]
				if oldOk && newOk {
					if opts.JSONFormat {
						oldValue, newValue = jsonutil.TryFormatOrWarn2(oldValue, newValue, r.Stderr, "")
					}
					oldName := fmt.Sprintf("%s#%s", opts.Name, secretversion.TruncateVersionID(oldVersionID))
					newName := fmt.Sprintf("%s#%s", opts.Name, secretversion.TruncateVersionID(newVersionID))
					diff := output.Diff(oldName, newName, oldValue, newValue)
					if diff != "" {
						_, _ = fmt.Fprintln(r.Stdout)
						_, _ = fmt.Fprint(r.Stdout, diff)
					}
				}
			}
		}

		if i < len(versions)-1 {
			_, _ = fmt.Fprintln(r.Stdout)
		}
	}

	return nil
}
