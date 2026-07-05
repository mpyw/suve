// Package keyvault implements the provider.Store contract (Reader/Writer/Tagger)
// for Azure Key Vault secrets. It confines all Key Vault SDK types to this
// package: opaque version-id resolution and tag read-modify-write live here, so
// no Azure type escapes the provider seam. Spec PARSING stays generic via
// azurekvversion.Parse.
//
// Azure Key Vault differs from AWS Secrets Manager in ways that shape this
// adapter:
//
//   - Versions are opaque strings (32-character hex ids) or the empty "current"
//     alias; there are no staging labels (a ":LABEL" spec is rejected by
//     azurekvversion).
//   - There is no create-only API: SetSecret always creates a new version. To
//     honor Create's create-only contract this adapter probes with GetSecret
//     first and reports provider.ErrAlreadyExists when the secret exists. This
//     check-then-set is inherently racy; a concurrent create is not detected.
//   - Tags live on a secret version and are mutated via an UpdateSecretProperties
//     read-modify-write against the current version.
package keyvault

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"sort"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/azurekvversion"
)

// Client is the narrow Key Vault secrets surface this adapter needs. The list
// methods return drained slices rather than the SDK's pagers so tests can mock
// the interface trivially; the production adapter (see Wrap) confines the pager
// draining and the concrete *azsecrets.Client to this package.
type Client interface {
	GetSecret(ctx context.Context, name, version string) (azsecrets.GetSecretResponse, error)
	SetSecret(
		ctx context.Context, name string, params azsecrets.SetSecretParameters,
	) (azsecrets.SetSecretResponse, error)
	DeleteSecret(ctx context.Context, name string) (azsecrets.DeleteSecretResponse, error)
	UpdateSecretProperties(
		ctx context.Context, name, version string, params azsecrets.UpdateSecretPropertiesParameters,
	) (azsecrets.UpdateSecretPropertiesResponse, error)
	ListSecretProperties(ctx context.Context) ([]*azsecrets.SecretProperties, error)
	ListSecretPropertiesVersions(ctx context.Context, name string) ([]*azsecrets.SecretProperties, error)
}

// Store is the Key Vault implementation of provider.Store. Like the Google
// Cloud secret store it implements neither Restorer nor Describer.
type Store struct {
	client Client
}

// Compile-time assertion that Store implements the provider contract.
var _ provider.Store = (*Store)(nil)

// New builds a Store backed by the given client.
func New(client Client) *Store {
	return &Store{client: client}
}

// secretVersion is a provider-neutral snapshot of one Key Vault secret version,
// extracted from the SDK's SecretProperties so ordering and shifting logic never
// touch Azure types directly.
type secretVersion struct {
	id      string
	created *time.Time
	enabled bool
}

// Resolve parses the version spec (generic) and resolves it to an opaque
// VersionRef holding the version id (or "" for the current version). A ~shift is
// applied by walking the versions newest-first; a "#<id>" without a shift needs
// no listing. A ":LABEL" spec is rejected by azurekvversion.Parse.
func (s *Store) Resolve(ctx context.Context, name, spec string) (provider.VersionRef, error) {
	parsed, err := azurekvversion.Parse(name + spec)
	if err != nil {
		return provider.VersionRef{}, err
	}

	if !parsed.HasShift() {
		if parsed.Absolute.ID != nil {
			return provider.NewVersionRef(*parsed.Absolute.ID), nil
		}

		// No absolute spec: current version.
		return provider.NewVersionRef(""), nil
	}

	// A shift requires the version list, newest first.
	versions, err := s.versionsNewestFirst(ctx, name)
	if err != nil {
		return provider.VersionRef{}, err
	}

	if len(versions) == 0 {
		return provider.VersionRef{}, fmt.Errorf("secret has no versions: %s", name)
	}

	baseIdx := 0

	if parsed.Absolute.ID != nil {
		want := *parsed.Absolute.ID

		_, idx, found := lo.FindIndexOf(versions, func(v secretVersion) bool {
			return v.id == want
		})
		if !found {
			return provider.VersionRef{}, fmt.Errorf("version not found: %s", want)
		}

		baseIdx = idx
	}

	targetIdx := baseIdx + parsed.Shift
	if targetIdx >= len(versions) {
		return provider.VersionRef{}, fmt.Errorf("version shift out of range: ~%d", parsed.Shift)
	}

	return provider.NewVersionRef(versions[targetIdx].id), nil
}

