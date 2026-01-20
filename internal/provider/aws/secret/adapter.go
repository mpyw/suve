// Package secret provides AWS Secrets Manager adapter implementing provider interfaces.
package secret

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
)

// Client combines all AWS Secrets Manager API interfaces required by the adapter.
type Client interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIDsAPI
	secretapi.ListSecretsAPI
	secretapi.CreateSecretAPI
	secretapi.PutSecretValueAPI
	secretapi.DeleteSecretAPI
	secretapi.RestoreSecretAPI
	secretapi.TagResourceAPI
	secretapi.UntagResourceAPI
	secretapi.DescribeSecretAPI
}

// Adapter implements provider.SecretService for AWS Secrets Manager.
type Adapter struct {
	client Client
}

// NewAdapter creates a new AWS Secrets Manager adapter using the default AWS configuration.
func NewAdapter(ctx context.Context) (*Adapter, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &Adapter{client: secretsmanager.NewFromConfig(cfg)}, nil
}

// New creates a new AWS Secrets Manager adapter from an existing client.
func New(client Client) *Adapter {
	return &Adapter{client: client}
}

// ============================================================================
// SecretReader Implementation
// ============================================================================

// GetSecret retrieves a secret by name with optional version/stage specifier.
func (a *Adapter) GetSecret(
	ctx context.Context, name string, versionID string, versionStage string,
) (*model.Secret, error) {
	input := &secretapi.GetSecretValueInput{
		SecretId: lo.ToPtr(name),
	}

	if versionID != "" {
		input.VersionId = lo.ToPtr(versionID)
	}

	if versionStage != "" {
		input.VersionStage = lo.ToPtr(versionStage)
	}

	output, err := a.client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	return convertGetSecretValueOutput(output), nil
}

// GetSecretVersions retrieves all versions of a secret.
func (a *Adapter) GetSecretVersions(
	ctx context.Context, name string,
) ([]*model.SecretVersion, error) {
	input := &secretapi.ListSecretVersionIDsInput{
		SecretId: lo.ToPtr(name),
	}

	var versions []*model.SecretVersion

	// Paginate through all versions
	for {
		output, err := a.client.ListSecretVersionIds(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list secret versions: %w", err)
		}

		for _, v := range output.Versions {
			versions = append(versions, convertSecretVersion(&v))
		}

		if output.NextToken == nil {
			break
		}

		input.NextToken = output.NextToken
	}

	return versions, nil
}

// ListSecrets lists all secrets.
func (a *Adapter) ListSecrets(ctx context.Context) ([]*model.SecretListItem, error) {
	var items []*model.SecretListItem

	// Paginate through all secrets
	paginator := secretapi.NewListSecretsPaginator(a.client, &secretapi.ListSecretsInput{})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}

		for _, entry := range output.SecretList {
			items = append(items, convertSecretListEntry(&entry))
		}
	}

	return items, nil
}

// ============================================================================
// SecretWriter Implementation
// ============================================================================

// CreateSecret creates a new secret.
func (a *Adapter) CreateSecret(ctx context.Context, secret *model.Secret) (*model.SecretWriteResult, error) {
	input := &secretapi.CreateSecretInput{
		Name:         lo.ToPtr(secret.Name),
		SecretString: lo.ToPtr(secret.Value),
	}

	if secret.Description != "" {
		input.Description = lo.ToPtr(secret.Description)
	}

	if len(secret.Tags) > 0 {
		input.Tags = convertToAWSTags(secret.Tags)
	}

	// Apply AWS-specific metadata if present
	if meta, ok := model.TypedSecretMetadata[model.AWSSecretMeta](secret); ok {
		if meta.KmsKeyID != "" {
			input.KmsKeyId = lo.ToPtr(meta.KmsKeyID)
		}
	}

	output, err := a.client.CreateSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	return &model.SecretWriteResult{
		Name:    lo.FromPtr(output.Name),
		Version: lo.FromPtr(output.VersionId),
		ARN:     lo.FromPtr(output.ARN),
	}, nil
}

// UpdateSecret updates the value of an existing secret.
func (a *Adapter) UpdateSecret(ctx context.Context, name string, value string) (*model.SecretWriteResult, error) {
	input := &secretapi.PutSecretValueInput{
		SecretId:     lo.ToPtr(name),
		SecretString: lo.ToPtr(value),
	}

	output, err := a.client.PutSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	return &model.SecretWriteResult{
		Name:    lo.FromPtr(output.Name),
		Version: lo.FromPtr(output.VersionId),
		ARN:     lo.FromPtr(output.ARN),
	}, nil
}

// DeleteSecret deletes a secret.
func (a *Adapter) DeleteSecret(ctx context.Context, name string, forceDelete bool) (*model.SecretDeleteResult, error) {
	input := &secretapi.DeleteSecretInput{
		SecretId:                   lo.ToPtr(name),
		ForceDeleteWithoutRecovery: lo.ToPtr(forceDelete),
	}

	output, err := a.client.DeleteSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to delete secret: %w", err)
	}

	return &model.SecretDeleteResult{
		Name:         lo.FromPtr(output.Name),
		ARN:          lo.FromPtr(output.ARN),
		DeletionDate: output.DeletionDate,
	}, nil
}

// ============================================================================
// SecretTagger Implementation
// ============================================================================

