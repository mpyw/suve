package gcloud_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/gcloud"
)

// TestEmulatorEnvVar pins the emulator seam env var name; callers (and docs)
// depend on this exact string.
func TestEmulatorEnvVar(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "SUVE_GCLOUD_SECRETMANAGER_ENDPOINT", gcloud.EmulatorEnvVar)
}

// TestFactory_Store_UnsupportedKind verifies that Google Cloud (which has no
// parameter store) rejects KindParam and any unknown kind with
// provider.ErrUnsupportedKind, without touching the network.
func TestFactory_Store_UnsupportedKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind provider.Kind
	}{
		{name: "param has no google cloud backend", kind: provider.KindParam},
		{name: "unknown kind", kind: provider.Kind("bogus")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := gcloud.Factory{}.Store(t.Context(), provider.GoogleCloudScope("my-project"), tt.kind)
			require.ErrorIs(t, err, provider.ErrUnsupportedKind)
			assert.Nil(t, store)
		})
	}
}

// TestFactory_Store_EmulatorSeam exercises the EmulatorEnvVar seam. These
// subtests mutate a process-global env var via t.Setenv, so they must not run
// in parallel.
func TestFactory_Store_EmulatorSeam(t *testing.T) {
	t.Run("valid endpoint builds a store over plaintext gRPC", func(t *testing.T) {
		// A well-formed target dials lazily: the client is constructed without
		// any network round-trip, so this succeeds offline.
		t.Setenv(gcloud.EmulatorEnvVar, "localhost:9090")

		store, err := gcloud.Factory{}.Store(t.Context(), provider.GoogleCloudScope("my-project"), provider.KindSecret)
		require.NoError(t, err)
		assert.NotNil(t, store)
	})

	t.Run("unparsable endpoint surfaces a clear dial error", func(t *testing.T) {
		// A control character makes gRPC target parsing fail synchronously,
		// exercising the emulator dial-error branch.
		t.Setenv(gcloud.EmulatorEnvVar, "\tbad-endpoint")

		store, err := gcloud.Factory{}.Store(t.Context(), provider.GoogleCloudScope("my-project"), provider.KindSecret)
		require.Error(t, err)
		assert.Nil(t, store)
		assert.Contains(t, err.Error(), "failed to create Google Cloud Secret Manager client")
		assert.Contains(t, err.Error(), "emulator")
	})

	t.Run("no emulator falls through to the ADC client constructor", func(t *testing.T) {
		// With the seam unset the client is built from Application Default
		// Credentials. The outcome depends on ambient credentials, so accept
		// either a constructed store or a wrapped construction error, while
		// still exercising the non-emulator branch.
		t.Setenv(gcloud.EmulatorEnvVar, "")

		store, err := gcloud.Factory{}.Store(t.Context(), provider.GoogleCloudScope("my-project"), provider.KindSecret)
		if err != nil {
			assert.Nil(t, store)
			assert.Contains(t, err.Error(), "failed to create Google Cloud Secret Manager client")
		} else {
			assert.NotNil(t, store)
		}
	})
}

// TestRegister verifies Register wires the Google Cloud factory into an existing
// registry under provider.ProviderGoogleCloud. Resolution is proven indirectly:
// a registered factory yields ErrUnsupportedKind for KindParam, whereas an
// unregistered provider would yield ErrNoFactory.
func TestRegister(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	gcloud.Register(reg)

	_, err := reg.Store(t.Context(), provider.GoogleCloudScope("my-project"), provider.KindParam)
	require.ErrorIs(t, err, provider.ErrUnsupportedKind)
	assert.NotErrorIs(t, err, provider.ErrNoFactory)
}

// TestNewRegistry verifies NewRegistry returns a registry with only the Google
// Cloud provider registered.
func TestNewRegistry(t *testing.T) {
	t.Parallel()

	reg := gcloud.NewRegistry()

	t.Run("google cloud is registered", func(t *testing.T) {
		t.Parallel()

		_, err := reg.Store(t.Context(), provider.GoogleCloudScope("my-project"), provider.KindParam)
		require.ErrorIs(t, err, provider.ErrUnsupportedKind)
		assert.NotErrorIs(t, err, provider.ErrNoFactory)
	})

	t.Run("other providers are not registered", func(t *testing.T) {
		t.Parallel()

		_, err := reg.Store(t.Context(), provider.AWSScope("123456789012", "us-east-1"), provider.KindSecret)
		require.ErrorIs(t, err, provider.ErrNoFactory)
	})
}
