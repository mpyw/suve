package keyvault_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/keyvault"
)

// serverError returns a non-404 Key Vault error, which must NOT map to
// provider.ErrNotFound.
func serverError() error {
	return &azcore.ResponseError{StatusCode: http.StatusInternalServerError}
}

// verPair is a (version id, created time) pair for building version-list
// fixtures. A nil created time means the version has no Created attribute.
type verPair struct {
	version string
	created *time.Time
}

// versionsFixture builds a listVersFunc returning the given version pairs.
func versionsFixture(pairs ...verPair) func(context.Context, string) ([]*azsecrets.SecretProperties, error) {
	return func(_ context.Context, name string) ([]*azsecrets.SecretProperties, error) {
		out := make([]*azsecrets.SecretProperties, 0, len(pairs))
		for _, p := range pairs {
			out = append(out, &azsecrets.SecretProperties{
				ID:         secretID(name, p.version),
				Attributes: &azsecrets.SecretAttributes{Created: p.created},
			})
		}

		return out, nil
	}
}

// at returns a pointer to a fixed-year (2024) UTC date, for building ordered
// version timestamps in tests.
func at(mo time.Month, d int) *time.Time {
	t := time.Date(2024, mo, d, 0, 0, 0, 0, time.UTC)

	return &t
}

// TestResolve_ShiftResolution exhaustively covers the ~SHIFT resolution logic,
// which is the most provider-difference-prone path: Key Vault versions are
// opaque ids ordered only by creation time, so shifts walk a sorted list.
func TestResolve_ShiftResolution(t *testing.T) {
	t.Parallel()

	// Three versions, distinct timestamps: v3 (newest) .. v1 (oldest).
	threeVersions := versionsFixture(
		verPair{"v1", at(1, 1)},
		verPair{"v3", at(3, 15)},
		verPair{"v2", at(2, 10)},
	)

	tests := []struct {
		name    string
		spec    string
		want    string // expected resolved version id
		wantErr string // substring; empty means no error
	}{
		{name: "no shift is current", spec: "", want: ""},
		{name: "shift one back", spec: "~1", want: "v2"},
		{name: "shift two back reaches oldest", spec: "~2", want: "v1"},
		{name: "cumulative shift ~1~1", spec: "~1~1", want: "v1"},
		{name: "bare tilde is one back", spec: "~", want: "v2"},
		{name: "double tilde is two back", spec: "~~", want: "v1"},
		{name: "shift out of range", spec: "~3", wantErr: "out of range"},
		{name: "absolute id then shift", spec: "#v3~1", want: "v2"},
		{name: "absolute id no shift needs no ordering", spec: "#v2", want: "v2"},
		{name: "absolute id then shift to oldest", spec: "#v3~2", want: "v1"},
		{name: "absolute id not found with shift", spec: "#ghost~1", wantErr: "version not found"},
		{name: "absolute id shift out of range", spec: "#v1~1", wantErr: "out of range"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := keyvault.New(&mockClient{listVersFunc: threeVersions})
			ref, err := store.Resolve(t.Context(), "my-secret", tc.spec)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, ref.ID())
		})
	}
}

// TestResolve_ShiftWithNoVersions verifies a shift against a secret with no
// versions is a clear error, not a panic or out-of-range index.
func TestResolve_ShiftWithNoVersions(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listVersFunc: func(_ context.Context, _ string) ([]*azsecrets.SecretProperties, error) {
			return nil, nil
		},
	}
	store := keyvault.New(m)

	_, err := store.Resolve(t.Context(), "my-secret", "~1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no versions")
}

// TestResolve_ShiftListError verifies a listing failure during shift resolution
// is surfaced (and not-found is mapped to provider.ErrNotFound).
func TestResolve_ShiftListError(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listVersFunc: func(_ context.Context, _ string) ([]*azsecrets.SecretProperties, error) {
			return nil, notFound()
		},
	}
	store := keyvault.New(m)

	_, err := store.Resolve(t.Context(), "my-secret", "~1")
	require.ErrorIs(t, err, provider.ErrNotFound)
}

// TestResolve_OrderingEqualTimestamps documents the #314 fix: Key Vault Created
// timestamps are second-granular and the list-versions API returns versions
// unordered, so two versions written in the same second are indistinguishable
// by time. A deterministic version-id tie-break (descending) is applied, so the
// ~N result no longer depends on the arbitrary API/input order.
func TestResolve_OrderingEqualTimestamps(t *testing.T) {
	t.Parallel()

	same := at(5, 20)

	// Both input orders must yield the same result: with the id-desc tie-break
	// "second" > "first", so "second" is newest (index 0) and ~1 is "first".
	for _, order := range [][]verPair{
		{{"first", same}, {"second", same}},
		{{"second", same}, {"first", same}},
	} {
		m := &mockClient{listVersFunc: versionsFixture(order...)}
		store := keyvault.New(m)

		prev, err := store.Resolve(t.Context(), "my-secret", "~1")
		require.NoError(t, err)
		assert.Equal(t, "first", prev.ID(), "input order %v", order)
	}
}

