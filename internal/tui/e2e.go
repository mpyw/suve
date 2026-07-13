//go:build e2e

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
