// White-box tests of show.go.
//declscope:namespace show

package param

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

	spec, err := version.AzureAppConfiguration.Parse("app/timeout")
	require.NoError(t, err)

	presenter := newShowPresenter(store, spec)
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

// TestShowPresenter_RenderJSON covers the App Configuration show presenter's
// JSON path: name/value plus the optional "modified" timestamp and the tags map.
func TestShowPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Empty(t, spec)

			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:     name,
				Value:    "30",
				Type:     domain.ValueTypePlaintext,
				Modified: &modified, // unversioned, but carries a modified time
				Tags:     []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := version.AzureAppConfiguration.Parse("app/timeout")
	require.NoError(t, err)

	presenter := newShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	require.NoError(t, presenter.RenderJSON(&buf, value))

	var out struct {
		Name     string            `json:"name"`
		Modified string            `json:"modified"`
		Tags     map[string]string `json:"tags"`
		Value    string            `json:"value"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "app/timeout", out.Name)
	assert.Equal(t, "30", out.Value)
	assert.Equal(t, timeutil.FormatRFC3339(modified), out.Modified)
	assert.Equal(t, map[string]string{"env": "prod"}, out.Tags)
}
