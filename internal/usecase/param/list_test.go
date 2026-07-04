package param_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

// listNames returns a ListFunc yielding the given names.
func listNames(names ...string) func(context.Context) ([]string, error) {
	return func(_ context.Context) ([]string, error) {
		return names, nil
	}
}

func TestListUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{ListFunc: listNames()}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestListUseCase_Execute_WithPrefix(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{ListFunc: listNames("/app/config", "/app/secret")}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{Prefix: "/app"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_Recursive(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{ListFunc: listNames("/app/config", "/app/sub/nested")}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{Prefix: "/app", Recursive: true})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_OneLevelExcludesNested(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{ListFunc: listNames("/app/config", "/app/sub/nested")}

	uc := &param.ListUseCase{Reader: store}

	// Without --recursive, only immediate children of the prefix are returned.
	output, err := uc.Execute(t.Context(), param.ListInput{Prefix: "/app"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, "/app/config", output.Entries[0].Name)
}

// TestListUseCase_Execute_PrefixHierarchy verifies path-hierarchy semantics:
// prefix "/app" matches "/app" and its descendants but NOT the sibling
// "/application" (a bare string-prefix match would wrongly include it).
func TestListUseCase_Execute_PrefixHierarchy(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: listNames("/app/config", "/application/other", "/app"),
	}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{Prefix: "/app", Recursive: true})
	require.NoError(t, err)

	names := make([]string, len(output.Entries))
	for i, e := range output.Entries {
		names[i] = e.Name
	}

	assert.Contains(t, names, "/app/config")
	assert.Contains(t, names, "/app")
	assert.NotContains(t, names, "/application/other")
}

func TestListUseCase_Execute_WithFilter(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{ListFunc: listNames("/app/config", "/app/secret", "/app/other")}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{Filter: "config|secret"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_InvalidFilter(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{ListFunc: listNames()}

	uc := &param.ListUseCase{Reader: store}

	_, err := uc.Execute(t.Context(), param.ListInput{Filter: "[invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter regex")
}

func TestListUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return nil, errAWS
		},
	}

	uc := &param.ListUseCase{Reader: store}

	_, err := uc.Execute(t.Context(), param.ListInput{})
	require.Error(t, err)
}

func TestListUseCase_Execute_WithValue(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"/app/config": "config-value",
		"/app/secret": "secret-value",
	}
	store := &providermock.Store{
		ListFunc: listNames("/app/config", "/app/secret"),
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: values[name]}, nil
		},
	}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{WithValue: true})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	valueMap := make(map[string]string)

	for _, entry := range output.Entries {
		require.NotNil(t, entry.Value, "entry %s should have value", entry.Name)
		require.NoError(t, entry.Error, "entry %s should not have error", entry.Name)
		valueMap[entry.Name] = *entry.Value
	}

	assert.Equal(t, "config-value", valueMap["/app/config"])
	assert.Equal(t, "secret-value", valueMap["/app/secret"])
}

func TestListUseCase_Execute_WithValue_PartialError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: listNames("/app/config", "/app/invalid"),
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			if name == "/app/invalid" {
				return nil, fmt.Errorf("parameter not found: %s", name)
			}

			return &domain.Entry{Name: name, Value: "config-value"}, nil
		},
	}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{WithValue: true})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	for _, entry := range output.Entries {
		switch entry.Name {
		case "/app/config":
			require.NotNil(t, entry.Value)
			assert.Equal(t, "config-value", *entry.Value)
			require.NoError(t, entry.Error)
		case "/app/invalid":
			assert.Nil(t, entry.Value)
			require.Error(t, entry.Error)
			assert.Contains(t, entry.Error.Error(), "parameter not found")
		default:
			t.Errorf("unexpected entry: %s", entry.Name)
		}
	}
}

func TestListUseCase_Execute_WithValue_GetError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: listNames("/app/param1", "/app/param2"),
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errAccessDenied
		},
	}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{WithValue: true})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	for _, entry := range output.Entries {
		assert.Nil(t, entry.Value)
		require.Error(t, entry.Error)
		assert.Contains(t, entry.Error.Error(), "access denied")
	}
}

func TestListUseCase_Execute_WithValue_Many(t *testing.T) {
	t.Parallel()

	const numParams = 15

	names := make([]string, numParams)

	expectedValues := make(map[string]string, numParams)

	for i := range numParams {
		name := fmt.Sprintf("/app/param%d", i)
		names[i] = name
		expectedValues[name] = fmt.Sprintf("value%d", i)
	}

	store := &providermock.Store{
		ListFunc: listNames(names...),
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: expectedValues[name]}, nil
		},
	}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{WithValue: true})
	require.NoError(t, err)
	assert.Len(t, output.Entries, numParams)

	for _, entry := range output.Entries {
		require.NotNil(t, entry.Value, "entry %s should have value", entry.Name)
		require.NoError(t, entry.Error, "entry %s should not have error", entry.Name)
		assert.Equal(t, expectedValues[entry.Name], *entry.Value)
	}
}

func TestListUseCase_Execute_WithValue_Empty(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{ListFunc: listNames()}

	uc := &param.ListUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.ListInput{WithValue: true})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}
