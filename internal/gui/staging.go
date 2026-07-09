//go:build production || dev

package gui

import (
	"errors"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/timeutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// =============================================================================
// Staging Types
// =============================================================================

// StagingStatusResult represents the result of staging status.
type StagingStatusResult struct {
	Param      []StagingEntry    `json:"param"`
	Secret     []StagingEntry    `json:"secret"`
	ParamTags  []StagingTagEntry `json:"paramTags"`
	SecretTags []StagingTagEntry `json:"secretTags"`
}

// StagingEntry represents a staged entry change.
type StagingEntry struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace the entry is staged under
	// (empty for the null/default namespace and every other provider). The
	// frontend shows it as a badge and passes it back to unstage/edit/delete.
	Namespace string  `json:"namespace"`
	Operation string  `json:"operation"`
	Value     *string `json:"value,omitempty"`
	StagedAt  string  `json:"stagedAt"`
}

// StagingTagEntry represents a staged tag change.
type StagingTagEntry struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace of the tagged item (empty for
	// the null/default namespace and every other provider).
	Namespace  string            `json:"namespace"`
	AddTags    map[string]string `json:"addTags,omitempty"`
	RemoveTags []string          `json:"removeTags,omitempty"`
	StagedAt   string            `json:"stagedAt"`
}

