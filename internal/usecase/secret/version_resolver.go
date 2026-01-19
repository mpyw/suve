package secret

import "github.com/mpyw/suve/internal/api/secretapi"

// VersionResolverClient is the shared interface for use cases that need
// secret version resolution (listing versions and fetching values).
//
// Deprecated: Use provider.SecretReader instead.
type VersionResolverClient interface {
	secretapi.GetSecretValueAPI
	secretapi.ListSecretVersionIDsAPI
}
