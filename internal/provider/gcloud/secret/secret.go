// Package secret implements the provider.Store contract (Reader/Writer/Tagger)
// for Google Cloud Secret Manager. It confines all Secret Manager SDK types to
// this package: resource-path construction and integer-version resolution live
// here, so no Google Cloud type escapes the provider seam. Spec PARSING stays
// generic via gcloudversion.Parse.
//
// Google Cloud Secret Manager differs from AWS Secrets Manager in three ways
// that shape this adapter:
//
//   - Versions are positive integers ("1", "2", ...) or the "latest" alias;
//     there are no staging labels (a ":LABEL" spec is rejected by gcloudversion).
//   - Deletion is permanent (no recovery window), so this store implements
//     neither provider.Restorer nor provider.Describer.
//   - Tags are secret "labels" mutated via an UpdateSecret read-modify-write.
//   - A description is stored as a secret ANNOTATION under the "description" key.
//     Google Cloud secrets have no native description field, but annotations
//     (secretmanagerpb.Secret.Annotations, distinct from the labels that back
//     suve's tag axis) are exactly "for client tools to store their own state",
//     so a description annotation adds description support with no collision.
package secret

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/samber/lo"
	"github.com/samber/lo/it"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/gcloudversion"
)

// Client is the narrow Secret Manager surface this adapter needs. The list
// methods return drained slices rather than the SDK's iterators so tests can
// mock the interface trivially; the production adapter (see Wrap) confines the
// iterator draining and the concrete *secretmanager.Client to this package.
type Client interface {
	AccessSecretVersion(
		ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest,
	) (*secretmanagerpb.AccessSecretVersionResponse, error)
	GetSecretVersion(
		ctx context.Context, req *secretmanagerpb.GetSecretVersionRequest,
	) (*secretmanagerpb.SecretVersion, error)
	ListSecretVersions(
		ctx context.Context, req *secretmanagerpb.ListSecretVersionsRequest,
	) ([]*secretmanagerpb.SecretVersion, error)
	ListSecrets(
		ctx context.Context, req *secretmanagerpb.ListSecretsRequest,
	) ([]*secretmanagerpb.Secret, error)
	GetSecret(
		ctx context.Context, req *secretmanagerpb.GetSecretRequest,
	) (*secretmanagerpb.Secret, error)
	CreateSecret(
		ctx context.Context, req *secretmanagerpb.CreateSecretRequest,
	) (*secretmanagerpb.Secret, error)
	AddSecretVersion(
		ctx context.Context, req *secretmanagerpb.AddSecretVersionRequest,
	) (*secretmanagerpb.SecretVersion, error)
	DeleteSecret(ctx context.Context, req *secretmanagerpb.DeleteSecretRequest) error
	UpdateSecret(
		ctx context.Context, req *secretmanagerpb.UpdateSecretRequest,
	) (*secretmanagerpb.Secret, error)
}

// Store is the Secret Manager implementation of provider.Store. Unlike the AWS
// Secrets Manager store it implements neither Restorer nor Describer: Google
// Cloud secret deletion is permanent.
type Store struct {
	client  Client
	project string
}

// Compile-time assertion that Store implements the provider contract.
var _ provider.Store = (*Store)(nil)

// descriptionAnnotation is the secret-annotation key under which suve stores a
// secret's free-text description. Annotations are distinct from labels (which
// back the tag axis), so this never collides with a user tag.
const descriptionAnnotation = "description"

// New builds a Store backed by the given client for the given project id.
func New(client Client, project string) *Store {
	return &Store{client: client, project: project}
}

// parent returns the project resource path "projects/{project}".
func (s *Store) parent() string {
	return "projects/" + s.project
}

// secretPath returns the secret resource path "projects/{project}/secrets/{name}".
func (s *Store) secretPath(name string) string {
	return fmt.Sprintf("projects/%s/secrets/%s", s.project, name)
}

// versionPath returns the version resource path
// "projects/{project}/secrets/{name}/versions/{version}" ("latest" is a valid version alias).
func (s *Store) versionPath(name, version string) string {
	return fmt.Sprintf("projects/%s/secrets/%s/versions/%s", s.project, name, version)
}

