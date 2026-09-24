//go:build production || dev

// The per-item staging bindings: add, edit, delete, unstage, and the tag
// changes.
//
// The staging*.go files are one set of App methods split by concern, so they
// share staging.go's namespace.
//declscope:namespace staging

package gui

import (
	"errors"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// StagingAdd stages a create operation for a new item.
//
// namespace selects the Azure App Configuration namespace to stage the create
// under (param service only); empty is the null/default namespace. Staging is
// per-(store, namespace), so aligning the scope here keeps the staged create in
// the same namespace it is applied to (#431). It must name a single concrete
// namespace — a filter value (`*` / `,`-list) is rejected. It is ignored for the
// secret service and for non-App-Configuration providers.
func (a *App) StagingAdd(service, name, value, namespace string) (*StagingAddResult, error) {
	sc := a.currentScope()

	store, err := a.getStagingStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	// For App Configuration the existence check must run under the target
	// namespace, and the staged entry records that namespace as part of its
	// identity; other providers ignore it.
	strategy, namespace, err := a.stagingEditStrategyForNamespace(sc, service, namespace)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.AddUseCase{
		Strategy: strategy,
		Store:    store,
	}

	result, err := uc.Execute(a.ctx, stagingusecase.AddInput{
		Key:   staging.EntryKey{Name: name, Namespace: namespace},
		Value: value,
	})
	if err != nil {
		return nil, err
	}

	return &StagingAddResult{Name: result.Name}, nil
}

// stagingEditStrategyForNamespace returns the edit strategy for a staged entry and the
// validated namespace. For the App Configuration param service the strategy is
// scoped to the target namespace (rejecting a `*`/`,` filter value); for every
// other service the base strategy is returned and the namespace is empty.
func (a *App) stagingEditStrategyForNamespace(sc provider.Scope, service, namespace string) (staging.EditStrategy, string, error) {
	if service == string(staging.ServiceParam) && hasParamNamespaces(sc) {
		literal, err := a.validateParamNamespaceScoped(sc, namespace)
		if err != nil {
			return nil, "", err
		}

		strategy, err := a.paramStrategyForNamespaceScoped(sc, literal)
		if err != nil {
			return nil, "", err
		}

		return strategy, literal, nil
	}

	strategy, err := a.strategyAsScoped[staging.EditStrategy](sc, service)
	if err != nil {
		return nil, "", err
	}

	return strategy, "", nil
}

// StagingEdit stages an update operation for an existing item. namespace selects
// the Azure App Configuration namespace of the setting (empty for the
// null/default namespace and every other provider).
func (a *App) StagingEdit(service, name, value, namespace string) (*StagingEditResult, error) {
	sc := a.currentScope()

	store, err := a.getStagingStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, namespace, err := a.stagingEditStrategyForNamespace(sc, service, namespace)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	result, err := uc.Execute(a.ctx, stagingusecase.EditInput{
		Key:   staging.EntryKey{Name: name, Namespace: namespace},
		Value: value,
	})
	if err != nil {
		return nil, err
	}

	return &StagingEditResult{Name: result.Name}, nil
}

// StagingDelete stages a delete operation for an existing item. namespace
// selects the Azure App Configuration namespace of the setting (empty for the
// null/default namespace and every other provider).
func (a *App) StagingDelete(service, name string, force bool, recoveryWindow int, namespace string) (*StagingDeleteResult, error) {
	sc := a.currentScope()

	store, err := a.getStagingStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	var strategy staging.DeleteStrategy

	if service == string(staging.ServiceParam) && hasParamNamespaces(sc) {
		namespace, err = a.validateParamNamespaceScoped(sc, namespace)
		if err != nil {
			return nil, err
		}

		strategy, err = a.paramStrategyForNamespaceScoped(sc, namespace)
	} else {
		namespace = ""
		strategy, err = a.strategyAsScoped[staging.DeleteStrategy](sc, service)
	}

	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}

	result, err := uc.Execute(a.ctx, stagingusecase.DeleteInput{
		Key:            staging.EntryKey{Name: name, Namespace: namespace},
		Force:          force,
		RecoveryWindow: recoveryWindow,
	})
	if err != nil {
		return nil, err
	}

	return &StagingDeleteResult{Name: result.Name}, nil
}

