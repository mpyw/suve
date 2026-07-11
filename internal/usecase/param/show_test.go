package param_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/awsparamversion"
)

func TestShowUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, name, spec string) (provider.VersionRef, error) {
			assert.Equal(t, "/app/config", name)
			assert.Empty(t, spec) // latest: no suffix

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:     "/app/config",
				Value:    "secret-value",
				Version:  domain.Version{ID: "5"},
				Type:     domain.ValueTypeSecret,
				Modified: &now,
			}, nil
		},
	}

	uc := &param.ShowUseCase{Reader: store}

	spec, err := awsparamversion.Parse("/app/config")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{Spec: spec})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, "secret-value", output.Value)
	assert.Equal(t, int64(5), output.Version)
	assert.Equal(t, domain.ValueTypeSecret, output.Type)
	assert.NotNil(t, output.LastModified)
}

func TestShowUseCase_Execute_WithVersion(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Equal(t, "#3", spec)

			return provider.NewVersionRef("3"), nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    "/app/config",
				Value:   "old-value",
				Version: domain.Version{ID: "3"},
				Type:    domain.ValueTypePlaintext,
			}, nil
		},
	}

	uc := &param.ShowUseCase{Reader: store}

	spec, err := awsparamversion.Parse("/app/config#3")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{Spec: spec})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, "old-value", output.Value)
	assert.Equal(t, int64(3), output.Version)
}

func TestShowUseCase_Execute_WithShift(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Equal(t, "~1", spec)

			return provider.NewVersionRef("2"), nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    "/app/config",
				Value:   "v2",
				Version: domain.Version{ID: "2"},
				Type:    domain.ValueTypePlaintext,
			}, nil
		},
	}

	uc := &param.ShowUseCase{Reader: store}

	spec, err := awsparamversion.Parse("/app/config~1")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{Spec: spec})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, "v2", output.Value)
	assert.Equal(t, int64(2), output.Version)
}

func TestShowUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, errAWS
		},
	}

	uc := &param.ShowUseCase{Reader: store}

	spec, err := awsparamversion.Parse("/app/config")
	require.NoError(t, err)

	_, err = uc.Execute(t.Context(), param.ShowInput{Spec: spec})
	require.Error(t, err)
}

func TestShowUseCase_Execute_ResolveError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, errAWS
		},
	}

	uc := &param.ShowUseCase{Reader: store}

	spec, err := awsparamversion.Parse("/app/config")
	require.NoError(t, err)

	_, err = uc.Execute(t.Context(), param.ShowInput{Spec: spec})
	require.Error(t, err)
}

func TestShowUseCase_Execute_NoLastModified(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    "/app/config",
				Value:   "value",
				Version: domain.Version{ID: "1"},
				Type:    domain.ValueTypePlaintext,
			}, nil
		},
	}

	uc := &param.ShowUseCase{Reader: store}

	spec, err := awsparamversion.Parse("/app/config")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{Spec: spec})
	require.NoError(t, err)
	assert.Nil(t, output.LastModified)
}

func TestShowUseCase_Execute_WithTags(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    "/app/config",
				Value:   "value",
				Version: domain.Version{ID: "1"},
				Type:    domain.ValueTypePlaintext,
				Tags: []domain.Tag{
					{Key: "env", Value: "prod"},
					{Key: "team", Value: "backend"},
				},
			}, nil
		},
	}

	uc := &param.ShowUseCase{Reader: store}

	spec, err := awsparamversion.Parse("/app/config")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{Spec: spec})
	require.NoError(t, err)
	assert.Len(t, output.Tags, 2)
	assert.Equal(t, "env", output.Tags[0].Key)
	assert.Equal(t, "prod", output.Tags[0].Value)
	assert.Equal(t, "team", output.Tags[1].Key)
	assert.Equal(t, "backend", output.Tags[1].Value)
}
