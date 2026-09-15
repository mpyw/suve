//go:build e2e

//declscope:namespace app
//
// NewE2EModel hands the App model (run.go's newModel) to the e2e suite,
// so this file belongs to the same unit.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/provider"
)

// NewE2EModel builds the root TUI model for a launched scope and initial service
// exactly as Run does — the same registry-backed read/write/staging seams over
// real provider stores — but returns the tea.Model instead of running a live
// program. The emulator-backed e2e suite (package e2e, build tag `e2e`) drives
// this model through teatest against localstack, exercising the real data path
// (data source → usecase → provider store → emulator) rather than mocks.
//
// It is compiled only under the `e2e` build tag and is never linked into a
// shipped binary; internal/tui itself still imports no cloud SDK, so the
// architecture boundary is unchanged.
func NewE2EModel(ctx context.Context, scope provider.Scope, service string) (tea.Model, error) {
	return newModel(ctx, scope, service)
}
