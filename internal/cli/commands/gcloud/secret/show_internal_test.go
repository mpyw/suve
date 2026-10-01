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
				Name:        name,
				Value:       "s3cr3t",
				Type:        domain.ValueTypeSecret,
				Version:     domain.Version{ID: "3", State: "enabled", Created: &created},
				Description: "app credentials",
				Tags:        []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := version.GoogleCloudSecretManager.Parse("my-secret")
	require.NoError(t, err)

	presenter := newShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)
	presenter.RenderText(&buf, value)

	out := buf.String()
	assert.Contains(t, out, "my-secret")
	assert.Contains(t, out, "Version")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "s3cr3t")
	assert.Contains(t, out, "env")
	// The "description" annotation surfaces as a Description field in the read view.
	assert.Contains(t, out, "Description")
	assert.Contains(t, out, "app credentials")
	// No ARN or Stages fields for Google Cloud.
	assert.NotContains(t, out, "ARN")
	assert.NotContains(t, out, "Stages")

	// RenderJSON emits the structured view over the same fetched entry.
	var jsonBuf bytes.Buffer
	require.NoError(t, presenter.RenderJSON(&jsonBuf, value))

	var showOut struct {
		Name        string            `json:"name"`
		Version     string            `json:"version"`
		State       string            `json:"state"`
		Description string            `json:"description"`
		Created     string            `json:"created"`
		Labels      map[string]string `json:"labels"`
		Value       string            `json:"value"`
	}
	require.NoError(t, json.Unmarshal(jsonBuf.Bytes(), &showOut))
	assert.Equal(t, "my-secret", showOut.Name)
	assert.Equal(t, "3", showOut.Version)
	assert.Equal(t, "enabled", showOut.State)
	assert.Equal(t, "app credentials", showOut.Description)
	assert.Equal(t, "s3cr3t", showOut.Value)
	assert.Equal(t, map[string]string{"env": "prod"}, showOut.Labels)
	assert.NotEmpty(t, showOut.Created)
}