// TestHistory_NilCreatedSortsLast verifies versions with no Created timestamp
// are ordered after those that have one (newest-first), rather than crashing.
func TestHistory_NilCreatedSortsLast(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listVersFunc: versionsFixture(
			verPair{"no-time", nil},
			verPair{"dated", at(1, 5)},
		),
	}
	store := keyvault.New(m)

	versions, err := store.History(t.Context(), "my-secret")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	// Dated version is "newer" than the one with no timestamp.
	assert.Equal(t, "dated", versions[0].ID)
	assert.Equal(t, "no-time", versions[1].ID)
}

// TestHistory_ListError verifies History surfaces a listing error (mapped to
// provider.ErrNotFound for a 404).
func TestHistory_ListError(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listVersFunc: func(_ context.Context, _ string) ([]*azsecrets.SecretProperties, error) {
			return nil, notFound()
		},
	}
	store := keyvault.New(m)

	_, err := store.History(t.Context(), "my-secret")
	require.ErrorIs(t, err, provider.ErrNotFound)
}

// TestGet_ByExplicitVersion verifies a resolved opaque version id is passed
// through to GetSecret (the version-specific read path).
func TestGet_ByExplicitVersion(t *testing.T) {
	t.Parallel()

	var gotVersion string

	m := &mockClient{
		getFunc: func(_ context.Context, name, version string) (azsecrets.GetSecretResponse, error) {
			gotVersion = version

			return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{
				ID:    secretID(name, version),
				Value: lo.ToPtr("older-value"),
			}}, nil
		},
	}
	store := keyvault.New(m)

	entry, err := store.Get(t.Context(), "my-secret", provider.NewVersionRef("abc123"))
	require.NoError(t, err)
	assert.Equal(t, "abc123", gotVersion)
	assert.Equal(t, "older-value", entry.Value)
}

// TestGet_NilFieldsTolerated verifies a response with a nil ID and nil Enabled
// flag does not crash and yields empty version id / no label.
func TestGet_NilFieldsTolerated(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, _, _ string) (azsecrets.GetSecretResponse, error) {
			return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{
				ID:         nil,
				Value:      lo.ToPtr("v"),
				Attributes: &azsecrets.SecretAttributes{Enabled: nil},
			}}, nil
		},
	}
	store := keyvault.New(m)

	entry, err := store.Get(t.Context(), "my-secret", provider.VersionRef{})
	require.NoError(t, err)
	assert.Empty(t, entry.Version.ID)
	assert.Empty(t, entry.Version.Label)
}

// TestStore_ErrorPaths covers the non-not-found error branches of the write and
// list operations, ensuring they wrap the operation rather than masquerading as
// not-found.
func TestStore_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("list error", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{
			listFunc: func(_ context.Context) ([]*azsecrets.SecretProperties, error) {
				return nil, serverError()
			},
		})
		_, err := store.List(t.Context())
		require.Error(t, err)
		assert.NotErrorIs(t, err, provider.ErrNotFound)
	})

	t.Run("create existence-check error", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{
			getFunc: func(_ context.Context, _, _ string) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, serverError()
			},
		})
		_, err := store.Create(t.Context(), "s", "v", domain.ValueTypeSecret, "")
		require.Error(t, err)
		assert.NotErrorIs(t, err, provider.ErrAlreadyExists)
	})

	t.Run("put error", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{
			setFunc: func(_ context.Context, _ string, _ azsecrets.SetSecretParameters) (azsecrets.SetSecretResponse, error) {
				return azsecrets.SetSecretResponse{}, serverError()
			},
		})
		_, err := store.Put(t.Context(), "s", "v", domain.ValueTypeSecret, "")
		require.Error(t, err)
	})

	t.Run("delete error", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{
			deleteFunc: func(_ context.Context, _ string) (azsecrets.DeleteSecretResponse, error) {
				return azsecrets.DeleteSecretResponse{}, serverError()
			},
		})
		err := store.Delete(t.Context(), "s")
		require.Error(t, err)
	})

	t.Run("tag current-tags fetch error", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{
			getFunc: func(_ context.Context, _, _ string) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{}, serverError()
			},
		})
		err := store.Tag(t.Context(), "s", map[string]string{"env": "prod"})
		require.Error(t, err)
	})

	t.Run("untag update error", func(t *testing.T) {
		t.Parallel()

		store := keyvault.New(&mockClient{
			getFunc: func(_ context.Context, name, _ string) (azsecrets.GetSecretResponse, error) {
				return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{
					ID:   secretID(name, "v1"),
					Tags: map[string]*string{"env": lo.ToPtr("prod")},
				}}, nil
			},
			updateFunc: func(_ context.Context, _, _ string, _ updParams) (updResp, error) {
				return updResp{}, serverError()
			},
		})
		err := store.Untag(t.Context(), "s", []string{"env"})
		require.Error(t, err)
	})

	t.Run("empty tag/untag is a no-op", func(t *testing.T) {
		t.Parallel()

		// No client funcs set: if Tag/Untag tried to call the client with an
		// empty set, the nil func would panic. They must short-circuit.
		store := keyvault.New(&mockClient{})
		require.NoError(t, store.Tag(t.Context(), "s", nil))
		require.NoError(t, store.Untag(t.Context(), "s", nil))
	})
}
