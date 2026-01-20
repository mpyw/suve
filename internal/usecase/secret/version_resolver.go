package secret

import "github.com/mpyw/suve/internal/provider"

// VersionResolverClient is the shared interface for use cases that need
// secret version resolution (listing versions and fetching values).
//
//nolint:iface // Intentional type alias for semantic clarity
type VersionResolverClient interface {
	provider.SecretReader
}
