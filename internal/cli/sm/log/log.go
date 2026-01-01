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
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/output"
	"github.com/mpyw/suve/internal/smutil"
)

// Client is the interface for the log command.
type Client interface {
	smapi.ListSecretVersionIdsAPI
	smapi.GetSecretValueAPI
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
		Usage:     "Show secret version history",
		ArgsUsage: "<name>",
		Description: `Display the version history of a secret, showing each version's
UUID (truncated), staging labels, and creation date.

Output is sorted with the most recent version first (use --reverse to flip).
Version UUIDs are truncated to 8 characters for readability.

Use -p/--patch to show the diff between consecutive versions (like git log -p).

Use -j/--json with -p to format JSON values before diffing (keys are always sorted).

EXAMPLES:
   suve sm log my-secret           Show last 10 versions (default)
   suve sm log -n 5 my-secret      Show last 5 versions
   suve sm log -p my-secret        Show versions with diffs
   suve sm log -p -j my-secret     Show diffs with JSON formatting
   suve sm log --reverse my-secret Show oldest first`,
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
		return fmt.Errorf("usage: suve sm log <name>")
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

	client, err := awsutil.NewSMClient(ctx)
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

	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Track if we've warned about invalid JSON (only warn once)
	jsonWarned := false

	for i, v := range versions {
		versionID := lo.FromPtr(v.VersionId)
		versionLabel := fmt.Sprintf("Version %s", smutil.TruncateVersionID(versionID))
		if len(v.VersionStages) > 0 {
			versionLabel += " " + green(fmt.Sprintf("%v", v.VersionStages))
		}
		_, _ = fmt.Fprintln(r.Stdout, yellow(versionLabel))
		if v.CreatedDate != nil {
			_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", cyan("Date:"), v.CreatedDate.Format(time.RFC3339))
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
					oldName := fmt.Sprintf("%s#%s", opts.Name, smutil.TruncateVersionID(oldVersionID))
					newName := fmt.Sprintf("%s#%s", opts.Name, smutil.TruncateVersionID(newVersionID))
					diff := output.DiffWithJSON(oldName, newName, oldValue, newValue, opts.JSONFormat, &jsonWarned, r.Stderr)
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
