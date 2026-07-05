package param_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/azure/param"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/azure"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
)

// TestCommandValidation exercises the acid-test parse rejection: App
// Configuration is unversioned, so any version specifier fails before any store
// is resolved (no Azure credentials needed) — and never panics.
func TestCommandValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "show missing key",
			args:    []string{"suve", "azure", "param", "show"},
			wantErr: "usage:",
		},
		{
			name:    "show rejects version number",
			args:    []string{"suve", "azure", "param", "show", "my-key#1"},
			wantErr: "does not support versions",
		},
		{
			name:    "show rejects shift",
			args:    []string{"suve", "azure", "param", "show", "my-key~1"},
			wantErr: "does not support versions",
		},
		{
			name:    "show rejects label",
			args:    []string{"suve", "azure", "param", "show", "my-key:prod"},
			wantErr: "does not support versions",
		},
		{
			name:    "create missing args",
			args:    []string{"suve", "azure", "param", "create"},
			wantErr: "usage:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appcli.MakeApp()
			err := app.Run(t.Context(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
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
