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

// globalStatusRunner shows the staged changes of every service of one provider,
// one "Staged <service> changes" block per service that has any.
type globalStatusRunner struct {
	// services lists the provider services in stable display order.
	services []GlobalServiceSpec
	// store, when set, is used for every service (a test seam). When nil each
	// service resolves its own working store via its spec's ScopeResolver — Azure
	// App Configuration and Key Vault live in separate staging buckets.
	store  store.ReadWriteOperator
	stdout io.Writer
	stderr io.Writer
}

// globalStatusOptions holds options for the all-service status command.
type globalStatusOptions struct {
	verbose bool
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
			r := &globalStatusRunner{
				services: gcfg.Services,
				stdout:   cmd.Root().Writer,
				stderr:   cmd.Root().ErrWriter,
			}

			return r.run(ctx, globalStatusOptions{verbose: cmd.Bool("verbose")})
		},
	}
}

// run executes the all-service status command. Each service reads its OWN
// store through the per-service StatusUseCase; a service whose scope is not
// configured is skipped (it can hold no staged state).
func (r *globalStatusRunner) run(ctx context.Context, opts globalStatusOptions) error {
	services, err := gatherGlobalServices(ctx, r.services, globalStoreFor(r.store))
	if err != nil {
		return err
	}

	presenter := &statusRunner{stdout: r.stdout, stderr: r.stderr}
	printed := false

	for _, svc := range globalStagedServices(services) {
		useCase := &stagingusecase.StatusUseCase{Strategy: svc.spec.ParserFactory(), Store: svc.store}

		result, err := useCase.Execute(ctx, stagingusecase.StatusInput{})
		if err != nil {
			return err
		}

		if printed {
			output.Println(r.stdout, "")
		}

		presenter.printService(result, opts.verbose)

		printed = true
	}

	if !printed {
		output.Info(r.stdout, "No changes staged.")
	}

	return nil
}
