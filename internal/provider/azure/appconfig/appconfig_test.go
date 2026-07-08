package appconfig_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig/v2"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
)

func notFound() error {
	return &azcore.ResponseError{StatusCode: http.StatusNotFound}
}

func preconditionFailed() error {
	return &azcore.ResponseError{StatusCode: http.StatusPreconditionFailed}
}

func conflict() error {
	return &azcore.ResponseError{StatusCode: http.StatusConflict}
}

func serverError() error {
	return &azcore.ResponseError{StatusCode: http.StatusInternalServerError}
}

// mockClient is a configurable in-test implementation of appconfig.Client.
// Single-key methods receive the resolved literal label; ListSettings receives
// the LabelFilter.
type mockClient struct {
	getFunc func(ctx context.Context, key, label string) (azappconfig.GetSettingResponse, error)
	setFunc func(
		ctx context.Context, key, value, label string, tags map[string]*string, etag *azcore.ETag,
	) (azappconfig.SetSettingResponse, error)
	addFunc    func(ctx context.Context, key, value, label string) (azappconfig.AddSettingResponse, error)
	deleteFunc func(ctx context.Context, key, label string) (azappconfig.DeleteSettingResponse, error)
	listFunc   func(ctx context.Context, filter string) ([]azappconfig.Setting, error)
}

func (m *mockClient) GetSetting(ctx context.Context, key, label string) (azappconfig.GetSettingResponse, error) {
	return m.getFunc(ctx, key, label)
}

func (m *mockClient) SetSetting(
	ctx context.Context, key, value, label string, tags map[string]*string, etag *azcore.ETag,
) (azappconfig.SetSettingResponse, error) {
	return m.setFunc(ctx, key, value, label, tags, etag)
}

func (m *mockClient) AddSetting(
	ctx context.Context, key, value, label string,
) (azappconfig.AddSettingResponse, error) {
	return m.addFunc(ctx, key, value, label)
}

func (m *mockClient) DeleteSetting(ctx context.Context, key, label string) (azappconfig.DeleteSettingResponse, error) {
	return m.deleteFunc(ctx, key, label)
}

func (m *mockClient) ListSettings(ctx context.Context, filter string) ([]azappconfig.Setting, error) {
	return m.listFunc(ctx, filter)
}

func TestResolve_BareNameLatest(t *testing.T) {
	t.Parallel()

	store := appconfig.New(&mockClient{}, "")
	ref, err := store.Resolve(t.Context(), "my-key", "")
	require.NoError(t, err)
	assert.True(t, ref.IsLatest())
}

func TestResolve_RejectsVersionSpecs(t *testing.T) {
	t.Parallel()

	store := appconfig.New(&mockClient{}, "")

	for _, spec := range []string{"#1", "#abc", "~1", ":prod"} {
		_, err := store.Resolve(t.Context(), "my-key", spec)
		require.Error(t, err, "spec=%q", spec)
	}
}

func TestResolve_RejectsFilterNamespace(t *testing.T) {
	t.Parallel()

	for _, ns := range []string{"*", "dev,prod", "dev*"} {
		store := appconfig.New(&mockClient{}, ns)
		_, err := store.Resolve(t.Context(), "my-key", "")
		require.Error(t, err, "ns=%q", ns)
		assert.Contains(t, err.Error(), "single-item operation needs one")
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, key, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
				Key:   lo.ToPtr(key),
				Value: lo.ToPtr("30"),
				Tags:  map[string]*string{"env": lo.ToPtr("prod")},
			}}, nil
		},
	}
	store := appconfig.New(m, "")

	entry, err := store.Get(t.Context(), "app/timeout", provider.VersionRef{})
	require.NoError(t, err)
	assert.Equal(t, "app/timeout", entry.Name)
	assert.Equal(t, "30", entry.Value)
	assert.Equal(t, domain.ValueTypePlaintext, entry.Type)
	assert.Empty(t, entry.Version.ID) // unversioned
	assert.Equal(t, []domain.Tag{{Key: "env", Value: "prod"}}, entry.Tags)
}

