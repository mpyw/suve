// Package secret implements the provider.Store, provider.Restorer and
// provider.Describer contracts for AWS Secrets Manager. It confines all
// Secrets Manager SDK types to this package: version/label/shift resolution
// lives here, so AWS staging labels (AWSCURRENT etc.) never leak past this
// boundary. Spec PARSING stays generic via secretversion.Parse.
package secret

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// Client is the narrow Secrets Manager surface this adapter needs. The concrete
// *secretsmanager.Client satisfies it; tests provide their own mock.
//
//nolint:interfacebloat // mirrors the Secrets Manager operations this adapter uses; splitting adds no value
type Client interface {
	GetSecretValue(
		ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.GetSecretValueOutput, error)
	ListSecretVersionIds(
		ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.ListSecretVersionIdsOutput, error)
	DescribeSecret(
		ctx context.Context, params *secretsmanager.DescribeSecretInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.DescribeSecretOutput, error)
	CreateSecret(
		ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.CreateSecretOutput, error)
	UpdateSecret(
		ctx context.Context, params *secretsmanager.UpdateSecretInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.UpdateSecretOutput, error)
	RotateSecret(
		ctx context.Context, params *secretsmanager.RotateSecretInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.RotateSecretOutput, error)
	DeleteSecret(
		ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.DeleteSecretOutput, error)
	RestoreSecret(
		ctx context.Context, params *secretsmanager.RestoreSecretInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.RestoreSecretOutput, error)
	TagResource(
		ctx context.Context, params *secretsmanager.TagResourceInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.TagResourceOutput, error)
	UntagResource(
		ctx context.Context, params *secretsmanager.UntagResourceInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.UntagResourceOutput, error)
	ListSecrets(
		ctx context.Context, params *secretsmanager.ListSecretsInput, optFns ...func(*secretsmanager.Options),
	) (*secretsmanager.ListSecretsOutput, error)
}

// Store is the Secrets Manager implementation of provider.Store (+ Restorer, Describer).
type Store struct {
	client Client
}

// Compile-time assertions that Store implements the provider contracts.
var (
	_ provider.Store     = (*Store)(nil)
	_ provider.Restorer  = (*Store)(nil)
	_ provider.Describer = (*Store)(nil)
)

// New builds a Store backed by the given Secrets Manager client.
func New(client Client) *Store {
	return &Store{client: client}
}

// Resolve parses the version spec (generic) and resolves it (AWS-specific) to
// an opaque VersionRef holding a concrete version ID. Staging labels
// (:AWSCURRENT etc.) and ~shift are resolved here against ListSecretVersionIds,
// so no AWS label escapes this package. An empty/latest spec resolves to the
// latest ref (empty id).
func (s *Store) Resolve(ctx context.Context, name, spec string) (provider.VersionRef, error) {
	parsed, err := secretversion.Parse(name + spec)
	if err != nil {
		return provider.VersionRef{}, err
	}

	// Fast paths that need no listing.
	if !parsed.HasShift() {
		switch {
		case parsed.Absolute.ID != nil:
			return provider.NewVersionRef(*parsed.Absolute.ID), nil
		case parsed.Absolute.Label == nil:
			// No absolute spec at all: latest/current.
			return provider.NewVersionRef(""), nil
		}
	}

	// Label resolution and/or shift require the full version list.
	versions, err := s.client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		return provider.VersionRef{}, fmt.Errorf("failed to list versions: %w", err)
	}

	list := versions.Versions
	if len(list) == 0 {
		return provider.VersionRef{}, fmt.Errorf("secret not found or has no versions: %s", name)
	}

	sortNewestFirst(list)

	baseIdx, err := baseIndex(list, parsed.Absolute)
	if err != nil {
		return provider.VersionRef{}, err
	}

	targetIdx := baseIdx + parsed.Shift
	if targetIdx >= len(list) {
		return provider.VersionRef{}, fmt.Errorf("version shift out of range: ~%d", parsed.Shift)
	}

	return provider.NewVersionRef(aws.ToString(list[targetIdx].VersionId)), nil
}

// baseIndex finds the starting index in a newest-first version list for the
// given absolute spec (version ID or staging label). No absolute spec anchors
// at the newest version (index 0).
func baseIndex(list []types.SecretVersionsListEntry, abs secretversion.AbsoluteSpec) (int, error) {
	switch {
	case abs.ID != nil:
		_, idx, found := lo.FindIndexOf(list, func(v types.SecretVersionsListEntry) bool {
			return aws.ToString(v.VersionId) == *abs.ID
		})
		if !found {
			return 0, fmt.Errorf("version ID not found: %s", *abs.ID)
		}

		return idx, nil
	case abs.Label != nil:
		_, idx, found := lo.FindIndexOf(list, func(v types.SecretVersionsListEntry) bool {
			return slices.Contains(v.VersionStages, *abs.Label)
		})
		if !found {
			return 0, fmt.Errorf("version label not found: %s", *abs.Label)
		}

		return idx, nil
	default:
		return 0, nil
	}
}

