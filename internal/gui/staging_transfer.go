//go:build production || dev

// Staging export and import: the envelope ports, the native file pickers, and
// the export, import and inspect bindings.
//
// The staging*.go files are one set of App methods split by concern, so they
// share staging.go's namespace.
//declscope:namespace staging

package gui

import (
	"context"
	"errors"
	"fmt"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// StagingExportResult represents the result of exporting the working staging
// area to a per-service envelope file.
type StagingExportResult struct {
	EntryCount int `json:"entryCount"`
	TagCount   int `json:"tagCount"`
}

// StagingImportResult represents the result of importing a per-service envelope
// file into the working staging area.
type StagingImportResult struct {
	Merged     bool `json:"merged"`
	EntryCount int  `json:"entryCount"`
	TagCount   int  `json:"tagCount"`
}

// StagingEnvelopeInfoResult describes an export file's plaintext header so the frontend
// can decide whether to prompt for a passphrase (Encrypted) and warn on a
// scope/service mismatch (ScopeMatches) BEFORE any passphrase is supplied.
type StagingEnvelopeInfoResult struct {
	// Encrypted reports whether the payload is passphrase-encrypted.
	Encrypted bool `json:"encrypted"`
	// Provider is the scope provider string embedded in the envelope.
	Provider string `json:"provider"`
	// Scope is the scope key (provider.Scope.Key()) embedded in the envelope.
	Scope string `json:"scope"`
	// Service is the staging service the payload holds ("param" or "secret").
	Service string `json:"service"`
	// ScopeMatches reports whether the envelope's scope matches the scope the
	// selected service resolves to under the active provider.
	ScopeMatches bool `json:"scopeMatches"`
	// WorkingHasChanges reports whether the working staging area for the
	// envelope's declared service already holds staged entries or tag changes.
	// The frontend prompts for merge/overwrite only when it is true, deciding
	// from the import metadata instead of the (possibly stale/unloaded) view
	// state — matching the CLI, which prompts only when the working area is
	// non-empty.
	WorkingHasChanges bool `json:"workingHasChanges"`
}

// errStagingStoreNotFileStore is returned when the resolved staging store cannot serve
// the working-area drain/unstage/update operations (should never happen for the
// file/mock stores).
var errStagingStoreNotFileStore = stringError("staging store does not support import/export")

// getStagingWorkingFileStoreScoped resolves the per-service working store as a
// WorkingStore (bulk Drain plus the per-key unstage and atomic Update the
// export/import use cases need) from an already-snapshotted scope (#560). It
// goes through getStagingStoreScoped so the test seam and the per-service scope
// resolution (the #445 fix: param → App Configuration bucket, secret → Key
// Vault bucket) are shared with every other staging op.
func (a *App) getStagingWorkingFileStoreScoped(sc provider.Scope, kind provider.Kind) (store.WorkingStore, error) {
	s, err := a.getStagingStoreScoped(sc, kind)
	if err != nil {
		return nil, err
	}

	fs, ok := s.(store.WorkingStore)
	if !ok {
		return nil, errStagingStoreNotFileStore
	}

	return fs, nil
}

// stagingEnvelopeWriteTarget adapts file.WriteEnvelopeFile to the export use case's
// EnvelopeWriter port. It binds the destination path (chosen via the native save
// dialog), the per-service scope (kept in the plaintext header), and the
// passphrase, so the use case only supplies the service and its state.
type stagingEnvelopeWriteTarget struct {
	path       string
	scope      provider.Scope
	passphrase string
}

// WriteEnvelope writes svc's state to the bound destination path.
func (t *stagingEnvelopeWriteTarget) WriteEnvelope(_ context.Context, svc staging.Service, state *staging.State) error {
	return file.WriteEnvelopeFile(t.path, t.scope, svc, state, t.passphrase)
}

// stagingEnvelopeReadSource adapts a validated file.Envelope to the import use case's
// EnvelopeReader port. Only the service the header declares yields data; any
// other service is an empty state (skipped).
type stagingEnvelopeReadSource struct {
	env        *file.Envelope
	passphrase string
}

// ReadState decodes (and decrypts when encrypted) the envelope for svc.
func (s *stagingEnvelopeReadSource) ReadState(_ context.Context, svc staging.Service) (*staging.State, error) {
	if string(svc) != s.env.Service {
		return staging.NewEmptyState(), nil
	}

	return s.env.DecodeState(s.passphrase)
}

// StagingPickExportPath opens the native Save dialog for choosing an export
// destination file, prefilled with defaultName. It returns an empty path (no
// error) when the user cancels, which the frontend treats as an aborted flow.
func (a *App) StagingPickExportPath(defaultName string) (string, error) {
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Export staged changes",
		DefaultFilename: defaultName,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "JSON files (*.json)", Pattern: "*.json"},
		},
	})
}

