package staging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
)

// versionedTraits are the fixed, provider-specific strings and flags of a
// versioned staging strategy.
//
//declscope:package // each versioned provider's hooks type fills it in
type versionedTraits struct {
	// service is the staging service the strategy serves.
	service Service
	// serviceName is the user-friendly service name ("Secrets Manager").
	serviceName string
	// itemName names one item in messages and errors ("secret", "parameter").
	itemName string
	// tagsFetchError prefixes the error FetchCurrentTags returns when the
	// read fails for a reason other than "not found".
	tagsFetchError string
	// hasDeleteOptions reports whether the service takes staged delete options.
	hasDeleteOptions bool
}

// versionedHooks is the per-provider behavior a versionedStrategy delegates to.
// Implementations are zero-size types, so a zero-value strategy (no store) still
// works as a parser.
type versionedHooks interface {
	// traits returns the provider's fixed strings and flags.
	traits() versionedTraits
	// parse splits input into the name and the version suffix that
	// provider.Reader.Resolve expects ("" when no version is given).
	parse(input string) (name, suffix string, err error)
	// versionLabel renders a version id for diff and reset output.
	versionLabel(id string) string
	// isSecret reports whether the current value is secret material.
	isSecret(entry *domain.Entry) bool
	// deleteOptions translates staged delete options into provider options.
	deleteOptions(o *DeleteOptions) []provider.DeleteOption
	// create applies a staged create.
	create(ctx context.Context, store provider.Store, name string, entry Entry) error
	// update applies a staged update.
	update(ctx context.Context, store provider.Store, name string, entry Entry) error
}

// versionedStrategy implements FullStrategy for a versioned service over a
// provider.Store, with the provider specifics supplied by H. It carries no cloud
// SDK dependency. A nil store yields a parser-only strategy
// (ParseName/ParseSpec).
//
//declscope:package // embedded by each versioned provider's exported strategy type
type versionedStrategy[H versionedHooks] struct {
	store provider.Store
	//declscope:private
	hooks H
}

// Service returns the service type.
func (s *versionedStrategy[H]) Service() Service {
	return s.hooks.traits().service
}

// ServiceName returns the user-friendly service name.
func (s *versionedStrategy[H]) ServiceName() string {
	return s.hooks.traits().serviceName
}

// ItemName returns the item name for messages.
func (s *versionedStrategy[H]) ItemName() string {
	return s.hooks.traits().itemName
}

// HasDeleteOptions reports whether the service has delete options.
func (s *versionedStrategy[H]) HasDeleteOptions() bool {
	return s.hooks.traits().hasDeleteOptions
}

// Apply applies a staged operation.
func (s *versionedStrategy[H]) Apply(ctx context.Context, name string, entry Entry) error {
	switch entry.Operation {
	case OperationCreate:
		return s.hooks.create(ctx, s.store, name, entry)
	case OperationUpdate:
		return s.hooks.update(ctx, s.store, name, entry)
	case OperationDelete:
		return s.applyDelete(ctx, name, entry)
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}
}

func (s *versionedStrategy[H]) applyDelete(ctx context.Context, name string, entry Entry) error {
	if err := s.store.Delete(ctx, name, s.hooks.deleteOptions(entry.DeleteOptions)...); err != nil {
		// Already deleted is considered success.
		if errors.Is(err, provider.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("failed to delete %s: %w", s.hooks.traits().itemName, err)
	}

	return nil
}

// ApplyTags applies staged tag changes. Additions are applied before removals.
func (s *versionedStrategy[H]) ApplyTags(ctx context.Context, name string, tagEntry TagEntry) error {
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

// FetchLastModified returns the last modified time of the item. It returns a
// *ResourceNotFoundError when the item does not exist, so callers can tell
// "missing" apart from "exists but has no modification time" (the latter returns
// a zero time with a nil error).
func (s *versionedStrategy[H]) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return time.Time{}, &ResourceNotFoundError{Err: err}
		}

		return time.Time{}, fmt.Errorf("failed to get %s: %w", s.hooks.traits().itemName, err)
	}

	return lo.FromPtr(entry.Modified), nil
}

