//go:build e2e

// AWS e2e setup. These tests run against localstack, an AWS-compatible service.
//
// Environment variables:
//   - SUVE_LOCALSTACK_ENDPOINT: Full localstack URL (default:
//     http://localhost:4566). Host runs use the default; the in-container
//     (compose.test.yaml) runner points it at http://localstack:4566.

package e2e_test

import (
	"os"
	"testing"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging/store/file"
)

func getEndpoint() string {
	// Single full-URL knob. Host/manual runs get the default localhost URL; the
	// in-container (compose.test.yaml) runner sets it to http://localstack:4566
	// to reach the emulator by service name over the closed compose network.
	if endpoint := os.Getenv("SUVE_LOCALSTACK_ENDPOINT"); endpoint != "" {
		return endpoint
	}

	return "http://localhost:4566"
}

// setupEnv sets up environment variables for localstack.
func setupEnv(t *testing.T) {
	t.Helper()

	endpoint := getEndpoint()

	// Set AWS environment variables for localstack
	t.Setenv("AWS_ENDPOINT_URL", endpoint)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
}

// newStore creates a new staging store for E2E tests.
// localstack uses account ID "000000000000" and region "us-east-1".
//
// It uses NewWorkingStore (not NewStore) so the read path shares the exact key
// resolution the CLI uses when it writes: SUVE_STAGING_KEY env var -> OS
// keychain -> plaintext. Both the Dockerized runner and the CI e2e jobs set
// SUVE_STAGING_KEY, so the working store is encrypted and this must decrypt with
// the same key. (A keychain-less runner with no key would otherwise fall back to
// plaintext, which is now refused for non-interactive writes without consent.)
func newStore() *file.Store {
	s, err := file.NewWorkingStore(provider.AWSScope("000000000000", "us-east-1"))
	if err != nil {
		panic(err)
	}

	return s
}

// newStoreForAccount creates a staging store for a specific account and region.
func newStoreForAccount(accountID, region string) *file.Store {
	s, err := file.NewWorkingStore(provider.AWSScope(accountID, region))
	if err != nil {
		panic(err)
	}

	return s
}