// Resolve parses the version spec (generic) and resolves it to an opaque
// VersionRef holding the integer version string (or "" for latest). A ~shift is
// applied by walking ALL versions (any state) newest-first — the same anchor a
// bare name resolves to, so a `~N` never skips disabled/destroyed versions; a
// "#<int>" without a shift needs no listing. A ":LABEL" spec is rejected by
// gcloudversion.Parse.
func (s *Store) Resolve(ctx context.Context, name, spec string) (provider.VersionRef, error) {
	parsed, err := gcloudversion.Parse(name + spec)
	if err != nil {
		return provider.VersionRef{}, err
	}

	if !parsed.HasShift() {
		if parsed.Absolute.Version != nil {
			return provider.NewVersionRef(strconv.FormatInt(*parsed.Absolute.Version, 10)), nil
		}

		// No absolute spec: latest/current.
		return provider.NewVersionRef(""), nil
	}

	// A shift counts back from `latest`, so it walks the full version list
	// (any state), newest first — the same anchor the bare name resolves to.
	versions, err := s.versionsNewestFirst(ctx, name)
	if err != nil {
		return provider.VersionRef{}, err
	}

	if len(versions) == 0 {
		return provider.VersionRef{}, fmt.Errorf("secret has no versions: %s", name)
	}

	baseIdx := 0

	if parsed.Absolute.Version != nil {
		want := strconv.FormatInt(*parsed.Absolute.Version, 10)

		_, idx, found := lo.FindIndexOf(versions, func(v *secretmanagerpb.SecretVersion) bool {
			return versionNumber(v.GetName()) == want
		})
		if !found {
			return provider.VersionRef{}, fmt.Errorf("version not found: %s", want)
		}

		baseIdx = idx
	}

	targetIdx := baseIdx + parsed.Shift
	if targetIdx < 0 || targetIdx >= len(versions) {
		return provider.VersionRef{}, fmt.Errorf("version shift out of range: ~%d", parsed.Shift)
	}

	return provider.NewVersionRef(versionNumber(versions[targetIdx].GetName())), nil
}

// versionsNewestFirst lists ALL the secret's versions (any state) sorted by
// version number, newest (highest) first.
//
// This is the same set History shows and the same ordering the `latest` alias
// anchors at, so a ~shift counts back positionally from whatever the bare name
// resolves to. Filtering to ENABLED here (as an earlier version did) made a
// bare `~N` skip disabled/destroyed versions that `latest` still counts,
// yielding a version further back than "N before the current one".
func (s *Store) versionsNewestFirst(ctx context.Context, name string) ([]*secretmanagerpb.SecretVersion, error) {
	versions, err := s.client.ListSecretVersions(ctx, &secretmanagerpb.ListSecretVersionsRequest{
		Parent: s.secretPath(name),
	})
	if err != nil {
		return nil, mapError(err, name, "list secret versions")
	}

	sortNewestFirst(versions)

	return versions, nil
}

// Get retrieves the secret value at the given ref (latest when ref is latest)
// and maps it to a domain.Entry. Type is always secret; the integer version and
// creation time populate Version; the secret's labels become Tags and its
// "description" annotation becomes Description. Extra is left empty (Google
// Cloud has no ARN-like metadata to surface).
func (s *Store) Get(ctx context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
	version := ref.ID()
	if version == "" {
		version = "latest"
	}

	access, err := s.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: s.versionPath(name, version),
	})
	if err != nil {
		return nil, mapError(err, name, "access secret version")
	}

	resolvedName := access.GetName()

	entry := &domain.Entry{
		Name:  name,
		Value: string(access.GetPayload().GetData()),
		Type:  domain.ValueTypeSecret,
		Version: domain.Version{
			ID: versionNumber(resolvedName),
		},
	}

	// Best-effort: the access response carries no timestamps, so fetch the
	// version metadata for the creation time.
	if sv, verr := s.client.GetSecretVersion(ctx, &secretmanagerpb.GetSecretVersionRequest{
		Name: resolvedName,
	}); verr == nil && sv != nil {
		created := toTime(sv.GetCreateTime())
		entry.Version.Created = created
		entry.Version.State = stateLabel(sv.GetState())
		entry.Modified = created
	}

	// Best-effort: labels and the description annotation live on the secret, not
	// the version.
	if sec, serr := s.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{
		Name: s.secretPath(name),
	}); serr == nil && sec != nil {
		entry.Tags = mapLabels(sec.GetLabels())
		entry.Description = sec.GetAnnotations()[descriptionAnnotation]
	}

	return entry, nil
}

