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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/smapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
)

// Client is the interface for the log command.
type Client interface {
	smapi.ListSecretVersionIdsAPI
	smapi.GetSecretValueAPI
}

// Command returns the log command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "log",
		Usage:     "Show secret version history",
		ArgsUsage: "<name>",
		Description: `Display the version history of a secret, showing each version's
UUID (truncated), staging labels, and creation date.

Output is sorted with the most recent version first.
Version UUIDs are truncated to 8 characters for readability.

Use -p/--patch to show the diff between consecutive versions (like git log -p).

Use -j/--json with -p to format JSON values before diffing (keys are always sorted).

EXAMPLES:
   suve sm log my-secret           Show last 10 versions (default)
   suve sm log -n 5 my-secret      Show last 5 versions
   suve sm log -p my-secret        Show versions with diffs
   suve sm log -p -j my-secret     Show diffs with JSON formatting`,
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
		},
		Action: action,
	}
}

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("secret name required")
	}

	name := c.Args().First()
	maxResults := int32(c.Int("number"))
	showPatch := c.Bool("patch")
	jsonFormat := c.Bool("json")

	// Warn if --json is used without -p
	if jsonFormat && !showPatch {
		output.Warning(c.App.ErrWriter, "--json has no effect without -p/--patch")
	}

	client, err := awsutil.NewSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, c.App.ErrWriter, name, maxResults, showPatch, jsonFormat)
}

// Run executes the log command.
// If showPatch is true, displays the diff between consecutive versions.
// If jsonFormat is true, formats JSON values before diffing.
func Run(ctx context.Context, client Client, w io.Writer, errW io.Writer, name string, maxResults int32, showPatch bool, jsonFormat bool) error {
	result, err := client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId:   aws.String(name),
		MaxResults: aws.Int32(maxResults),
	})
	if err != nil {
		return fmt.Errorf("failed to list secret versions: %w", err)
	}

	versions := result.Versions
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].CreatedDate == nil {
			return false
		}
		if versions[j].CreatedDate == nil {
			return true
		}
		return versions[i].CreatedDate.After(*versions[j].CreatedDate)
	})

	// If showing patches, fetch all secret values upfront
	var secretValues map[string]string
	if showPatch {
		secretValues = make(map[string]string)
		for _, v := range versions {
			versionID := aws.ToString(v.VersionId)
			secretResult, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
				SecretId:  aws.String(name),
				VersionId: aws.String(versionID),
			})
			if err != nil {
				// Skip versions that can't be retrieved
				continue
			}
			secretValues[versionID] = aws.ToString(secretResult.SecretString)
		}
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Track if we've warned about invalid JSON (only warn once)
	jsonWarned := false

	for i, v := range versions {
		versionID := aws.ToString(v.VersionId)
		truncatedID := versionID
		if len(truncatedID) > 8 {
			truncatedID = truncatedID[:8]
		}
		versionLabel := fmt.Sprintf("Version %s", truncatedID)
		if len(v.VersionStages) > 0 {
			versionLabel += " " + green(fmt.Sprintf("%v", v.VersionStages))
		}
		_, _ = fmt.Fprintln(w, yellow(versionLabel))
		if v.CreatedDate != nil {
			_, _ = fmt.Fprintf(w, "%s %s\n", cyan("Date:"), v.CreatedDate.Format(time.RFC3339))
		}

		if showPatch {
			// Show diff with previous version (next in array since we sorted newest first)
			if i < len(versions)-1 {
				nextVersionID := aws.ToString(versions[i+1].VersionId)
				oldValue, oldOk := secretValues[nextVersionID]
				newValue, newOk := secretValues[versionID]
				if oldOk && newOk {
					// Format as JSON if enabled
					if jsonFormat {
						if !jsonutil.IsJSON(oldValue) || !jsonutil.IsJSON(newValue) {
							if !jsonWarned {
								output.Warning(errW, "--json has no effect: some values are not valid JSON")
								jsonWarned = true
							}
						} else {
							oldValue = jsonutil.Format(oldValue)
							newValue = jsonutil.Format(newValue)
						}
					}

					oldTruncatedID := nextVersionID
					if len(oldTruncatedID) > 8 {
						oldTruncatedID = oldTruncatedID[:8]
					}
					oldName := fmt.Sprintf("%s#%s", name, oldTruncatedID)
					newName := fmt.Sprintf("%s#%s", name, truncatedID)
					diff := output.Diff(oldName, newName, oldValue, newValue)
					if diff != "" {
						_, _ = fmt.Fprintln(w)
						_, _ = fmt.Fprint(w, diff)
					}
				}
			}
		}

		if i < len(versions)-1 {
			_, _ = fmt.Fprintln(w)
		}
	}

	return nil
}
