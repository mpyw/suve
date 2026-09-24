package staging

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version"
)

// AWSParamStrategy implements ServiceStrategy for SSM Parameter Store over a
// provider.Store. Parameter Store specifics:
//
//   - Versions are integers, parsed with version.ParameterStore (#N, ~SHIFT).
//   - Values carry a type (String / SecureString / StringList); a SecureString
//     is masked in diffs, and an edit keeps the existing type unless it staged
//     one.
//   - There are no delete options.
//
// The zero value (no store) is a parser-only strategy (ParseName/ParseSpec).
type AWSParamStrategy struct {
	versionedStrategy[awsParamHooks]
}

// NewAWSParamStrategy creates a new SSM Parameter Store strategy over the given
// provider store. A nil store is allowed for parser-only use.
func NewAWSParamStrategy(store provider.Store) *AWSParamStrategy {
	return &AWSParamStrategy{versionedStrategy[awsParamHooks]{store: store}}
}

// AWSParamParserFactory creates a Parser without provider access.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func AWSParamParserFactory() Parser {
	return NewAWSParamStrategy(nil)
}

// awsParamHooks supplies the Parameter Store specifics.
type awsParamHooks struct{}

func (awsParamHooks) traits() versionedTraits {
	return versionedTraits{
		service:        ServiceParam,
		serviceName:    "SSM Parameter Store",
		itemName:       "parameter",
		tagsFetchError: "failed to get tags",
	}
}

func (awsParamHooks) parse(input string) (name, suffix string, err error) {
	spec, err := version.ParameterStore.Parse(input)
	if err != nil {
		return "", "", err
	}

	return spec.Name, version.ParameterStore.Suffix(spec), nil
}

func (awsParamHooks) versionLabel(id string) string { return "#" + id }

// isSecret reports a SecureString as secret material even on the param
// service, so the staged diff masks it (#677).
func (awsParamHooks) isSecret(entry *domain.Entry) bool {
	return entry.Type == domain.ValueTypeSecret
}

func (awsParamHooks) deleteOptions(*DeleteOptions) []provider.DeleteOption { return nil }

func (awsParamHooks) create(ctx context.Context, store provider.Store, name string, entry Entry) error {
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

	if _, err := store.Create(ctx, name, *entry.Value, valueType, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to create parameter: %w", err)
	}

	return nil
}

func (awsParamHooks) update(ctx context.Context, store provider.Store, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// A missing parameter is a hard error.
	existing, err := store.Get(ctx, name, provider.VersionRef{})
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
	if _, err := store.Put(ctx, name, *entry.Value, valueType, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to update parameter: %w", err)
	}

	return nil
}
