package secret_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awssecret "github.com/mpyw/suve/internal/cli/commands/aws/secret"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/version/awssecretversion"
)

// TestShowPresenter_RendersDescription guards #753: a secret carrying a
// description must surface it in both the text and JSON show output.
func TestShowPresenter_RendersDescription(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:        name,
				Value:       "s3cr3t",
				Type:        domain.ValueTypeSecret,
				Version:     domain.Version{ID: "v1"},
				Description: "app credentials",
			}, nil
		},
	}

	spec, err := awssecretversion.Parse("my-secret")
	require.NoError(t, err)

	presenter := awssecret.NewShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)

	presenter.RenderText(&buf, value)
	text := buf.String()
	assert.Contains(t, text, "Description")
	assert.Contains(t, text, "app credentials")

	buf.Reset()
	require.NoError(t, presenter.RenderJSON(&buf, value))

	var jsonOut map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &jsonOut))
	assert.Equal(t, "app credentials", jsonOut["description"])
}

// TestShowPresenter_OmitsEmptyDescription guards that a secret without a
// description renders neither the text field nor the JSON key.
func TestShowPresenter_OmitsEmptyDescription(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "s3cr3t", Type: domain.ValueTypeSecret}, nil
		},
	}

	spec, err := awssecretversion.Parse("my-secret")
	require.NoError(t, err)

	presenter := awssecret.NewShowPresenter(store, spec)
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	value := presenter.Value(false, &errBuf)

	presenter.RenderText(&buf, value)
	assert.NotContains(t, buf.String(), "Description")

	buf.Reset()
	require.NoError(t, presenter.RenderJSON(&buf, value))

	var jsonOut map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &jsonOut))
	_, hasDescription := jsonOut["description"]
	assert.False(t, hasDescription)
}