// History returns the secret's version history, newest first. The per-version
// state (enabled/disabled/destroyed) is surfaced in the neutral Version.State
// for display; destroyed/disabled versions have no accessible value.
func (s *Store) History(ctx context.Context, name string) ([]domain.Version, error) {
	versions, err := s.client.ListSecretVersions(ctx, &secretmanagerpb.ListSecretVersionsRequest{
		Parent: s.secretPath(name),
	})
	if err != nil {
		return nil, mapError(err, name, "list secret versions")
	}

	sortNewestFirst(versions)

	return lo.Map(versions, func(v *secretmanagerpb.SecretVersion, _ int) domain.Version {
		return domain.Version{
			ID:      versionNumber(v.GetName()),
			State:   stateLabel(v.GetState()),
			Created: toTime(v.GetCreateTime()),
		}
	}), nil
}

// List returns the short names of all secrets in the project.
func (s *Store) List(ctx context.Context) ([]string, error) {
	secrets, err := s.client.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{
		Parent: s.parent(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	// The total makes a successful-but-empty result (wrong project) visible at
	// a glance; the parent pins down which project was actually queried.
	debug.From(ctx).Logf("gcloud secretmanager: ListSecrets (%s) -> %d secrets\n", s.parent(), len(secrets))

	return lo.Map(secrets, func(sec *secretmanagerpb.Secret, _ int) string {
		return shortName(sec.GetName())
	}), nil
}

// Create creates a new secret (create-only) and adds its initial value as the
// first version. It returns a wrapped provider.ErrAlreadyExists if the secret
// already exists. The valueType is ignored (Google Cloud values are always
// secret); a non-empty description is stored as the "description" annotation.
func (s *Store) Create(
	ctx context.Context, name, value string, _ domain.ValueType, description string, _ ...provider.WriteOption,
) (domain.Version, error) {
	_, err := s.client.CreateSecret(ctx, s.createRequest(name, description))
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return domain.Version{}, fmt.Errorf("%w: %s", provider.ErrAlreadyExists, name)
		}

		return domain.Version{}, fmt.Errorf("failed to create secret: %w", err)
	}

	return s.addVersion(ctx, name, value)
}

// Put adds a new version to the secret (upsert). If the secret does not yet
// exist it is created first, then the version is added. The valueType is
// ignored; a non-empty description is written to the "description" annotation
// (updating it on an already-existing secret, mirroring the AWS Put contract).
//
// Unlike AWS (whose UpdateSecret sets value and description atomically), Secret
// Manager needs a separate UpdateSecret for the annotation, so on an existing
// secret the new version is committed first and the annotation second. A failure
// to write the annotation is reported (never silently dropped, per #666), even
// though the value version has already landed.
func (s *Store) Put(
	ctx context.Context, name, value string, _ domain.ValueType, description string, _ ...provider.WriteOption,
) (domain.Version, error) {
	sv, err := s.client.AddSecretVersion(ctx, s.addRequest(name, value))
	if err == nil {
		// The secret already existed: update its description annotation to match.
		if aerr := s.applyDescription(ctx, name, description); aerr != nil {
			return domain.Version{}, aerr
		}

		return domain.Version{ID: versionNumber(sv.GetName())}, nil
	}

	// Upsert semantics: create the secret on first write, then add the version.
	if status.Code(err) != codes.NotFound {
		return domain.Version{}, fmt.Errorf("failed to add secret version: %w", err)
	}

	if _, cerr := s.client.CreateSecret(ctx, s.createRequest(name, description)); cerr != nil {
		// A concurrent create is fine; any other failure is fatal.
		if status.Code(cerr) != codes.AlreadyExists {
			return domain.Version{}, fmt.Errorf("failed to create secret: %w", cerr)
		}

		// A concurrent create won the race, so our annotation was not applied;
		// set it now to honor the requested description.
		if aerr := s.applyDescription(ctx, name, description); aerr != nil {
			return domain.Version{}, aerr
		}
	}

	return s.addVersion(ctx, name, value)
}

