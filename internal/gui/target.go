//go:build production || dev

package gui

import (
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging/binding"
)

// ScopeTargetSegment is one labeled part of a ScopeTarget, such as "account"
// and its id. Value is empty when the part is unknown or unset; the sidebar
// shows "?" for it.
type ScopeTargetSegment struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ScopeTarget describes what the selected scope points at, for the sidebar. It
// mirrors provider.Target, the descriptor the TUI status bar and the CLI
// confirmation prompts render too.
type ScopeTarget struct {
	Segments []ScopeTargetSegment `json:"segments"`
	// Pending is true when some values need ResolveScopeTarget (AWS: the STS
	// caller identity).
	Pending bool `json:"pending"`
}

// GetScopeTarget describes the selected scope without any network call. When
// the result is pending, the frontend calls ResolveScopeTarget to fill it in.
func (a *App) GetScopeTarget() ScopeTarget {
	return scopeTargetFrom(targetScope(a.currentScope()).Target())
}

// ResolveScopeTarget describes the selected scope, running the provider's
// identity lookup when the scope cannot describe itself (AWS: STS).
func (a *App) ResolveScopeTarget() (ScopeTarget, error) {
	target, err := binding.ResolveTarget(a.ctx, targetScope(a.currentScope()), nil)
	if err != nil {
		return ScopeTarget{}, err
	}

	return scopeTargetFrom(target), nil
}

// targetScope drops the App Configuration namespace: the sidebar shows it as
// its namespace filter control, so the target would only repeat it.
func targetScope(sc provider.Scope) provider.Scope {
	sc.AppConfigNamespace = ""

	return sc
}

// scopeTargetFrom converts the neutral target to the frontend DTO.
func scopeTargetFrom(t provider.Target) ScopeTarget {
	return ScopeTarget{
		Segments: lo.Map(t.Segments, func(s provider.TargetSegment, _ int) ScopeTargetSegment {
			return ScopeTargetSegment{Label: s.Label, Value: s.Value}
		}),
		Pending: t.Pending,
	}
}
