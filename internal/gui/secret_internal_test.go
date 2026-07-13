//go:build production || dev

package gui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
)

// TestSecretCreate_ThreadsDescription asserts the GUI SecretCreate binding
// forwards the description argument into the create use case, so a description
// set in the GUI form reaches the provider writer (#767).
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretCreate_ThreadsDescription(t *testing.T) {
	var gotDescription string

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			gotDescription = description

			return domain.Version{ID: "v1"}, nil
		},
	}

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(provider.ProviderAWS, fakeFactory{store: store})

	t.Cleanup(func() { registry = orig })

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	_, err := app.SecretCreate("my-secret", "v", "app credentials")
	require.NoError(t, err)
	assert.Equal(t, "app credentials", gotDescription)
}

// TestSecretUpdate_ThreadsDescription asserts the GUI SecretUpdate binding
// forwards the description into the update use case's Put (#767).
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretUpdate_ThreadsDescription(t *testing.T) {
	var gotDescription string

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "old", Type: domain.ValueTypeSecret}, nil
		},
		PutFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			gotDescription = description

			return domain.Version{ID: "v2"}, nil
		},
	}

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(provider.ProviderAWS, fakeFactory{store: store})

	t.Cleanup(func() { registry = orig })

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	_, err := app.SecretUpdate("my-secret", "v2", "rotated key")
	require.NoError(t, err)
	assert.Equal(t, "rotated key", gotDescription)
}

// TestSecretCreate_DropsDescriptionForAzure asserts the server-side guard: Azure
// Key Vault ignores descriptions, so the binding drops any value even if the
// frontend (which hides the input) is bypassed — defense in depth (#767).
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretCreate_DropsDescriptionForAzure(t *testing.T) {
	var gotDescription string

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			gotDescription = description

			return domain.Version{ID: "v1"}, nil
		},
	}

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(provider.ProviderAzure, fakeFactory{store: store})

	t.Cleanup(func() { registry = orig })

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAzure, VaultName: "vault"}}

	_, err := app.SecretCreate("kv-secret", "v", "should-be-dropped")
	require.NoError(t, err)
	assert.Empty(t, gotDescription, "Azure Key Vault must not receive a description")
}