// versionsNewestFirst lists the secret's versions sorted by creation time,
// newest first.
func (s *Store) versionsNewestFirst(ctx context.Context, name string) ([]secretVersion, error) {
	props, err := s.client.ListSecretPropertiesVersions(ctx, name)
	if err != nil {
		return nil, mapError(err, name, "list secret versions")
	}

	versions := lo.Map(props, func(p *azsecrets.SecretProperties, _ int) secretVersion {
		return toSecretVersion(p)
	})

	sortNewestFirst(versions)

	return versions, nil
}

// Get retrieves the secret value at the given ref (current when ref is latest)
// and maps it to a domain.Entry. Type is always secret; the opaque version id
// and creation time populate Version; the secret's tags become Tags.
func (s *Store) Get(ctx context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
	resp, err := s.client.GetSecret(ctx, name, ref.ID())
	if err != nil {
		return nil, mapError(err, name, "get secret")
	}

	entry := &domain.Entry{
		Name:  name,
		Value: lo.FromPtr(resp.Value),
		Type:  domain.ValueTypeSecret,
		Version: domain.Version{
			ID: versionID(resp.ID),
		},
		Tags: mapTags(resp.Tags),
	}

	if attr := resp.Attributes; attr != nil {
		entry.Version.Created = attr.Created
		entry.Version.Label = enabledLabel(attr.Enabled)
		entry.Modified = attr.Updated
	}

	return entry, nil
}

// History returns the secret's version history, newest first. The per-version
// enabled/disabled state is surfaced in the neutral Version.Label for display.
func (s *Store) History(ctx context.Context, name string) ([]domain.Version, error) {
	versions, err := s.versionsNewestFirst(ctx, name)
	if err != nil {
		return nil, err
	}

	return lo.Map(versions, func(v secretVersion, _ int) domain.Version {
		return domain.Version{
			ID:      v.id,
			Label:   boolLabel(v.enabled),
			Created: v.created,
		}
	}), nil
}

