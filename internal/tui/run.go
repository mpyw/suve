package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/provider/aws/infra"
	"github.com/mpyw/suve/internal/provider/azure"
	"github.com/mpyw/suve/internal/provider/gcloud"
	"github.com/mpyw/suve/internal/tui/components"
)

// registry is the provider registry backing the TUI's read/write operations. It
// is the same composition point the CLI and GUI use
// (internal/cli/commands/internal/client.go, internal/gui/app.go): AWS (param +
// secret), Google Cloud (secret), and Azure (Key Vault secret + App
// Configuration param) are registered so any launched scope resolves a store.
// The TUI composes it through the provider packages — never a cloud SDK
// directly — keeping the SDK-confinement boundary intact.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
var registry = func() *provider.Registry {
	reg := aws.NewRegistry()
	gcloud.Register(reg)
	azure.Register(reg)

	return reg
}()

// Run starts the TUI for a fixed provider scope and initial service. Provider
// and scope are resolved by the caller (the --tui launch wiring) and never
// change for the process lifetime — relaunch to switch. service preselects the
// initial tab ("param"/"secret", or "" for the group default). It mirrors the
// GUI's Run entry shape (internal/gui/run.go) adapted to the terminal.
func Run(ctx context.Context, scope provider.Scope, service string) error {
	if err := ensureResolvable(ctx, scope); err != nil {
		return err
	}

	factory := newSourceFactory(ctx, scope)

	// The page fetch commands receive the Run context through the model's runCtx
	// field (config.runCtx below), not as a call parameter — contextcheck cannot
	// see the field-threaded context, so it is silenced here.
	//nolint:contextcheck // Run context is threaded via config.runCtx into every page fetch command
	model := newApp(config{
		scope:         scope,
		service:       service,
		fetchIdentity: awsIdentityFetcher(ctx),
		sourceFor:     factory.sourceFor,
		mutatorFor:    factory.mutatorFor,
		runCtx:        ctx,
	})

	// Alt-screen and mouse capture are requested through the model's returned
	// tea.View (Bubble Tea v2 reads them there each frame), so the program needs
	// only the context here.
	_, err := tea.NewProgram(model, tea.WithContext(ctx)).Run()

	return err
}

// ensureResolvable verifies the launched scope can resolve at least one store
// through the registry, turning an unusable scope (e.g. an Azure scope with
// neither a vault nor a store) into a clear launch error before the alt-screen
// takes over. It performs no network calls: store construction is lazy.
func ensureResolvable(ctx context.Context, scope provider.Scope) error {
	kinds := scope.SupportedKinds()
	if len(kinds) == 0 {
		return fmt.Errorf("no service is available for the %s scope; check the launch flags/environment", scope.Provider)
	}

	var lastErr error

	for _, kind := range kinds {
		if _, err := registry.Store(ctx, scope, kind); err != nil {
			lastErr = err

			continue
		}

		return nil
	}

	return lastErr
}

// awsIdentityFetcher builds the status bar's AWS identity fetcher as a closure
// over the Run context, so the model never stores a context nor imports the AWS
// provider package. It is the only TUI seam that touches internal/provider/aws.
func awsIdentityFetcher(ctx context.Context) identityFetcher {
	return func() (components.AWSIdentity, error) {
		id, err := infra.GetAWSIdentity(ctx)
		if err != nil {
			return components.AWSIdentity{}, err
		}

		return components.AWSIdentity{
			Account: id.AccountID,
			Region:  id.Region,
			Profile: id.Profile,
		}, nil
	}
}