// GetTags retrieves all tags for a secret.
func (a *Adapter) GetTags(ctx context.Context, name string) (map[string]string, error) {
	input := &secretapi.DescribeSecretInput{
		SecretId: lo.ToPtr(name),
	}

	output, err := a.client.DescribeSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe secret: %w", err)
	}

	return convertFromAWSTags(output.Tags), nil
}

// AddTags adds or updates tags on a secret.
func (a *Adapter) AddTags(ctx context.Context, name string, tags map[string]string) error {
	input := &secretapi.TagResourceInput{
		SecretId: lo.ToPtr(name),
		Tags:     convertToAWSTags(tags),
	}

	_, err := a.client.TagResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to add tags: %w", err)
	}

	return nil
}

// RemoveTags removes tags from a secret by key names.
func (a *Adapter) RemoveTags(ctx context.Context, name string, keys []string) error {
	input := &secretapi.UntagResourceInput{
		SecretId: lo.ToPtr(name),
		TagKeys:  keys,
	}

	_, err := a.client.UntagResource(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to remove tags: %w", err)
	}

	return nil
}

// ============================================================================
// SecretRestorer Implementation (Optional Interface)
// ============================================================================

// RestoreSecret restores a previously deleted secret.
func (a *Adapter) RestoreSecret(ctx context.Context, name string) (*model.SecretRestoreResult, error) {
	input := &secretapi.RestoreSecretInput{
		SecretId: lo.ToPtr(name),
	}

	output, err := a.client.RestoreSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to restore secret: %w", err)
	}

	return &model.SecretRestoreResult{
		Name: lo.FromPtr(output.Name),
		ARN:  lo.FromPtr(output.ARN),
	}, nil
}

// ============================================================================
// SecretDescriber Implementation (Optional Interface)
// ============================================================================

// DescribeSecret retrieves secret metadata without the value.
func (a *Adapter) DescribeSecret(ctx context.Context, name string) (*model.SecretListItem, error) {
	input := &secretapi.DescribeSecretInput{
		SecretId: lo.ToPtr(name),
	}

	output, err := a.client.DescribeSecret(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe secret: %w", err)
	}

	return convertDescribeSecretOutput(output), nil
}

// ============================================================================
// Compile-time Interface Checks
// ============================================================================

var (
	_ provider.SecretReader    = (*Adapter)(nil)
	_ provider.SecretWriter    = (*Adapter)(nil)
	_ provider.SecretTagger    = (*Adapter)(nil)
	_ provider.SecretService   = (*Adapter)(nil)
	_ provider.SecretRestorer  = (*Adapter)(nil)
	_ provider.SecretDescriber = (*Adapter)(nil)
)

// ============================================================================
// Conversion Helpers (internal)
// ============================================================================

func convertGetSecretValueOutput(o *secretapi.GetSecretValueOutput) *model.Secret {
	if o == nil {
		return nil
	}

	return &model.Secret{
		Name:      lo.FromPtr(o.Name),
		Value:     lo.FromPtr(o.SecretString),
		Version:   lo.FromPtr(o.VersionId),
		CreatedAt: o.CreatedDate,
		Metadata: model.AWSSecretMeta{
			ARN:           lo.FromPtr(o.ARN),
			VersionStages: o.VersionStages,
		},
	}
}

func convertSecretVersion(v *secretapi.SecretVersionsListEntry) *model.SecretVersion {
	if v == nil {
		return nil
	}

	return &model.SecretVersion{
		Version:   lo.FromPtr(v.VersionId),
		CreatedAt: v.CreatedDate,
		Metadata: model.AWSSecretVersionMeta{
			VersionStages: v.VersionStages,
		},
	}
}

func convertSecretListEntry(e *secretapi.SecretListEntry) *model.SecretListItem {
	if e == nil {
		return nil
	}

	return &model.SecretListItem{
		Name:        lo.FromPtr(e.Name),
		Description: lo.FromPtr(e.Description),
		CreatedAt:   e.CreatedDate,
		UpdatedAt:   e.LastChangedDate,
		Tags:        convertFromAWSTags(e.Tags),
		Metadata: model.AWSSecretListItemMeta{
			ARN:         lo.FromPtr(e.ARN),
			DeletedDate: e.DeletedDate,
		},
	}
}

func convertDescribeSecretOutput(o *secretapi.DescribeSecretOutput) *model.SecretListItem {
	if o == nil {
		return nil
	}

	return &model.SecretListItem{
		Name:        lo.FromPtr(o.Name),
		Description: lo.FromPtr(o.Description),
		CreatedAt:   o.CreatedDate,
		UpdatedAt:   o.LastChangedDate,
		Tags:        convertFromAWSTags(o.Tags),
		Metadata: model.AWSSecretListItemMeta{
			ARN:         lo.FromPtr(o.ARN),
			DeletedDate: o.DeletedDate,
		},
	}
}

func convertToAWSTags(tags map[string]string) []secretapi.Tag {
	result := make([]secretapi.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, secretapi.Tag{
			Key:   lo.ToPtr(k),
			Value: lo.ToPtr(v),
		})
	}

	return result
}

func convertFromAWSTags(tags []secretapi.Tag) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			result[*tag.Key] = *tag.Value
		}
	}

	return result
}
