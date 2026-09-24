//go:build production || dev

package gui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
	"github.com/mpyw/suve/internal/staging/binding"
)

// errScopeIncomplete is returned by SelectScope when none of the provider's
// services has its scope field set, so no service could be resolved. The
// wrapped message names the missing fields by their form labels, since the
// frontend collects them through form inputs rather than command-line flags.
var errScopeIncomplete = stringError("scope is incomplete")

// hydrateScope fills empty resource fields on an initial launch scope from the
// environment through detect.HydrateScope (the rule `suve tui` shares): a
// flag-supplied value wins, an unset one falls back to env. An unknown provider
// fails with errInvalidProvider.
//
//declscope:package // NewApp hydrates the launch scope with it
func hydrateScope(s provider.Scope) (provider.Scope, error) {
	hydrated, err := detect.HydrateScope(detect.OSEnvironment(), s)
	if err != nil {
		return provider.Scope{}, fmt.Errorf("%w: %q", errInvalidProvider, s.Provider)
	}

	return hydrated, nil
}

// ScopeSelection is the frontend-supplied provider + scope for read/write
// operations. Only the fields relevant to the chosen provider are read:
//   - aws: (none; the ambient AWS config supplies the region)
//   - googlecloud: ProjectID
//   - azure: VaultName (Key Vault secret) and/or StoreName (App Configuration
//     param); Namespace is the optional App Configuration namespace (Azure
//     calls it a "label"), applied only to the App Configuration store — empty
//     means the default (null) namespace.
type ScopeSelection struct {
	Provider  string `json:"provider"`
	ProjectID string `json:"projectId"`
	VaultName string `json:"vaultName"`
	StoreName string `json:"storeName"`
	Namespace string `json:"namespace"`
}

// SelectScope sets the current read/write provider scope. It performs no
// network calls; store construction (and any credential resolution) is deferred
// to the next param/secret operation. For Azure the App Configuration namespace
// (sel.Namespace) is carried on the scope; it is only meaningful for the App
// Configuration store and harmless for every other provider/store.
func (a *App) SelectScope(sel ScopeSelection) error {
	scope, err := scopeFromSelection(sel)
	if err != nil {
		return err
	}

	a.scopeMu.Lock()
	a.scope = scope
	a.scopeMu.Unlock()

	return nil
}

// scopeField binds one capability scope-field name to the ScopeSelection
// property that carries it and the provider.Scope field it fills. It mirrors
// SCOPE_FIELDS in frontend/src/lib/scopeFields.ts.
type scopeField struct {
	// label names the field in validation errors.
	label string
	get   func(ScopeSelection) string
	set   func(*provider.Scope, string)
}

// scopeFields maps every capability scope-field name to its binding.
//
//nolint:gochecknoglobals // static lookup table
var scopeFields = map[string]scopeField{
	"project": {
		label: "project ID",
		get:   func(sel ScopeSelection) string { return sel.ProjectID },
		set:   func(sc *provider.Scope, v string) { sc.ProjectID = v },
	},
	"vault": {
		label: "Key Vault name",
		get:   func(sel ScopeSelection) string { return sel.VaultName },
		set:   func(sc *provider.Scope, v string) { sc.VaultName = v },
	},
	"store": {
		label: "App Configuration store name",
		get:   func(sel ScopeSelection) string { return sel.StoreName },
		set:   func(sc *provider.Scope, v string) { sc.StoreName = v },
	},
	"namespace": {
		label: "App Configuration namespace",
		get:   func(sel ScopeSelection) string { return sel.Namespace },
		set:   func(sc *provider.Scope, v string) { sc.AppConfigNamespace = v },
	},
}

