// The all-service (cross-service) stage commands form one namespace with
// global.go, which holds their shared config and store gathering.
//declscope:namespace global

package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging/store"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// GlobalResetRunner unstages every service of one provider.
type GlobalResetRunner struct {
	// Services lists the provider services in stable display order.
	Services []GlobalServiceSpec
	// Store, when set, is used for every service (a test seam). When nil each
	// service resolves its own working store via its spec's ScopeResolver.
	Store  store.ReadWriteOperator
	Stdout io.Writer
	Stderr io.Writer
}

// NewGlobalResetCommand creates the provider-wide `stage reset` command.
func NewGlobalResetCommand(gcfg GlobalConfig) *cli.Command {
	return &cli.Command{
		Name:  "reset",
		Usage: "Unstage all changes",
		Description: fmt.Sprintf(`Remove all staged changes from the staging area.

This does not affect %s - it only clears the local staging area.

Use '%s <service> reset' for service-specific operations.

EXAMPLES:
   %s reset --all    Unstage all changes`,
			gcfg.ProviderLabel, gcfg.CommandPath, gcfg.CommandPath),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Unstage all changes (required)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Require --all flag for safety
			if !cmd.Bool("all") {
				output.Warning(cmd.Root().ErrWriter, "no effect without --all flag")
				output.Hint(cmd.Root().ErrWriter, "Use '%s reset --all' to unstage all changes", gcfg.CommandPath)

				return nil
			}

			r := &GlobalResetRunner{
				Services: gcfg.Services,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}

			return r.Run(ctx)
		},
	}
}

// Run executes the all-service reset command. Each configured service is reset
// in its OWN store through the per-service ResetUseCase; a service whose scope
// is not configured is skipped (it can hold no staged state).
func (r *GlobalResetRunner) Run(ctx context.Context) error {
	services, err := gatherGlobalServices(ctx, r.Services, globalStoreFor(r.Store))
	if err != nil {
		return err
	}

	var (
		totalCount int
		summaries  []string
	)

	for _, svc := range services {
		parser := svc.spec.ParserFactory()
		useCase := &stagingusecase.ResetUseCase{Parser: parser, Store: svc.store}

		result, err := useCase.Execute(ctx, stagingusecase.ResetInput{All: true})
		if err != nil {
			return err
		}

		totalCount += result.Count
		summaries = append(summaries, strconv.Itoa(result.Count)+" "+parser.ServiceName())
	}

	if totalCount == 0 {
		output.Info(r.Stdout, "No changes staged.")

		return nil
	}

	output.Success(r.Stdout, "Unstaged all changes (%s)", strings.Join(summaries, ", "))

	return nil
}
