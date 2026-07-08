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
