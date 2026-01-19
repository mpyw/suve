package provider

import "context"

// Tagger provides tag management operations for resources.
// This is a unified interface that both ParameterTagger and SecretTagger satisfy.
//
// All provider adapters implementing either ParameterTagger or SecretTagger
// automatically satisfy this interface since the method signatures are identical.
//
//nolint:iface // Intentionally identical to ParameterTagger/SecretTagger for unified access.
type Tagger interface {
	// GetTags retrieves all tags for a resource.
	GetTags(ctx context.Context, name string) (map[string]string, error)

	// AddTags adds or updates tags on a resource.
	AddTags(ctx context.Context, name string, tags map[string]string) error

	// RemoveTags removes tags from a resource by key names.
	RemoveTags(ctx context.Context, name string, keys []string) error
}

// Ensure ParameterTagger and SecretTagger satisfy Tagger interface.
var (
	_ Tagger = (ParameterTagger)(nil)
	_ Tagger = (SecretTagger)(nil)
)