// StagingPickImportPath opens the native Open dialog for choosing an export file to
// import. It returns an empty path (no error) when the user cancels.
func (a *App) StagingPickImportPath() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Import staged changes",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "JSON files (*.json)", Pattern: "*.json"},
		},
	})
}

// StagingExport writes the working staging area for a single concrete service
// out to path as a per-service envelope, mirroring `stage <svc> export`. The
// working area is cleared afterwards unless keep is true. An empty passphrase
// stores the payload as plaintext (base64 only); the frontend warns first.
//
// The scope and working store are resolved PER SERVICE via
// stagingScopeForKind — never the combined stagingScope — so an Azure App
// Configuration param exports under the App Configuration bucket and a Key Vault
// secret under the Key Vault bucket (#445).
func (a *App) StagingExport(path, service, passphrase string, keep bool) (*StagingExportResult, error) {
	// Snapshot the scope once so the envelope header and the working area bind to
	// the same scope even if SelectScope lands mid-call (#560).
	sc := a.currentScope()

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	scope, err := a.stagingScopeForKindScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	working, err := a.getStagingWorkingFileStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.ExportUseCase{
		Working: working,
		Target: &stagingEnvelopeWriteTarget{
			path:       path,
			scope:      scope,
			passphrase: passphrase,
		},
	}

	result, err := uc.Execute(a.ctx, stagingusecase.ExportInput{Service: svc, Keep: keep})
	if err != nil {
		// A non-fatal error means the file was written but clearing the working
		// area failed; the export itself succeeded, so report the counts.
		var expErr *stagingusecase.ExportError
		if !errors.As(err, &expErr) || !expErr.NonFatal {
			return nil, err
		}
	}

	return &StagingExportResult{
		EntryCount: result.EntryCount,
		TagCount:   result.TagCount,
	}, nil
}

// StagingInspectImportFile reads and validates the plaintext envelope header at path
// WITHOUT decoding the (possibly encrypted) payload, so the frontend can prompt
// for a passphrase only when needed and warn on a scope/service mismatch. The
// envelope's scope is compared against the scope its declared service resolves
// to under the active provider (the #445 per-service resolution).
func (a *App) StagingInspectImportFile(path string) (*StagingEnvelopeInfoResult, error) {
	env, err := file.ReadEnvelopeFile(path)
	if err != nil {
		return nil, err
	}

	encrypted, err := env.IsEncryptedPayload()
	if err != nil {
		return nil, err
	}

	scope, err := a.stagingScopeForKind(kindForService(env.Service))
	if err != nil {
		return nil, err
	}

	workingHasChanges, err := a.stagingWorkingHasChangesForService(env.Service)
	if err != nil {
		return nil, err
	}

	return &StagingEnvelopeInfoResult{
		Encrypted:         encrypted,
		Provider:          env.Provider,
		Scope:             env.Scope,
		Service:           env.Service,
		ScopeMatches:      env.Scope == scope.Key(),
		WorkingHasChanges: workingHasChanges,
	}, nil
}

// stagingWorkingHasChangesForService reports whether the per-service working staging
// area holds any staged entries or tag changes for the given service. The GUI
// uses it (via StagingInspectImportFile) to prompt for merge/overwrite only when the
// working area is non-empty, mirroring the CLI's import behavior. The working
// store is file-based, so this makes no network calls. An unrecognized service
// (a malformed envelope) reports no changes rather than erroring.
func (a *App) stagingWorkingHasChangesForService(service string) (bool, error) {
	// A malformed envelope may name an unknown service; treat that as no working
	// changes rather than resolving a store for it.
	if service != string(staging.ServiceParam) && service != string(staging.ServiceSecret) {
		return false, nil
	}

	svc := staging.Service(service)

	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return false, err
	}

	entries, err := store.ListEntries(a.ctx, svc)
	if err != nil {
		return false, err
	}

	if len(entries[svc]) > 0 {
		return true, nil
	}

	tags, err := store.ListTags(a.ctx, svc)
	if err != nil {
		return false, err
	}

	return len(tags[svc]) > 0, nil
}

