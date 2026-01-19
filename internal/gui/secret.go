//go:build production || dev

package gui

import (
	awssecret "github.com/mpyw/suve/internal/provider/aws/secret"
	"github.com/mpyw/suve/internal/usecase/secret"
	"github.com/mpyw/suve/internal/version/secretversion"
)

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

// SecretShow shows a secret value.
func (a *App) SecretShow(specStr string) (*SecretShowResult, error) {
	spec, err := secretversion.Parse(specStr)
	if err != nil {
		return nil, err
	}

	adapter, err := awssecret.NewAdapter(a.ctx)
	if err != nil {
		return nil, err
	}

	uc := &secret.ShowUseCase{Client: adapter}

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
	adapter, err := awssecret.NewAdapter(a.ctx)
	if err != nil {
		return err
	}

	uc := &secret.TagUseCase{Client: adapter}

	return uc.Execute(a.ctx, secret.TagInput{
		Name: name,
		Add:  map[string]string{key: value},
	})
}

// SecretRemoveTag removes a tag from a secret.
func (a *App) SecretRemoveTag(name, key string) error {
	adapter, err := awssecret.NewAdapter(a.ctx)
	if err != nil {
		return err
	}

	uc := &secret.TagUseCase{Client: adapter}

	return uc.Execute(a.ctx, secret.TagInput{
		Name:   name,
		Remove: []string{key},
	})
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
