package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/usecase/secret"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
	"github.com/mpyw/suve/internal/version/paramversion"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// App struct holds application state and dependencies.
type App struct {
	ctx context.Context

	// AWS clients (lazily initialized)
	paramClient  *ssm.Client
	secretClient *secretsmanager.Client

	// Staging store
	stagingStore *staging.Store
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// =============================================================================
// Param Operations
// =============================================================================

// ParamListResult represents the result of listing parameters.
type ParamListResult struct {
	Entries   []ParamListEntry `json:"entries"`
	NextToken string           `json:"nextToken,omitempty"`
}

// ParamListEntry represents a single parameter in the list.
type ParamListEntry struct {
	Name  string  `json:"name"`
	Type  string  `json:"type"`
	Value *string `json:"value,omitempty"`
}

// ParamList lists SSM parameters.
func (a *App) ParamList(prefix string, recursive bool, withValue bool, filter string, maxResults int, nextToken string) (*ParamListResult, error) {
	client, err := a.getParamClient()
	if err != nil {
		return nil, err
	}

	uc := &param.ListUseCase{Client: client}
	result, err := uc.Execute(a.ctx, param.ListInput{
		Prefix:     prefix,
		Recursive:  recursive,
		WithValue:  withValue,
		Filter:     filter,
		MaxResults: maxResults,
		NextToken:  nextToken,
	})
	if err != nil {
		return nil, err
	}

	entries := make([]ParamListEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = ParamListEntry{
			Name:  e.Name,
			Type:  e.Type,
			Value: e.Value,
		}
	}

	return &ParamListResult{Entries: entries, NextToken: result.NextToken}, nil
}

// ParamShowTag represents a tag key-value pair.
type ParamShowTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ParamShowResult represents the result of showing a parameter.
type ParamShowResult struct {
	Name         string         `json:"name"`
	Value        string         `json:"value"`
	Version      int64          `json:"version"`
	Type         string         `json:"type"`
	Description  string         `json:"description,omitempty"`
	LastModified string         `json:"lastModified,omitempty"`
	Tags         []ParamShowTag `json:"tags"`
}

// ParamShow shows a parameter value.
func (a *App) ParamShow(specStr string) (*ParamShowResult, error) {
	spec, err := paramversion.Parse(specStr)
	if err != nil {
		return nil, err
	}

	client, err := a.getParamClient()
	if err != nil {
		return nil, err
	}

	uc := &param.ShowUseCase{Client: client}
	result, err := uc.Execute(a.ctx, param.ShowInput{Spec: spec})
	if err != nil {
		return nil, err
	}

	r := &ParamShowResult{
		Name:        result.Name,
		Value:       result.Value,
		Version:     result.Version,
		Type:        string(result.Type),
		Description: result.Description,
		Tags:        make([]ParamShowTag, 0, len(result.Tags)),
	}
	if result.LastModified != nil {
		r.LastModified = result.LastModified.Format("2006-01-02T15:04:05Z07:00")
	}
	for _, tag := range result.Tags {
		r.Tags = append(r.Tags, ParamShowTag{
			Key:   tag.Key,
			Value: tag.Value,
		})
	}
	return r, nil
}

// ParamLogResult represents the result of showing parameter history.
type ParamLogResult struct {
	Name    string          `json:"name"`
	Entries []ParamLogEntry `json:"entries"`
}

// ParamLogEntry represents a single version in the history.
type ParamLogEntry struct {
	Version      int64  `json:"version"`
	Value        string `json:"value"`
	Type         string `json:"type"`
	IsCurrent    bool   `json:"isCurrent"`
	LastModified string `json:"lastModified,omitempty"`
}

// ParamLog shows parameter version history.
func (a *App) ParamLog(name string, maxResults int32) (*ParamLogResult, error) {
	client, err := a.getParamClient()
	if err != nil {
		return nil, err
	}

	uc := &param.LogUseCase{Client: client}
	result, err := uc.Execute(a.ctx, param.LogInput{
		Name:       name,
		MaxResults: maxResults,
	})
	if err != nil {
		return nil, err
	}

	entries := make([]ParamLogEntry, len(result.Entries))
	for i, e := range result.Entries {
		entry := ParamLogEntry{
			Version:   e.Version,
			Value:     e.Value,
			Type:      string(e.Type),
			IsCurrent: e.IsCurrent,
		}
		if e.LastModified != nil {
			entry.LastModified = e.LastModified.Format("2006-01-02T15:04:05Z07:00")
		}
		entries[i] = entry
	}

	return &ParamLogResult{Name: result.Name, Entries: entries}, nil
}

// ParamDiffResult represents the result of comparing parameters.
type ParamDiffResult struct {
	OldName  string `json:"oldName"`
	NewName  string `json:"newName"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

// ParamDiff compares two parameter versions.
func (a *App) ParamDiff(spec1Str, spec2Str string) (*ParamDiffResult, error) {
	spec1, err := paramversion.Parse(spec1Str)
	if err != nil {
		return nil, err
	}
	spec2, err := paramversion.Parse(spec2Str)
	if err != nil {
		return nil, err
	}

	client, err := a.getParamClient()
	if err != nil {
		return nil, err
	}

	uc := &param.DiffUseCase{Client: client}
	result, err := uc.Execute(a.ctx, param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	if err != nil {
		return nil, err
	}

	return &ParamDiffResult{
		OldName:  result.OldName,
		NewName:  result.NewName,
		OldValue: result.OldValue,
		NewValue: result.NewValue,
	}, nil
}

// ParamSetResult represents the result of setting a parameter.
type ParamSetResult struct {
	Name      string `json:"name"`
	Version   int64  `json:"version"`
	IsCreated bool   `json:"isCreated"`
}

// ParamSet creates or updates a parameter.
func (a *App) ParamSet(name, value, paramType string) (*ParamSetResult, error) {
	client, err := a.getParamClient()
	if err != nil {
		return nil, err
	}

	uc := &param.SetUseCase{Client: client}
	result, err := uc.Execute(a.ctx, param.SetInput{
		Name:  name,
		Value: value,
		Type:  paramapi.ParameterType(paramType),
	})
	if err != nil {
		return nil, err
	}

	return &ParamSetResult{
		Name:      result.Name,
		Version:   result.Version,
		IsCreated: result.IsCreated,
	}, nil
}

// ParamDeleteResult represents the result of deleting a parameter.
type ParamDeleteResult struct {
	Name string `json:"name"`
}

// ParamDelete deletes a parameter.
func (a *App) ParamDelete(name string) (*ParamDeleteResult, error) {
	client, err := a.getParamClient()
	if err != nil {
		return nil, err
	}

	uc := &param.DeleteUseCase{Client: client}
	result, err := uc.Execute(a.ctx, param.DeleteInput{Name: name})
	if err != nil {
		return nil, err
	}

	return &ParamDeleteResult{Name: result.Name}, nil
}

// ParamAddTag adds or updates a tag on a parameter.
func (a *App) ParamAddTag(name, key, value string) error {
	client, err := a.getParamClient()
	if err != nil {
		return err
	}

	uc := &param.TagUseCase{Client: client}
	return uc.Execute(a.ctx, param.TagInput{
		Name: name,
		Add:  map[string]string{key: value},
	})
}

// ParamRemoveTag removes a tag from a parameter.
func (a *App) ParamRemoveTag(name, key string) error {
	client, err := a.getParamClient()
	if err != nil {
		return err
	}

	uc := &param.TagUseCase{Client: client}
	return uc.Execute(a.ctx, param.TagInput{
		Name:   name,
		Remove: []string{key},
	})
}

// =============================================================================
// Secret Operations
// =============================================================================

// SecretListResult represents the result of listing secrets.
type SecretListResult struct {
	Entries   []SecretListEntry `json:"entries"`
	NextToken string            `json:"nextToken,omitempty"`
}

// SecretListEntry represents a single secret in the list.
type SecretListEntry struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}

// SecretList lists Secrets Manager secrets.
func (a *App) SecretList(prefix string, withValue bool, filter string, maxResults int, nextToken string) (*SecretListResult, error) {
	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.ListUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.ListInput{
		Prefix:     prefix,
		WithValue:  withValue,
		Filter:     filter,
		MaxResults: maxResults,
		NextToken:  nextToken,
	})
	if err != nil {
		return nil, err
	}

	entries := make([]SecretListEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = SecretListEntry{
			Name:  e.Name,
			Value: e.Value,
		}
	}

	return &SecretListResult{Entries: entries, NextToken: result.NextToken}, nil
}

// SecretShowTag represents a tag key-value pair.
type SecretShowTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SecretShowResult represents the result of showing a secret.
type SecretShowResult struct {
	Name         string          `json:"name"`
	ARN          string          `json:"arn"`
	VersionID    string          `json:"versionId"`
	VersionStage []string        `json:"versionStage"`
	Value        string          `json:"value"`
	Description  string          `json:"description,omitempty"`
	CreatedDate  string          `json:"createdDate,omitempty"`
	Tags         []SecretShowTag `json:"tags"`
}

// SecretShow shows a secret value.
func (a *App) SecretShow(specStr string) (*SecretShowResult, error) {
	spec, err := secretversion.Parse(specStr)
	if err != nil {
		return nil, err
	}

	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.ShowUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.ShowInput{Spec: spec})
	if err != nil {
		return nil, err
	}

	r := &SecretShowResult{
		Name:         result.Name,
		ARN:          result.ARN,
		VersionID:    result.VersionID,
		VersionStage: result.VersionStage,
		Value:        result.Value,
		Description:  result.Description,
		Tags:         make([]SecretShowTag, 0, len(result.Tags)),
	}
	if result.CreatedDate != nil {
		r.CreatedDate = result.CreatedDate.Format("2006-01-02T15:04:05Z07:00")
	}
	for _, tag := range result.Tags {
		r.Tags = append(r.Tags, SecretShowTag{
			Key:   tag.Key,
			Value: tag.Value,
		})
	}
	return r, nil
}

// SecretLogResult represents the result of showing secret history.
type SecretLogResult struct {
	Name    string           `json:"name"`
	Entries []SecretLogEntry `json:"entries"`
}

// SecretLogEntry represents a single version in the history.
type SecretLogEntry struct {
	VersionID string   `json:"versionId"`
	Stages    []string `json:"stages"`
	Value     string   `json:"value"`
	IsCurrent bool     `json:"isCurrent"`
	Created   string   `json:"created,omitempty"`
}

// SecretLog shows secret version history.
func (a *App) SecretLog(name string, maxResults int32) (*SecretLogResult, error) {
	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.LogUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.LogInput{
		Name:       name,
		MaxResults: maxResults,
	})
	if err != nil {
		return nil, err
	}

	entries := make([]SecretLogEntry, len(result.Entries))
	for i, e := range result.Entries {
		entry := SecretLogEntry{
			VersionID: e.VersionID,
			Stages:    e.VersionStage,
			Value:     e.Value,
			IsCurrent: e.IsCurrent,
		}
		if e.CreatedDate != nil {
			entry.Created = e.CreatedDate.Format("2006-01-02T15:04:05Z07:00")
		}
		entries[i] = entry
	}

	return &SecretLogResult{Name: result.Name, Entries: entries}, nil
}

// SecretCreateResult represents the result of creating a secret.
type SecretCreateResult struct {
	Name      string `json:"name"`
	VersionID string `json:"versionId"`
	ARN       string `json:"arn"`
}

// SecretCreate creates a new secret.
func (a *App) SecretCreate(name, value string) (*SecretCreateResult, error) {
	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.CreateUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.CreateInput{
		Name:  name,
		Value: value,
	})
	if err != nil {
		return nil, err
	}

	return &SecretCreateResult{
		Name:      result.Name,
		VersionID: result.VersionID,
		ARN:       result.ARN,
	}, nil
}

// SecretUpdateResult represents the result of updating a secret.
type SecretUpdateResult struct {
	Name      string `json:"name"`
	VersionID string `json:"versionId"`
	ARN       string `json:"arn"`
}

// SecretUpdate updates an existing secret.
func (a *App) SecretUpdate(name, value string) (*SecretUpdateResult, error) {
	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.UpdateUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.UpdateInput{
		Name:  name,
		Value: value,
	})
	if err != nil {
		return nil, err
	}

	return &SecretUpdateResult{
		Name:      result.Name,
		VersionID: result.VersionID,
		ARN:       result.ARN,
	}, nil
}

// SecretDeleteResult represents the result of deleting a secret.
type SecretDeleteResult struct {
	Name         string `json:"name"`
	DeletionDate string `json:"deletionDate,omitempty"`
	ARN          string `json:"arn"`
}

// SecretDelete deletes a secret (with recovery window).
func (a *App) SecretDelete(name string, force bool) (*SecretDeleteResult, error) {
	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.DeleteUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.DeleteInput{
		Name:  name,
		Force: force,
	})
	if err != nil {
		return nil, err
	}

	r := &SecretDeleteResult{
		Name: result.Name,
		ARN:  result.ARN,
	}
	if result.DeletionDate != nil {
		r.DeletionDate = result.DeletionDate.Format("2006-01-02T15:04:05Z07:00")
	}
	return r, nil
}

// SecretAddTag adds or updates a tag on a secret.
func (a *App) SecretAddTag(name, key, value string) error {
	client, err := a.getSecretClient()
	if err != nil {
		return err
	}

	uc := &secret.TagUseCase{Client: client}
	return uc.Execute(a.ctx, secret.TagInput{
		Name: name,
		Add:  map[string]string{key: value},
	})
}

// SecretRemoveTag removes a tag from a secret.
func (a *App) SecretRemoveTag(name, key string) error {
	client, err := a.getSecretClient()
	if err != nil {
		return err
	}

	uc := &secret.TagUseCase{Client: client}
	return uc.Execute(a.ctx, secret.TagInput{
		Name:   name,
		Remove: []string{key},
	})
}

// SecretDiffResult represents the result of comparing secrets.
type SecretDiffResult struct {
	OldName      string `json:"oldName"`
	OldVersionID string `json:"oldVersionId"`
	OldValue     string `json:"oldValue"`
	NewName      string `json:"newName"`
	NewVersionID string `json:"newVersionId"`
	NewValue     string `json:"newValue"`
}

// SecretDiff compares two secret versions.
func (a *App) SecretDiff(spec1Str, spec2Str string) (*SecretDiffResult, error) {
	spec1, err := secretversion.Parse(spec1Str)
	if err != nil {
		return nil, err
	}
	spec2, err := secretversion.Parse(spec2Str)
	if err != nil {
		return nil, err
	}

	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.DiffUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	if err != nil {
		return nil, err
	}

	return &SecretDiffResult{
		OldName:      result.OldName,
		OldVersionID: result.OldVersionID,
		OldValue:     result.OldValue,
		NewName:      result.NewName,
		NewVersionID: result.NewVersionID,
		NewValue:     result.NewValue,
	}, nil
}

// SecretRestoreResult represents the result of restoring a secret.
type SecretRestoreResult struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

// SecretRestore restores a deleted secret.
func (a *App) SecretRestore(name string) (*SecretRestoreResult, error) {
	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.RestoreUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.RestoreInput{Name: name})
	if err != nil {
		return nil, err
	}

	return &SecretRestoreResult{
		Name: result.Name,
		ARN:  result.ARN,
	}, nil
}

// =============================================================================
// Staging Operations
// =============================================================================

// StagingStatusResult represents the result of staging status.
type StagingStatusResult struct {
	SSM     []StagingEntry    `json:"ssm"`
	SM      []StagingEntry    `json:"sm"`
	SSMTags []StagingTagEntry `json:"ssmTags"`
	SMTags  []StagingTagEntry `json:"smTags"`
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
	Name      string              `json:"name"`
	AddTags   map[string]string   `json:"addTags,omitempty"`
	RemoveTags maputil.Set[string] `json:"removeTags,omitempty"`
	StagedAt  string              `json:"stagedAt"`
}

// StagingStatus gets the current staging status.
func (a *App) StagingStatus() (*StagingStatusResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	ssmParser, _ := a.getParser("ssm")
	smParser, _ := a.getParser("sm")

	// SSM status
	ssmUC := &stagingusecase.StatusUseCase{
		Strategy: ssmParser,
		Store:    store,
	}
	ssmResult, err := ssmUC.Execute(a.ctx, stagingusecase.StatusInput{})
	if err != nil {
		return nil, err
	}

	// SM status
	smUC := &stagingusecase.StatusUseCase{
		Strategy: smParser,
		Store:    store,
	}
	smResult, err := smUC.Execute(a.ctx, stagingusecase.StatusInput{})
	if err != nil {
		return nil, err
	}

	result := &StagingStatusResult{
		SSM:     make([]StagingEntry, len(ssmResult.Entries)),
		SM:      make([]StagingEntry, len(smResult.Entries)),
		SSMTags: make([]StagingTagEntry, len(ssmResult.TagEntries)),
		SMTags:  make([]StagingTagEntry, len(smResult.TagEntries)),
	}

	for i, e := range ssmResult.Entries {
		result.SSM[i] = StagingEntry{
			Name:      e.Name,
			Operation: string(e.Operation),
			Value:     e.Value,
			StagedAt:  e.StagedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	for i, e := range smResult.Entries {
		result.SM[i] = StagingEntry{
			Name:      e.Name,
			Operation: string(e.Operation),
			Value:     e.Value,
			StagedAt:  e.StagedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	for i, t := range ssmResult.TagEntries {
		result.SSMTags[i] = StagingTagEntry{
			Name:       t.Name,
			AddTags:    t.Add,
			RemoveTags: t.Remove,
			StagedAt:   t.StagedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	for i, t := range smResult.TagEntries {
		result.SMTags[i] = StagingTagEntry{
			Name:       t.Name,
			AddTags:    t.Add,
			RemoveTags: t.Remove,
			StagedAt:   t.StagedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return result, nil
}

// StagingApplyEntryResult represents a single entry apply result.
type StagingApplyEntryResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// StagingApplyTagResult represents a single tag apply result.
type StagingApplyTagResult struct {
	Name       string              `json:"name"`
	AddTags    map[string]string   `json:"addTags,omitempty"`
	RemoveTags maputil.Set[string] `json:"removeTags,omitempty"`
	Error      string              `json:"error,omitempty"`
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
			RemoveTags: r.RemoveTag,
		}
		if r.Error != nil {
			tagResult.Error = r.Error.Error()
		}
		output.TagResults = append(output.TagResults, tagResult)
	}

	return output, err
}

// StagingResetResult represents the result of resetting staged changes.
type StagingResetResult struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Count       int    `json:"count,omitempty"`
	ServiceName string `json:"serviceName"`
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

// StagingAddResult represents the result of staging an add operation.
type StagingAddResult struct {
	Name string `json:"name"`
}

// StagingAdd stages a create operation for a new item.
func (a *App) StagingAdd(service, name, value string) (*StagingAddResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	parser, err := a.getParser(service)
	if err != nil {
		return nil, err
	}

	uc := &stagingusecase.AddUseCase{
		Strategy: parser,
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

// StagingEditResult represents the result of staging an edit operation.
type StagingEditResult struct {
	Name string `json:"name"`
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

// StagingDeleteResult represents the result of staging a delete operation.
type StagingDeleteResult struct {
	Name string `json:"name"`
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

// StagingUnstageResult represents the result of unstaging an item.
type StagingUnstageResult struct {
	Name string `json:"name"`
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
	if err := store.UnstageEntry(svc, name); err != nil && err != staging.ErrNotStaged {
		return nil, err
	}

	// Unstage tags (ignore ErrNotStaged)
	if err := store.UnstageTag(svc, name); err != nil && err != staging.ErrNotStaged {
		return nil, err
	}

	return &StagingUnstageResult{Name: name}, nil
}

// StagingAddTagResult represents the result of staging a tag addition.
type StagingAddTagResult struct {
	Name string `json:"name"`
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
	result, err := uc.Execute(a.ctx, stagingusecase.TagInput{
		Name:    name,
		AddTags: map[string]string{key: value},
	})
	if err != nil {
		return nil, err
	}

	return &StagingAddTagResult{Name: result.Name}, nil
}

// StagingRemoveTagResult represents the result of staging a tag removal.
type StagingRemoveTagResult struct {
	Name string `json:"name"`
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
	result, err := uc.Execute(a.ctx, stagingusecase.TagInput{
		Name:       name,
		RemoveTags: maputil.NewSet(key),
	})
	if err != nil {
		return nil, err
	}

	return &StagingRemoveTagResult{Name: result.Name}, nil
}

// StagingCancelAddTagResult represents the result of canceling a staged tag addition.
type StagingCancelAddTagResult struct {
	Name string `json:"name"`
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
	tagEntry, err := store.GetTag(svc, name)
	if err != nil {
		return nil, err
	}

	// Remove key from Add
	delete(tagEntry.Add, key)

	// If tag entry has no meaningful content, unstage it
	if len(tagEntry.Add) == 0 && tagEntry.Remove.Len() == 0 {
		if err := store.UnstageTag(svc, name); err != nil {
			return nil, err
		}
	} else {
		if err := store.StageTag(svc, name, *tagEntry); err != nil {
			return nil, err
		}
	}

	return &StagingCancelAddTagResult{Name: name}, nil
}

// StagingCancelRemoveTagResult represents the result of canceling a staged tag removal.
type StagingCancelRemoveTagResult struct {
	Name string `json:"name"`
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
	tagEntry, err := store.GetTag(svc, name)
	if err != nil {
		return nil, err
	}

	// Remove key from Remove set
	tagEntry.Remove.Remove(key)

	// If tag entry has no meaningful content, unstage it
	if len(tagEntry.Add) == 0 && tagEntry.Remove.Len() == 0 {
		if err := store.UnstageTag(svc, name); err != nil {
			return nil, err
		}
	} else {
		if err := store.StageTag(svc, name, *tagEntry); err != nil {
			return nil, err
		}
	}

	return &StagingCancelRemoveTagResult{Name: name}, nil
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
	Name       string              `json:"name"`
	AddTags    map[string]string   `json:"addTags,omitempty"`
	RemoveTags maputil.Set[string] `json:"removeTags,omitempty"`
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
			RemoveTags: t.Remove,
		}
	}

	return &StagingDiffResult{
		ItemName:   result.ItemName,
		Entries:    entries,
		TagEntries: tagEntries,
	}, nil
}

// errInvalidService is returned when an invalid service is specified.
var errInvalidService = errorString("invalid service: must be 'ssm' or 'sm'")

type errorString string

func (e errorString) Error() string { return string(e) }

// =============================================================================
// Helper methods
// =============================================================================

func (a *App) getParamClient() (*ssm.Client, error) {
	if a.paramClient != nil {
		return a.paramClient, nil
	}

	client, err := infra.NewParamClient(a.ctx)
	if err != nil {
		return nil, err
	}
	a.paramClient = client
	return client, nil
}

func (a *App) getSecretClient() (*secretsmanager.Client, error) {
	if a.secretClient != nil {
		return a.secretClient, nil
	}

	client, err := infra.NewSecretClient(a.ctx)
	if err != nil {
		return nil, err
	}
	a.secretClient = client
	return client, nil
}

func (a *App) getStagingStore() (*staging.Store, error) {
	if a.stagingStore != nil {
		return a.stagingStore, nil
	}

	store, err := staging.NewStore()
	if err != nil {
		return nil, err
	}
	a.stagingStore = store
	return store, nil
}

func (a *App) getService(service string) (staging.Service, error) {
	switch service {
	case "ssm":
		return staging.ServiceParam, nil
	case "sm":
		return staging.ServiceSecret, nil
	default:
		return "", errInvalidService
	}
}

func (a *App) getParser(service string) (staging.Parser, error) {
	switch service {
	case "ssm":
		return &staging.ParamStrategy{}, nil
	case "sm":
		return &staging.SecretStrategy{}, nil
	default:
		return nil, errInvalidService
	}
}

func (a *App) getEditStrategy(service string) (staging.EditStrategy, error) {
	switch service {
	case "ssm":
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		return staging.NewParamStrategy(client), nil
	case "sm":
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		return staging.NewSecretStrategy(client), nil
	default:
		return nil, errInvalidService
	}
}

func (a *App) getDeleteStrategy(service string) (staging.DeleteStrategy, error) {
	switch service {
	case "ssm":
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		return staging.NewParamStrategy(client), nil
	case "sm":
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		return staging.NewSecretStrategy(client), nil
	default:
		return nil, errInvalidService
	}
}

func (a *App) getApplyStrategy(service string) (staging.ApplyStrategy, error) {
	switch service {
	case "ssm":
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		return staging.NewParamStrategy(client), nil
	case "sm":
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		return staging.NewSecretStrategy(client), nil
	default:
		return nil, errInvalidService
	}
}

func (a *App) getDiffStrategy(service string) (staging.DiffStrategy, error) {
	switch service {
	case "ssm":
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		return staging.NewParamStrategy(client), nil
	case "sm":
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		return staging.NewSecretStrategy(client), nil
	default:
		return nil, errInvalidService
	}
}