// sortNewestFirst sorts version entries by creation date, newest first.
func sortNewestFirst(list []types.SecretVersionsListEntry) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedDate == nil {
			return false
		}

		if list[j].CreatedDate == nil {
			return true
		}

		return list[i].CreatedDate.After(*list[j].CreatedDate)
	})
}

// Get retrieves the secret at the given ref (current when ref is latest) and
// maps it to a domain.Entry with description and tags. Type is always secret.
func (s *Store) Get(ctx context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(name),
	}
	if !ref.IsLatest() {
		input.VersionId = aws.String(ref.ID())
	}

	out, err := s.client.GetSecretValue(ctx, input)
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("%w: %s", provider.ErrNotFound, name)
		}

		return nil, fmt.Errorf("failed to get secret value: %w", err)
	}

	entry := &domain.Entry{
		Name:  aws.ToString(out.Name),
		Value: aws.ToString(out.SecretString),
		Type:  domain.ValueTypeSecret,
		Version: domain.Version{
			ID:      aws.ToString(out.VersionId),
			Label:   firstStage(out.VersionStages),
			Created: out.CreatedDate,
		},
		Modified: out.CreatedDate,
		Extra:    []domain.Field{{Label: "ARN", Value: aws.ToString(out.ARN)}},
	}

	// Description and tags are best-effort via DescribeSecret.
	desc, err := s.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(name),
	})
	if err == nil && desc != nil {
		entry.Description = aws.ToString(desc.Description)
		entry.Tags = mapTags(desc.Tags)
	}

	return entry, nil
}

// History returns the secret's version history, newest first.
func (s *Store) History(ctx context.Context, name string) ([]domain.Version, error) {
	out, err := s.client.ListSecretVersionIds(ctx, &secretsmanager.ListSecretVersionIdsInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list secret versions: %w", err)
	}

	list := out.Versions
	sortNewestFirst(list)

	return lo.Map(list, func(v types.SecretVersionsListEntry, _ int) domain.Version {
		return domain.Version{
			ID:      aws.ToString(v.VersionId),
			Label:   firstStage(v.VersionStages),
			Created: v.CreatedDate,
		}
	}), nil
}

// List returns the names of all secrets, paging through ListSecrets.
func (s *Store) List(ctx context.Context) ([]string, error) {
	d := debug.From(ctx)

	var (
		names []string
		token *string
		pages int
	)

	for {
		out, err := s.client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{
			NextToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}

		pages++
		d.Logf("aws secretsmanager: ListSecrets page %d -> %d secrets\n", pages, len(out.SecretList))

		for _, sec := range out.SecretList {
			names = append(names, aws.ToString(sec.Name))
		}

		if aws.ToString(out.NextToken) == "" {
			break
		}

		token = out.NextToken
	}

	// The total makes a successful-but-empty result (wrong region/account)
	// visible at a glance, which a bodyless HTTP log cannot.
	d.Logf("aws secretsmanager: ListSecrets total %d secrets in %d page(s)\n", len(names), pages)

	return names, nil
}

// Create creates a new secret and returns the resulting version. It returns a
// wrapped provider.ErrAlreadyExists if the secret already exists (it never
// writes a new version of an existing secret). The valueType is ignored
// (Secrets Manager values are always secret).
func (s *Store) Create(
	ctx context.Context, name, value string, _ domain.ValueType, description string, opts ...provider.WriteOption,
) (domain.Version, error) {
	input := &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String(value),
	}
	if description != "" {
		input.Description = aws.String(description)
	}

	applyCreateOptions(input, opts)

	created, err := s.client.CreateSecret(ctx, input)
	if err != nil {
		var exists *types.ResourceExistsException
		if errors.As(err, &exists) {
			return domain.Version{}, fmt.Errorf("%w: %s", provider.ErrAlreadyExists, name)
		}

		return domain.Version{}, fmt.Errorf("failed to create secret: %w", err)
	}

	if err := s.applyRotation(ctx, name, opts); err != nil {
		return domain.Version{}, err
	}

	return domain.Version{ID: aws.ToString(created.VersionId)}, nil
}