func TestGet_AppliesNamespaceLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		wantLabel string
	}{
		{name: "empty is null label", namespace: "", wantLabel: ""},
		{name: "literal namespace", namespace: "dev", wantLabel: "dev"},
		{name: "escaped star decodes to literal", namespace: `\*`, wantLabel: "*"},
		{name: "escaped comma decodes to literal", namespace: `foo\,bar`, wantLabel: "foo,bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotLabel string

			m := &mockClient{
				getFunc: func(_ context.Context, key, label string) (azappconfig.GetSettingResponse, error) {
					gotLabel = label

					return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
						Key:   lo.ToPtr(key),
						Value: lo.ToPtr("v"),
					}}, nil
				},
			}
			store := appconfig.New(m, tt.namespace)

			_, err := store.Get(t.Context(), "app/timeout", provider.VersionRef{})
			require.NoError(t, err)
			assert.Equal(t, tt.wantLabel, gotLabel)
		})
	}
}

func TestGet_RejectsFilterNamespace(t *testing.T) {
	t.Parallel()

	for _, ns := range []string{"*", "dev,prod"} {
		called := false
		m := &mockClient{
			getFunc: func(_ context.Context, _, _ string) (azappconfig.GetSettingResponse, error) {
				called = true

				return azappconfig.GetSettingResponse{}, nil
			},
		}
		store := appconfig.New(m, ns)

		_, err := store.Get(t.Context(), "app/timeout", provider.VersionRef{})
		require.Error(t, err, "ns=%q", ns)
		assert.Contains(t, err.Error(), "single-item operation needs one")
		assert.False(t, called, "client must not be called when the namespace is a filter")
	}
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, _, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{}, notFound()
		},
	}
	store := appconfig.New(m, "")

	_, err := store.Get(t.Context(), "missing", provider.VersionRef{})
	require.ErrorIs(t, err, provider.ErrNotFound)
}

func TestGet_NoTags(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, key, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
				Key:   lo.ToPtr(key),
				Value: lo.ToPtr("v"),
			}}, nil
		},
	}
	store := appconfig.New(m, "")

	entry, err := store.Get(t.Context(), "app/timeout", provider.VersionRef{})
	require.NoError(t, err)
	assert.Nil(t, entry.Tags)
}

func TestGet_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, _, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{}, serverError()
		},
	}
	store := appconfig.New(m, "")

	_, err := store.Get(t.Context(), "app/timeout", provider.VersionRef{})
	require.Error(t, err)
	require.NotErrorIs(t, err, provider.ErrNotFound)
	assert.Contains(t, err.Error(), "get setting")
}

func TestHistory_Unsupported(t *testing.T) {
	t.Parallel()

	store := appconfig.New(&mockClient{}, "")

	versions, err := store.History(t.Context(), "my-key")
	require.Error(t, err)
	assert.Nil(t, versions)
	assert.Contains(t, err.Error(), "does not support versions")
}

func TestList(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listFunc: func(_ context.Context, _ string) ([]azappconfig.Setting, error) {
			return []azappconfig.Setting{
				{Key: lo.ToPtr("beta")},
				{Key: lo.ToPtr("alpha")},
				{Key: lo.ToPtr("beta")}, // duplicate across labels
			}, nil
		},
	}
	store := appconfig.New(m, "")

	names, err := store.List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, names)
}

func TestList_ForwardsFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		namespace  string
		wantFilter string
	}{
		{name: "empty is null-label filter", namespace: "", wantFilter: "\x00"},
		{name: "wildcard all", namespace: "*", wantFilter: "*"},
		{name: "OR-list", namespace: "dev,prod", wantFilter: "dev,prod"},
		{name: "prefix wildcard", namespace: "dev*", wantFilter: "dev*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotFilter string

			m := &mockClient{
				listFunc: func(_ context.Context, filter string) ([]azappconfig.Setting, error) {
					gotFilter = filter

					return nil, nil
				},
			}
			store := appconfig.New(m, tt.namespace)

			_, err := store.List(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.wantFilter, gotFilter)
		})
	}
}

func TestList_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listFunc: func(_ context.Context, _ string) ([]azappconfig.Setting, error) {
			return nil, serverError()
		},
	}
	store := appconfig.New(m, "")

	_, err := store.List(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list settings")
}

