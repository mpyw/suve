package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/infra"
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
	Entries []ParamListEntry `json:"entries"`
}

// ParamListEntry represents a single parameter in the list.
type ParamListEntry struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}

// ParamList lists SSM parameters.
func (a *App) ParamList(prefix string, recursive bool, withValue bool) (*ParamListResult, error) {
	client, err := a.getParamClient()
	if err != nil {
		return nil, err
	}

	uc := &param.ListUseCase{Client: client}
	result, err := uc.Execute(a.ctx, param.ListInput{
		Prefix:    prefix,
		Recursive: recursive,
		WithValue: withValue,
	})
	if err != nil {
		return nil, err
	}

	entries := make([]ParamListEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = ParamListEntry{
			Name:  e.Name,
			Value: e.Value,
		}
	}

	return &ParamListResult{Entries: entries}, nil
}

// ParamShowResult represents the result of showing a parameter.
type ParamShowResult struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	Version      int64  `json:"version"`
	Type         string `json:"type"`
	LastModified string `json:"lastModified,omitempty"`
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
		Name:    result.Name,
		Value:   result.Value,
		Version: result.Version,
		Type:    string(result.Type),
	}
	if result.LastModified != nil {
		r.LastModified = result.LastModified.Format("2006-01-02T15:04:05Z07:00")
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

// =============================================================================
// Secret Operations
// =============================================================================

// SecretListResult represents the result of listing secrets.
type SecretListResult struct {
	Entries []SecretListEntry `json:"entries"`
}

// SecretListEntry represents a single secret in the list.
type SecretListEntry struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}

// SecretList lists Secrets Manager secrets.
func (a *App) SecretList(prefix string, withValue bool) (*SecretListResult, error) {
	client, err := a.getSecretClient()
	if err != nil {
		return nil, err
	}

	uc := &secret.ListUseCase{Client: client}
	result, err := uc.Execute(a.ctx, secret.ListInput{
		Prefix:    prefix,
		WithValue: withValue,
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

	return &SecretListResult{Entries: entries}, nil
}

// SecretShowResult represents the result of showing a secret.
type SecretShowResult struct {
	Name         string   `json:"name"`
	ARN          string   `json:"arn"`
	VersionID    string   `json:"versionId"`
	VersionStage []string `json:"versionStage"`
	Value        string   `json:"value"`
	CreatedDate  string   `json:"createdDate,omitempty"`
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
	}
	if result.CreatedDate != nil {
		r.CreatedDate = result.CreatedDate.Format("2006-01-02T15:04:05Z07:00")
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

// =============================================================================
// Staging Operations
// =============================================================================

// StagingStatusResult represents the result of staging status.
type StagingStatusResult struct {
	SSM []StagingEntry `json:"ssm"`
	SM  []StagingEntry `json:"sm"`
}

// StagingEntry represents a staged change.
type StagingEntry struct {
	Name      string  `json:"name"`
	Operation string  `json:"operation"`
	Value     *string `json:"value,omitempty"`
	StagedAt  string  `json:"stagedAt"`
}

// StagingStatus gets the current staging status.
func (a *App) StagingStatus() (*StagingStatusResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	// SSM status
	ssmUC := &stagingusecase.StatusUseCase{
		Strategy: &staging.ParamStrategy{},
		Store:    store,
	}
	ssmResult, err := ssmUC.Execute(a.ctx, stagingusecase.StatusInput{})
	if err != nil {
		return nil, err
	}

	// SM status
	smUC := &stagingusecase.StatusUseCase{
		Strategy: &staging.SecretStrategy{},
		Store:    store,
	}
	smResult, err := smUC.Execute(a.ctx, stagingusecase.StatusInput{})
	if err != nil {
		return nil, err
	}

	result := &StagingStatusResult{
		SSM: make([]StagingEntry, len(ssmResult.Entries)),
		SM:  make([]StagingEntry, len(smResult.Entries)),
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

	return result, nil
}

// StagingApplyResultEntry represents a single apply result.
type StagingApplyResultEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// StagingApplyResult represents the result of applying staged changes.
type StagingApplyResult struct {
	ServiceName string                    `json:"serviceName"`
	Results     []StagingApplyResultEntry `json:"results"`
	Conflicts   []string                  `json:"conflicts,omitempty"`
	Succeeded   int                       `json:"succeeded"`
	Failed      int                       `json:"failed"`
}

// StagingApply applies staged changes for a service.
func (a *App) StagingApply(service string, ignoreConflicts bool) (*StagingApplyResult, error) {
	store, err := a.getStagingStore()
	if err != nil {
		return nil, err
	}

	var strategy staging.ApplyStrategy
	switch service {
	case "ssm":
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		strategy = &staging.ParamStrategy{Client: client}
	case "sm":
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		strategy = &staging.SecretStrategy{Client: client}
	default:
		return nil, errInvalidService
	}

	uc := &stagingusecase.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}
	result, err := uc.Execute(a.ctx, stagingusecase.ApplyInput{
		IgnoreConflicts: ignoreConflicts,
	})

	output := &StagingApplyResult{
		ServiceName: result.ServiceName,
		Conflicts:   result.Conflicts,
		Succeeded:   result.Succeeded,
		Failed:      result.Failed,
	}

	for _, r := range result.Results {
		entry := StagingApplyResultEntry{Name: r.Name}
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
		output.Results = append(output.Results, entry)
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

	var parser staging.Parser
	switch service {
	case "ssm":
		parser = &staging.ParamStrategy{}
	case "sm":
		parser = &staging.SecretStrategy{}
	default:
		return nil, errInvalidService
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
