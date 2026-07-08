package secret

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// logJSONItem represents a single version entry in JSON output.
type logJSONItem struct {
	VersionID string   `json:"versionId"`
	Stages    []string `json:"stages,omitempty"`
	Created   string   `json:"created,omitempty"`
	Value     *string  `json:"value,omitempty"` // nil when error, pointer to distinguish from empty string
	Error     string   `json:"error,omitempty"`
}

// logPresenter renders Secrets Manager log output byte-for-byte as before.
type logPresenter struct {
	uc           *secret.LogUseCase
	req          genericlog.Request
	result       *secret.LogOutput
	secretValues map[string]string
}

// NewLogPresenter builds a secret log presenter over the given reader and request.
// It is exported for the shared golden-output test harness.
func NewLogPresenter(reader provider.Reader, req genericlog.Request) genericlog.Presenter {
	return &logPresenter{uc: &secret.LogUseCase{Reader: reader}, req: req}
}

func (p *logPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, secret.LogInput{
		Name:       p.req.Name,
		MaxResults: p.req.MaxResults,
		Since:      p.req.Since,
		Until:      p.req.Until,
		Reverse:    p.req.Reverse,
	})
	if err != nil {
		return err
	}

	p.result = result

	// Build secret values map for patch mode
	p.secretValues = make(map[string]string)

	for _, entry := range result.Entries {
		if entry.Error == nil {
			p.secretValues[entry.VersionID] = entry.Value
		}
	}

	return nil
}

func (p *logPresenter) Len() int { return len(p.result.Entries) }

func (p *logPresenter) RenderJSON(stdout io.Writer) error {
	entries := p.result.Entries
	items := make([]logJSONItem, 0, len(entries))

	for _, entry := range entries {
		item := logJSONItem{
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

	return output.WriteJSON(stdout, items)
}

func (p *logPresenter) RenderOneline(stdout io.Writer, i, _ int) {
	entry := p.result.Entries[i]

	// Compact one-line format: VERSION_ID  DATE  [LABELS]
	dateStr := ""
	if entry.CreatedDate != nil {
		dateStr = entry.CreatedDate.Format("2006-01-02")
	}

	labelsStr := ""
	if len(entry.VersionStage) > 0 {
		labelsStr = colors.For(stdout).Current(fmt.Sprintf(" %v", entry.VersionStage))
	}

	output.Printf(stdout, "%s%s  %s%s\n",
		colors.For(stdout).Version(secretversion.TruncateVersionID(entry.VersionID)),
		labelsStr,
		colors.For(stdout).FieldLabel(dateStr),
		"",
	)
}

func (p *logPresenter) RenderHeader(stdout io.Writer, i int) {
	entry := p.result.Entries[i]

	versionLabel := fmt.Sprintf("Version %s", secretversion.TruncateVersionID(entry.VersionID))
	if len(entry.VersionStage) > 0 {
		versionLabel += " " + colors.For(stdout).Current(fmt.Sprintf("%v", entry.VersionStage))
	}

	output.Println(stdout, colors.For(stdout).Version(versionLabel))

	if entry.CreatedDate != nil {
		output.Printf(stdout, "%s %s\n", colors.For(stdout).FieldLabel("Date:"), timeutil.FormatRFC3339(*entry.CreatedDate))
	}
}

// RenderValue is a no-op: Secrets Manager log does not show a default value
// preview (matching the pre-refactor behavior).
func (p *logPresenter) RenderValue(_ io.Writer, _, _ int) {}

func (p *logPresenter) RenderPatch(stdout, stderr io.Writer, i int, parseJSON, reverse bool) {
	entries := p.result.Entries
	parentIdx, oldest := genericlog.PatchParent(i, len(entries), reverse)

	newEntry := entries[i]

	newValue, newOk := p.secretValues[newEntry.VersionID]
	if !newOk {
		return
	}

	var oldValue, oldName string

	if oldest {
		// The oldest version in the window has no parent to diff against. Render
		// its creation (all-added) diff, but only when it is genuinely the
		// initial version — otherwise a --number/date-filter window cut would
		// masquerade as a creation.
		if !p.result.InitialIncluded {
			return
		}

		oldName = p.result.Name

		if parseJSON {
			newValue = jsonutil.TryFormatOrWarn(newValue, stderr, "")
		}
	} else {
		oldEntry := entries[parentIdx]

		var oldOk bool

		oldValue, oldOk = p.secretValues[oldEntry.VersionID]
		if !oldOk {
			return
		}

		oldName = fmt.Sprintf("%s#%s", p.result.Name, secretversion.TruncateVersionID(oldEntry.VersionID))

		if parseJSON {
			oldValue, newValue = jsonutil.TryFormatOrWarn2(oldValue, newValue, stderr, "")
		}
	}

	newName := fmt.Sprintf("%s#%s", p.result.Name, secretversion.TruncateVersionID(newEntry.VersionID))

	diff := output.Diff(stdout, oldName, newName, oldValue, newValue)
	if diff != "" {
		output.Println(stdout, "")
		output.Print(stdout, diff)
	}
}

// LogCommand returns the Secrets Manager log command.
func LogCommand() *cli.Command {
	return genericlog.Command(genericlog.Config{
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
		UsageError: "usage: suve secret log <name>",
		Flags: []cli.Flag{
			&cli.Int32Flag{
				Name:    "number",
				Aliases: []string{"n"},
				Value:   10, //nolint:mnd // default number of versions to display
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
		NewPresenter: func(ctx context.Context, req genericlog.Request) (genericlog.Presenter, error) {
			store, err := cliinternal.SecretStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewLogPresenter(store, req), nil
		},
	})
}