// FetchCurrent fetches the current value for diffing.
func (s *versionedStrategy[H]) FetchCurrent(ctx context.Context, name string) (*FetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		return nil, err
	}

	return &FetchResult{
		Value:      entry.Value,
		Identifier: s.hooks.versionLabel(entry.Version.ID),
		Secret:     s.hooks.isSecret(entry),
	}, nil
}

// FetchCurrentTags fetches the current tags. A missing item or an item with no
// tags yields nil.
func (s *versionedStrategy[H]) FetchCurrentTags(ctx context.Context, name string) (map[string]string, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return nil, nil //nolint:nilnil // intentional: no tags for non-existent resource
		}

		return nil, fmt.Errorf("%s: %w", s.hooks.traits().tagsFetchError, err)
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

// ParseName parses and validates a name for editing (no version specifier).
func (s *versionedStrategy[H]) ParseName(input string) (string, error) {
	name, suffix, err := s.hooks.parse(input)
	if err != nil {
		return "", err
	}

	if suffix != "" {
		return "", fmt.Errorf("%s name must not contain a version specifier", s.hooks.traits().itemName)
	}

	return name, nil
}

// FetchCurrentValue fetches the current value for editing.
// Returns *ResourceNotFoundError if the item doesn't exist.
func (s *versionedStrategy[H]) FetchCurrentValue(ctx context.Context, name string) (*EditFetchResult, error) {
	entry, err := s.store.Get(ctx, name, provider.VersionRef{})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return nil, &ResourceNotFoundError{Err: err}
		}

		return nil, err
	}

	return &EditFetchResult{
		Value:        entry.Value,
		LastModified: lo.FromPtr(entry.Modified),
	}, nil
}

// ParseSpec parses a version spec string for reset.
func (s *versionedStrategy[H]) ParseSpec(input string) (name string, hasVersion bool, err error) {
	name, suffix, err := s.hooks.parse(input)
	if err != nil {
		return "", false, err
	}

	return name, suffix != "", nil
}

// FetchVersion fetches the value for a specific version.
func (s *versionedStrategy[H]) FetchVersion(ctx context.Context, input string) (value string, versionLabel string, err error) {
	name, suffix, err := s.hooks.parse(input)
	if err != nil {
		return "", "", err
	}

	ref, err := s.store.Resolve(ctx, name, suffix)
	if err != nil {
		return "", "", err
	}

	entry, err := s.store.Get(ctx, name, ref)
	if err != nil {
		return "", "", err
	}

	return entry.Value, s.hooks.versionLabel(entry.Version.ID), nil
}

// versionedSecretHooks supplies the hooks shared by the secret services: values
// are always secret material, versions render as "#ID", there are no delete
// options, and create/update write the value as a new secret version. A
// provider's hooks type embeds it and overrides what differs.
//
//declscope:package // embedded by each versioned secret provider's hooks type
type versionedSecretHooks struct{}

func (versionedSecretHooks) versionLabel(id string) string { return "#" + id }

func (versionedSecretHooks) isSecret(*domain.Entry) bool { return true }

func (versionedSecretHooks) deleteOptions(*DeleteOptions) []provider.DeleteOption { return nil }

func (versionedSecretHooks) create(ctx context.Context, store provider.Store, name string, entry Entry) error {
	if _, err := store.Create(ctx, name, lo.FromPtr(entry.Value), domain.ValueTypeSecret, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

// update writes the value as a new secret version.
//
//declscope:package // AWS Secrets Manager's hooks wrap it with a binary-overwrite guard
func (versionedSecretHooks) update(ctx context.Context, store provider.Store, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// Secret versions are immutable: Put adds a new version and, when provided,
	// updates the description in the same operation.
	if _, err := store.Put(ctx, name, *entry.Value, domain.ValueTypeSecret, lo.FromPtr(entry.Description)); err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	return nil
}