// StagingApplyEntryResult represents a single entry apply result.
type StagingApplyEntryResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// StagingApplyTagResult represents a single tag apply result.
type StagingApplyTagResult struct {
	Name       string            `json:"name"`
	AddTags    map[string]string `json:"addTags,omitempty"`
	RemoveTags []string          `json:"removeTags,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// StagingApplyResult represents the result of applying staged changes.
type StagingApplyResult struct {
	ServiceName    string                    `json:"serviceName"`
	EntryResults   []StagingApplyEntryResult `json:"entryResults"`
	TagResults     []StagingApplyTagResult   `json:"tagResults"`
	Conflicts      []string                  `json:"conflicts,omitempty"`
	EntrySucceeded int                       `json:"entrySucceeded"`
	EntryFailed    int                       `json:"entryFailed"`
	TagSucceeded   int                       `json:"tagSucceeded"`
	TagFailed      int                       `json:"tagFailed"`
}

// StagingResetResult represents the result of resetting staged changes.
type StagingResetResult struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Count       int    `json:"count,omitempty"`
	ServiceName string `json:"serviceName"`
}

// StagingAddResult represents the result of staging an add operation.
type StagingAddResult struct {
	Name string `json:"name"`
}

// StagingEditResult represents the result of staging an edit operation.
type StagingEditResult struct {
	Name string `json:"name"`
}

// StagingDeleteResult represents the result of staging a delete operation.
type StagingDeleteResult struct {
	Name string `json:"name"`
}

// StagingUnstageResult represents the result of unstaging an item.
type StagingUnstageResult struct {
	Name string `json:"name"`
}

// StagingAddTagResult represents the result of staging a tag addition.
type StagingAddTagResult struct {
	Name string `json:"name"`
}

// StagingRemoveTagResult represents the result of staging a tag removal.
type StagingRemoveTagResult struct {
	Name string `json:"name"`
}

// StagingCancelAddTagResult represents the result of canceling a staged tag addition.
type StagingCancelAddTagResult struct {
	Name string `json:"name"`
}

// StagingCancelRemoveTagResult represents the result of canceling a staged tag removal.
type StagingCancelRemoveTagResult struct {
	Name string `json:"name"`
}

// StagingDiffResult represents the result of diffing staged changes.
type StagingDiffResult struct {
	ItemName   string                `json:"itemName"`
	Entries    []StagingDiffEntry    `json:"entries"`
	TagEntries []StagingDiffTagEntry `json:"tagEntries"`
}

// StagingDiffEntry represents a single diff entry. RemoteValue/RemoteIdentifier
// are the current value and version identifier on the provider being compared
// against (AWS today; the field names are provider-neutral so Google Cloud and
// Azure fit without another rename).
type StagingDiffEntry struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace of the entry (empty for the
	// null/default namespace and every other provider).
	Namespace        string  `json:"namespace"`
	Type             string  `json:"type"` // "normal", "create", "autoUnstaged", "warning"
	Operation        string  `json:"operation,omitempty"`
	RemoteValue      string  `json:"remoteValue,omitempty"`
	RemoteIdentifier string  `json:"remoteIdentifier,omitempty"`
	StagedValue      string  `json:"stagedValue,omitempty"`
	Description      *string `json:"description,omitempty"`
	Warning          string  `json:"warning,omitempty"`
}

// StagingDiffTagEntry represents a single diff tag entry.
type StagingDiffTagEntry struct {
	Name string `json:"name"`
	// Namespace is the App Configuration namespace of the tagged item (empty for
	// the null/default namespace and every other provider).
	Namespace  string            `json:"namespace"`
	AddTags    map[string]string `json:"addTags,omitempty"`
	RemoveTags map[string]string `json:"removeTags,omitempty"` // key=current value from AWS
}

// Enum → frontend-string lookup tables. Kept as immutable package-level maps so
// the conversion sites stay a single lookup instead of a repeated switch.
//
//nolint:gochecknoglobals // immutable enum→string lookup tables
var (
	applyStatusNames = map[stagingusecase.ApplyResultStatus]string{
		stagingusecase.ApplyResultCreated: "created",
		stagingusecase.ApplyResultUpdated: "updated",
		stagingusecase.ApplyResultDeleted: "deleted",
		stagingusecase.ApplyResultFailed:  "failed",
	}

	resetTypeNames = map[stagingusecase.ResetResultType]string{
		stagingusecase.ResetResultUnstaged:      "unstaged",
		stagingusecase.ResetResultUnstagedAll:   "unstagedAll",
		stagingusecase.ResetResultRestored:      "restored",
		stagingusecase.ResetResultNotStaged:     "notStaged",
		stagingusecase.ResetResultNothingStaged: "nothingStaged",
		stagingusecase.ResetResultSkipped:       "skipped",
	}

	diffEntryTypeNames = map[stagingusecase.DiffEntryType]string{
		stagingusecase.DiffEntryNormal:       "normal",
		stagingusecase.DiffEntryCreate:       "create",
		stagingusecase.DiffEntryAutoUnstaged: "autoUnstaged",
		stagingusecase.DiffEntryWarning:      "warning",
	}
)

// =============================================================================
// Staging Methods
// =============================================================================

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
		store, err := a.getStagingStore(provider.KindParam)
		if err != nil {
			return nil, err
		}

		parser, _ := a.getParser(string(staging.ServiceParam))

		paramResult, err = (&stagingusecase.StatusUseCase{Strategy: parser, Store: store}).Execute(a.ctx, stagingusecase.StatusInput{})
		if err != nil {
			return nil, err
		}
	}

	if scope.SupportsService(provider.KindSecret) {
		store, err := a.getStagingStore(provider.KindSecret)
		if err != nil {
			return nil, err
		}

		parser, _ := a.getParser(string(staging.ServiceSecret))

		secretResult, err = (&stagingusecase.StatusUseCase{Strategy: parser, Store: store}).Execute(a.ctx, stagingusecase.StatusInput{})
		if err != nil {
			return nil, err
		}
	}

	return &StagingStatusResult{
		Param:      toStagingEntries(statusEntries(paramResult)),
		Secret:     toStagingEntries(statusEntries(secretResult)),
		ParamTags:  toStagingTagEntries(statusTagEntries(paramResult)),
		SecretTags: toStagingTagEntries(statusTagEntries(secretResult)),
	}, nil
}

// statusEntries / statusTagEntries safely read a (possibly nil, when the
// service is unsupported by the active scope) StatusOutput.
func statusEntries(o *stagingusecase.StatusOutput) []stagingusecase.StatusEntry {
	if o == nil {
		return nil
	}

	return o.Entries
}

func statusTagEntries(o *stagingusecase.StatusOutput) []stagingusecase.StatusTagEntry {
	if o == nil {
		return nil
	}

	return o.TagEntries
}

// toStagingEntries converts use-case status entries into the frontend DTO,
// formatting timestamps as RFC3339.
func toStagingEntries(entries []stagingusecase.StatusEntry) []StagingEntry {
	out := make([]StagingEntry, len(entries))
	for i, e := range entries {
		out[i] = StagingEntry{
			Name:      e.Name,
			Namespace: e.Namespace,
			Operation: string(e.Operation),
			Value:     e.Value,
			StagedAt:  timeutil.FormatRFC3339(e.StagedAt),
		}
	}

	return out
}

// toStagingTagEntries converts use-case status tag entries into the frontend
// DTO, formatting timestamps as RFC3339.
func toStagingTagEntries(tags []stagingusecase.StatusTagEntry) []StagingTagEntry {
	out := make([]StagingTagEntry, len(tags))
	for i, t := range tags {
		out[i] = StagingTagEntry{
			Name:       t.Name,
			Namespace:  t.Namespace,
			AddTags:    t.Add,
			RemoveTags: t.Remove.Values(),
			StagedAt:   timeutil.FormatRFC3339(t.StagedAt),
		}
	}

	return out
}

// StagingApply applies staged changes for a service.
func (a *App) StagingApply(service string, ignoreConflicts bool) (*StagingApplyResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, err := a.getApplyStrategy(service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// App Configuration stages entries across namespaces in one store; apply each
	// under its own namespace via a namespace-scoped strategy.
	if service == string(staging.ServiceParam) && a.isAppConfigParam() {
		uc.StrategyFor = func(namespace string) (staging.ApplyStrategy, error) {
			return a.appConfigParamStrategyForNamespace(namespace)
		}
	}

	result, err := uc.Execute(a.ctx, stagingusecase.ApplyInput{
		IgnoreConflicts: ignoreConflicts,
	})
	if err != nil {
		return nil, err
	}

	output := &StagingApplyResult{
		ServiceName:    result.ServiceName,
		Conflicts:      result.Conflicts,
		EntrySucceeded: result.EntrySucceeded,
		EntryFailed:    result.EntryFailed,
		TagSucceeded:   result.TagSucceeded,
		TagFailed:      result.TagFailed,
	}

	for _, r := range result.EntryResults {
		entry := StagingApplyEntryResult{
			Name:   r.Name,
			Status: applyStatusNames[r.Status],
		}
		if r.Status == stagingusecase.ApplyResultFailed && r.Error != nil {
			entry.Error = r.Error.Error()
		}

		output.EntryResults = append(output.EntryResults, entry)
	}

	for _, r := range result.TagResults {
		tagResult := StagingApplyTagResult{
			Name:       r.Name,
			AddTags:    r.AddTags,
			RemoveTags: r.RemoveTag.Values(),
		}
		if r.Error != nil {
			tagResult.Error = r.Error.Error()
		}

		output.TagResults = append(output.TagResults, tagResult)
	}

	return output, nil
}

// StagingReset resets (unstages) all staged changes for a service.
func (a *App) StagingReset(service string) (*StagingResetResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	parser, err := a.getParser(service)
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
		Type:        resetTypeNames[result.Type],
	}, nil
}

// StagingAdd stages a create operation for a new item.
//
// namespace selects the Azure App Configuration namespace to stage the create
// under (param service only); empty is the null/default namespace. Staging is
// per-(store, namespace), so aligning the scope here keeps the staged create in
// the same namespace it is applied to (#431). It must name a single concrete
// namespace — a filter value (`*` / `,`-list) is rejected. It is ignored for the
// secret service and for non-App-Configuration providers.
func (a *App) StagingAdd(service, name, value, namespace string) (*StagingAddResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	// For App Configuration the existence check must run under the target
	// namespace, and the staged entry records that namespace as part of its
	// identity; other providers ignore it.
	strategy, namespace, err := a.editStrategyForNamespace(service, namespace)
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

// editStrategyForNamespace returns the edit strategy for a staged entry and the
// validated namespace. For the App Configuration param service the strategy is
// scoped to the target namespace (rejecting a `*`/`,` filter value); for every
// other service the base strategy is returned and the namespace is empty.
func (a *App) editStrategyForNamespace(service, namespace string) (staging.EditStrategy, string, error) {
	if service == string(staging.ServiceParam) && a.isAppConfigParam() {
		literal, err := a.validateParamNamespace(namespace)
		if err != nil {
			return nil, "", err
		}

		strategy, err := a.appConfigParamStrategyForNamespace(literal)
		if err != nil {
			return nil, "", err
		}

		return strategy, literal, nil
	}

	strategy, err := a.getEditStrategy(service)
	if err != nil {
		return nil, "", err
	}

	return strategy, "", nil
}

// StagingEdit stages an update operation for an existing item. namespace selects
// the Azure App Configuration namespace of the setting (empty for the
// null/default namespace and every other provider).
func (a *App) StagingEdit(service, name, value, namespace string) (*StagingEditResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, namespace, err := a.editStrategyForNamespace(service, namespace)
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
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	var strategy staging.DeleteStrategy

	if service == string(staging.ServiceParam) && a.isAppConfigParam() {
		namespace, err = a.validateParamNamespace(namespace)
		if err != nil {
			return nil, err
		}

		strategy, err = a.appConfigParamStrategyForNamespace(namespace)
	} else {
		namespace = ""
		strategy, err = a.getDeleteStrategy(service)
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
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, ns, err := a.editStrategyForNamespace(service, namespace)
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
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, ns, err := a.editStrategyForNamespace(service, namespace)
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

// StagingDiff shows diff between staged changes and AWS.
func (a *App) StagingDiff(service string, name string) (*StagingDiffResult, error) {
	store, err := a.getStagingStore(kindForService(service))
	if err != nil {
		return nil, err
	}

	strategy, err := a.getDiffStrategy(service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.DiffUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// App Configuration diffs each staged entry against its own namespace.
	if service == string(staging.ServiceParam) && a.isAppConfigParam() {
		uc.StrategyFor = func(namespace string) (staging.DiffStrategy, error) {
			return a.appConfigParamStrategyForNamespace(namespace)
		}
	}

	result, err := uc.Execute(a.ctx, stagingusecase.DiffInput{Name: name})
	if err != nil {
		return nil, err
	}

	entries := make([]StagingDiffEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = StagingDiffEntry{
			Name:             e.Name,
			Namespace:        e.Namespace,
			Type:             diffEntryTypeNames[e.Type],
			Operation:        string(e.Operation),
			RemoteValue:      e.AWSValue,
			RemoteIdentifier: e.AWSIdentifier,
			StagedValue:      e.StagedValue,
			Description:      e.Description,
			Warning:          e.Warning,
		}
	}

	tagEntries := make([]StagingDiffTagEntry, len(result.TagEntries))
	for i, t := range result.TagEntries {
		tagEntries[i] = StagingDiffTagEntry{
			Name:       t.Name,
			Namespace:  t.Namespace,
			AddTags:    t.Add,
			RemoveTags: t.Remove,
		}
	}

	return &StagingDiffResult{
		ItemName:   result.ItemName,
		Entries:    entries,
		TagEntries: tagEntries,
	}, nil
}

// =============================================================================
// Drain/Persist Types
// =============================================================================

// StagingDrainResult represents the result of draining from the stash file to the working staging area.
type StagingDrainResult struct {
	Merged     bool `json:"merged"`
	EntryCount int  `json:"entryCount"`
	TagCount   int  `json:"tagCount"`
}

// StagingPersistResult represents the result of persisting from the working staging area to the stash file.
type StagingPersistResult struct {
	EntryCount int `json:"entryCount"`
	TagCount   int `json:"tagCount"`
}

// StagingFileStatusResult represents the status of the staging file.
type StagingFileStatusResult struct {
	Exists    bool `json:"exists"`
	Encrypted bool `json:"encrypted"`
}

// =============================================================================
// Drain/Persist Methods
// =============================================================================

// StagingFileStatus checks if the staging file exists and whether it's encrypted.
func (a *App) StagingFileStatus() (*StagingFileStatusResult, error) {
	scope, err := a.stagingScope()
	if err != nil {
		return nil, err
	}

	fileStore, err := file.NewStashStore(scope)
	if err != nil {
		return nil, err
	}

	exists, err := fileStore.Exists()
	if err != nil {
		return nil, err
	}

	result := &StagingFileStatusResult{
		Exists: exists,
	}

	if exists {
		encrypted, err := fileStore.IsEncrypted()
		if err != nil {
			return nil, err
		}

		result.Encrypted = encrypted
	}

	return result, nil
}

// stashStores builds the stash (stash.json) and working (param.json/secret.json) file
// stores for the active provider's staging scope and resolves the optional
// service selector. It is the shared prelude of StagingDrain and StagingPersist.
// An empty service yields the zero staging.Service (all services).
func (a *App) stashStores(service, passphrase string) (stash, working *file.Store, svc staging.Service, err error) {
	scope, err := a.stagingScope()
	if err != nil {
		return nil, nil, "", err
	}

	stash, err = file.NewStashStoreWithPassphrase(scope, passphrase)
	if err != nil {
		return nil, nil, "", err
	}

	working, err = file.NewWorkingStore(scope)
	if err != nil {
		return nil, nil, "", err
	}

	if service != "" {
		svc, err = a.getService(service)
		if err != nil {
			return nil, nil, "", err
		}
	}

	return stash, working, svc, nil
}

// StagingDrain loads staged changes from the stash file into the working staging area.
// If the file is encrypted, passphrase must be provided.
// mode: "overwrite" or "merge" (default)
func (a *App) StagingDrain(service string, passphrase string, keep bool, mode string) (*StagingDrainResult, error) {
	stashStore, working, svc, err := a.stashStores(service, passphrase)
	if err != nil {
		return nil, err
	}

	drainMode := stagingusecase.StashModeMerge
	if mode == "overwrite" {
		drainMode = stagingusecase.StashModeOverwrite
	}

	uc := &stagingusecase.StashPopUseCase{
		Stash:   stashStore,
		Working: working,
	}

	result, err := uc.Execute(a.ctx, stagingusecase.StashPopInput{
		Service: svc,
		Keep:    keep,
		Mode:    drainMode,
	})
	if err != nil {
		return nil, err
	}

	return &StagingDrainResult{
		Merged:     result.Merged,
		EntryCount: result.EntryCount,
		TagCount:   result.TagCount,
	}, nil
}

// StagingPersist saves staged changes from the working staging area to the stash file.
// If passphrase is provided, the file will be encrypted.
// mode: "overwrite" or "merge"
func (a *App) StagingPersist(service string, passphrase string, keep bool, mode string) (*StagingPersistResult, error) {
	stashStore, working, svc, err := a.stashStores(service, passphrase)
	if err != nil {
		return nil, err
	}

	persistMode := stagingusecase.StashModeOverwrite
	if mode == "merge" {
		persistMode = stagingusecase.StashModeMerge
	}

	uc := &stagingusecase.StashPushUseCase{
		Working: working,
		Stash:   stashStore,
	}

	result, err := uc.Execute(a.ctx, stagingusecase.StashPushInput{
		Service: svc,
		Keep:    keep,
		Mode:    persistMode,
	})
	if err != nil {
		return nil, err
	}

	return &StagingPersistResult{
		EntryCount: result.EntryCount,
		TagCount:   result.TagCount,
	}, nil
}

// StagingDropResult represents the result of dropping the stash file.
type StagingDropResult struct {
	Dropped bool `json:"dropped"`
}

// StagingDrop deletes the staging file without loading it into memory.
// This works even for encrypted files since it just deletes the file directly.
func (a *App) StagingDrop() (*StagingDropResult, error) {
	scope, err := a.stagingScope()
	if err != nil {
		return nil, err
	}

	fileStore, err := file.NewStashStore(scope)
	if err != nil {
		return nil, err
	}

	exists, err := fileStore.Exists()
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, errors.New("no stashed changes to drop")
	}

	// Delete the file directly without reading its contents
	// This allows dropping encrypted files without passphrase
	if err := fileStore.Delete(); err != nil {
		return nil, err
	}

	return &StagingDropResult{
		Dropped: true,
	}, nil
}
