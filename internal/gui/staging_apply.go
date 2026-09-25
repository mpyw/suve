//go:build production || dev

// The bindings that act on a whole service's staged changes: apply and reset.
//
// The staging*.go files are one set of App methods split by concern, so they
// share staging.go's namespace.
//declscope:namespace staging

package gui

import (
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// StagingApply applies staged changes for a service.
func (a *App) StagingApply(service string, ignoreConflicts bool) (*StagingApplyResult, error) {
	uc, err := a.stagingApplyUseCase(a.currentScope(), service)
	if err != nil {
		return nil, err
	}

	result, err := uc.Execute(a.ctx, stagingusecase.ApplyInput{
		IgnoreConflicts: ignoreConflicts,
	})
	// A conflict rejection or a per-entry/tag failure returns a POPULATED result
	// alongside the error; the failure detail lives in the result's fields
	// (Conflicts, EntryFailed, TagFailed, per-entry Error). Surface that result so
	// the frontend can render the conflict panel and per-entry rows instead of
	// discarding everything into a bare error message, mirroring the CLI's
	// apply.go (which prints the result before returning the error). Only a nil
	// result (e.g. a store read failure) is a hard error with nothing to show.
	if result == nil {
		return nil, err
	}

	return newStagingApplyResult(result), nil
}

// StagingApplyAll applies the staged changes of every service the current scope
// supports as one operation, through the same GlobalApplyUseCase as the CLI's
// all-service `stage apply`: every service is conflict-checked before any is
// applied, so a conflict in one service leaves every service unapplied and
// staged. Like StagingApply, a conflict rejection or a per-entry failure returns
// the populated result with a nil error; only a hard failure (nothing applied,
// nothing to show) is an error.
func (a *App) StagingApplyAll(ignoreConflicts bool) (*StagingApplyResult, error) {
	sc := a.currentScope()

	uc := &stagingusecase.GlobalApplyUseCase{}

	for _, service := range []staging.Service{staging.ServiceParam, staging.ServiceSecret} {
		if !sc.SupportsService(kindForService(string(service))) {
			continue
		}

		// Like the CLI, initialize a provider client only for a service with
		// staged changes, so an unused service cannot block the apply.
		staged, err := a.stagingHasChanges(sc, service)
		if err != nil {
			return nil, err
		}

		if !staged {
			continue
		}

		svc, err := a.stagingApplyUseCase(sc, string(service))
		if err != nil {
			return nil, err
		}

		uc.Services = append(uc.Services, svc)
	}

	result, err := uc.Execute(a.ctx, stagingusecase.GlobalApplyInput{IgnoreConflicts: ignoreConflicts})
	if result == nil {
		return nil, err
	}

	return newStagingApplyAllResult(result), nil
}

// stagingHasChanges reports whether the service has any staged entry or tag
// change in its staging store.
func (a *App) stagingHasChanges(sc provider.Scope, service staging.Service) (bool, error) {
	store, err := a.getStagingStoreScoped(sc, kindForService(string(service)))
	if err != nil {
		return false, err
	}

	entries, err := store.ListEntries(a.ctx, service)
	if err != nil {
		return false, err
	}

	tags, err := store.ListTags(a.ctx, service)
	if err != nil {
		return false, err
	}

	return len(entries[service]) > 0 || len(tags[service]) > 0, nil
}

// stagingApplyUseCase builds the per-service apply use case over the service's
// staging store and strategy, both resolved from the same snapshotted scope.
func (a *App) stagingApplyUseCase(sc provider.Scope, service string) (*stagingusecase.ApplyUseCase, error) {
	store, err := a.getStagingStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, err := a.strategyAsScoped[staging.ApplyStrategy](sc, service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// App Configuration stages entries across namespaces in one store; apply each
	// under its own namespace via a namespace-scoped strategy.
	if service == string(staging.ServiceParam) && hasParamNamespaces(sc) {
		uc.StrategyFor = func(namespace string) (staging.ApplyStrategy, error) {
			return a.paramStrategyForNamespaceScoped(sc, namespace)
		}
	}

	return uc, nil
}

// newStagingApplyAllResult merges the all-service apply's per-service outputs
// into one frontend DTO. Each conflict is prefixed with its service name, since
// two services may stage the same name.
func newStagingApplyAllResult(result *stagingusecase.GlobalApplyOutput) *StagingApplyResult {
	output := &StagingApplyResult{
		Conflicts: lo.Map(result.Conflicts, func(c stagingusecase.GlobalApplyConflict, _ int) string {
			return c.ServiceName + ": " + c.Key.Label()
		}),
	}

	for _, svc := range result.Services {
		r := newStagingApplyResult(svc)
		output.EntryResults = append(output.EntryResults, r.EntryResults...)
		output.TagResults = append(output.TagResults, r.TagResults...)
		output.EntrySucceeded += r.EntrySucceeded
		output.EntryFailed += r.EntryFailed
		output.TagSucceeded += r.TagSucceeded
		output.TagFailed += r.TagFailed
	}

	return output
}

// newStagingApplyResult maps the apply use case's output into the frontend DTO.
// A post-apply unstage failure (UnstageError: the cloud write succeeded but the
// entry/tag could not be cleared from staging) is carried through so the
// frontend can warn, mirroring the CLI (internal/staging/cli/apply.go).
func newStagingApplyResult(result *stagingusecase.ApplyOutput) *StagingApplyResult {
	// Render each conflict's EntryKey with its namespace badge (bare name for the
	// empty/default namespace, so AWS/GCloud/Key Vault output is unchanged).
	conflicts := lo.Map(result.Conflicts, func(key staging.EntryKey, _ int) string { return key.Label() })

	output := &StagingApplyResult{
		ServiceName:    result.ServiceName,
		Conflicts:      conflicts,
		EntrySucceeded: result.EntrySucceeded,
		EntryFailed:    result.EntryFailed,
		TagSucceeded:   result.TagSucceeded,
		TagFailed:      result.TagFailed,
	}

	output.EntryResults = lo.Map(result.EntryResults, func(r stagingusecase.ApplyEntryResult, _ int) StagingApplyEntryResult {
		entry := StagingApplyEntryResult{
			Name:      r.Name,
			Namespace: r.Namespace,
			Status:    stagingApplyStatusNames[r.Status],
		}
		if r.Status == stagingusecase.ApplyResultFailed && r.Error != nil {
			entry.Error = r.Error.Error()
		}

		if r.UnstageError != nil {
			entry.UnstageError = r.UnstageError.Error()
		}

		return entry
	})

	output.TagResults = lo.Map(result.TagResults, func(r stagingusecase.ApplyTagResult, _ int) StagingApplyTagResult {
		tagResult := StagingApplyTagResult{
			Name:       r.Name,
			Namespace:  r.Namespace,
			AddTags:    r.AddTags,
			RemoveTags: r.RemoveTag.Values(),
		}
		if r.Error != nil {
			tagResult.Error = r.Error.Error()
		}

		if r.UnstageError != nil {
			tagResult.UnstageError = r.UnstageError.Error()
		}

		return tagResult
	})

	return output
}

// StagingReset resets (unstages) all staged changes for a service.
func (a *App) StagingReset(service string) (*StagingResetResult, error) {
	sc := a.currentScope()

	store, err := a.getStagingStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	parser, err := a.getParserScoped(sc, service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.ResetUseCase{
		Parser: parser,
		Store:  store,
	}

	result, err := uc.Execute(a.ctx, stagingusecase.ResetInput{All: true})
	if err != nil {
		return nil, err
	}

	return &StagingResetResult{
		ServiceName: result.ServiceName,
		Name:        result.Name,
		Count:       result.Count,
		Type:        stagingResetTypeNames[result.Type],
	}, nil
}
