// The all-service (cross-service) stage commands form one namespace with
// global.go, which holds their shared config and store gathering.
//declscope:namespace global

package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/cli/pager"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// globalDiffRunner diffs the staged changes of every service of one provider:
// all services' value entries first, then all services' tag changes.
type globalDiffRunner struct {
	// services lists one per-service diff use case per service with staged
	// changes, in stable display order.
	services []*stagingusecase.DiffUseCase
	// providerLabel is the human-readable provider name (e.g. "AWS") that
	// labels the remote side of every diff and warning.
	providerLabel string
	stdout        io.Writer
	stderr        io.Writer
}

// globalDiffOptions holds options for the all-service diff command.
type globalDiffOptions struct {
	parseJSON bool
	noPager   bool
}

// NewGlobalDiffCommand creates the provider-wide `stage diff` command.
func NewGlobalDiffCommand(gcfg GlobalConfig) *cli.Command {
	return &cli.Command{
		Name:  "diff",
		Usage: "Show diff of all staged changes",
		Description: fmt.Sprintf(`Compare all staged changes against the current %s values.

For comparing specific versions, use the per-service diff commands.

EXAMPLES:
   %s diff     Show diff of all staged changes
   %s diff -j  Show diff with JSON formatting`,
			gcfg.ProviderLabel, gcfg.CommandPath, gcfg.CommandPath),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "parse-json",
				Aliases: []string{"j"},
				Usage:   "Format JSON values before diffing (keys are always sorted)",
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager output",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() > 0 {
				return fmt.Errorf("usage: %s diff (no arguments)", gcfg.CommandPath)
			}

			return globalDiffAction(ctx, cmd, gcfg, globalWorkingStore)
		},
	}
}

// globalDiffUseCases builds one DiffUseCase per configured service that has
// staged changes. A provider client is initialized only for those services.
func globalDiffUseCases(
	ctx context.Context, gcfg GlobalConfig, resolve globalStoreResolver,
) ([]*stagingusecase.DiffUseCase, error) {
	services, err := gatherGlobalServices(ctx, gcfg.Services, resolve)
	if err != nil {
		return nil, err
	}

	var useCases []*stagingusecase.DiffUseCase

	for _, svc := range globalStagedServices(services) {
		strategy, err := svc.spec.Factory(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize %s client: %w", svc.spec.ParserFactory().ServiceName(), err)
		}

		useCases = append(useCases, &stagingusecase.DiffUseCase{
			Strategy:    strategy,
			Store:       svc.store,
			StrategyFor: diffStrategyFor(ctx, svc.spec.StrategyForNamespace),
			RemoteLabel: gcfg.ProviderLabel,
		})
	}

	return useCases, nil
}

func globalDiffAction(ctx context.Context, cmd *cli.Command, gcfg GlobalConfig, resolve globalStoreResolver) error {
	opts := globalDiffOptions{
		parseJSON: cmd.Bool("parse-json"),
		noPager:   cmd.Bool("no-pager"),
	}

	useCases, err := globalDiffUseCases(ctx, gcfg, resolve)
	if err != nil {
		return err
	}

	if len(useCases) == 0 {
		output.Warning(cmd.Root().ErrWriter, "nothing staged")

		return nil
	}

	r := &globalDiffRunner{
		services:      useCases,
		providerLabel: gcfg.ProviderLabel,
		stderr:        cmd.Root().ErrWriter,
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.noPager, func(w io.Writer) error {
		r.stdout = w

		return r.run(ctx, opts)
	})
}

// run executes the all-service diff: each service is diffed through its own
// DiffUseCase (which auto-unstages no-op and vanished entries), then rendered
// by the per-service diffRunner with the provider as the remote label.
func (r *globalDiffRunner) run(ctx context.Context, opts globalDiffOptions) error {
	results := make([]*stagingusecase.DiffOutput, 0, len(r.services))

	for _, useCase := range r.services {
		result, err := useCase.Execute(ctx, stagingusecase.DiffInput{})
		if err != nil {
			return err
		}

		results = append(results, result)
	}

	presenter := &diffRunner{
		stdout:             r.stdout,
		stderr:             r.stderr,
		providerLabel:      r.providerLabel,
		keptStagedWarnings: true,
	}
	diffOpts := diffOptions{parseJSON: opts.parseJSON, noPager: opts.noPager}
	first := true

	for _, result := range results {
		presenter.outputEntries(diffOpts, result.Entries, &first)
	}

	for _, result := range results {
		presenter.outputTagEntries(result.TagEntries, &first)
	}

	return nil
}
