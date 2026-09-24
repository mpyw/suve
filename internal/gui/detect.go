//go:build production || dev

package gui

import (
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

// Provider detection.

// DetectResult mirrors internal/provider/detect.Result for the frontend: the
// uniquely-active provider per service (empty when 0 or 2+ are active) plus the
// full active sets. It drives the GUI's initial provider selection — no
// priority order; when ambiguous the user picks.
type DetectResult struct {
	Param  string `json:"param"`
	Secret string `json:"secret"`
	Stage  string `json:"stage"`

	ParamActive  []string `json:"paramActive"`
	SecretActive []string `json:"secretActive"`
	StageActive  []string `json:"stageActive"`
}

// DetectProviders resolves which providers are active in the current
// environment (env-only, no network calls), for the GUI's initial selection.
func (a *App) DetectProviders() *DetectResult {
	r := detect.Resolve(detect.OSEnvironment())

	return &DetectResult{
		Param:        string(r.Param),
		Secret:       string(r.Secret),
		Stage:        string(r.Stage),
		ParamActive:  detectedProviderStrings(r.ParamActive),
		SecretActive: detectedProviderStrings(r.SecretActive),
		StageActive:  detectedProviderStrings(r.StageActive),
	}
}

// DetectInitialProvider resolves the initial provider for a bare `suve --gui`
// (and the standalone Wails entry) from the environment: the sole active
// provider (detect.Result.UniqueProvider, the rule `suve --tui` shares), or ""
// when zero or two-plus are active (the frontend then shows the selector).
// Env-only; no network calls.
func DetectInitialProvider() provider.Provider {
	return detect.Resolve(detect.OSEnvironment()).UniqueProvider()
}

func detectedProviderStrings(ps []provider.Provider) []string {
	return lo.Map(ps, func(p provider.Provider, _ int) string { return string(p) })
}
