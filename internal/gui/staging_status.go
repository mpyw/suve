//go:build production || dev

// The read side of the staging bindings: status, per-item check, and diff.
//
// The staging*.go files are one set of App methods split by concern, so they
// share staging.go's namespace.
//declscope:namespace staging

package gui

import (
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/timeutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// StagingStatus gets the current staging status. Only the services the active
// scope supports are queried (Google Cloud has no param service); unsupported
// services yield empty slices so the capability-gated frontend renders nothing.
func (a *App) StagingStatus() (*StagingStatusResult, error) {
	scope := a.currentScope()

	var paramResult, secretResult *stagingusecase.StatusOutput

	// Each service reads its OWN staging store: Azure's param (App Configuration)
	// and secret (Key Vault) live in separate on-disk buckets, so a single shared
	// store would read only one of them (and the wrong key). See stagingScopeForKind.
	if scope.SupportsService(provider.KindParam) {
		store, err := a.getStagingStoreScoped(scope, provider.KindParam)
		if err != nil {
			return nil, err
		}

		parser, _ := a.getParserScoped(scope, string(staging.ServiceParam))

		paramResult, err = (&stagingusecase.StatusUseCase{Strategy: parser, Store: store}).Execute(a.ctx, stagingusecase.StatusInput{})
		if err != nil {
			return nil, err
		}
	}

	if scope.SupportsService(provider.KindSecret) {
		store, err := a.getStagingStoreScoped(scope, provider.KindSecret)
		if err != nil {
			return nil, err
		}

		parser, _ := a.getParserScoped(scope, string(staging.ServiceSecret))

		secretResult, err = (&stagingusecase.StatusUseCase{Strategy: parser, Store: store}).Execute(a.ctx, stagingusecase.StatusInput{})
		if err != nil {
			return nil, err
		}
	}

	return &StagingStatusResult{
		Param:      toStagingEntries(stagingStatusEntries(paramResult)),
		Secret:     toStagingEntries(stagingStatusEntries(secretResult)),
		ParamTags:  toStagingTagEntries(stagingStatusTagEntries(paramResult)),
		SecretTags: toStagingTagEntries(stagingStatusTagEntries(secretResult)),
	}, nil
}

// stagingStatusEntries / stagingStatusTagEntries safely read a (possibly nil, when the
// service is unsupported by the active scope) StatusOutput.
func stagingStatusEntries(o *stagingusecase.StatusOutput) []stagingusecase.StatusEntry {
	if o == nil {
		return nil
	}

	return o.Entries
}

func stagingStatusTagEntries(o *stagingusecase.StatusOutput) []stagingusecase.StatusTagEntry {
	if o == nil {
		return nil
	}

	return o.TagEntries
}

// toStagingEntries converts use-case status entries into the frontend DTO,
// formatting timestamps as RFC3339.
func toStagingEntries(entries []stagingusecase.StatusEntry) []StagingEntry {
	return lo.Map(entries, func(e stagingusecase.StatusEntry, _ int) StagingEntry {
		return StagingEntry{
			Name:      e.Name,
			Namespace: e.Namespace,
			Operation: string(e.Operation),
			Value:     e.Value,
			StagedAt:  timeutil.FormatRFC3339(e.StagedAt),
		}
	})
}

// toStagingTagEntries converts use-case status tag entries into the frontend
// DTO, formatting timestamps as RFC3339.
func toStagingTagEntries(tags []stagingusecase.StatusTagEntry) []StagingTagEntry {
	return lo.Map(tags, func(t stagingusecase.StatusTagEntry, _ int) StagingTagEntry {
		return StagingTagEntry{
			Name:       t.Name,
			Namespace:  t.Namespace,
			AddTags:    t.Add,
			RemoveTags: t.Remove.Values(),
			StagedAt:   timeutil.FormatRFC3339(t.StagedAt),
		}
	})
}

// StagingCheckStatusResult holds the result of checking staged status for an item.
type StagingCheckStatusResult struct {
	HasEntry bool `json:"hasEntry"`
	HasTags  bool `json:"hasTags"`
}

// StagingCheckStatus checks if a specific item has staged entry or tag changes.
func (a *App) StagingCheckStatus(service, name, namespace string) (*StagingCheckStatusResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	result := &StagingCheckStatusResult{}
	key := staging.EntryKey{Name: name, Namespace: namespace}

	// Check for staged entry
	if _, err := store.GetEntry(a.ctx, svc, key); err == nil {
		result.HasEntry = true
	}

	// Check for staged tags
	if _, err := store.GetTag(a.ctx, svc, key); err == nil {
		result.HasTags = true
	}

	return result, nil
}

// StagingDiff shows diff between staged changes and the provider's current values.
func (a *App) StagingDiff(service string, name string) (*StagingDiffResult, error) {
	sc := a.currentScope()

	store, err := a.getStagingStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, err := a.strategyAsScoped[staging.DiffStrategy](sc, service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// App Configuration diffs each staged entry against its own namespace.
	if service == string(staging.ServiceParam) && hasParamNamespaces(sc) {
		uc.StrategyFor = func(namespace string) (staging.DiffStrategy, error) {
			return a.paramStrategyForNamespaceScoped(sc, namespace)
		}
	}

	result, err := uc.Execute(a.ctx, stagingusecase.DiffInput{Name: name})
	if err != nil {
		return nil, err
	}

	entries := lo.Map(result.Entries, func(e stagingusecase.DiffEntry, _ int) StagingDiffEntry {
		return StagingDiffEntry{
			Name:             e.Name,
			Namespace:        e.Namespace,
			Type:             stagingDiffEntryTypeNames[e.Type],
			Operation:        string(e.Operation),
			RemoteValue:      e.RemoteValue,
			RemoteIdentifier: e.RemoteIdentifier,
			StagedValue:      e.StagedValue,
			Description:      e.Description,
			Warning:          e.Warning,
			Secret:           e.Secret,
		}
	})

	tagEntries := lo.Map(result.TagEntries, func(t stagingusecase.DiffTagEntry, _ int) StagingDiffTagEntry {
		return StagingDiffTagEntry{
			Name:       t.Name,
			Namespace:  t.Namespace,
			AddTags:    t.Add,
			RemoveTags: t.Remove,
		}
	})

	return &StagingDiffResult{
		ItemName:   result.ItemName,
		Entries:    entries,
		TagEntries: tagEntries,
	}, nil
}