// StagingImport reads a per-service envelope file into the working staging area
// for the selected service, mirroring `stage <svc> import`. A service mismatch
// (the file holds another service's data) is a hard error, as is a provider
// mismatch (never overridable — a provider change is qualitatively different
// from an account/region/vault change). A scope mismatch is refused unless force
// is true; the frontend passes force after the user confirms the StagingInspectImportFile
// scope warning (the equivalent of the CLI's --force). This backend guard restores
// defense-in-depth parity with the CLI so a frontend regression cannot import
// cross-scope changes unchecked.
//
// mode is "merge" (default) or "overwrite"; it only matters when the working
// area already holds changes for the service. The working store is resolved per
// service via stagingScopeForKind (#445).
func (a *App) StagingImport(path, service, passphrase, mode string, force bool) (*StagingImportResult, error) {
	// Snapshot the scope once so the store, the working area, and the re-anchor
	// resolver all bind to the same scope even if SelectScope lands mid-call (#560).
	sc := a.currentScope()

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	env, err := file.ReadEnvelopeFile(path)
	if err != nil {
		return nil, err
	}

	if env.Service != service {
		return nil, fmt.Errorf(
			"import file holds %q data but %q was selected; choose the matching service", env.Service, service)
	}

	scope, err := a.stagingScopeForKindScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	if env.Provider != string(scope.Provider) {
		return nil, fmt.Errorf(
			"import file provider %q does not match the current provider %q; a provider change cannot be imported",
			env.Provider, scope.Provider)
	}

	if env.Scope != scope.Key() && !force {
		return nil, fmt.Errorf(
			"import file scope %q does not match the current scope %q; confirm the scope mismatch to import anyway",
			env.Scope, scope.Key())
	}

	working, err := a.getStagingWorkingFileStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	importMode := stagingusecase.ImportModeMerge
	if mode == "overwrite" {
		importMode = stagingusecase.ImportModeOverwrite
	}

	uc := &stagingusecase.ImportUseCase{
		Source: &stagingEnvelopeReadSource{
			env:        env,
			passphrase: passphrase,
		},
		Working: working,
	}

	// A forced scope mismatch is a cross-scope import: the envelope's
	// BaseModifiedAt values track the source scope's timeline, so re-anchor them
	// against the target scope's current LastModified (mirrors the CLI).
	reAnchor := force && env.Scope != scope.Key()
	if reAnchor {
		uc.ReAnchor, err = a.stagingImportReAnchorResolverScoped(sc, service)
		if err != nil {
			return nil, err
		}
	}

	result, err := uc.Execute(a.ctx, stagingusecase.ImportInput{Service: svc, Mode: importMode, ReAnchor: reAnchor})
	if err != nil {
		return nil, err
	}

	return &StagingImportResult{
		Merged:     result.Merged,
		EntryCount: result.EntryCount,
		TagCount:   result.TagCount,
	}, nil
}

// stagingImportReAnchorResolverScoped builds the resolver a cross-scope import uses to
// fetch the target scope's current LastModified, from an already-snapshotted
// scope (#560). App Configuration (param) keeps all namespaces in one staging
// store, so it resolves a strategy per namespace like the apply path; every
// other service resolves a single strategy built once.
func (a *App) stagingImportReAnchorResolverScoped(sc provider.Scope, service string) (stagingusecase.ReAnchorResolver, error) {
	if service == string(staging.ServiceParam) && hasParamNamespaces(sc) {
		return func(_ staging.Service, namespace string) (staging.ApplyStrategy, error) {
			return a.paramStrategyForNamespaceScoped(sc, namespace)
		}, nil
	}

	strategy, err := a.strategyAsScoped[staging.ApplyStrategy](sc, service)
	if err != nil {
		return nil, err
	}

	return func(staging.Service, string) (staging.ApplyStrategy, error) {
		return strategy, nil
	}, nil
}
