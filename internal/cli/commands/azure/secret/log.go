package secret

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/azure"
)

// logJSONItem represents a single version entry in JSON output.
type logJSONItem struct {
	Version string            `json:"version"`
	State   string            `json:"state,omitempty"`
	Created string            `json:"created,omitempty"`
	Value   *string           `json:"value,omitempty"`
	Tags    map[string]string `json:"tags,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// logPresenter renders Azure Key Vault log output.
type logPresenter struct {
	uc     *azure.LogUseCase
	req    genericlog.Request
	result *azure.LogOutput
	values map[string]string
}

// NewLogPresenter builds an Azure Key Vault log presenter over the given reader and request.
func NewLogPresenter(reader provider.Reader, req genericlog.Request) genericlog.Presenter {
	return &logPresenter{uc: &azure.LogUseCase{Reader: reader}, req: req}
}

func (p *logPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, azure.LogInput{
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
	p.values = make(map[string]string)

	for _, entry := range result.Entries {
		if entry.Error == nil {
			p.values[entry.Version] = entry.Value
		}
	}

	return nil
}

func (p *logPresenter) Len() int { return len(p.result.Entries) }

func (p *logPresenter) RenderJSON(stdout io.Writer) error {
	items := lo.Map(p.result.Entries, func(entry azure.LogEntry, _ int) logJSONItem {
		item := logJSONItem{Version: entry.Version, State: entry.State}

		if entry.CreatedDate != nil {
			item.Created = timeutil.FormatRFC3339(*entry.CreatedDate)
		}

		if entry.Error != nil {
			item.Error = entry.Error.Error()
		} else {
			item.Value = &entry.Value
		}

		if len(entry.Tags) > 0 {
			item.Tags = make(map[string]string, len(entry.Tags))
			for _, tag := range entry.Tags {
				item.Tags[tag.Key] = tag.Value
			}
		}

		return item
	})

	return output.WriteJSON(stdout, items)
}

func (p *logPresenter) RenderOneline(stdout io.Writer, i, _ int) {
	entry := p.result.Entries[i]

	dateStr := ""
	if entry.CreatedDate != nil {
		dateStr = timeutil.FormatDate(*entry.CreatedDate)
	}

	stateStr := ""
	if entry.State != "" {
		stateStr = colors.For(stdout).Current(fmt.Sprintf(" [%s]", entry.State))
	}

	output.Printf(stdout, "%s%s  %s\n",
		colors.For(stdout).Version(entry.Version),
		stateStr,
		colors.For(stdout).FieldLabel(dateStr),
	)
}

func (p *logPresenter) RenderHeader(stdout io.Writer, i int) {
	entry := p.result.Entries[i]

	versionLabel := fmt.Sprintf("Version %s", entry.Version)
	if entry.State != "" {
		versionLabel += " " + colors.For(stdout).Current(fmt.Sprintf("[%s]", entry.State))
	}

	output.Println(stdout, colors.For(stdout).Version(versionLabel))

	if entry.CreatedDate != nil {
		output.Printf(stdout, "%s %s\n", colors.For(stdout).FieldLabel("Date:"), timeutil.FormatRFC3339(*entry.CreatedDate))
	}

	// Key Vault tags are per version, so show this version's own tags.
	if len(entry.Tags) > 0 {
		pairs := lo.Map(entry.Tags, func(tag domain.Tag, _ int) string {
			return fmt.Sprintf("%s=%s", tag.Key, tag.Value)
		})

		output.Printf(stdout, "%s %s\n", colors.For(stdout).FieldLabel("Tags:"), strings.Join(pairs, ", "))
	}
}

// RenderValue is a no-op: like the AWS/GoogleCloud secret log, Azure Key Vault log does
// not show a default value preview.
func (p *logPresenter) RenderValue(_ io.Writer, _, _ int) {}

func (p *logPresenter) RenderPatch(stdout, stderr io.Writer, i int, parseJSON, reverse bool) {
	entries := p.result.Entries
	parentIdx, oldest := genericlog.PatchParent(i, len(entries), reverse)

	newEntry := entries[i]

	newValue, newOk := p.values[newEntry.Version]
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

		oldValue, oldOk = p.values[oldEntry.Version]
		if !oldOk {
			return
		}

		oldName = fmt.Sprintf("%s#%s", p.result.Name, oldEntry.Version)

		if parseJSON {
			oldValue, newValue = jsonutil.TryFormatOrWarn2(oldValue, newValue, stderr, "")
		}
	}

	newName := fmt.Sprintf("%s#%s", p.result.Name, newEntry.Version)

	diff := output.Diff(stdout, oldName, newName, oldValue, newValue)
	if diff != "" {
		output.Println(stdout, "")
		output.Print(stdout, diff)
	}
}

// LogCommand returns the Azure Key Vault log command.
func LogCommand() *cli.Command {
	return genericlog.Command(genericlog.Config{
		Usage:     "Show secret version history",
		ArgsUsage: argsUsageName,
		Description: `Display the version history of a secret, showing each version's
opaque id, state (enabled/disabled), and creation date.

Output is sorted with the most recent version first (use --reverse to flip).

Use --patch to show the diff between consecutive versions (like git log -p).
Note: disabled versions have no accessible value, so their diffs are skipped.

EXAMPLES:
   suve azure secret log my-secret                        Show last 10 versions
   suve azure secret log --patch my-secret                Show versions with diffs
   suve azure secret log --oneline my-secret              Compact one-line format
   suve azure secret log --output=json my-secret          Output as JSON`,
		UsageError: "usage: suve azure secret log <name>",
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
				Usage: "Show versions created after this date (RFC3339 format)",
			},
			&cli.StringFlag{
				Name:  "until",
				Usage: "Show versions created before this date (RFC3339 format)",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		NewPresenter: func(ctx context.Context, req genericlog.Request) (genericlog.Presenter, error) {
			store, err := cliinternal.AzureKeyVaultStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewLogPresenter(store, req), nil
		},
	})
}
