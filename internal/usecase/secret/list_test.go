package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// listStore builds a mock reader returning the given names, and (when values are
// requested) values/errors keyed by name.
func listStore(names []string, values map[string]string, errs map[string]error) *providermock.Store {
	return &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return names, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			if errs != nil {
				if err, ok := errs[name]; ok {
					return nil, err
				}
			}

			return &domain.Entry{Name: name, Value: values[name]}, nil
		},
	}
}

func TestListUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	uc := &secret.ListUseCase{Reader: listStore(nil, nil, nil)}

	output, err := uc.Execute(t.Context(), secret.ListInput{})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestListUseCase_Execute_WithSecrets(t *testing.T) {
	t.Parallel()

	uc := &secret.ListUseCase{Reader: listStore([]string{"secret-a", "secret-b"}, nil, nil)}

	output, err := uc.Execute(t.Context(), secret.ListInput{})
	require.NoError(t, err)
	require.Len(t, output.Entries, 2)
	assert.Equal(t, "secret-a", output.Entries[0].Name)
	assert.Equal(t, "secret-b", output.Entries[1].Name)
}

// TestListUseCase_Execute_WithPrefix is a genuine anti-regression test: the
// name filter is a case-sensitive PREFIX match applied client-side.
func TestListUseCase_Execute_WithPrefix(t *testing.T) {
	t.Parallel()

	uc := &secret.ListUseCase{Reader: listStore(
		[]string{"app/config", "app/secret", "other/thing", "App/upper"}, nil, nil,
	)}

	output, err := uc.Execute(t.Context(), secret.ListInput{Prefix: "app/"})
	require.NoError(t, err)

	names := lo.Map(output.Entries, func(e secret.ListEntry, _ int) string { return e.Name })
	assert.ElementsMatch(t, []string{"app/config", "app/secret"}, names)
	// Case-sensitive: "App/upper" must NOT match prefix "app/".
	assert.NotContains(t, names, "App/upper")
	// Non-prefix names must be excluded.
	assert.NotContains(t, names, "other/thing")
}

func TestListUseCase_Execute_WithFilter(t *testing.T) {
	t.Parallel()

	uc := &secret.ListUseCase{Reader: listStore(
		[]string{"config-a", "secret-b", "config-c"}, nil, nil,
	)}

	output, err := uc.Execute(t.Context(), secret.ListInput{Filter: "config"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_InvalidFilter(t *testing.T) {
	t.Parallel()

	uc := &secret.ListUseCase{Reader: listStore(nil, nil, nil)}

	_, err := uc.Execute(t.Context(), secret.ListInput{Filter: "[invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter regex")
}

func TestListUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return nil, errors.New("aws error")
		},
	}

	uc := &secret.ListUseCase{Reader: store}

	_, err := uc.Execute(t.Context(), secret.ListInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secrets")
}

func TestListUseCase_Execute_WithValue(t *testing.T) {
	t.Parallel()

	uc := &secret.ListUseCase{Reader: listStore(
		[]string{"secret-a", "secret-b"},
		map[string]string{"secret-a": "value-a", "secret-b": "value-b"},
		nil,
	)}

	output, err := uc.Execute(t.Context(), secret.ListInput{WithValue: true})
	require.NoError(t, err)
	require.Len(t, output.Entries, 2)

	for _, entry := range output.Entries {
		assert.NotNil(t, entry.Value)
		assert.NoError(t, entry.Error)
	}
}

func TestListUseCase_Execute_WithValue_PartialError(t *testing.T) {
	t.Parallel()

	uc := &secret.ListUseCase{Reader: listStore(
		[]string{"secret-a", "secret-error"},
		map[string]string{"secret-a": "value-a"},
		map[string]error{"secret-error": errors.New("fetch error")},
	)}

	output, err := uc.Execute(t.Context(), secret.ListInput{WithValue: true})
	require.NoError(t, err)
	require.Len(t, output.Entries, 2)

	var hasValue, hasError bool

	for _, entry := range output.Entries {
		if entry.Value != nil {
			hasValue = true
		}

		if entry.Error != nil {
			hasError = true
		}
	}

	assert.True(t, hasValue)
	assert.True(t, hasError)
}

// TestListUseCase_Execute_SortsNames verifies the list use case emits names in a
// stable alphabetical order regardless of the provider's native ordering (#480).
func TestListUseCase_Execute_SortsNames(t *testing.T) {
	t.Parallel()

	uc := &secret.ListUseCase{Reader: listStore([]string{"charlie", "alpha", "bravo"}, nil, nil)}

	output, err := uc.Execute(t.Context(), secret.ListInput{})
	require.NoError(t, err)

	names := make([]string, len(output.Entries))
	for i, e := range output.Entries {
		names[i] = e.Name
	}

	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, names)
}