// addVersion adds a new secret version and returns the resulting domain.Version.
func (s *Store) addVersion(ctx context.Context, name, value string) (domain.Version, error) {
	sv, err := s.client.AddSecretVersion(ctx, s.addRequest(name, value))
	if err != nil {
		return domain.Version{}, fmt.Errorf("failed to add secret version: %w", err)
	}

	return domain.Version{ID: versionNumber(sv.GetName())}, nil
}

// createRequest builds a CreateSecretRequest with automatic replication. A
// non-empty description is carried as the "description" annotation on the new
// secret; an empty description leaves annotations unset.
func (s *Store) createRequest(name, description string) *secretmanagerpb.CreateSecretRequest {
	sec := &secretmanagerpb.Secret{
		Replication: &secretmanagerpb.Replication{
			Replication: &secretmanagerpb.Replication_Automatic_{
				Automatic: &secretmanagerpb.Replication_Automatic{},
			},
		},
	}

	if description != "" {
		sec.Annotations = map[string]string{descriptionAnnotation: description}
	}

	return &secretmanagerpb.CreateSecretRequest{
		Parent:   s.parent(),
		SecretId: name,
		Secret:   sec,
	}
}

// applyDescription writes a non-empty description to the "description"
// annotation of an existing secret via a read-modify-write UpdateSecret,
// preserving any other annotations. An empty description is a no-op (Put/Create
// never clear an existing description, mirroring the AWS contract).
func (s *Store) applyDescription(ctx context.Context, name, description string) error {
	if description == "" {
		return nil
	}

	annotations, err := s.currentAnnotations(ctx, name)
	if err != nil {
		return err
	}

	annotations[descriptionAnnotation] = description

	_, err = s.client.UpdateSecret(ctx, &secretmanagerpb.UpdateSecretRequest{
		Secret: &secretmanagerpb.Secret{
			Name:        s.secretPath(name),
			Annotations: annotations,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"annotations"}},
	})
	if err != nil {
		return fmt.Errorf("failed to update secret description: %w", err)
	}

	return nil
}

// currentAnnotations fetches the secret's current annotations as a mutable map.
func (s *Store) currentAnnotations(ctx context.Context, name string) (map[string]string, error) {
	sec, err := s.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{
		Name: s.secretPath(name),
	})
	if err != nil {
		return nil, mapError(err, name, "get secret")
	}

	annotations := make(map[string]string, len(sec.GetAnnotations()))
	maps.Copy(annotations, sec.GetAnnotations())

	return annotations, nil
}

// addRequest builds an AddSecretVersionRequest carrying the given value.
func (s *Store) addRequest(name, value string) *secretmanagerpb.AddSecretVersionRequest {
	return &secretmanagerpb.AddSecretVersionRequest{
		Parent:  s.secretPath(name),
		Payload: &secretmanagerpb.SecretPayload{Data: []byte(value)},
	}
}

// Delete permanently deletes a secret. Google Cloud has no recovery window, so
// provider.DeleteOptions (AWS-specific) are ignored.
func (s *Store) Delete(ctx context.Context, name string, _ ...provider.DeleteOption) error {
	err := s.client.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{
		Name: s.secretPath(name),
	})
	if err != nil {
		return mapError(err, name, "delete secret")
	}

	return nil
}

