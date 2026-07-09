//go:build production || dev

package gui

import (
	"time"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// errRestoreUnsupported is returned when the active provider does not support
// restoring soft-deleted secrets.
var errRestoreUnsupported = stringError("restore is not supported by this provider")

// =============================================================================
// Secret Types
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

// SecretShowTag represents a tag key-value pair.
type SecretShowTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SecretShowResult represents the result of showing a secret.
//
// StagingLabels and State carry two independent concepts that must NOT be
// conflated (#419): StagingLabels holds AWS Secrets Manager staging labels
// (empty for other providers), while State holds the per-version lifecycle
// state (enabled/disabled/destroyed) for Google Cloud + Azure Key Vault (empty
// for AWS). A version never has both.
type SecretShowResult struct {
	Name          string          `json:"name"`
	ARN           string          `json:"arn"`
	VersionID     string          `json:"versionId"`
	StagingLabels []string        `json:"stagingLabels"`
	State         string          `json:"state,omitempty"`
	Value         string          `json:"value"`
	Description   string          `json:"description,omitempty"`
	CreatedDate   string          `json:"createdDate,omitempty"`
	Tags          []SecretShowTag `json:"tags"`
}

// SecretLogResult represents the result of showing secret history.
type SecretLogResult struct {
	Name    string           `json:"name"`
	Entries []SecretLogEntry `json:"entries"`
}

// SecretLogEntry represents a single version in the history.
//
// StagingLabels and State carry two independent concepts that must NOT be
// conflated (#419): StagingLabels holds AWS Secrets Manager staging labels
// (empty for other providers), while State holds the per-version lifecycle
// state (enabled/disabled/destroyed) for Google Cloud + Azure Key Vault (empty
// for AWS). A version never has both.
type SecretLogEntry struct {
	VersionID     string   `json:"versionId"`
	StagingLabels []string `json:"stagingLabels"`
	State         string   `json:"state,omitempty"`
	Value         string   `json:"value"`
	IsCurrent     bool     `json:"isCurrent"`
	Created       string   `json:"created,omitempty"`
	// Tags attached to THIS version (Azure Key Vault only; empty otherwise).
	Tags []SecretShowTag `json:"tags"`
}

// SecretCreateResult represents the result of creating a secret.
type SecretCreateResult struct {
	Name      string `json:"name"`
	VersionID string `json:"versionId"`
	ARN       string `json:"arn"`
}

// SecretUpdateResult represents the result of updating a secret.
type SecretUpdateResult struct {
	Name      string `json:"name"`
	VersionID string `json:"versionId"`
	ARN       string `json:"arn"`
}

// SecretDeleteResult represents the result of deleting a secret.
type SecretDeleteResult struct {
	Name         string `json:"name"`
	DeletionDate string `json:"deletionDate,omitempty"`
	ARN          string `json:"arn"`
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

// SecretRestoreResult represents the result of restoring a secret.
type SecretRestoreResult struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

// =============================================================================
// Secret Methods
// =============================================================================

// SecretList lists Secrets Manager secrets.
func (a *App) SecretList(prefix string, withValue bool, filter string, _ int, _ string) (*SecretListResult, error) {
	store, err := a.secretStore()
	if err != nil {
		return nil, err
	}

	uc := &secret.ListUseCase{Reader: store}

	result, err := uc.Execute(a.ctx, secret.ListInput{
		Prefix:    prefix,
		WithValue: withValue,
		Filter:    filter,
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

// SecretShow shows a secret value.
func (a *App) SecretShow(specStr string) (*SecretShowResult, error) {
	spec, err := a.parseSecretSpec(specStr)
	if err != nil {
		return nil, err
	}

	store, err := a.secretStore()
	if err != nil {
		return nil, err
	}

	uc := &secret.ShowUseCase{Reader: store}

	result, err := uc.Execute(a.ctx, secret.ShowInput{Spec: spec})
	if err != nil {
		return nil, err
	}

	r := &SecretShowResult{
		Name:          result.Name,
		ARN:           result.ARN,
		VersionID:     result.VersionID,
		StagingLabels: result.VersionStage,
		State:         result.State,
		Value:         result.Value,
		Description:   result.Description,
		Tags:          make([]SecretShowTag, 0, len(result.Tags)),
	}
	if result.CreatedDate != nil {
		r.CreatedDate = timeutil.FormatRFC3339(*result.CreatedDate)
	}

	for _, tag := range result.Tags {
		r.Tags = append(r.Tags, SecretShowTag{
			Key:   tag.Key,
			Value: tag.Value,
		})
	}

	return r, nil
}

// SecretLog shows secret version history.
func (a *App) SecretLog(name string, maxResults int32) (*SecretLogResult, error) {
	store, err := a.secretStore()
	if err != nil {
		return nil, err
	}

	uc := &secret.LogUseCase{Reader: store}

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
			VersionID:     e.VersionID,
			StagingLabels: e.VersionStage,
			State:         e.State,
			Value:         e.Value,
			IsCurrent:     e.IsCurrent,
			Tags:          make([]SecretShowTag, 0, len(e.Tags)),
		}
		if e.CreatedDate != nil {
			entry.Created = timeutil.FormatRFC3339(*e.CreatedDate)
		}

		for _, tag := range e.Tags {
			entry.Tags = append(entry.Tags, SecretShowTag{Key: tag.Key, Value: tag.Value})
		}

		entries[i] = entry
	}

	return &SecretLogResult{Name: result.Name, Entries: entries}, nil
}

// SecretCreate creates a new secret.
func (a *App) SecretCreate(name, value string) (*SecretCreateResult, error) {
	store, err := a.secretStore()
	if err != nil {
		return nil, err
	}

	uc := &secret.CreateUseCase{Writer: store}

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
	}, nil
}

// SecretUpdate updates an existing secret.
func (a *App) SecretUpdate(name, value string) (*SecretUpdateResult, error) {
	store, err := a.secretStore()
	if err != nil {
		return nil, err
	}

	uc := &secret.UpdateUseCase{Store: store}

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
	}, nil
}

// SecretDelete deletes a secret (with recovery window).
func (a *App) SecretDelete(name string, force bool) (*SecretDeleteResult, error) {
	store, err := a.secretStore()
	if err != nil {
		return nil, err
	}

	uc := &secret.DeleteUseCase{Store: store}

	var options []provider.DeleteOption
	if force {
		// Force-delete is AWS Secrets Manager only (mapped to
		// ForceDeleteWithoutRecovery); the frontend hides the checkbox elsewhere
		// (hasForceDelete=false), so force is never set for Key Vault or Google
		// Cloud, which soft-delete/retain by policy instead.
		options = append(options, provider.ForceDelete{})
	}

	result, err := uc.Execute(a.ctx, secret.DeleteInput{
		Name:    name,
		Options: options,
	})
	if err != nil {
		return nil, err
	}

	r := &SecretDeleteResult{
		Name: result.Name,
	}
	// The provider Delete returns only an error; when not forcing, compute the
	// scheduled deletion date client-side (now + AWS default recovery window).
	// Only AWS Secrets Manager has a recovery window: Google Cloud deletes
	// immediately and Key Vault retention is governed by vault policy, so a
	// synthetic "recoverable until" date there would be false. Gate on the
	// active provider (mirrors ServiceCapability.HasRecoveryWindow).
	if !force && a.currentScope().Provider == provider.ProviderAWS {
		const defaultRecoveryWindowDays = 30

		r.DeletionDate = timeutil.FormatRFC3339(time.Now().AddDate(0, 0, defaultRecoveryWindowDays))
	}

	return r, nil
}

// SecretAddTag adds or updates a tag on a secret.
func (a *App) SecretAddTag(name, key, value string) error {
	store, err := a.secretStore()
	if err != nil {
		return err
	}

	uc := &secret.TagUseCase{Tagger: store}

	return uc.Execute(a.ctx, secret.TagInput{
		Name: name,
		Add:  map[string]string{key: value},
	})
}

// SecretRemoveTag removes a tag from a secret.
func (a *App) SecretRemoveTag(name, key string) error {
	store, err := a.secretStore()
	if err != nil {
		return err
	}

	uc := &secret.TagUseCase{Tagger: store}

	return uc.Execute(a.ctx, secret.TagInput{
		Name:   name,
		Remove: []string{key},
	})
}

// SecretDiff compares two secret versions.
func (a *App) SecretDiff(spec1Str, spec2Str string) (*SecretDiffResult, error) {
	spec1, err := a.parseSecretSpec(spec1Str)
	if err != nil {
		return nil, err
	}

	spec2, err := a.parseSecretSpec(spec2Str)
	if err != nil {
		return nil, err
	}

	store, err := a.secretStore()
	if err != nil {
		return nil, err
	}

	uc := &secret.DiffUseCase{Reader: store}

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

// SecretRestore restores a deleted secret.
func (a *App) SecretRestore(name string) (*SecretRestoreResult, error) {
	store, err := a.secretStore()
	if err != nil {
		return nil, err
	}

	restorer, ok := store.(provider.Restorer)
	if !ok {
		return nil, errRestoreUnsupported
	}

	uc := &secret.RestoreUseCase{Restorer: restorer}

	result, err := uc.Execute(a.ctx, secret.RestoreInput{Name: name})
	if err != nil {
		return nil, err
	}

	return &SecretRestoreResult{
		Name: result.Name,
	}, nil
}
