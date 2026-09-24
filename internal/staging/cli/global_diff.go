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

// GlobalDiffRunner diffs the staged changes of every service of one provider:
// all services' value entries first, then all services' tag changes.
type GlobalDiffRunner struct {
	// Services lists one per-service diff use case per service with staged
	// changes, in stable display order.
	Services []*stagingusecase.DiffUseCase
	// ProviderLabel is the human-readable provider name (e.g. "AWS") that
	// labels the remote side of every diff and warning.
	ProviderLabel string
	Stdout        io.Writer
	Stderr        io.Writer
}

// GlobalDiffOptions holds options for the all-service diff command.
type GlobalDiffOptions struct {
	ParseJSON bool
	NoPager   bool
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
	opts := GlobalDiffOptions{
		ParseJSON: cmd.Bool("parse-json"),
		NoPager:   cmd.Bool("no-pager"),
	}

	useCases, err := globalDiffUseCases(ctx, gcfg, resolve)
	if err != nil {
		return err
	}

	if len(useCases) == 0 {
		output.Warning(cmd.Root().ErrWriter, "nothing staged")

		return nil
	}

	r := &GlobalDiffRunner{
		Services:      useCases,
		ProviderLabel: gcfg.ProviderLabel,
		Stderr:        cmd.Root().ErrWriter,
	}

	return pager.WithPagerWriter(cmd.Root().Writer, opts.NoPager, func(w io.Writer) error {
		r.Stdout = w

		return r.Run(ctx, opts)
	})
}

// Run executes the all-service diff: each service is diffed through its own
// DiffUseCase (which auto-unstages no-op and vanished entries), then rendered
// by the per-service DiffRunner with the provider as the remote label.
func (r *GlobalDiffRunner) Run(ctx context.Context, opts GlobalDiffOptions) error {
	results := make([]*stagingusecase.DiffOutput, 0, len(r.Services))

	for _, useCase := range r.Services {
		result, err := useCase.Execute(ctx, stagingusecase.DiffInput{})
		if err != nil {
			return err
		}

		results = append(results, result)
	}

	presenter := &DiffRunner{
		Stdout:             r.Stdout,
		Stderr:             r.Stderr,
		RemoteLabel:        r.ProviderLabel,
		keptStagedWarnings: true,
	}
	diffOpts := DiffOptions{ParseJSON: opts.ParseJSON, NoPager: opts.NoPager}
	first := true

	for _, result := range results {
		presenter.outputEntries(diffOpts, result.Entries, &first)
	}

	for _, result := range results {
		presenter.outputTagEntries(result.TagEntries, &first)
	}

	return nil
}
