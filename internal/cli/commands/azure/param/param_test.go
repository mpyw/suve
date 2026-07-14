package param_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/azure/param"
	genericdiff "github.com/mpyw/suve/internal/cli/commands/generic/diff"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/usecase/azure"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
)

// TestCommandValidation checks argument handling that fails before any store is
// resolved (no Azure credentials needed) — and never panics. App Configuration
// keys are unversioned but may legally contain ':' / '#' / '~', so those are
// treated as ordinary key characters rather than rejected as version specifiers.
func TestCommandValidation(t *testing.T) {
	t.Parallel()

	// missingKey / usage errors are raised before store resolution.
	usageTests := []struct {
		name string
		args []string
	}{
		{
			name: "show missing key",
			args: []string{"suve", "azure", "param", "show"},
		},
		{
			name: "create missing args",
			args: []string{"suve", "azure", "param", "create"},
		},
	}

	for _, tt := range usageTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appcli.MakeApp()
			err := app.Run(t.Context(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "usage:")
		})
	}

	// Specifier-like characters are legal key characters: the key is accepted
	// and the command proceeds to store resolution (which fails only because no
	// store is configured), never producing a version-related rejection.
	keyTests := []struct {
		name string
		args []string
	}{
		{"hash in key accepted", []string{"suve", "azure", "param", "show", "my-key#1"}},
		{"tilde in key accepted", []string{"suve", "azure", "param", "show", "my-key~1"}},
		{"colon label-like key accepted", []string{"suve", "azure", "param", "show", "my-key:prod"}},
		{"ASP.NET colon hierarchy accepted", []string{"suve", "azure", "param", "show", "Logging:LogLevel:Default"}},
	}

	for _, tt := range keyTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appcli.MakeApp()
			err := app.Run(t.Context(), tt.args)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "does not support versions")
			assert.Contains(t, err.Error(), "store specified")
		})
	}
}

// TestLogPresenter_AcidTest is the acid test: App Configuration has no version
// history, so the log presenter's Fetch surfaces a clean error and never
// crashes.
func TestLogPresenter_AcidTest(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			// Mirror the appconfig adapter's degradation.
			return nil, azureappconfigversion.ErrVersioningUnsupported
		},
	}

	presenter := param.NewLogPresenter(store, genericlog.Request{Name: "my-key"})
	err := presenter.Fetch(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support versions")
	// Render methods are safe to call even after a failed fetch (no panic).
	assert.Equal(t, 0, presenter.Len())
}

func TestShowPresenter(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec)

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   "30",
				Type:    domain.ValueTypePlaintext,
				Version: domain.Version{}, // unversioned
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := azureappconfigversion.Parse("app/timeout")
	require.NoError(t, err)

	presenter := param.NewShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	presenter.RenderText(&buf, value)

	out := buf.String()
	assert.Contains(t, out, "app/timeout")
	assert.Contains(t, out, "30")
	assert.Contains(t, out, "env")
	// No version metadata for App Configuration.
	assert.NotContains(t, out, "Version")
}

func TestCreateRunner(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "app/timeout", name)
			assert.Equal(t, "30", value)
			assert.Equal(t, domain.ValueTypePlaintext, vt)

			return domain.Version{}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &param.CreateRunner{
		UseCase: &azure.CreateUseCase{Writer: store},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	require.NoError(t, r.Run(t.Context(), param.CreateOptions{Name: "app/timeout", Value: "30"}))
	assert.Contains(t, buf.String(), "Created setting app/timeout")
}