// List returns the short names of all secrets in the vault.
func (s *Store) List(ctx context.Context) ([]string, error) {
	props, err := s.client.ListSecretProperties(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	return lo.Map(props, func(p *azsecrets.SecretProperties, _ int) string {
		return secretName(p.ID)
	}), nil
}

// Create creates a new secret (create-only) and returns the resulting version.
// Key Vault has no create-only API, so this probes with GetSecret first and
// returns a wrapped provider.ErrAlreadyExists if the secret already exists (see
// the package doc for the inherent race). The valueType and description are
// ignored (Key Vault values are always secret and carry no description field).
func (s *Store) Create(
	ctx context.Context, name, value string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
) (domain.Version, error) {
	_, err := s.client.GetSecret(ctx, name, "")

	switch {
	case err == nil:
		return domain.Version{}, fmt.Errorf("%w: %s", provider.ErrAlreadyExists, name)
	case !isNotFound(err):
		return domain.Version{}, fmt.Errorf("failed to check secret existence: %w", err)
	}

	return s.setSecret(ctx, name, value)
}

// Put adds a new version to the secret (upsert) and returns the resulting
// version. The valueType and description are ignored.
func (s *Store) Put(
	ctx context.Context, name, value string, _ domain.ValueType, _ string, _ ...provider.WriteOption,
) (domain.Version, error) {
	return s.setSecret(ctx, name, value)
}

// setSecret sets a new secret value (a new version) and returns the resulting
// domain.Version.
func (s *Store) setSecret(ctx context.Context, name, value string) (domain.Version, error) {
	resp, err := s.client.SetSecret(ctx, name, azsecrets.SetSecretParameters{Value: lo.ToPtr(value)})
	if err != nil {
		return domain.Version{}, fmt.Errorf("failed to set secret: %w", err)
	}

	return domain.Version{ID: versionID(resp.ID)}, nil
}

// Delete deletes a secret. Key Vault delete is a soft-delete when the vault has
// soft-delete enabled; provider.DeleteOptions (AWS-specific) are ignored.
func (s *Store) Delete(ctx context.Context, name string, _ ...provider.DeleteOption) error {
	_, err := s.client.DeleteSecret(ctx, name)
	if err != nil {
		return mapError(err, name, "delete secret")
	}

	return nil
}

// Tag adds or updates tags on the secret's current version via a
// read-modify-write UpdateSecretProperties.
func (s *Store) Tag(ctx context.Context, name string, add map[string]string) error {
	if len(add) == 0 {
		return nil
	}

	tags, err := s.currentTags(ctx, name)
	if err != nil {
		return err
	}

	maps.Copy(tags, add)

	return s.updateTags(ctx, name, tags)
}

// Untag removes tags (by key) from the secret's current version via a
// read-modify-write UpdateSecretProperties.
func (s *Store) Untag(ctx context.Context, name string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	tags, err := s.currentTags(ctx, name)
	if err != nil {
		return err
	}

	for _, k := range keys {
		delete(tags, k)
	}

	return s.updateTags(ctx, name, tags)
}

// currentTags fetches the current version's tags as a mutable map.
func (s *Store) currentTags(ctx context.Context, name string) (map[string]string, error) {
	resp, err := s.client.GetSecret(ctx, name, "")
	if err != nil {
		return nil, mapError(err, name, "get secret")
	}

	tags := make(map[string]string, len(resp.Tags))
	for k, v := range resp.Tags {
		tags[k] = lo.FromPtr(v)
	}

	return tags, nil
}

// updateTags writes the tags map back to the secret's current version.
func (s *Store) updateTags(ctx context.Context, name string, tags map[string]string) error {
	azTags := make(map[string]*string, len(tags))
	for k, v := range tags {
		azTags[k] = lo.ToPtr(v)
	}

	_, err := s.client.UpdateSecretProperties(ctx, name, "", azsecrets.UpdateSecretPropertiesParameters{
		Tags: azTags,
	})
	if err != nil {
		return fmt.Errorf("failed to update secret tags: %w", err)
	}

	return nil
}

// toSecretVersion maps SDK SecretProperties to the neutral secretVersion.
func toSecretVersion(p *azsecrets.SecretProperties) secretVersion {
	v := secretVersion{id: versionID(p.ID)}

	if attr := p.Attributes; attr != nil {
		v.created = attr.Created
		v.enabled = lo.FromPtr(attr.Enabled)
	}

	return v
}

// sortNewestFirst sorts versions by creation time, newest first. Versions with
// no creation time sort last.
func sortNewestFirst(versions []secretVersion) {
	sort.SliceStable(versions, func(i, j int) bool {
		ci, cj := versions[i].created, versions[j].created

		switch {
		case ci == nil:
			return false
		case cj == nil:
			return true
		default:
			return ci.After(*cj)
		}
	})
}

// versionID extracts the version segment from a secret ID (returns "" when the
// id is nil or carries no version).
func versionID(id *azsecrets.ID) string {
	if id == nil {
		return ""
	}

	return id.Version()
}

// secretName extracts the short name from a secret ID (returns "" when nil).
func secretName(id *azsecrets.ID) string {
	if id == nil {
		return ""
	}

	return id.Name()
}

// enabledLabel maps a nullable enabled flag to a display label; a nil flag
// yields "" (unknown).
func enabledLabel(enabled *bool) string {
	if enabled == nil {
		return ""
	}

	return boolLabel(*enabled)
}

// boolLabel maps an enabled flag to a lowercase display label.
func boolLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}

	return "disabled"
}

// mapTags converts an Azure tags map to a sorted slice of neutral domain tags
// (sorted by key for deterministic display).
func mapTags(tags map[string]*string) []domain.Tag {
	if len(tags) == 0 {
		return nil
	}

	keys := lo.Keys(tags)
	sort.Strings(keys)

	return lo.Map(keys, func(k string, _ int) domain.Tag {
		return domain.Tag{Key: k, Value: lo.FromPtr(tags[k])}
	})
}

// isNotFound reports whether err is an Azure 404 response error.
func isNotFound(err error) bool {
	var re *azcore.ResponseError

	return errors.As(err, &re) && re.StatusCode == http.StatusNotFound
}

// mapError maps an Azure 404 to provider.ErrNotFound and otherwise wraps the
// error with the given operation description.
func mapError(err error, name, op string) error {
	if isNotFound(err) {
		return fmt.Errorf("%w: %s", provider.ErrNotFound, name)
	}

	return fmt.Errorf("failed to %s: %w", op, err)
}
