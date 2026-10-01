// White-box tests of show.go.
//declscope:namespace show

package secret

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/version"
)

func TestShowPresenter(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec)

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   "s3cr3t",
				Type:    domain.ValueTypeSecret,
				Version: domain.Version{ID: "abc123", State: "enabled", Created: &created},
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := version.AzureKeyVault.Parse("my-secret")
	require.NoError(t, err)

	presenter := newShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	presenter.RenderText(&buf, value)

	out := buf.String()
	assert.Contains(t, out, "my-secret")
	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "s3cr3t")
	assert.Contains(t, out, "env")
}

// TestShowPresenter_RenderJSON covers the Key Vault show presenter's JSON path:
// name/version/state/created plus the tags map and value.
func TestShowPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec)

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   "s3cr3t",
				Type:    domain.ValueTypeSecret,
				Version: domain.Version{ID: "abc123", State: "enabled", Created: &created},
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := version.AzureKeyVault.Parse("my-secret")
	require.NoError(t, err)

	presenter := newShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	require.NoError(t, presenter.RenderJSON(&buf, value))

	var out struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		State   string            `json:"state"`
		Created string            `json:"created"`
		Tags    map[string]string `json:"tags"`
		Value   string            `json:"value"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "my-secret", out.Name)
	assert.Equal(t, "abc123", out.Version)
	assert.Equal(t, "enabled", out.State)
	assert.Equal(t, timeutil.FormatRFC3339(created), out.Created)
	assert.Equal(t, "s3cr3t", out.Value)
	assert.Equal(t, map[string]string{"env": "prod"}, out.Tags)
}