func TestListWithNamespaces(t *testing.T) {
	t.Parallel()

	var gotFilter string

	m := &mockClient{
		listFunc: func(_ context.Context, filter string) ([]azappconfig.Setting, error) {
			gotFilter = filter

			// Settings span the null (unlabeled) namespace and "dev"/"prd", in
			// an unsorted order to exercise the sort.
			return []azappconfig.Setting{
				{Key: lo.ToPtr("beta"), Label: lo.ToPtr("prd"), Value: lo.ToPtr("bp")},
				{Key: lo.ToPtr("alpha"), Value: lo.ToPtr("a-null")},
				{Key: lo.ToPtr("alpha"), Label: lo.ToPtr("dev"), Value: lo.ToPtr("ad")},
				{Key: lo.ToPtr("beta"), Value: lo.ToPtr("b-null")},
			}, nil
		},
	}
	// The store is scoped to "dev", but ListWithNamespaces must ignore that and
	// enumerate ALL namespaces via the "*" wildcard filter.
	store := appconfig.New(m, "dev")

	items, err := store.ListWithNamespaces(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "*", gotFilter, "must list across all namespaces regardless of the store's namespace")
	assert.Equal(t, []appconfig.KeyNamespace{
		{Key: "alpha", Namespace: "", Value: "a-null"},
		{Key: "alpha", Namespace: "dev", Value: "ad"},
		{Key: "beta", Namespace: "", Value: "b-null"},
		{Key: "beta", Namespace: "prd", Value: "bp"},
	}, items)
}

func TestListWithNamespaces_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listFunc: func(_ context.Context, _ string) ([]azappconfig.Setting, error) {
			return nil, serverError()
		},
	}
	store := appconfig.New(m, "")

	_, err := store.ListWithNamespaces(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list settings across namespaces")
}

func TestListWithNamespacesScoped(t *testing.T) {
	t.Parallel()

	settings := []azappconfig.Setting{
		{Key: lo.ToPtr("beta"), Label: lo.ToPtr("prd"), Value: lo.ToPtr("bp")},
		{Key: lo.ToPtr("alpha"), Value: lo.ToPtr("a-null")},
		{Key: lo.ToPtr("alpha"), Label: lo.ToPtr("dev"), Value: lo.ToPtr("ad")},
		{Key: lo.ToPtr("beta"), Value: lo.ToPtr("b-null")},
	}

	// The store namespace becomes the label filter verbatim, EXCEPT empty, which
	// maps to the reserved null-label filter — this is what makes the default
	// `param list` scope to the null namespace rather than "*".
	for name, tc := range map[string]struct {
		namespace  string
		wantFilter string
	}{
		"null namespace uses the null-label filter": {namespace: "", wantFilter: "\x00"},
		"single namespace forwarded":                {namespace: "dev", wantFilter: "dev"},
		"all-namespaces wildcard forwarded":         {namespace: "*", wantFilter: "*"},
		"OR-list forwarded":                         {namespace: "dev,prd", wantFilter: "dev,prd"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var gotFilter string

			m := &mockClient{
				listFunc: func(_ context.Context, filter string) ([]azappconfig.Setting, error) {
					gotFilter = filter

					return settings, nil
				},
			}
			store := appconfig.New(m, tc.namespace)

			items, err := store.ListWithNamespacesScoped(t.Context())
			require.NoError(t, err)

			assert.Equal(t, tc.wantFilter, gotFilter, "the store namespace must drive the label filter")
			// The service applies the filter; the store only sorts by (key, namespace).
			assert.Equal(t, []appconfig.KeyNamespace{
				{Key: "alpha", Namespace: "", Value: "a-null"},
				{Key: "alpha", Namespace: "dev", Value: "ad"},
				{Key: "beta", Namespace: "", Value: "b-null"},
				{Key: "beta", Namespace: "prd", Value: "bp"},
			}, items)
		})
	}
}

func TestListWithNamespacesScoped_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		listFunc: func(_ context.Context, _ string) ([]azappconfig.Setting, error) {
			return nil, serverError()
		},
	}
	store := appconfig.New(m, "")

	_, err := store.ListWithNamespacesScoped(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list settings with namespaces")
}

func TestCreate_AlreadyExists(t *testing.T) {
	t.Parallel()

	for name, errFn := range map[string]func() error{
		"precondition_failed": preconditionFailed,
		"conflict":            conflict,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := &mockClient{
				addFunc: func(_ context.Context, _, _, _ string) (azappconfig.AddSettingResponse, error) {
					return azappconfig.AddSettingResponse{}, errFn()
				},
			}
			store := appconfig.New(m, "")

			_, err := store.Create(t.Context(), "existing", "v", domain.ValueTypePlaintext, "")
			require.ErrorIs(t, err, provider.ErrAlreadyExists)
		})
	}
}

