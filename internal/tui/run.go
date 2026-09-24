//declscope:namespace app
//
// Run builds the App's config and runs the program, and newModel returns
// the App itself; the launch path and the model are one unit.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/builtin"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// registry is the provider registry backing the TUI's read/write operations. It
// is built by builtin.NewRegistry, the same composition the CLI and GUI use, so
// any launched scope resolves a store. The TUI reaches the clouds through the
// provider packages, never a cloud SDK directly.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
var registry = builtin.NewRegistry()

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
		scope:       scope,
		service:     service,
		fetchTarget: factory.resolveTarget,
		sourceFor:   factory.sourceFor,
		mutatorFor:  factory.mutatorFor,
		stagingFor:  factory.stagingService,
		runCtx:      ctx,
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
		return fmt.Errorf("no service is available for the %s scope; check the launch flags/environment", capability.DisplayName(scope.Provider))
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
