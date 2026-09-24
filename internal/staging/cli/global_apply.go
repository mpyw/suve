// The all-service (cross-service) stage commands form one namespace with
// global.go, which holds their shared config and store gathering.
//declscope:namespace global

package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/cli/output"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// GlobalApplyRunner applies the staged changes of every service of one provider
// through the GlobalApplyUseCase and reports the results, each line prefixed
// with its service name.
type GlobalApplyRunner struct {
	UseCase *stagingusecase.GlobalApplyUseCase
	// ProviderLabel is the human-readable provider name (e.g. "AWS").
	ProviderLabel   string
	Stdout          io.Writer
	Stderr          io.Writer
	IgnoreConflicts bool
}

// NewGlobalApplyCommand creates the provider-wide `stage apply` command.
func NewGlobalApplyCommand(gcfg GlobalConfig) *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Aliases: []string{"push"},
		Usage:   "Apply all staged changes",
		Description: fmt.Sprintf(`Apply all staged changes to the %s services.

After successful apply, the staged changes are cleared.

Use '%s status' to view all staged changes before applying.

CONFLICT DETECTION:
   Before applying, suve checks for conflicts to prevent lost updates:
   - For new resources: checks if someone else created it after staging
   - For existing resources: checks if it was modified after staging
   Use --ignore-conflicts to force apply despite conflicts.

EXAMPLES:
   %s apply                      Apply all staged changes (with confirmation)
   %s apply --yes                Apply without confirmation
   %s apply --ignore-conflicts   Apply even if conflicts detected`,
			gcfg.ProviderLabel, gcfg.CommandPath, gcfg.CommandPath, gcfg.CommandPath, gcfg.CommandPath),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  flagYes,
				Usage: usageSkipConfirm,
			},
			&cli.BoolFlag{
				Name:  "ignore-conflicts",
				Usage: fmt.Sprintf("Apply even if %s was modified after staging", gcfg.ProviderLabel),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return globalApplyAction(ctx, cmd, gcfg, globalWorkingStore)
		},
	}
}

// globalApplyUseCase builds the all-service apply over every configured service
// that has staged changes, plus the confirmation targets and the total staged
// count. A provider client is initialized only for those services.
func globalApplyUseCase(
	ctx context.Context, gcfg GlobalConfig, resolve globalStoreResolver,
) (useCase *stagingusecase.GlobalApplyUseCase, targets []string, totalStaged int, err error) {
	services, err := gatherGlobalServices(ctx, gcfg.Services, resolve)
	if err != nil {
		return nil, nil, 0, err
	}

	useCase = &stagingusecase.GlobalApplyUseCase{}

	for _, svc := range globalStagedServices(services) {
		strategy, err := svc.spec.Factory(ctx)
		if err != nil {
			return nil, nil, 0, err
		}

		totalStaged += svc.staged
		targets = append(targets, svc.target)
		useCase.Services = append(useCase.Services, &stagingusecase.ApplyUseCase{
			Strategy:    strategy,
			Store:       svc.store,
			StrategyFor: applyStrategyFor(ctx, svc.spec.StrategyForNamespace),
		})
	}

	return useCase, targets, totalStaged, nil
}

func globalApplyAction(ctx context.Context, cmd *cli.Command, gcfg GlobalConfig, resolve globalStoreResolver) error {
	useCase, targets, totalStaged, err := globalApplyUseCase(ctx, gcfg, resolve)
	if err != nil {
		return err
	}

	if totalStaged == 0 {
		output.Info(cmd.Root().Writer, "No changes staged.")

		return nil
	}

	// Confirm apply (once, across all services).
	prompter := &confirm.Prompter{
		Stdin:  cmd.Root().Reader,
		Stdout: cmd.Root().Writer,
		Stderr: cmd.Root().ErrWriter,
		Target: strings.Join(targets, ", "),
	}

	message := fmt.Sprintf("Apply %d staged change(s) to %s?", totalStaged, gcfg.ProviderLabel)

	confirmed, err := prompter.Confirm(message, cmd.Bool(flagYes))
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	r := &GlobalApplyRunner{
		UseCase:         useCase,
		ProviderLabel:   gcfg.ProviderLabel,
		Stdout:          cmd.Root().Writer,
		Stderr:          cmd.Root().ErrWriter,
		IgnoreConflicts: cmd.Bool("ignore-conflicts"),
	}

	return r.Run(ctx)
}

// Run applies every service's staged changes and reports the results: value
// changes service by service, then tag changes service by service, each result
// sorted by (name, namespace).
func (r *GlobalApplyRunner) Run(ctx context.Context) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.GlobalApplyInput{IgnoreConflicts: r.IgnoreConflicts})
	if result == nil {
		return err
	}

	if len(result.Conflicts) > 0 {
		for _, c := range result.Conflicts {
			output.Warning(r.Stderr, "conflict detected for %s (%s): %s was modified after staging",
				c.Key.Label(), c.ServiceName, r.ProviderLabel)
		}

		return fmt.Errorf("apply rejected: %d conflict(s) detected (use --ignore-conflicts to force)", len(result.Conflicts))
	}

	for _, svc := range result.Services {
		if len(svc.EntryResults) > 0 {
			output.Info(r.Stdout, "Applying %s...", svc.ServiceName)
			r.printEntryResults(svc)
		}
	}

	for _, svc := range result.Services {
		if len(svc.TagResults) > 0 {
			output.Info(r.Stdout, "Applying %s tags...", svc.ServiceName)
			r.printTagResults(svc)
		}
	}

	return err
}

func (r *GlobalApplyRunner) printEntryResults(svc *stagingusecase.ApplyOutput) {
	results := slices.Clone(svc.EntryResults)
	slices.SortFunc(results, func(a, b stagingusecase.ApplyEntryResult) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Namespace, b.Namespace))
	})

	for _, entry := range results {
		if entry.Error != nil {
			output.Failed(r.Stderr, svc.ServiceName+": "+entry.Name, entry.Error)

			continue
		}

		switch entry.Status {
		case stagingusecase.ApplyResultCreated:
			output.Success(r.Stdout, "%s: Created %s", svc.ServiceName, entry.Name)
		case stagingusecase.ApplyResultUpdated:
			output.Success(r.Stdout, "%s: Updated %s", svc.ServiceName, entry.Name)
		case stagingusecase.ApplyResultDeleted:
			output.Success(r.Stdout, "%s: Deleted %s", svc.ServiceName, entry.Name)
		case stagingusecase.ApplyResultFailed:
			// Unreachable: a Failed status always carries an Error (handled above).
		}

		if entry.UnstageError != nil {
			output.Warning(r.Stderr, "failed to clear staging for %s: %v", entry.Name, entry.UnstageError)
		}
	}
}

func (r *GlobalApplyRunner) printTagResults(svc *stagingusecase.ApplyOutput) {
	results := slices.Clone(svc.TagResults)
	slices.SortFunc(results, func(a, b stagingusecase.ApplyTagResult) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Namespace, b.Namespace))
	})

	for _, tag := range results {
		if tag.Error != nil {
			output.Failed(r.Stderr, svc.ServiceName+": "+tag.Name+" (tags)", tag.Error)

			continue
		}

		output.Success(r.Stdout, "%s: Tagged %s%s", svc.ServiceName, tag.Name, FormatTagApplySummary(tag))

		if tag.UnstageError != nil {
			output.Warning(r.Stderr, "failed to clear staging for %s tags: %v", tag.Name, tag.UnstageError)
		}
	}
}