func TestCreate_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		addFunc: func(_ context.Context, _, _, _ string) (azappconfig.AddSettingResponse, error) {
			return azappconfig.AddSettingResponse{}, serverError()
		},
	}
	store := appconfig.New(m, "")

	_, err := store.Create(t.Context(), "new-key", "v", domain.ValueTypePlaintext, "")
	require.Error(t, err)
	require.NotErrorIs(t, err, provider.ErrAlreadyExists)
	assert.Contains(t, err.Error(), "create setting")
}

func TestCreate_New(t *testing.T) {
	t.Parallel()

	var added, addedLabel string

	m := &mockClient{
		addFunc: func(_ context.Context, key, value, label string) (azappconfig.AddSettingResponse, error) {
			added, addedLabel = key, label

			assert.Equal(t, "v", value)

			return azappconfig.AddSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "dev")

	version, err := store.Create(t.Context(), "new-key", "v", domain.ValueTypePlaintext, "")
	require.NoError(t, err)
	assert.Equal(t, "new-key", added)
	assert.Equal(t, "dev", addedLabel)
	assert.Empty(t, version.ID) // unversioned
}

func TestCreate_RejectsFilterNamespace(t *testing.T) {
	t.Parallel()

	called := false
	m := &mockClient{
		addFunc: func(_ context.Context, _, _, _ string) (azappconfig.AddSettingResponse, error) {
			called = true

			return azappconfig.AddSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "dev,prod")

	_, err := store.Create(t.Context(), "new-key", "v", domain.ValueTypePlaintext, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single-item operation needs one")
	assert.False(t, called)
}

func TestPut(t *testing.T) {
	t.Parallel()

	var setKey, setVal, setLabel string

	m := &mockClient{
		// Put first GETs the current tags to re-send them; here the setting does
		// not yet exist, so there are none to preserve.
		getFunc: func(_ context.Context, _, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{}, notFound()
		},
		setFunc: func(
			_ context.Context, key, value, label string, _ map[string]*string, _ *azcore.ETag,
		) (azappconfig.SetSettingResponse, error) {
			setKey, setVal, setLabel = key, value, label

			return azappconfig.SetSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "dev")

	version, err := store.Put(t.Context(), "app/timeout", "60", domain.ValueTypePlaintext, "")
	require.NoError(t, err)
	assert.Equal(t, "app/timeout", setKey)
	assert.Equal(t, "60", setVal)
	assert.Equal(t, "dev", setLabel)
	assert.Empty(t, version.ID)
}

// TestPut_PreservesExistingTags asserts a value update re-sends the setting's
// current tags so the PUT (which replaces the whole key-value) does not clear
// them.
func TestPut_PreservesExistingTags(t *testing.T) {
	t.Parallel()

	var sentTags map[string]*string

	m := &mockClient{
		getFunc: func(_ context.Context, key, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
				Key:   lo.ToPtr(key),
				Value: lo.ToPtr("old"),
				Tags:  map[string]*string{"env": lo.ToPtr("prod"), "team": lo.ToPtr("core")},
			}}, nil
		},
		setFunc: func(
			_ context.Context, _, value, _ string, tags map[string]*string, _ *azcore.ETag,
		) (azappconfig.SetSettingResponse, error) {
			sentTags = tags

			assert.Equal(t, "new", value)

			return azappconfig.SetSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "")

	_, err := store.Put(t.Context(), "app/timeout", "new", domain.ValueTypePlaintext, "")
	require.NoError(t, err)
	assert.Equal(t, map[string]*string{"env": lo.ToPtr("prod"), "team": lo.ToPtr("core")}, sentTags)
}

func TestPut_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		getFunc: func(_ context.Context, _, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{}, notFound()
		},
		setFunc: func(
			_ context.Context, _, _, _ string, _ map[string]*string, _ *azcore.ETag,
		) (azappconfig.SetSettingResponse, error) {
			return azappconfig.SetSettingResponse{}, serverError()
		},
	}
	store := appconfig.New(m, "")

	_, err := store.Put(t.Context(), "app/timeout", "60", domain.ValueTypePlaintext, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set setting")
}

func TestPut_RejectsFilterNamespace(t *testing.T) {
	t.Parallel()

	called := false
	m := &mockClient{
		setFunc: func(
			_ context.Context, _, _, _ string, _ map[string]*string, _ *azcore.ETag,
		) (azappconfig.SetSettingResponse, error) {
			called = true

			return azappconfig.SetSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "*")

	_, err := store.Put(t.Context(), "app/timeout", "60", domain.ValueTypePlaintext, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single-item operation needs one")
	assert.False(t, called)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	var deleted, deletedLabel string

	m := &mockClient{
		deleteFunc: func(_ context.Context, key, label string) (azappconfig.DeleteSettingResponse, error) {
			deleted, deletedLabel = key, label

			return azappconfig.DeleteSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "dev")

	require.NoError(t, store.Delete(t.Context(), "app/timeout"))
	assert.Equal(t, "app/timeout", deleted)
	assert.Equal(t, "dev", deletedLabel)
}

func TestDelete_NotFound(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		deleteFunc: func(_ context.Context, _, _ string) (azappconfig.DeleteSettingResponse, error) {
			return azappconfig.DeleteSettingResponse{}, notFound()
		},
	}
	store := appconfig.New(m, "")

	err := store.Delete(t.Context(), "missing")
	require.ErrorIs(t, err, provider.ErrNotFound)
}

func TestDelete_Error(t *testing.T) {
	t.Parallel()

	m := &mockClient{
		deleteFunc: func(_ context.Context, _, _ string) (azappconfig.DeleteSettingResponse, error) {
			return azappconfig.DeleteSettingResponse{}, serverError()
		},
	}
	store := appconfig.New(m, "")

	err := store.Delete(t.Context(), "app/timeout")
	require.Error(t, err)
	require.NotErrorIs(t, err, provider.ErrNotFound)
	assert.Contains(t, err.Error(), "delete setting")
}

func TestDelete_RejectsFilterNamespace(t *testing.T) {
	t.Parallel()

	called := false
	m := &mockClient{
		deleteFunc: func(_ context.Context, _, _ string) (azappconfig.DeleteSettingResponse, error) {
			called = true

			return azappconfig.DeleteSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "a,b")

	err := store.Delete(t.Context(), "app/timeout")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single-item operation needs one")
	assert.False(t, called)
}

// derefTags flattens a map[string]*string to a map[string]string for easy
// assertion.
func derefTags(tags map[string]*string) map[string]string {
	out := make(map[string]string, len(tags))
	for k, v := range tags {
		out[k] = lo.FromPtr(v)
	}

	return out
}

func TestTag_EmptyIsNoOp(t *testing.T) {
	t.Parallel()

	// A store whose funcs would panic if called: an empty mutation must not touch
	// the client.
	store := appconfig.New(&mockClient{}, "")

	require.NoError(t, store.Tag(t.Context(), "k", nil))
	require.NoError(t, store.Untag(t.Context(), "k", nil))
}

// TestTag_MergesAndPreservesValue asserts Tag GET-merge-PUTs: it adds/updates
// the given tags onto the existing ones, re-sends the unchanged value, and
// carries the current ETag as the OnlyIfUnchanged precondition under the
// namespace label.
func TestTag_MergesAndPreservesValue(t *testing.T) {
	t.Parallel()

	var (
		gotValue string
		gotLabel string
		gotTags  map[string]*string
		gotETag  *azcore.ETag
	)

	etag := azcore.ETag("v1")
	m := &mockClient{
		getFunc: func(_ context.Context, key, label string) (azappconfig.GetSettingResponse, error) {
			assert.Equal(t, "dev", label)

			return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
				Key:   lo.ToPtr(key),
				Value: lo.ToPtr("keep-me"),
				Tags:  map[string]*string{"env": lo.ToPtr("dev"), "keep": lo.ToPtr("yes")},
				ETag:  lo.ToPtr(etag),
			}}, nil
		},
		setFunc: func(
			_ context.Context, _, value, label string, tags map[string]*string, e *azcore.ETag,
		) (azappconfig.SetSettingResponse, error) {
			gotValue, gotLabel, gotTags, gotETag = value, label, tags, e

			return azappconfig.SetSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "dev")

	require.NoError(t, store.Tag(t.Context(), "app/timeout", map[string]string{"env": "prod", "team": "core"}))
	assert.Equal(t, "keep-me", gotValue)
	assert.Equal(t, "dev", gotLabel)
	assert.Equal(t, &etag, gotETag)
	assert.Equal(t, map[string]string{"env": "prod", "keep": "yes", "team": "core"}, derefTags(gotTags))
}

// TestUntag_RemovesKeys asserts Untag deletes the given keys from the current
// tags and re-PUTs the rest with the unchanged value.
func TestUntag_RemovesKeys(t *testing.T) {
	t.Parallel()

	var gotTags map[string]*string

	m := &mockClient{
		getFunc: func(_ context.Context, key, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
				Key:   lo.ToPtr(key),
				Value: lo.ToPtr("v"),
				Tags:  map[string]*string{"env": lo.ToPtr("prod"), "team": lo.ToPtr("core")},
			}}, nil
		},
		setFunc: func(
			_ context.Context, _, _, _ string, tags map[string]*string, _ *azcore.ETag,
		) (azappconfig.SetSettingResponse, error) {
			gotTags = tags

			return azappconfig.SetSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "")

	// Removing an absent key is a no-op alongside a present one.
	require.NoError(t, store.Untag(t.Context(), "app/timeout", []string{"env", "absent"}))
	assert.Equal(t, map[string]string{"team": "core"}, derefTags(gotTags))
}

// TestTag_RetriesOn412ThenSucceeds asserts a single ETag conflict triggers a
// re-GET-and-retry that then succeeds.
func TestTag_RetriesOn412ThenSucceeds(t *testing.T) {
	t.Parallel()

	var gets, sets int

	m := &mockClient{
		getFunc: func(_ context.Context, key, _ string) (azappconfig.GetSettingResponse, error) {
			gets++

			return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
				Key:   lo.ToPtr(key),
				Value: lo.ToPtr("v"),
			}}, nil
		},
		setFunc: func(
			_ context.Context, _, _, _ string, _ map[string]*string, _ *azcore.ETag,
		) (azappconfig.SetSettingResponse, error) {
			sets++
			if sets == 1 {
				return azappconfig.SetSettingResponse{}, preconditionFailed()
			}

			return azappconfig.SetSettingResponse{}, nil
		},
	}
	store := appconfig.New(m, "")

	require.NoError(t, store.Tag(t.Context(), "k", map[string]string{"env": "prod"}))
	assert.Equal(t, 2, gets)
	assert.Equal(t, 2, sets)
}

// TestTag_SurfacesPersistentConflict asserts that a PUT that keeps conflicting
// (412) is retried a bounded number of times and then surfaced as an error.
func TestTag_SurfacesPersistentConflict(t *testing.T) {
	t.Parallel()

	var sets int

	m := &mockClient{
		getFunc: func(_ context.Context, key, _ string) (azappconfig.GetSettingResponse, error) {
			return azappconfig.GetSettingResponse{Setting: azappconfig.Setting{
				Key:   lo.ToPtr(key),
				Value: lo.ToPtr("v"),
			}}, nil
		},
		setFunc: func(
			_ context.Context, _, _, _ string, _ map[string]*string, _ *azcore.ETag,
		) (azappconfig.SetSettingResponse, error) {
			sets++

			return azappconfig.SetSettingResponse{}, preconditionFailed()
		},
	}
	store := appconfig.New(m, "")

	err := store.Tag(t.Context(), "k", map[string]string{"env": "prod"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concurrent modification")
	// The retry loop is bounded (3 attempts here).
	assert.Equal(t, 3, sets)
}

// TestTag_GetError surfaces a non-conflict error from the GET without retrying.
func TestTag_GetError(t *testing.T) {
	t.Parallel()

	var gets int

	m := &mockClient{
		getFunc: func(_ context.Context, _, _ string) (azappconfig.GetSettingResponse, error) {
			gets++

			return azappconfig.GetSettingResponse{}, notFound()
		},
	}
	store := appconfig.New(m, "")

	err := store.Tag(t.Context(), "k", map[string]string{"env": "prod"})
	require.ErrorIs(t, err, provider.ErrNotFound)
	assert.Equal(t, 1, gets)
}

// TestTag_RejectsFilterNamespace asserts a filter (all/multiple) namespace is
// rejected before any client call.
func TestTag_RejectsFilterNamespace(t *testing.T) {
	t.Parallel()

	store := appconfig.New(&mockClient{}, "dev,prod")

	err := store.Tag(t.Context(), "k", map[string]string{"env": "prod"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single-item operation needs one")

	err = store.Untag(t.Context(), "k", []string{"env"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single-item operation needs one")
}

// Compile-time assertion that the mock satisfies the adapter's Client interface.
var _ appconfig.Client = (*mockClient)(nil)
