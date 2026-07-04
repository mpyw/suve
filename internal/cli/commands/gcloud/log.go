package gcloud

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
	"github.com/mpyw/suve/internal/usecase/gcp"
)

// logJSONItem represents a single version entry in JSON output.
type logJSONItem struct {
	Version string  `json:"version"`
	State   string  `json:"state,omitempty"`
	Created string  `json:"created,omitempty"`
	Value   *string `json:"value,omitempty"`
	Error   string  `json:"error,omitempty"`
}

// logPresenter renders Google Cloud Secret Manager log output.
type logPresenter struct {
	uc     *gcp.LogUseCase
	req    genericlog.Request
	result *gcp.LogOutput
	values map[string]string
}

// NewLogPresenter builds a Google Cloud log presenter over the given reader and request.
func NewLogPresenter(reader provider.Reader, req genericlog.Request) genericlog.Presenter {
	return &logPresenter{uc: &gcp.LogUseCase{Reader: reader}, req: req}
}

func (p *logPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, gcp.LogInput{
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
	entries := p.result.Entries
	items := make([]logJSONItem, 0, len(entries))

	for _, entry := range entries {
		item := logJSONItem{Version: entry.Version, State: entry.State}

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

	dateStr := ""
	if entry.CreatedDate != nil {
		dateStr = entry.CreatedDate.Format("2006-01-02")
	}

	stateStr := ""
	if entry.State != "" {
		stateStr = colors.Current(fmt.Sprintf(" [%s]", entry.State))
	}

	output.Printf(stdout, "%s%s  %s\n",
		colors.Version(entry.Version),
		stateStr,
		colors.FieldLabel(dateStr),
	)
}

func (p *logPresenter) RenderHeader(stdout io.Writer, i int) {
	entry := p.result.Entries[i]

	versionLabel := fmt.Sprintf("Version %s", entry.Version)
	if entry.State != "" {
		versionLabel += " " + colors.Current(fmt.Sprintf("[%s]", entry.State))
	}

	output.Println(stdout, colors.Version(versionLabel))

	if entry.CreatedDate != nil {
		output.Printf(stdout, "%s %s\n", colors.FieldLabel("Date:"), timeutil.FormatRFC3339(*entry.CreatedDate))
	}
}

// RenderValue is a no-op: like the AWS secret log, Google Cloud log does not
// show a default value preview.
func (p *logPresenter) RenderValue(_ io.Writer, _, _ int) {}

func (p *logPresenter) RenderPatch(stdout, stderr io.Writer, i int, parseJSON, reverse bool) {
	entries := p.result.Entries

	var oldIdx, newIdx int

	if reverse {
		if i < len(entries)-1 {
			oldIdx, newIdx = i, i+1
		} else {
			oldIdx = -1
		}
	} else {
		if i < len(entries)-1 {
			oldIdx, newIdx = i+1, i
		} else {
			oldIdx = -1
		}
	}

	if oldIdx < 0 {
		return
	}

	oldVersion := entries[oldIdx].Version
	newVersion := entries[newIdx].Version

	oldValue, oldOk := p.values[oldVersion]

	newValue, newOk := p.values[newVersion]
	if !oldOk || !newOk {
		return
	}

	if parseJSON {
		oldValue, newValue = jsonutil.TryFormatOrWarn2(oldValue, newValue, stderr, "")
	}

	oldName := fmt.Sprintf("%s#%s", p.result.Name, oldVersion)
	newName := fmt.Sprintf("%s#%s", p.result.Name, newVersion)

	diff := output.Diff(oldName, newName, oldValue, newValue)
	if diff != "" {
		output.Println(stdout, "")
		output.Print(stdout, diff)
	}
}

// LogCommand returns the Google Cloud Secret Manager log command.
func LogCommand() *cli.Command {
	return genericlog.Command(genericlog.Config{
		Usage:     "Show secret version history",
		ArgsUsage: "<name>",
		Description: `Display the version history of a secret, showing each version's
integer number, state (enabled/disabled/destroyed), and creation date.

Output is sorted with the most recent version first (use --reverse to flip).

Use --patch to show the diff between consecutive versions (like git log -p).
Note: disabled and destroyed versions have no accessible value, so their
diffs are skipped.

EXAMPLES:
   suve gcloud secret log my-secret                        Show last 10 versions
   suve gcloud secret log --patch my-secret                Show versions with diffs
   suve gcloud secret log --oneline my-secret              Compact one-line format
   suve gcloud secret log --output=json my-secret          Output as JSON`,
		UsageError: "usage: suve gcloud secret log <name>",
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
			store, err := cliinternal.GCPSecretStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewLogPresenter(store, req), nil
		},
	})
}
