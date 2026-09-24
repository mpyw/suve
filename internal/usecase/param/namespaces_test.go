package param_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/usecase/param"
)

// NamespaceLister the namespace-aware list use case depends on.
type namespaceListerMock struct {
	rows []param.ListNamespacesRow
	err  error
}

func (m *namespaceListerMock) ListNamespaces(_ context.Context) ([]param.ListNamespacesRow, error) {
	return m.rows, m.err
}

func TestListNamespacesUseCase(t *testing.T) {
	t.Parallel()

	// The store (service) already sorts by (key, namespace); the use case only
	// layers the client-side key filters on top and preserves order.
	rows := []param.ListNamespacesRow{
		{Key: "app/a", Namespace: "", Value: "a-null"},
		{Key: "app/a", Namespace: "dev", Value: "a-dev"},
		{Key: "app/b", Namespace: "prd", Value: "b-prd"},
		{Key: "other", Namespace: "", Value: "o"},
	}

	t.Run("without value carries namespace and drops value", func(t *testing.T) {
		t.Parallel()

		uc := &param.ListNamespacesUseCase{Lister: &namespaceListerMock{rows: rows}}
		out, err := uc.Execute(t.Context(), param.ListNamespacesInput{})
		require.NoError(t, err)

		assert.Equal(t, []param.ListNamespacesEntry{
			{Namespace: "", Name: "app/a"},
			{Namespace: "dev", Name: "app/a"},
			{Namespace: "prd", Name: "app/b"},
			{Namespace: "", Name: "other"},
		}, out.Entries)
	})

	t.Run("prefix filters on the key, keeping every namespace of a match", func(t *testing.T) {
		t.Parallel()

		uc := &param.ListNamespacesUseCase{Lister: &namespaceListerMock{rows: rows}}
		out, err := uc.Execute(t.Context(), param.ListNamespacesInput{Prefix: "app/", WithValue: true})
		require.NoError(t, err)

		assert.Equal(t, []param.ListNamespacesEntry{
			{Namespace: "", Name: "app/a", Value: lo.ToPtr("a-null")},
			{Namespace: "dev", Name: "app/a", Value: lo.ToPtr("a-dev")},
			{Namespace: "prd", Name: "app/b", Value: lo.ToPtr("b-prd")},
		}, out.Entries)
	})

	t.Run("regex filters on the key", func(t *testing.T) {
		t.Parallel()

		uc := &param.ListNamespacesUseCase{Lister: &namespaceListerMock{rows: rows}}
		out, err := uc.Execute(t.Context(), param.ListNamespacesInput{Filter: "^other$"})
		require.NoError(t, err)

		assert.Equal(t, []param.ListNamespacesEntry{{Namespace: "", Name: "other"}}, out.Entries)
	})

	t.Run("invalid regex is a usage error", func(t *testing.T) {
		t.Parallel()

		uc := &param.ListNamespacesUseCase{Lister: &namespaceListerMock{rows: rows}}
		_, err := uc.Execute(t.Context(), param.ListNamespacesInput{Filter: "["})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid filter regex")
	})

	t.Run("lister error propagates", func(t *testing.T) {
		t.Parallel()

		uc := &param.ListNamespacesUseCase{Lister: &namespaceListerMock{err: errors.New("boom")}}
		_, err := uc.Execute(t.Context(), param.ListNamespacesInput{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list entries")
	})
}

// TestListUseCase_SortsNames verifies the list use case emits names in a stable
// alphabetical order regardless of the provider's native ordering. This covers