// Tag adds or updates labels on a secret via a read-modify-write UpdateSecret.
func (s *Store) Tag(ctx context.Context, name string, add map[string]string) error {
	if len(add) == 0 {
		return nil
	}

	labels, err := s.currentLabels(ctx, name)
	if err != nil {
		return err
	}

	maps.Copy(labels, add)

	return s.updateLabels(ctx, name, labels)
}

// Untag removes labels (by key) from a secret via a read-modify-write UpdateSecret.
func (s *Store) Untag(ctx context.Context, name string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	labels, err := s.currentLabels(ctx, name)
	if err != nil {
		return err
	}

	for _, k := range keys {
		delete(labels, k)
	}

	return s.updateLabels(ctx, name, labels)
}

// currentLabels fetches the secret's current labels as a mutable map.
func (s *Store) currentLabels(ctx context.Context, name string) (map[string]string, error) {
	sec, err := s.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{
		Name: s.secretPath(name),
	})
	if err != nil {
		return nil, mapError(err, name, "get secret")
	}

	labels := make(map[string]string, len(sec.GetLabels()))
	maps.Copy(labels, sec.GetLabels())

	return labels, nil
}

// updateLabels writes the labels map back to the secret with a labels field mask.
func (s *Store) updateLabels(ctx context.Context, name string, labels map[string]string) error {
	_, err := s.client.UpdateSecret(ctx, &secretmanagerpb.UpdateSecretRequest{
		Secret: &secretmanagerpb.Secret{
			Name:   s.secretPath(name),
			Labels: labels,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"labels"}},
	})
	if err != nil {
		return fmt.Errorf("failed to update secret labels: %w", err)
	}

	return nil
}

// mapError maps a gRPC NOT_FOUND to provider.ErrNotFound and otherwise wraps the
// error with the given operation description.
func mapError(err error, name, op string) error {
	if status.Code(err) == codes.NotFound {
		return fmt.Errorf("%w: %s", provider.ErrNotFound, name)
	}

	return fmt.Errorf("failed to %s: %w", op, err)
}

// versionNumber extracts the trailing integer version segment from a version
// resource name ("projects/P/secrets/S/versions/N" -> "N").
func versionNumber(resourceName string) string {
	return lastSegment(resourceName)
}

// shortName extracts the trailing secret name segment from a secret resource
// name ("projects/P/secrets/S" -> "S").
func shortName(resourceName string) string {
	return lastSegment(resourceName)
}

// lastSegment returns the substring after the final '/', or the whole string
// when there is no '/'.
func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}

	return s
}

// stateLabel maps a SecretVersion state to a lowercase display label; the
// STATE_UNSPECIFIED zero value yields "".
func stateLabel(state secretmanagerpb.SecretVersion_State) string {
	switch state {
	case secretmanagerpb.SecretVersion_ENABLED:
		return "enabled"
	case secretmanagerpb.SecretVersion_DISABLED:
		return "disabled"
	case secretmanagerpb.SecretVersion_DESTROYED:
		return "destroyed"
	default:
		return ""
	}
}

// sortNewestFirst sorts versions by integer version number, highest (newest)
// first. Names that do not parse as integers sort last.
func sortNewestFirst(versions []*secretmanagerpb.SecretVersion) {
	sort.SliceStable(versions, func(i, j int) bool {
		return versionInt(versions[i]) > versionInt(versions[j])
	})
}

// versionInt parses a version's integer number, returning -1 when unparseable.
func versionInt(v *secretmanagerpb.SecretVersion) int64 {
	n, err := strconv.ParseInt(versionNumber(v.GetName()), 10, 64)
	if err != nil {
		return -1
	}

	return n
}

// toTime converts a protobuf timestamp to a *time.Time, or nil when absent.
func toTime(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}

	t := ts.AsTime()

	return &t
}

// mapLabels converts a Google Cloud labels map to a sorted slice of neutral
// domain tags (sorted by key for deterministic display).
func mapLabels(labels map[string]string) []domain.Tag {
	if len(labels) == 0 {
		return nil
	}

	return slices.Collect(it.Map(maputil.SortedKeys(labels), func(k string) domain.Tag {
		return domain.Tag{Key: k, Value: labels[k]}
	}))
}
