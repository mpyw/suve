package staging

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/awsparamversion"
)

// AWSParamStrategy implements ServiceStrategy for SSM Parameter Store. It is backed
// by a provider.Store rather than an AWS SDK client, so it carries no cloud SDK
// dependency. A nil store yields a parser-only strategy (ParseName/ParseSpec).
type AWSParamStrategy struct {
	store provider.Store
}

// NewAWSParamStrategy creates a new SSM Parameter Store strategy over the given
// provider store. A nil store is allowed for parser-only use.
func NewAWSParamStrategy(store provider.Store) *AWSParamStrategy {
	return &AWSParamStrategy{store: store}
}

// Service returns the service type.
func (s *AWSParamStrategy) Service() Service {
	return ServiceParam
}

// ServiceName returns the user-friendly service name.
func (s *AWSParamStrategy) ServiceName() string {
	return "SSM Parameter Store"
}

// ItemName returns the item name for messages.
func (s *AWSParamStrategy) ItemName() string {
	return "parameter"
}

// HasDeleteOptions returns false as SSM Parameter Store doesn't have delete options.
func (s *AWSParamStrategy) HasDeleteOptions() bool {
	return false
}

// Apply applies a staged operation to SSM Parameter Store.
func (s *AWSParamStrategy) Apply(ctx context.Context, name string, entry Entry) error {
	switch entry.Operation {
	case OperationCreate:
		return s.applyCreate(ctx, name, entry)
	case OperationUpdate:
		return s.applyUpdate(ctx, name, entry)
	case OperationDelete:
		return s.applyDelete(ctx, name)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *AWSParamStrategy) applyCreate(ctx context.Context, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// Create is create-only (never overwrites an existing parameter). Use the
	// staged value type (String / SecureString / StringList); an unset type
	// defaults to plaintext String, matching entries staged before the staging
	// model carried a type.
	valueType := entry.ValueType
	if valueType == "" {
		valueType = domain.ValueTypePlaintext
	}

	if _, err := s.store.Create(ctx, name, *entry.Value, valueType, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to create parameter: %w", err)
	}

	return nil
}

func (s *AWSParamStrategy) applyUpdate(ctx context.Context, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// A missing parameter is a hard error.
	existing, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return fmt.Errorf("parameter not found: %s", name)
		}

		return fmt.Errorf("failed to get existing parameter: %w", err)
	}

	// Use the staged value type when the edit specified one; otherwise preserve
	// the existing parameter type (an unset staged type must never downgrade a
	// SecureString to plain String).
	valueType := existing.Type
	if entry.ValueType != "" {
		valueType = entry.ValueType
	}

	// Put overwrites the existing parameter.
	if _, err := s.store.Put(ctx, name, *entry.Value, valueType, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to update parameter: %w", err)
	}

	return nil
}

func (s *AWSParamStrategy) applyDelete(ctx context.Context, name string) error {
	if err := s.store.Delete(ctx, name); err != nil {
		// Already deleted is considered success.
		if errors.Is(err, provider.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("failed to delete parameter: %w", err)
	}

	return nil
}

// ApplyTags applies staged tag changes to SSM Parameter Store.
func (s *AWSParamStrategy) ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error {
	if len(tagEntry.Add) > 0 {
		if err := s.store.Tag(ctx, name, tagEntry.Add); err != nil {
			return err
		}
	}

	if tagEntry.Remove.Len() > 0 {
		if err := s.store.Untag(ctx, name, tagEntry.Remove.Values()); err != nil {
			return err
		}
	}

	return nil
}

// FetchLastModified returns the last modified time of the parameter. It returns
// a *ResourceNotFoundError when the parameter does not exist, so callers can
// tell "missing" apart from "exists but has no modification time" (the latter
// returns a zero time with a nil error).
func (s *AWSParamStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return time.Time{}, &ResourceNotFoundError{Err: err}
		}

		return time.Time{}, fmt.Errorf("failed to get parameter: %w", err)
	}

	if entry.Modified != nil {
		return *entry.Modified, nil
	}

	return time.Time{}, nil
}

// FetchCurrent fetches the current value from SSM Parameter Store for diffing.
func (s *AWSParamStrategy) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		return nil, err
	}

	return &FetchResult{
		Value:      entry.Value,
		Identifier: "#" + entry.Version.ID,
	}, nil
}

// FetchCurrentTags fetches the current tags from SSM Parameter Store.
func (s *AWSParamStrategy) FetchCurrentTags(ctx context.Context, name string) (map[string]string, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		// Parameter not found - return nil (no tags available)
		if errors.Is(err, provider.ErrNotFound) {
			return nil, nil //nolint:nilnil // intentional: no tags for non-existent resource
		}

		return nil, fmt.Errorf("failed to get tags: %w", err)
	}

	if len(entry.Tags) == 0 {
		return nil, nil //nolint:nilnil // intentional: resource exists but has no tags
	}

	tags := make(map[string]string, len(entry.Tags))
	for _, tag := range entry.Tags {
		tags[tag.Key] = tag.Value
	}

	return tags, nil
}

// ParseName parses and validates a name for editing.
func (s *AWSParamStrategy) ParseName(input string) (string, error) {
	spec, err := awsparamversion.Parse(input)
	if err != nil {
		return "", err
	}

	if spec.Absolute.Version != nil || spec.Shift > 0 {
		return "", fmt.Errorf("parameter name must not contain a version specifier")
	}

	return spec.Name, nil
}

// FetchCurrentValue fetches the current value from SSM Parameter Store for editing.
// Returns *ResourceNotFoundError if the parameter doesn't exist.
func (s *AWSParamStrategy) FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return nil, &ResourceNotFoundError{Err: err}
		}

		return nil, err
	}

	result := &EditFetchResult{
		Value: entry.Value,
	}

	if entry.Modified != nil {
		result.LastModified = *entry.Modified
	}

	return result, nil
}

// ParseSpec parses a version spec string for reset.
func (s *AWSParamStrategy) ParseSpec(input string) (name string, hasVersion bool, err error) {
	spec, err := awsparamversion.Parse(input)
	if err != nil {
		return "", false, err
	}

	hasVersion = spec.Absolute.Version != nil || spec.Shift > 0

	return spec.Name, hasVersion, nil
}

// FetchVersion fetches the value for a specific version.
func (s *AWSParamStrategy) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	spec, err := awsparamversion.Parse(input)
	if err != nil {
		return "", "", err
	}

	ref, err := s.store.Resolve(ctx, spec.Name, paramSpecSuffix(spec))
	if err != nil {
		return "", "", err
	}

	entry, err := s.store.Get(ctx, spec.Name, ref)
	if err != nil {
		return "", "", err
	}

	return entry.Value, "#" + entry.Version.ID, nil
}

// paramSpecSuffix reconstructs the version-spec suffix (the part after the name)
// so that name+suffix re-parses to an equivalent spec, as provider.Reader.Resolve expects.
func paramSpecSuffix(spec *awsparamversion.Spec) string {
	var b strings.Builder

	if spec.Absolute.Version != nil {
		b.WriteString("#")
		b.WriteString(strconv.FormatInt(*spec.Absolute.Version, 10))
	}

	if spec.Shift > 0 {
		b.WriteString("~")
		b.WriteString(strconv.Itoa(spec.Shift))
	}

	return b.String()
}

// AWSParamParserFactory creates a Parser without provider access.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func AWSParamParserFactory() Parser {
	return NewAWSParamStrategy(nil)
}
