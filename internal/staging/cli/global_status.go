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
	"github.com/mpyw/suve/internal/staging/store"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// GlobalStatusRunner shows the staged changes of every service of one provider,
// one "Staged <service> changes" block per service that has any.
type GlobalStatusRunner struct {
	// Services lists the provider services in stable display order.
	Services []GlobalServiceSpec
	// Store, when set, is used for every service (a test seam). When nil each
	// service resolves its own working store via its spec's ScopeResolver — Azure
	// App Configuration and Key Vault live in separate staging buckets.
	Store  store.ReadWriteOperator
	Stdout io.Writer
	Stderr io.Writer
}

// GlobalStatusOptions holds options for the all-service status command.
type GlobalStatusOptions struct {
	Verbose bool
}

// NewGlobalStatusCommand creates the provider-wide `stage status` command.
func NewGlobalStatusCommand(gcfg GlobalConfig) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show all staged changes",
		Description: fmt.Sprintf(`Display all staged changes for the %s services.

Use -v/--verbose to show detailed information including the staged values.

EXAMPLES:
   %s status     Show all staged changes
   %s status -v  Show detailed information`,
			gcfg.ProviderLabel, gcfg.CommandPath, gcfg.CommandPath),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show detailed information including values",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			r := &GlobalStatusRunner{
				Services: gcfg.Services,
				Stdout:   cmd.Root().Writer,
				Stderr:   cmd.Root().ErrWriter,
			}

			return r.Run(ctx, GlobalStatusOptions{Verbose: cmd.Bool("verbose")})
		},
	}
}

// Run executes the all-service status command. Each service reads its OWN
// store through the per-service StatusUseCase; a service whose scope is not
// configured is skipped (it can hold no staged state).
func (r *GlobalStatusRunner) Run(ctx context.Context, opts GlobalStatusOptions) error {
	services, err := gatherGlobalServices(ctx, r.Services, globalStoreFor(r.Store))
	if err != nil {
		return err
	}

	presenter := &StatusRunner{Stdout: r.Stdout, Stderr: r.Stderr}
	printed := false

	for _, svc := range globalStagedServices(services) {
		useCase := &stagingusecase.StatusUseCase{Strategy: svc.spec.ParserFactory(), Store: svc.store}

		result, err := useCase.Execute(ctx, stagingusecase.StatusInput{})
		if err != nil {
			return err
		}

		if printed {
			output.Println(r.Stdout, "")
		}

		presenter.printService(result, opts.Verbose)

		printed = true
	}

	if !printed {
		output.Info(r.Stdout, "No changes staged.")
	}

	return nil
}
