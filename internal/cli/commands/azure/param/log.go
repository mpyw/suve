package param

import (
	"context"
	"io"

	"github.com/urfave/cli/v3"

	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/usecase/azure"
)

// logPresenter renders Azure App Configuration log output. App Configuration has
// no version history: Fetch always surfaces the provider's
// ErrVersioningUnsupported error, so the render methods are never reached (they
// exist only to satisfy the genericlog.Presenter interface).
type logPresenter struct {
	uc  *azure.LogUseCase
	req genericlog.Request
}

// NewLogPresenter builds an Azure App Configuration log presenter over the given reader and request.
func NewLogPresenter(reader provider.Reader, req genericlog.Request) genericlog.Presenter {
	return &logPresenter{uc: &azure.LogUseCase{Reader: reader}, req: req}
}

// Fetch always returns an error: App Configuration keeps no version history.
func (p *logPresenter) Fetch(ctx context.Context) error {
	_, err := p.uc.Execute(ctx, azure.LogInput{
		Name:       p.req.Name,
		MaxResults: p.req.MaxResults,
		Since:      p.req.Since,
		Until:      p.req.Until,
		Reverse:    p.req.Reverse,
	})

	return err
}

func (p *logPresenter) Len() int                            { return 0 }
func (p *logPresenter) RenderJSON(_ io.Writer) error        { return nil }
func (p *logPresenter) RenderOneline(_ io.Writer, _, _ int) {}
func (p *logPresenter) RenderHeader(_ io.Writer, _ int)     {}
func (p *logPresenter) RenderValue(_ io.Writer, _, _ int)   {}

func (p *logPresenter) RenderPatch(_, _ io.Writer, _ int, _, _ bool) {}

// LogCommand returns the Azure App Configuration log command. Because App
// Configuration is unversioned, running it produces a clear error (it never
// crashes).
func LogCommand() *cli.Command {
	return genericlog.Command(genericlog.Config{
		Usage:     "Show setting version history (unsupported)",
		ArgsUsage: argsUsageKey,
		Description: `App Configuration has no version history.

This command exists for parity with the other providers but always reports that
version history is unsupported. It never crashes.

EXAMPLES:
   suve azure param log my-key                            Reports "history unsupported"`,
		UsageError: "usage: suve azure param log <key>",
		Flags: []cli.Flag{
			&cli.Int32Flag{
				Name:    "number",
				Aliases: []string{"n"},
				Value:   10, //nolint:mnd // default number of versions to display
				Usage:   "Number of versions to show (unsupported)",
			},
			&cli.BoolFlag{
				Name:    "patch",
				Aliases: []string{"p"},
				Usage:   "Show diff between consecutive versions (unsupported)",
			},
			&cli.BoolFlag{
				Name:    "parse-json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (unsupported)",
			},
			&cli.BoolFlag{
				Name:  "oneline",
				Usage: "Compact one-line-per-version format (unsupported)",
			},
			&cli.BoolFlag{
				Name:  "reverse",
				Usage: "Show oldest versions first (unsupported)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
			&cli.StringFlag{
				Name:  "since",
				Usage: "Show versions created after this date (RFC3339 format, unsupported)",
			},
			&cli.StringFlag{
				Name:  "until",
				Usage: "Show versions created before this date (RFC3339 format, unsupported)",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format: text (default) or json",
			},
		},
		NewPresenter: func(ctx context.Context, req genericlog.Request) (genericlog.Presenter, error) {
			store, err := cliinternal.AzureAppConfigStore(ctx)
			if err != nil {
				return nil, err
			}

			return NewLogPresenter(store, req), nil
		},
	})
}
