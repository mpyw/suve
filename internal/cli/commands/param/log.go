package param

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/colors"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/cli/commands/param/paramtype"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/terminal"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/param"
)

// logJSONItem represents a single version entry in JSON output.
type logJSONItem struct {
	Version  int64  `json:"version"`
	Type     string `json:"type"`
	Modified string `json:"modified,omitempty"`
	Value    string `json:"value"`
}

// logPresenter renders SSM Parameter Store log output byte-for-byte as before.
type logPresenter struct {
	uc     *param.LogUseCase
	req    genericlog.Request
	result *param.LogOutput
}

// NewLogPresenter builds a param log presenter over the given reader and request.
// It is exported for the shared golden-output test harness.
func NewLogPresenter(reader provider.Reader, req genericlog.Request) genericlog.Presenter {
	return &logPresenter{uc: &param.LogUseCase{Reader: reader}, req: req}
}

func (p *logPresenter) Fetch(ctx context.Context) error {
	result, err := p.uc.Execute(ctx, param.LogInput{
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

	return nil
}

func (p *logPresenter) Len() int { return len(p.result.Entries) }

func (p *logPresenter) RenderJSON(stdout io.Writer) error {
	entries := p.result.Entries
	items := make([]logJSONItem, len(entries))

	for i, entry := range entries {
		items[i] = logJSONItem{
			Version: entry.Version,
			Type:    paramtype.Display(entry.Type),
			Value:   entry.Value,
		}
		if entry.LastModified != nil {
			items[i].Modified = timeutil.FormatRFC3339(*entry.LastModified)
		}
	}

	return output.WriteJSON(stdout, items)
}

func (p *logPresenter) RenderOneline(stdout io.Writer, i, maxValueLength int) {
	entry := p.result.Entries[i]

	// Compact one-line format: VERSION  DATE  VALUE_PREVIEW
	dateStr := ""
	if entry.LastModified != nil {
		dateStr = timeutil.FormatDate(*entry.LastModified)
	}

	value := entry.Value
	// Determine max length for oneline mode
	maxLen := maxValueLength
	if maxLen == 0 {
		// Auto: use terminal width minus overhead for metadata
		// Reserve ~30 chars for: version (6) + current mark (10) + date (10) + separators (4)
		termWidth := terminal.GetWidthFromWriter(stdout)

		const metadataOverhead = 30 // version + current mark + date + separators

		const minValueLength = 10

		maxLen = max(termWidth-metadataOverhead, minValueLength)
	}

	// Replace control characters so a multi-line value can't break the
	// one-line-per-version layout, then truncate by runes (#340).
	value = truncateRunes(sanitizeControl(value), maxLen)

	currentMark := ""
	if entry.IsCurrent {
		currentMark = colors.For(stdout).Current(" (current)")
	}

	output.Printf(stdout, "%s%s  %s  %s\n",
		colors.For(stdout).Version(strconv.FormatInt(entry.Version, 10)),
		currentMark,
		colors.For(stdout).FieldLabel(dateStr),
		value,
	)
}

func (p *logPresenter) RenderHeader(stdout io.Writer, i int) {
	entry := p.result.Entries[i]

	versionLabel := fmt.Sprintf("Version %d", entry.Version)
	if entry.IsCurrent {
		versionLabel += " " + colors.For(stdout).Current("(current)")
	}

	output.Println(stdout, colors.For(stdout).Version(versionLabel))

	if entry.LastModified != nil {
		output.Printf(stdout, "%s %s\n", colors.For(stdout).FieldLabel("Date:"), timeutil.FormatRFC3339(*entry.LastModified))
	}
}

func (p *logPresenter) RenderValue(stdout io.Writer, i, maxValueLength int) {
	entry := p.result.Entries[i]

	// Show value preview (unlimited unless --max-value-length specified).
	// Truncate by runes so multi-byte content is never cut mid-rune (#340).
	// Newlines are preserved here: normal mode intentionally shows the full
	// multi-line value under each version's header.
	value := truncateRunes(entry.Value, maxValueLength)

	output.Printf(stdout, "%s\n", value)
}

func (p *logPresenter) RenderPatch(stdout, stderr io.Writer, i int, parseJSON, reverse bool) {
	entries := p.result.Entries
	parentIdx, oldest := genericlog.PatchParent(i, len(entries), reverse)

	newEntry := entries[i]
	newValue := newEntry.Value

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
		oldValue = oldEntry.Value
		oldName = fmt.Sprintf("%s#%d", p.result.Name, oldEntry.Version)

		if parseJSON {
			oldValue, newValue = jsonutil.TryFormatOrWarn2(oldValue, newValue, stderr, "")
		}
	}

	newName := fmt.Sprintf("%s#%d", p.result.Name, newEntry.Version)

	diff := output.Diff(stdout, oldName, newName, oldValue, newValue)
	if diff != "" {
		output.Println(stdout, "")
		output.Print(stdout, diff)
	}
}

// truncateRunes shortens s to at most maxLen runes, appending "..." only when
// it actually trims. Counting runes rather than bytes keeps multi-byte
// characters (e.g. Japanese text, emoji) whole (#340). A maxLen <= 0 disables
// truncation.
func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	return string(runes[:maxLen]) + "..."
}

// sanitizeControl replaces every control character (newlines, tabs, etc.) with
// a visible ␤ so a value cannot break the one-line-per-version layout (#340).
func sanitizeControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '␤'
		}

		return r
	}, s)
}

// LogCommand returns the SSM Parameter Store log command.
func LogCommand() *cli.Command {
	return genericlog.Command(genericlog.Config{
		Usage:     "Show parameter version history",
		ArgsUsage: "<name>",
		Description: `Display the version history of a parameter, showing each version's
number, modification date, and a preview of the value.

Output is sorted with the most recent version first (use --reverse to flip).
Value preview truncation depends on the mode:
  - Normal mode: Full value is shown (no truncation)
  - Oneline mode: Truncated to roughly (terminal width - 30); about 20 chars when width is unavailable

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
		UsageError: "usage: suve param log <name>",
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
		NewPresenter: func(ctx context.Context, req genericlog.Request) (genericlog.Presenter, error) {
			store, err := cliinternal.ParamStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewLogPresenter(store, req), nil
		},
	})
}