// Put creates the secret, or updates it (new version + metadata) if it already
// exists. The valueType is ignored (Secrets Manager values are always secret).
// On an existing secret the description is updated as well (via UpdateSecret),
// unlike Create which is create-only.
func (s *Store) Put(
	ctx context.Context, name, value string, _ domain.ValueType, description string, opts ...provider.WriteOption,
) (domain.Version, error) {
	createInput := &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String(value),
	}
	if description != "" {
		createInput.Description = aws.String(description)
	}

	applyCreateOptions(createInput, opts)

	created, err := s.client.CreateSecret(ctx, createInput)
	if err == nil {
		if err := s.applyRotation(ctx, name, opts); err != nil {
			return domain.Version{}, err
		}

		return domain.Version{ID: aws.ToString(created.VersionId)}, nil
	}

	// Already exists: update the value (new version) and metadata in one call.
	var exists *types.ResourceExistsException
	if !errors.As(err, &exists) {
		return domain.Version{}, fmt.Errorf("failed to create secret: %w", err)
	}

	updateInput := &secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(name),
		SecretString: aws.String(value),
	}
	if description != "" {
		updateInput.Description = aws.String(description)
	}

	applyUpdateOptions(updateInput, opts)

	updated, err := s.client.UpdateSecret(ctx, updateInput)
	if err != nil {
		return domain.Version{}, fmt.Errorf("failed to update secret: %w", err)
	}

	if err := s.applyRotation(ctx, name, opts); err != nil {
		return domain.Version{}, err
	}

	return domain.Version{ID: aws.ToString(updated.VersionId)}, nil
}

// applyRotation issues a RotateSecret request when a RotationRules option with a
// non-zero interval was provided; otherwise it is a no-op.
func (s *Store) applyRotation(ctx context.Context, name string, opts []provider.WriteOption) error {
	rules, ok := rotationOption(opts)
	if !ok {
		return nil
	}

	if _, err := s.client.RotateSecret(ctx, rotationInput(name, rules)); err != nil {
		return fmt.Errorf("failed to configure secret rotation: %w", err)
	}

	return nil
}

// Delete schedules a secret for deletion. DeleteOptions select immediate
// deletion (ForceDelete) or a custom recovery window (RecoveryWindow); with no
// options the AWS default recovery window applies.
func (s *Store) Delete(ctx context.Context, name string, opts ...provider.DeleteOption) error {
	input := &secretsmanager.DeleteSecretInput{
		SecretId: aws.String(name),
	}

	applyDeleteOptions(input, opts)

	_, err := s.client.DeleteSecret(ctx, input)
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return fmt.Errorf("%w: %s", provider.ErrNotFound, name)
		}

		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// Restore cancels a pending deletion for a secret.
func (s *Store) Restore(ctx context.Context, name string) error {
	_, err := s.client.RestoreSecret(ctx, &secretsmanager.RestoreSecretInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("failed to restore secret: %w", err)
	}

	return nil
}

// Describe returns the secret's metadata (description, tags, current version)
// without fetching its value. Type is always secret.
func (s *Store) Describe(ctx context.Context, name string) (*domain.Entry, error) {
	desc, err := s.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("%w: %s", provider.ErrNotFound, name)
		}

		return nil, fmt.Errorf("failed to describe secret: %w", err)
	}

	entry := &domain.Entry{
		Name:        aws.ToString(desc.Name),
		Type:        domain.ValueTypeSecret,
		Description: aws.ToString(desc.Description),
		Tags:        mapTags(desc.Tags),
		Modified:    desc.LastChangedDate,
		Extra:       []domain.Field{{Label: "ARN", Value: aws.ToString(desc.ARN)}},
	}

	// Best-effort: surface the current (AWSCURRENT) version if present.
	for versionID, stages := range desc.VersionIdsToStages {
		if slices.Contains(stages, "AWSCURRENT") {
			entry.Version = domain.Version{
				ID:      versionID,
				Label:   firstStage(stages),
				Created: desc.CreatedDate,
			}

			break
		}
	}

	return entry, nil
}

// Tag adds or updates tags on a secret.
func (s *Store) Tag(ctx context.Context, name string, add map[string]string) error {
	if len(add) == 0 {
		return nil
	}

	tags := lo.MapToSlice(add, func(k, v string) types.Tag {
		return types.Tag{Key: aws.String(k), Value: aws.String(v)}
	})

	_, err := s.client.TagResource(ctx, &secretsmanager.TagResourceInput{
		SecretId: aws.String(name),
		Tags:     tags,
	})
	if err != nil {
		return fmt.Errorf("failed to add tags: %w", err)
	}

	return nil
}

// Untag removes tags (by key) from a secret.
func (s *Store) Untag(ctx context.Context, name string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	_, err := s.client.UntagResource(ctx, &secretsmanager.UntagResourceInput{
		SecretId: aws.String(name),
		TagKeys:  keys,
	})
	if err != nil {
		return fmt.Errorf("failed to remove tags: %w", err)
	}

	return nil
}

// firstStage returns the first staging label, or "" when there are none. It
// keeps the AWS label confined to a provider-neutral, informational Label.
func firstStage(stages []string) string {
	if len(stages) == 0 {
		return ""
	}

	return stages[0]
}

// mapTags maps Secrets Manager tags to provider-neutral domain tags.
func mapTags(tags []types.Tag) []domain.Tag {
	if len(tags) == 0 {
		return nil
	}

	return lo.Map(tags, func(t types.Tag, _ int) domain.Tag {
		return domain.Tag{Key: aws.ToString(t.Key), Value: aws.ToString(t.Value)}
	})
}
