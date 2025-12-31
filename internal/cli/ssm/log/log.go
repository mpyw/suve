// Package log provides the SSM log command for viewing parameter version history.
//
// The log command displays version history with optional patch/diff output,
// similar to git log. Use -p/--patch to show differences between consecutive versions.
package log

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/mpyw/suve/internal/api/ssmapi"
	"github.com/mpyw/suve/internal/awsutil"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/output"
)

// Client is the interface for the log command.
type Client interface {
	ssmapi.GetParameterHistoryAPI
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

func action(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("parameter name required")
	}

	name := c.Args().First()
	maxResults := int32(c.Int("number"))
	showPatch := c.Bool("patch")
	jsonFormat := c.Bool("json")
	reverse := c.Bool("reverse")

	// Warn if --json is used without -p
	if jsonFormat && !showPatch {
		output.Warning(c.App.ErrWriter, "--json has no effect without -p/--patch")
	}

	client, err := awsutil.NewSSMClient(c.Context)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	return Run(c.Context, client, c.App.Writer, c.App.ErrWriter, name, maxResults, showPatch, jsonFormat, reverse)
}

// Run executes the log command.
// If showPatch is true, displays the diff between consecutive versions.
// If jsonFormat is true, formats JSON values before diffing.
func Run(ctx context.Context, client Client, w io.Writer, errW io.Writer, name string, maxResults int32, showPatch bool, jsonFormat bool, reverse bool) error {
	result, err := client.GetParameterHistory(ctx, &ssm.GetParameterHistoryInput{
		Name:           aws.String(name),
		MaxResults:     aws.Int32(maxResults),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("failed to get parameter history: %w", err)
	}

	params := result.Parameters
	if len(params) == 0 {
		return nil
	}

	// AWS returns oldest first; reverse to show newest first (unless --reverse)
	if !reverse {
		for i, j := 0, len(params)-1; i < j; i, j = i+1, j-1 {
			params[i], params[j] = params[j], params[i]
		}
	}

	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Track if we've warned about invalid JSON (only warn once)
	jsonWarned := false

	// Find the current (latest) version index
	currentIdx := 0
	if reverse {
		currentIdx = len(params) - 1
	}

	for i, param := range params {
		versionLabel := fmt.Sprintf("Version %d", param.Version)
		if i == currentIdx {
			versionLabel += " " + green("(current)")
		}
		_, _ = fmt.Fprintln(w, yellow(versionLabel))
		if param.LastModifiedDate != nil {
			_, _ = fmt.Fprintf(w, "%s %s\n", cyan("Date:"), param.LastModifiedDate.Format(time.RFC3339))
		}

		if showPatch {
			// Determine old/new indices based on order
			var oldIdx, newIdx int
			if reverse {
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
				oldValue := aws.ToString(params[oldIdx].Value)
				newValue := aws.ToString(params[newIdx].Value)

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

				oldName := fmt.Sprintf("%s#%d", name, params[oldIdx].Version)
				newName := fmt.Sprintf("%s#%d", name, params[newIdx].Version)
				diff := output.Diff(oldName, newName, oldValue, newValue)
				if diff != "" {
					_, _ = fmt.Fprintln(w)
					_, _ = fmt.Fprint(w, diff)
				}
			}
		} else {
			// Show truncated value preview
			value := aws.ToString(param.Value)
			if len(value) > 50 {
				value = value[:50] + "..."
			}
			_, _ = fmt.Fprintf(w, "%s\n", value)
		}

		if i < len(params)-1 {
			_, _ = fmt.Fprintln(w)
		}
	}

	return nil
}
