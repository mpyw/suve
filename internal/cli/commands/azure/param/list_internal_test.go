// White-box tests of list.go.
//declscope:namespace list

package param

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/providermock"
	paramusecase "github.com/mpyw/suve/internal/usecase/param"
)

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
		require.NoError(t, r.run(t.Context(), listOptions{}))
		assert.Equal(t, "(NULL)\tapp/a\ndev\tapp/a\n", buf.String())
	})

	t.Run("--show appends the value column", func(t *testing.T) {
		t.Parallel()

		var buf, errBuf bytes.Buffer

		r := nsListRunner(rows, nil, &buf, &errBuf)
		require.NoError(t, r.run(t.Context(), listOptions{show: true}))
		assert.Equal(t, "(NULL)\tapp/a\ta-null\ndev\tapp/a\ta-dev\n", buf.String())
	})

	t.Run("json carries the raw namespace (empty, not (NULL))", func(t *testing.T) {
		t.Parallel()

		var buf, errBuf bytes.Buffer

		r := nsListRunner(rows, nil, &buf, &errBuf)
		require.NoError(t, r.run(t.Context(), listOptions{output: output.FormatJSON}))
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
	require.NoError(t, r.run(t.Context(), listOptions{hideNS: true}))
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
			require.NoError(t, r.run(t.Context(), listOptions{show: true, hideNS: true, namespace: ns}))
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
	require.NoError(t, r.run(t.Context(), listOptions{show: true, hideNS: true, namespace: "*"}))
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
	require.NoError(t, r.run(t.Context(), listOptions{show: true, hideNS: true, namespace: "dev"}))
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

	r := &listRunner{
		keyOnly: &paramusecase.ListUseCase{Reader: reader},
		stdout:  &buf,
		stderr:  &errBuf,
	}
	require.NoError(t, r.run(t.Context(), listOptions{}))
	assert.Equal(t, "k1\nk2\n", buf.String())
}

// namespaceListerStub is the App-Config-specific lister the namespaced list
// path depends on.
type namespaceListerStub struct {
	rows []appconfig.KeyNamespace
}

func (s *namespaceListerStub) ListWithNamespacesScoped(_ context.Context) ([]appconfig.KeyNamespace, error) {
	return s.rows, nil
}

func nsListRunner(rows []appconfig.KeyNamespace, keyOnlyReader provider.Reader, out, errOut *bytes.Buffer) *listRunner {
	return &listRunner{
		namespace: &paramusecase.ListNamespacesUseCase{Lister: newNamespaceLister(&namespaceListerStub{rows: rows})},
		keyOnly:   &paramusecase.ListUseCase{Reader: keyOnlyReader},
		stdout:    out,
		stderr:    errOut,
	}
}
