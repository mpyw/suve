package param_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

// resolveBySuffix returns a ResolveFunc mapping a version-spec suffix to a ref.
func resolveBySuffix(mapping map[string]string) func(context.Context, string, string) (provider.VersionRef, error) {
	return func(_ context.Context, _, spec string) (provider.VersionRef, error) {
		return provider.NewVersionRef(mapping[spec]), nil
	}
}

// getByRef returns a GetFunc mapping a ref id to an entry value/version.
func getByRef(values map[string]string) func(context.Context, string, provider.VersionRef) (*domain.Entry, error) {
	return func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
		id := ref.ID()

		val, ok := values[id]
		if !ok {
			return nil, errUnexpectedCall
		}

		return &domain.Entry{Name: name, Value: val, Version: domain.Version{ID: id}}, nil
	}
}

func TestDiffUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: resolveBySuffix(map[string]string{"#1": "1", "#2": "2"}),
		GetFunc:     getByRef(map[string]string{"1": "old-value", "2": "new-value"}),
	}

	uc := &param.DiffUseCase{Reader: store}

	name1, suffix1 := "/app/config", "#1"
	name2, suffix2 := "/app/config", "#2"

	output, err := uc.Execute(t.Context(), param.DiffInput{Name1: name1, Suffix1: suffix1, Name2: name2, Suffix2: suffix2})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.OldName)
	assert.Equal(t, "1", output.OldVersion)
	assert.Equal(t, "old-value", output.OldValue)
	assert.Equal(t, "/app/config", output.NewName)
	assert.Equal(t, "2", output.NewVersion)
	assert.Equal(t, "new-value", output.NewValue)
	assert.False(t, output.Secret, "a plaintext (String) param diff is not secret")
}

// getByRefTyped is getByRef but stamps every entry with a fixed value type, so a
// test can exercise the SecureString-secret path.
func getByRefTyped(values map[string]string, typ domain.ValueType) func(context.Context, string, provider.VersionRef) (*domain.Entry, error) {
	return func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
		id := ref.ID()

		val, ok := values[id]
		if !ok {
			return nil, errUnexpectedCall
		}

		return &domain.Entry{Name: name, Value: val, Type: typ, Version: domain.Version{ID: id}}, nil
	}
}

// TestDiffUseCase_Execute_SecureStringIsSecret pins that a SecureString param
// diff carries Secret=true, so both frontends mask it (#677/#702).
func TestDiffUseCase_Execute_SecureStringIsSecret(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: resolveBySuffix(map[string]string{"#1": "1", "#2": "2"}),
		GetFunc:     getByRefTyped(map[string]string{"1": "old-secret", "2": "new-secret"}, domain.ValueTypeSecret),
	}

	uc := &param.DiffUseCase{Reader: store}

	name1, suffix1 := "/app/config", "#1"
	name2, suffix2 := "/app/config", "#2"

	output, err := uc.Execute(t.Context(), param.DiffInput{Name1: name1, Suffix1: suffix1, Name2: name2, Suffix2: suffix2})
	require.NoError(t, err)
	assert.True(t, output.Secret, "a SecureString param diff must be flagged secret")
}

func TestDiffUseCase_Execute_Spec1Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errGetParameter
		},
	}

	uc := &param.DiffUseCase{Reader: store}

	name1, suffix1 := "/app/config", "#1"
	name2, suffix2 := "/app/config", "#2"

	_, err := uc.Execute(t.Context(), param.DiffInput{Name1: name1, Suffix1: suffix1, Name2: name2, Suffix2: suffix2})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_Spec2Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: resolveBySuffix(map[string]string{"#1": "1", "#2": "2"}),
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			if ref.ID() == "2" {
				return nil, errGetParameter
			}

			return &domain.Entry{Name: name, Value: "old-value", Version: domain.Version{ID: "1"}}, nil
		},
	}

	uc := &param.DiffUseCase{Reader: store}

	name1, suffix1 := "/app/config", "#1"
	name2, suffix2 := "/app/config", "#2"

	_, err := uc.Execute(t.Context(), param.DiffInput{Name1: name1, Suffix1: suffix1, Name2: name2, Suffix2: suffix2})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_WithLatest(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: resolveBySuffix(map[string]string{"#3": "3", "": ""}),
		GetFunc:     getByRef(map[string]string{"3": "old-value", "": "latest-value"}),
	}

	uc := &param.DiffUseCase{Reader: store}

	name1, suffix1 := "/app/config", "#3"
	name2, suffix2 := "/app/config", ""

	output, err := uc.Execute(t.Context(), param.DiffInput{Name1: name1, Suffix1: suffix1, Name2: name2, Suffix2: suffix2})
	require.NoError(t, err)
	assert.Equal(t, "3", output.OldVersion)
	// Latest ref has an empty id; version renders as 0.
	assert.Equal(t, "latest-value", output.NewValue)
}

func TestDiffUseCase_Execute_WithShift(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: resolveBySuffix(map[string]string{"~2": "1", "~1": "2"}),
		GetFunc:     getByRef(map[string]string{"1": "v1", "2": "v2"}),
	}

	uc := &param.DiffUseCase{Reader: store}

	name1, suffix1 := "/app/config", "~2" // 2 versions back from latest (v3 -> v1)
	name2, suffix2 := "/app/config", "~1" // 1 version back from latest (v3 -> v2)

	output, err := uc.Execute(t.Context(), param.DiffInput{Name1: name1, Suffix1: suffix1, Name2: name2, Suffix2: suffix2})
	require.NoError(t, err)
	assert.Equal(t, "1", output.OldVersion)
	assert.Equal(t, "v1", output.OldValue)
	assert.Equal(t, "2", output.NewVersion)
	assert.Equal(t, "v2", output.NewValue)
}

func TestDiffUseCase_Execute_WithShift_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, errHistoryFailed
		},
	}

	uc := &param.DiffUseCase{Reader: store}

	name1, suffix1 := "/app/config", "~1"
	name2, suffix2 := "/app/config", ""

	_, err := uc.Execute(t.Context(), param.DiffInput{Name1: name1, Suffix1: suffix1, Name2: name2, Suffix2: suffix2})
	assert.Error(t, err)
}

// TestDiffUseCase_Execute_DistinctNames verifies each side is resolved under its
// own name, so an unversioned store (Azure App Configuration) can compare two
// distinct keys.
func TestDiffUseCase_Execute_DistinctNames(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, name, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec)

			return provider.NewVersionRef(name), nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "val-" + name, Type: domain.ValueTypePlaintext}, nil
		},
	}

	uc := &param.DiffUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.DiffInput{Name1: "key-a", Name2: "key-b"})
	require.NoError(t, err)
	assert.Equal(t, "key-a", output.OldName)
	assert.Equal(t, "val-key-a", output.OldValue)
	assert.Equal(t, "key-b", output.NewName)
	assert.Equal(t, "val-key-b", output.NewValue)
	assert.Empty(t, output.OldVersion, "an unversioned store yields no version id")
	assert.False(t, output.Secret)
}