// StagingUnstage removes an item from staging (both entry and tags). namespace
// selects the Azure App Configuration namespace of the entry (empty for the
// null/default namespace and every other provider).
func (a *App) StagingUnstage(service, name, namespace string) (*StagingUnstageResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	key := staging.EntryKey{Name: name, Namespace: namespace}

	// Unstage entry (ignore ErrNotStaged)
	if err := store.UnstageEntry(a.ctx, svc, key); err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}

	// Unstage tags (ignore ErrNotStaged)
	if err := store.UnstageTag(a.ctx, svc, key); err != nil && !errors.Is(err, staging.ErrNotStaged) {
		return nil, err
	}

	return &StagingUnstageResult{Name: name}, nil
}

// StagingAddTag stages adding a tag to an item. namespace selects the Azure App
// Configuration namespace of the tagged setting (empty for the null/default
// namespace and every other provider); it scopes both the strategy and the
// staged tag's (name, namespace) key.
func (a *App) StagingAddTag(service, name, key, value, namespace string) (*StagingAddTagResult, error) {
	sc := a.currentScope()

	store, err := a.getStagingStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, ns, err := a.stagingEditStrategyForNamespace(sc, service, namespace)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	result, err := uc.Tag(a.ctx, stagingusecase.TagInput{
		Key:  staging.EntryKey{Name: name, Namespace: ns},
		Tags: map[string]string{key: value},
	})
	if err != nil {
		return nil, err
	}

	return &StagingAddTagResult{Name: result.Name}, nil
}

// StagingRemoveTag stages removing a tag from an item. namespace selects the
// Azure App Configuration namespace of the tagged setting (empty for the
// null/default namespace and every other provider).
func (a *App) StagingRemoveTag(service, name, key, namespace string) (*StagingRemoveTagResult, error) {
	sc := a.currentScope()

	store, err := a.getStagingStoreScoped(sc, kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, ns, err := a.stagingEditStrategyForNamespace(sc, service, namespace)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	result, err := uc.Untag(a.ctx, stagingusecase.UntagInput{
		Key:     staging.EntryKey{Name: name, Namespace: ns},
		TagKeys: maputil.NewSet(key),
	})
	if err != nil {
		return nil, err
	}

	return &StagingRemoveTagResult{Name: result.Name}, nil
}

// StagingCancelAddTag cancels a staged tag addition (removes from Add only).
// namespace selects the Azure App Configuration namespace of the tagged setting.
func (a *App) StagingCancelAddTag(service, name, key, namespace string) (*StagingCancelAddTagResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	entryKey := staging.EntryKey{Name: name, Namespace: namespace}

	// Get existing tag entry
	tagEntry, err := store.GetTag(a.ctx, svc, entryKey)
	if err != nil {
		return nil, err
	}

	// Remove key from Add
	delete(tagEntry.Add, key)

	// If tag entry has no meaningful content, unstage it
	if len(tagEntry.Add) == 0 && tagEntry.Remove.Len() == 0 {
		if err := store.UnstageTag(a.ctx, svc, entryKey); err != nil {
			return nil, err
		}
	} else {
		if err := store.StageTag(a.ctx, svc, entryKey, *tagEntry); err != nil {
			return nil, err
		}
	}

	return &StagingCancelAddTagResult{Name: name}, nil
}

// StagingCancelRemoveTag cancels a staged tag removal (removes from Remove only).
// namespace selects the Azure App Configuration namespace of the tagged setting.
func (a *App) StagingCancelRemoveTag(service, name, key, namespace string) (*StagingCancelRemoveTagResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	entryKey := staging.EntryKey{Name: name, Namespace: namespace}

	// Get existing tag entry
	tagEntry, err := store.GetTag(a.ctx, svc, entryKey)
	if err != nil {
		return nil, err
	}

	// Remove key from Remove set
	tagEntry.Remove.Remove(key)

	// If tag entry has no meaningful content, unstage it
	if len(tagEntry.Add) == 0 && tagEntry.Remove.Len() == 0 {
		if err := store.UnstageTag(a.ctx, svc, entryKey); err != nil {
			return nil, err
		}
	} else {
		if err := store.StageTag(a.ctx, svc, entryKey, *tagEntry); err != nil {
			return nil, err
		}
	}

	return &StagingCancelRemoveTagResult{Name: name}, nil
}