// scopeFromSelection maps a frontend selection to a provider.Scope from the
// provider's capability: it copies only the provider's ScopeFields, and rejects
// the selection when no service has its ScopeField set. A service with no
// ScopeField (AWS) is always available, so such a provider needs no field. For
// Azure a single scope carries both VaultName and StoreName, so the registry can
// build either the Key Vault (secret) or App Configuration (param) store from
// it; at least one must be set.
func scopeFromSelection(sel ScopeSelection) (provider.Scope, error) {
	pc, ok := capability.Provider(provider.Provider(sel.Provider))
	if !ok {
		return provider.Scope{}, fmt.Errorf("%w: %q", errInvalidProvider, sel.Provider)
	}

	scope := provider.Scope{Provider: provider.Provider(pc.Provider)}
	for _, name := range pc.ScopeFields {
		if f, ok := scopeFields[name]; ok {
			f.set(&scope, f.get(sel))
		}
	}

	available := lo.ContainsBy(pc.Services, func(sc capability.ServiceCapability) bool {
		return sc.ScopeField == "" || scopeFields[sc.ScopeField].get(sel) != ""
	})
	if !available {
		labels := lo.Map(pc.Services, func(sc capability.ServiceCapability, _ int) string {
			return "the " + scopeFields[sc.ScopeField].label
		})

		return provider.Scope{}, fmt.Errorf("%w: %s requires %s", errScopeIncomplete, pc.DisplayName, strings.Join(labels, " or "))
	}

	return scope, nil
}

// currentScope returns the active read/write scope.
//
//declscope:package // shared with the capability, param, secret, spec and staging namespaces
func (a *App) currentScope() provider.Scope {
	a.scopeMu.RLock()
	defer a.scopeMu.RUnlock()

	return a.scope
}

// GetCurrentScope returns the active read/write scope as a ScopeSelection so the
// frontend can prefill its provider/scope forms (including the env-derived
// initial values from GOOGLE_CLOUD_PROJECT / AZURE_*) instead of silently wiping
// the backend scope on first render.
func (a *App) GetCurrentScope() *ScopeSelection {
	return selectionFromScope(a.currentScope())
}

// EnvScope returns the env-derived scope defaults for an ARBITRARY provider,
// hydrated from the ambient environment (GOOGLE_CLOUD_PROJECT / AZURE_KEYVAULT_NAME
// / AZURE_APPCONFIG_NAME / AZURE_APPCONFIG_NAMESPACE). It is the direct analog of
// the CLI's per-provider env resolution: each provider group reads its own env
// independently of detect, so an explicitly-selected provider always resolves its
// scope from env even in a mixed-env shell. GetCurrentScope only surfaces the
// launch provider's env-derived scope; the frontend calls this to fill the scope
// form for any OTHER provider it switches to. An unknown provider is an error.
func (a *App) EnvScope(providerName string) (*ScopeSelection, error) {
	scope, err := hydrateScope(provider.Scope{Provider: provider.Provider(providerName)})
	if err != nil {
		return nil, err
	}

	return selectionFromScope(scope), nil
}

// selectionFromScope is the inverse of scopeFromSelection: it projects a
// provider.Scope back to the frontend DTO. Fields irrelevant to the provider
// stay empty.
func selectionFromScope(s provider.Scope) *ScopeSelection {
	return &ScopeSelection{
		Provider:  string(s.Provider),
		ProjectID: s.ProjectID,
		VaultName: s.VaultName,
		StoreName: s.StoreName,
		Namespace: s.AppConfigNamespace,
	}
}

// stagingScopeForKind resolves the staging scope for ONE service kind through
// the shared staging binding, so the GUI keys staging exactly like the CLI and
// TUI: Azure's two services are independent resources with separate buckets
// (App Configuration by store name, Key Vault by vault name), AWS is keyed by
// the STS caller identity (account/region), and Google Cloud by the project. An
// unknown (or unselected) provider is errInvalidProvider.
//
//declscope:package // shared with the staging namespace
func (a *App) stagingScopeForKind(kind provider.Kind) (provider.Scope, error) {
	return a.stagingScopeForKindScoped(a.currentScope(), kind)
}

// stagingScopeForKindScoped is stagingScopeForKind resolved from an
// already-snapshotted scope, so a staging binding can pair its store and
// strategy against the SAME scope even if SelectScope lands between the two
// resolutions (#560).
//
//declscope:package // shared with the staging namespace
func (a *App) stagingScopeForKindScoped(sc provider.Scope, kind provider.Kind) (provider.Scope, error) {
	resolved, err := binding.StagingScope(a.ctx, sc, kind, nil)
	if errors.Is(err, binding.ErrUnknownProvider) {
		return provider.Scope{}, fmt.Errorf("%w: %q", errInvalidProvider, sc.Provider)
	}

	if err != nil {
		return provider.Scope{}, err
	}

	return resolved.Scope, nil
}