func TestDeleteRunner(t *testing.T) {
	t.Parallel()

	var deleted string

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
			deleted = name

			return nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &param.DeleteRunner{
		UseCase: &azure.DeleteUseCase{Store: store},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	require.NoError(t, r.Run(t.Context(), param.DeleteOptions{Name: "app/timeout"}))
	assert.Equal(t, "app/timeout", deleted)
	assert.Contains(t, buf.String(), "Deleted setting app/timeout")
}

// namespaceListerStub is the App-Config-specific lister the namespaced list
// path depends on.
type namespaceListerStub struct {
	rows []appconfig.KeyNamespace
}

func (s *namespaceListerStub) ListWithNamespacesScoped(_ context.Context) ([]appconfig.KeyNamespace, error) {
	return s.rows, nil
}

func nsListRunner(rows []appconfig.KeyNamespace, keyOnlyReader provider.Reader, out, errOut *bytes.Buffer) *param.ListRunner {
	return &param.ListRunner{
		Namespace: &azure.ListNamespacesUseCase{Lister: &namespaceListerStub{rows: rows}},
		KeyOnly:   &azure.ListUseCase{Reader: keyOnlyReader},
		Stdout:    out,
		Stderr:    errOut,
	}
}

func TestListRunner_NamespaceColumn(t *testing.T) {
	t.Parallel()

	rows := []appconfig.KeyNamespace{
		{Key: "app/a", Namespace: "", Value: "a-null"},
		{Key: "app/a", Namespace: "dev", Value: "a-dev"},
	}

	t.Run("default prepends NAMESPACE column and renders null as (NULL)", func(t *testing.T) {
		t.Parallel()

		var buf, errBuf bytes.Buffer

		r := nsListRunner(rows, nil, &buf, &errBuf)
		require.NoError(t, r.Run(t.Context(), param.ListOptions{}))
		assert.Equal(t, "(NULL)\tapp/a\ndev\tapp/a\n", buf.String())
	})

	t.Run("--show appends the value column", func(t *testing.T) {
		t.Parallel()

		var buf, errBuf bytes.Buffer

		r := nsListRunner(rows, nil, &buf, &errBuf)
		require.NoError(t, r.Run(t.Context(), param.ListOptions{Show: true}))
		assert.Equal(t, "(NULL)\tapp/a\ta-null\ndev\tapp/a\ta-dev\n", buf.String())
	})

	t.Run("json carries the raw namespace (empty, not (NULL))", func(t *testing.T) {
		t.Parallel()

		var buf, errBuf bytes.Buffer

		r := nsListRunner(rows, nil, &buf, &errBuf)
		require.NoError(t, r.Run(t.Context(), param.ListOptions{Output: output.FormatJSON}))
		assert.JSONEq(t, `[{"namespace":"","name":"app/a"},{"namespace":"dev","name":"app/a"}]`, buf.String())
	})
}

func TestListRunner_HideNamespace(t *testing.T) {
	t.Parallel()

	// --hide-namespace falls back to the neutral key-only listing (Reader.List),
	// so no NAMESPACE column appears and keys are deduped by the provider.
	reader := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return []string{"app/a", "app/b"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := nsListRunner(nil, reader, &buf, &errBuf)
	require.NoError(t, r.Run(t.Context(), param.ListOptions{HideNS: true}))
	assert.Equal(t, "app/a\napp/b\n", buf.String())
}

// TestListRunner_HideNamespaceShowWildcard asserts --hide-namespace --show under
// a non-literal namespace (wildcard/OR/prefix) sources values from the
// namespaced list — which already carries them — instead of per-key Get (which
// cannot address all/multiple namespaces and would make every row an error).
// The neutral reader must not be touched at all.
func TestListRunner_HideNamespaceShowWildcard(t *testing.T) {
	t.Parallel()

	rows := []appconfig.KeyNamespace{
		{Key: "a", Namespace: "dev", Value: "1"},
		{Key: "b", Namespace: "prod", Value: "2"},
	}

	for _, ns := range []string{"*", "dev,prod", "dev*"} {
		t.Run(ns, func(t *testing.T) {
			t.Parallel()

			var buf, errBuf bytes.Buffer

			// An unconfigured neutral reader errors on any call, proving the
			// diverted path never falls back to it.
			r := nsListRunner(rows, &providermock.Store{}, &buf, &errBuf)
			require.NoError(t, r.Run(t.Context(), param.ListOptions{Show: true, HideNS: true, Namespace: ns}))
			assert.Equal(t, "a\t1\nb\t2\n", buf.String())
		})
	}
}

// TestListRunner_HideNamespaceShowWildcardAmbiguous asserts a key that resolves
// to different values across the matched namespaces becomes an error row rather
// than an arbitrary pick, while unambiguous keys still show their value.
func TestListRunner_HideNamespaceShowWildcardAmbiguous(t *testing.T) {
	t.Parallel()

	rows := []appconfig.KeyNamespace{
		{Key: "dup", Namespace: "dev", Value: "1"},
		{Key: "dup", Namespace: "prod", Value: "2"},
		{Key: "uniq", Namespace: "dev", Value: "ok"},
	}

	var buf, errBuf bytes.Buffer

	r := nsListRunner(rows, &providermock.Store{}, &buf, &errBuf)
	require.NoError(t, r.Run(t.Context(), param.ListOptions{Show: true, HideNS: true, Namespace: "*"}))
	assert.Equal(t, "dup\t<error: value differs across namespaces; drop --hide-namespace to see each>\nuniq\tok\n", buf.String())
}

// TestListRunner_HideNamespaceShowLiteral asserts a single literal namespace with
// --show keeps using the neutral per-key path (Reader.List + Get), unaffected by
// the wildcard diversion.
func TestListRunner_HideNamespaceShowLiteral(t *testing.T) {
	t.Parallel()

	reader := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return []string{"k"}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "v"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	// Namespaced rows would resolve to a different value; the literal path must
	// win, proving it did not divert.
	rows := []appconfig.KeyNamespace{{Key: "k", Namespace: "dev", Value: "WRONG"}}
	r := nsListRunner(rows, reader, &buf, &errBuf)
	require.NoError(t, r.Run(t.Context(), param.ListOptions{Show: true, HideNS: true, Namespace: "dev"}))
	assert.Equal(t, "k\tv\n", buf.String())
}

func TestListRunner_NonAppConfigNoColumn(t *testing.T) {
	t.Parallel()

	// A store without the App-Config extension leaves Namespace nil, so the
	// NAMESPACE column never appears even without --hide-namespace.
	reader := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return []string{"k1", "k2"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &param.ListRunner{
		KeyOnly: &azure.ListUseCase{Reader: reader},
		Stdout:  &buf,
		Stderr:  &errBuf,
	}
	require.NoError(t, r.Run(t.Context(), param.ListOptions{}))
	assert.Equal(t, "k1\nk2\n", buf.String())
}

// TestShowPresenter_RenderJSON covers the App Configuration show presenter's
// JSON path: name/value plus the optional "modified" timestamp and the tags map.
func TestShowPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec)

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   "30",
				Type:    domain.ValueTypePlaintext,
				Version: domain.Version{Created: &modified}, // unversioned, but carries a modified time
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := azureappconfigversion.Parse("app/timeout")
	require.NoError(t, err)

	presenter := param.NewShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	require.NoError(t, presenter.RenderJSON(&buf, value))

	var out struct {
		Name     string            `json:"name"`
		Modified string            `json:"modified"`
		Tags     map[string]string `json:"tags"`
		Value    string            `json:"value"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "app/timeout", out.Name)
	assert.Equal(t, "30", out.Value)
	assert.Equal(t, timeutil.FormatRFC3339(modified), out.Modified)
	assert.Equal(t, map[string]string{"env": "prod"}, out.Tags)
}

// TestDiffPresenter_RenderJSON drives the App Configuration diff presenter end to
// end through the generic Runner with --output=json, covering NewDiffPresenter,
// Fetch, OldValue/NewValue, Labels, and RenderJSON. App Configuration is
// unversioned, so diff compares two distinct keys with an empty suffix.
func TestDiffPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	values := map[string]string{"key-a": "alpha", "key-b": "beta"}

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec) // App Configuration is unversioned.

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: values[name]}, nil
		},
	}

	spec1, err := azureappconfigversion.Parse("key-a")
	require.NoError(t, err)
	spec2, err := azureappconfigversion.Parse("key-b")
	require.NoError(t, err)

	presenter := param.NewDiffPresenter(store, spec1, spec2)

	var stdout, stderr bytes.Buffer

	r := &genericdiff.Runner{
		Presenter: presenter,
		Options:   genericdiff.Options{Output: output.FormatJSON},
		Stdout:    &stdout,
		Stderr:    &stderr,
	}
	require.NoError(t, r.Run(t.Context()))

	var out struct {
		OldName   string `json:"oldName"`
		OldValue  string `json:"oldValue"`
		NewName   string `json:"newName"`
		NewValue  string `json:"newValue"`
		Identical bool   `json:"identical"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, "key-a", out.OldName)
	assert.Equal(t, "alpha", out.OldValue)
	assert.Equal(t, "key-b", out.NewName)
	assert.Equal(t, "beta", out.NewValue)
	assert.False(t, out.Identical)
}

// TestLogPresenter_RenderStubs covers the App Configuration log presenter's
// render stubs. App Configuration has no version history, so these methods only
// exist to satisfy the genericlog.Presenter interface and must be safe no-ops.
func TestLogPresenter_RenderStubs(t *testing.T) {
	t.Parallel()

	presenter := param.NewLogPresenter(&providermock.Store{}, genericlog.Request{Name: "my-key"})

	var buf, errBuf bytes.Buffer

	assert.Equal(t, 0, presenter.Len())
	require.NoError(t, presenter.RenderJSON(&buf))
	presenter.RenderOneline(&buf, 0, 0)
	presenter.RenderHeader(&buf, 0)
	presenter.RenderValue(&buf, 0, 0)
	presenter.RenderPatch(&buf, &errBuf, 0, false, false)

	assert.Empty(t, buf.String())
	assert.Empty(t, errBuf.String())
}
