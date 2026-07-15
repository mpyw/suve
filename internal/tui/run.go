package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/provider/aws/infra"
	"github.com/mpyw/suve/internal/provider/azure"
	"github.com/mpyw/suve/internal/provider/gcloud"
	"github.com/mpyw/suve/internal/staging/store/file"
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
	model, err := newModel(ctx, scope, service)
	if err != nil {
		return err
	}

	// The staging store's plaintext-fallback warning writes straight to stderr;
	// firing during the alt-screen (it always does in a keychain-less cloud
	// shell) would corrupt the display. Capture it for the program's lifetime and
	// replay it to stderr once the normal screen is restored.
	warnings := &lockedBuffer{}
	prevWarn := file.SetWarnWriter(warnings)

	defer func() {
		file.SetWarnWriter(prevWarn)

		if s := warnings.String(); s != "" {
			// Deliberate stderr write: replay the captured staging warning now
			// that the alt-screen is closed and the normal screen is restored.
			//nolint:forbidigo // one-off warning replay after the TUI exits
			_, _ = fmt.Fprint(os.Stderr, s)
		}
	}()

	// Alt-screen and mouse capture are requested through the model's returned
	// tea.View (Bubble Tea v2 reads them there each frame), so the program needs
	// only the context here. Browser cloud-shell corruption is handled in the
	// model via a continuous full-repaint loop (see cloudShellRepaintCmd).
	_, err = tea.NewProgram(model, tea.WithContext(ctx)).Run()

	return err
}

// lockedBuffer is a concurrency-safe bytes.Buffer: the staging store may emit its
// plaintext warning from an async data-load goroutine while Run reads the
// captured text after the program exits.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// newModel builds the root app model for a launched scope and initial service,
// wiring the registry-backed sourceFactory as the read/write/staging seams. It
// is the shared construction point for the interactive Run entry above and the
// e2e harness (NewE2EModel), which drives the very same model through teatest
// against emulator-backed provider stores instead of a live terminal.
func newModel(ctx context.Context, scope provider.Scope, service string) (*App, error) {
	if err := ensureResolvable(ctx, scope); err != nil {
		return nil, err
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
		stagingFor:    factory.stagingService,
		runCtx:        ctx,
	})

	return model, nil
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
