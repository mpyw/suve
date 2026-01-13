//go:build production || dev

package gui

import (
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
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
	Name      string  `json:"name"`
	Operation string  `json:"operation"`
	Value     *string `json:"value,omitempty"`
	StagedAt  string  `json:"stagedAt"`
}

// StagingTagEntry represents a staged tag change.
type StagingTagEntry struct {
	Name       string            `json:"name"`
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

// StagingDiffEntry represents a single diff entry.
type StagingDiffEntry struct {
	Name          string  `json:"name"`
	Type          string  `json:"type"` // "normal", "create", "autoUnstaged", "warning"
	Operation     string  `json:"operation,omitempty"`
	AWSValue      string  `json:"awsValue,omitempty"`
	AWSIdentifier string  `json:"awsIdentifier,omitempty"`
	StagedValue   string  `json:"stagedValue,omitempty"`
	Description   *string `json:"description,omitempty"`
	Warning       string  `json:"warning,omitempty"`
}

// StagingDiffTagEntry represents a single diff tag entry.
type StagingDiffTagEntry struct {
	Name       string            `json:"name"`
	AddTags    map[string]string `json:"addTags,omitempty"`
	RemoveTags []string          `json:"removeTags,omitempty"`
}

// =============================================================================
// Staging Methods
// =============================================================================

// StagingStatus gets the current staging status.
func (a *App) StagingStatus() (*StagingStatusResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	paramParser, _ := a.getParser(string(staging.ServiceParam))
	secretParser, _ := a.getParser(string(staging.ServiceSecret))

	// SSM Parameter Store status
	paramUC := &stagingusecase.StatusUseCase{
		Strategy: paramParser,
		Store:    store,
	}
	paramResult, err := paramUC.Execute(a.ctx, stagingusecase.StatusInput{})
	if err != nil {
		return nil, err
	}

	// Secrets Manager status
	secretUC := &stagingusecase.StatusUseCase{
		Strategy: secretParser,
		Store:    store,
	}
	secretResult, err := secretUC.Execute(a.ctx, stagingusecase.StatusInput{})
	if err != nil {
		return nil, err
	}

	result := &StagingStatusResult{
		Param:      make([]StagingEntry, len(paramResult.Entries)),
		Secret:     make([]StagingEntry, len(secretResult.Entries)),
		ParamTags:  make([]StagingTagEntry, len(paramResult.TagEntries)),
		SecretTags: make([]StagingTagEntry, len(secretResult.TagEntries)),
	}

	for i, e := range paramResult.Entries {
		result.Param[i] = StagingEntry{
			Name:      e.Name,
			Operation: string(e.Operation),
			Value:     e.Value,
			StagedAt:  e.StagedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	for i, e := range secretResult.Entries {
		result.Secret[i] = StagingEntry{
			Name:      e.Name,
			Operation: string(e.Operation),
			Value:     e.Value,
			StagedAt:  e.StagedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	for i, t := range paramResult.TagEntries {
		result.ParamTags[i] = StagingTagEntry{
			Name:       t.Name,
			AddTags:    t.Add,
			RemoveTags: t.Remove.Values(),
			StagedAt:   t.StagedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	for i, t := range secretResult.TagEntries {
		result.SecretTags[i] = StagingTagEntry{
			Name:       t.Name,
			AddTags:    t.Add,
			RemoveTags: t.Remove.Values(),
			StagedAt:   t.StagedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return result, nil
}

// StagingApply applies staged changes for a service.
func (a *App) StagingApply(service string, ignoreConflicts bool) (*StagingApplyResult, error) {
	store, err := a.getStagingStore()
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
	result, err := uc.Execute(a.ctx, stagingusecase.ApplyInput{
		IgnoreConflicts: ignoreConflicts,
	})

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
			Name: r.Name,
		}
		switch r.Status {
		case stagingusecase.ApplyResultCreated:
			entry.Status = "created"
		case stagingusecase.ApplyResultUpdated:
			entry.Status = "updated"
		case stagingusecase.ApplyResultDeleted:
			entry.Status = "deleted"
		case stagingusecase.ApplyResultFailed:
			entry.Status = "failed"
			if r.Error != nil {
				entry.Error = r.Error.Error()
			}
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

	return output, err
}

// StagingReset resets (unstages) all staged changes for a service.
func (a *App) StagingReset(service string) (*StagingResetResult, error) {
	store, err := a.getStagingStore()
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

	output := &StagingResetResult{
		ServiceName: result.ServiceName,
		Name:        result.Name,
		Count:       result.Count,
	}

	switch result.Type {
	case stagingusecase.ResetResultUnstaged:
		output.Type = "unstaged"
	case stagingusecase.ResetResultUnstagedAll:
		output.Type = "unstagedAll"
	case stagingusecase.ResetResultRestored:
		output.Type = "restored"
	case stagingusecase.ResetResultNotStaged:
		output.Type = "notStaged"
	case stagingusecase.ResetResultNothingStaged:
		output.Type = "nothingStaged"
	}

	return output, nil
}

// StagingAdd stages a create operation for a new item.
func (a *App) StagingAdd(service, name, value string) (*StagingAddResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	strategy, err := a.getEditStrategy(service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.AddUseCase{
		Strategy: strategy,
		Store:    store,
	}
	result, err := uc.Execute(a.ctx, stagingusecase.AddInput{
		Name:  name,
		Value: value,
	})
	if err != nil {
		return nil, err
	}

	return &StagingAddResult{Name: result.Name}, nil
}

// StagingEdit stages an update operation for an existing item.
func (a *App) StagingEdit(service, name, value string) (*StagingEditResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	strategy, err := a.getEditStrategy(service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}
	result, err := uc.Execute(a.ctx, stagingusecase.EditInput{
		Name:  name,
		Value: value,
	})
	if err != nil {
		return nil, err
	}

	return &StagingEditResult{Name: result.Name}, nil
}

// StagingDelete stages a delete operation for an existing item.
func (a *App) StagingDelete(service, name string, force bool, recoveryWindow int) (*StagingDeleteResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	strategy, err := a.getDeleteStrategy(service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.DeleteUseCase{
		Strategy: strategy,
		Store:    store,
	}
	result, err := uc.Execute(a.ctx, stagingusecase.DeleteInput{
		Name:           name,
		Force:          force,
		RecoveryWindow: recoveryWindow,
	})
	if err != nil {
		return nil, err
	}

	return &StagingDeleteResult{Name: result.Name}, nil
}

// StagingUnstage removes an item from staging (both entry and tags).
func (a *App) StagingUnstage(service, name string) (*StagingUnstageResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	// Unstage entry (ignore ErrNotStaged)
	if err := store.UnstageEntry(a.ctx, svc, name); err != nil && err != staging.ErrNotStaged {
		return nil, err
	}

	// Unstage tags (ignore ErrNotStaged)
	if err := store.UnstageTag(a.ctx, svc, name); err != nil && err != staging.ErrNotStaged {
		return nil, err
	}

	return &StagingUnstageResult{Name: name}, nil
}

// StagingAddTag stages adding a tag to an item.
func (a *App) StagingAddTag(service, name, key, value string) (*StagingAddTagResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	strategy, err := a.getEditStrategy(service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}
	result, err := uc.Tag(a.ctx, stagingusecase.TagInput{
		Name: name,
		Tags: map[string]string{key: value},
	})
	if err != nil {
		return nil, err
	}

	return &StagingAddTagResult{Name: result.Name}, nil
}

// StagingRemoveTag stages removing a tag from an item.
func (a *App) StagingRemoveTag(service, name, key string) (*StagingRemoveTagResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	strategy, err := a.getEditStrategy(service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}
	result, err := uc.Untag(a.ctx, stagingusecase.UntagInput{
		Name:    name,
		TagKeys: maputil.NewSet(key),
	})
	if err != nil {
		return nil, err
	}

	return &StagingRemoveTagResult{Name: result.Name}, nil
}

// StagingCancelAddTag cancels a staged tag addition (removes from Add only).
func (a *App) StagingCancelAddTag(service, name, key string) (*StagingCancelAddTagResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	// Get existing tag entry
	tagEntry, err := store.GetTag(a.ctx, svc, name)
	if err != nil {
		return nil, err
	}

	// Remove key from Add
	delete(tagEntry.Add, key)

	// If tag entry has no meaningful content, unstage it
	if len(tagEntry.Add) == 0 && tagEntry.Remove.Len() == 0 {
		if err := store.UnstageTag(a.ctx, svc, name); err != nil {
			return nil, err
		}
	} else {
		if err := store.StageTag(a.ctx, svc, name, *tagEntry); err != nil {
			return nil, err
		}
	}

	return &StagingCancelAddTagResult{Name: name}, nil
}

// StagingCancelRemoveTag cancels a staged tag removal (removes from Remove only).
func (a *App) StagingCancelRemoveTag(service, name, key string) (*StagingCancelRemoveTagResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	// Get existing tag entry
	tagEntry, err := store.GetTag(a.ctx, svc, name)
	if err != nil {
		return nil, err
	}

	// Remove key from Remove set
	tagEntry.Remove.Remove(key)

	// If tag entry has no meaningful content, unstage it
	if len(tagEntry.Add) == 0 && tagEntry.Remove.Len() == 0 {
		if err := store.UnstageTag(a.ctx, svc, name); err != nil {
			return nil, err
		}
	} else {
		if err := store.StageTag(a.ctx, svc, name, *tagEntry); err != nil {
			return nil, err
		}
	}

	return &StagingCancelRemoveTagResult{Name: name}, nil
}

// StagingCheckStatus checks if a specific item has staged changes.
type StagingCheckStatusResult struct {
	HasEntry bool `json:"hasEntry"`
	HasTags  bool `json:"hasTags"`
}

// StagingCheckStatus checks if a specific item has staged entry or tag changes.
func (a *App) StagingCheckStatus(service, name string) (*StagingCheckStatusResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	svc, err := a.getService(service)
	if err != nil {
		return nil, err
	}

	result := &StagingCheckStatusResult{}

	// Check for staged entry
	if _, err := store.GetEntry(a.ctx, svc, name); err == nil {
		result.HasEntry = true
	}

	// Check for staged tags
	if _, err := store.GetTag(a.ctx, svc, name); err == nil {
		result.HasTags = true
	}

	return result, nil
}

// StagingDiff shows diff between staged changes and AWS.
func (a *App) StagingDiff(service string, name string) (*StagingDiffResult, error) {
	store, err := a.getStagingStore()
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
	result, err := uc.Execute(a.ctx, stagingusecase.DiffInput{Name: name})
	if err != nil {
		return nil, err
	}

	entries := make([]StagingDiffEntry, len(result.Entries))
	for i, e := range result.Entries {
		entry := StagingDiffEntry{
			Name:          e.Name,
			Operation:     string(e.Operation),
			AWSValue:      e.AWSValue,
			AWSIdentifier: e.AWSIdentifier,
			StagedValue:   e.StagedValue,
			Description:   e.Description,
			Warning:       e.Warning,
		}
		switch e.Type {
		case stagingusecase.DiffEntryNormal:
			entry.Type = "normal"
		case stagingusecase.DiffEntryCreate:
			entry.Type = "create"
		case stagingusecase.DiffEntryAutoUnstaged:
			entry.Type = "autoUnstaged"
		case stagingusecase.DiffEntryWarning:
			entry.Type = "warning"
		}
		entries[i] = entry
	}

	tagEntries := make([]StagingDiffTagEntry, len(result.TagEntries))
	for i, t := range result.TagEntries {
		tagEntries[i] = StagingDiffTagEntry{
			Name:       t.Name,
			AddTags:    t.Add,
			RemoveTags: t.Remove.Values(),
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

// StagingDrainResult represents the result of draining from file to agent.
type StagingDrainResult struct {
	Merged     bool `json:"merged"`
	EntryCount int  `json:"entryCount"`
	TagCount   int  `json:"tagCount"`
}

// StagingPersistResult represents the result of persisting from agent to file.
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
	identity, err := infra.GetAWSIdentity(a.ctx)
	if err != nil {
		return nil, err
	}

	fileStore, err := file.NewStore(identity.AccountID, identity.Region)
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

// StagingDrain loads staged changes from file into agent memory.
// If the file is encrypted, passphrase must be provided.
func (a *App) StagingDrain(service string, passphrase string, keep bool, force bool, merge bool) (*StagingDrainResult, error) {
	identity, err := infra.GetAWSIdentity(a.ctx)
	if err != nil {
		return nil, err
	}

	fileStore, err := file.NewStoreWithPassphrase(identity.AccountID, identity.Region, passphrase)
	if err != nil {
		return nil, err
	}

	agentStore, err := a.getAgentStore()
	if err != nil {
		return nil, err
	}

	var svc staging.Service
	if service != "" {
		svc, err = a.getService(service)
		if err != nil {
			return nil, err
		}
	}

	uc := &stagingusecase.DrainUseCase{
		FileStore:  fileStore,
		AgentStore: agentStore,
	}
	result, err := uc.Execute(a.ctx, stagingusecase.DrainInput{
		Service: svc,
		Keep:    keep,
		Force:   force,
		Merge:   merge,
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

// StagingPersist saves staged changes from agent memory to file.
// If passphrase is provided, the file will be encrypted.
func (a *App) StagingPersist(service string, passphrase string, keep bool) (*StagingPersistResult, error) {
	identity, err := infra.GetAWSIdentity(a.ctx)
	if err != nil {
		return nil, err
	}

	fileStore, err := file.NewStoreWithPassphrase(identity.AccountID, identity.Region, passphrase)
	if err != nil {
		return nil, err
	}

	agentStore, err := a.getAgentStore()
	if err != nil {
		return nil, err
	}

	var svc staging.Service
	if service != "" {
		svc, err = a.getService(service)
		if err != nil {
			return nil, err
		}
	}

	uc := &stagingusecase.PersistUseCase{
		AgentStore: agentStore,
		FileStore:  fileStore,
	}
	result, err := uc.Execute(a.ctx, stagingusecase.PersistInput{
		Service: svc,
		Keep:    keep,
	})
	if err != nil {
		return nil, err
	}

	return &StagingPersistResult{
		EntryCount: result.EntryCount,
		TagCount:   result.TagCount,
	}, nil
}
