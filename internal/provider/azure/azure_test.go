package azure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure"
)

// TestStore_UnsupportedKind verifies an unknown kind yields ErrUnsupportedKind.
func TestStore_UnsupportedKind(t *testing.T) {
	t.Parallel()

	_, err := azure.Factory{}.Store(t.Context(), provider.Scope{}, provider.Kind("bogus"))
	require.ErrorIs(t, err, provider.ErrUnsupportedKind)
}

// TestAppConfigStore_RequiresStoreName verifies a missing store name is a clear
// error before any client construction.
func TestAppConfigStore_RequiresStoreName(t *testing.T) {
	t.Parallel()

	_, err := azure.Factory{}.Store(t.Context(), provider.Scope{}, provider.KindParam)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--store-name")
}

// TestAppConfigStore_ConnectionStringError exercises the emulator seam: with the
// connection-string env var set to a malformed value, the App Configuration
// store construction fails from NewClientFromConnectionString rather than
// falling through to DefaultAzureCredential.
func TestAppConfigStore_ConnectionStringError(t *testing.T) {
	t.Setenv(azure.AppConfigConnStringEnvVar, "this-is-not-a-valid-connection-string")

	_, err := azure.Factory{}.Store(t.Context(), provider.Scope{StoreName: "suve-test"}, provider.KindParam)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection string")
}
