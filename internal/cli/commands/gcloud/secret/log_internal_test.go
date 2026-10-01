// White-box tests of log.go.
//declscope:namespace log

package secret

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
)

func TestLogPresenter(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "2", State: "enabled", Created: &created},
				{ID: "1", State: "destroyed", Created: &created},
			}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			// getValue resolves "#<version>".
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			if ref.ID() == "1" {
				// Destroyed version value is inaccessible.
				return nil, errors.New("cannot access destroyed version")
			}

			return &domain.Entry{Value: "v" + ref.ID()}, nil
		},
	}

	presenter := newLogPresenter(store, generic.LogRequest{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))
	assert.Equal(t, 2, presenter.Len())

	var buf bytes.Buffer

	presenter.RenderHeader(&buf, 0)
	out := buf.String()
	assert.Contains(t, out, "Version 2")
	assert.Contains(t, out, "enabled")

	// RenderValue is a no-op for Google Cloud log (no default value preview).
	var valueBuf bytes.Buffer
	presenter.RenderValue(&valueBuf, 0, 0)
	assert.Empty(t, valueBuf.String())

	// RenderOneline emits a compact per-version line with the state tag.
	var onelineBuf bytes.Buffer
	presenter.RenderOneline(&onelineBuf, 0, 0)
	oneline := onelineBuf.String()
	assert.Contains(t, oneline, "2")
	assert.Contains(t, oneline, "enabled")

	// RenderJSON serializes every version; the destroyed version surfaces its
	// fetch error instead of a value.
	var jsonBuf bytes.Buffer
	require.NoError(t, presenter.RenderJSON(&jsonBuf))

	var items []struct {
		Version string  `json:"version"`
		State   string  `json:"state"`
		Created string  `json:"created"`
		Value   *string `json:"value"`
		Error   string  `json:"error"`
	}
	require.NoError(t, json.Unmarshal(jsonBuf.Bytes(), &items))
	require.Len(t, items, 2)
	assert.Equal(t, "2", items[0].Version)
	assert.Equal(t, "enabled", items[0].State)
	require.NotNil(t, items[0].Value)
	assert.Equal(t, "v2", *items[0].Value)
	assert.Empty(t, items[0].Error)
	assert.Equal(t, "1", items[1].Version)
	assert.Equal(t, "destroyed", items[1].State)
	assert.Nil(t, items[1].Value)
	assert.Contains(t, items[1].Error, "cannot access destroyed version")

	// RenderPatch skips versions whose value is inaccessible: i=0 (v2) has a
	// destroyed parent (v1), and i=1 (v1) is itself destroyed. Both branches
	// return without emitting a diff.
	var patchBuf, patchErrBuf bytes.Buffer
	presenter.RenderPatch(&patchBuf, &patchErrBuf, 0, false, false)
	presenter.RenderPatch(&patchBuf, &patchErrBuf, 1, false, false)
	assert.Empty(t, patchBuf.String())
}

func TestLogPresenter_Patch(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "2", State: "enabled", Created: &created},
				{ID: "1", State: "enabled", Created: &created},
			}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "v" + ref.ID()}, nil
		},
	}

	presenter := newLogPresenter(store, generic.LogRequest{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf, errBuf bytes.Buffer

	// i=0 is the newest version (v2): patch against its parent v1.
	presenter.RenderPatch(&buf, &errBuf, 0, false, false)
	// i=1 is the oldest/initial version (v1): all-added creation diff.
	presenter.RenderPatch(&buf, &errBuf, 1, false, false)

	out := buf.String()
	assert.Contains(t, out, "-v1")
	assert.Contains(t, out, "+v2")
	assert.Contains(t, out, "my-secret#1")
	assert.Contains(t, out, "my-secret#2")
	// The initial version renders its all-added creation diff (+v1).
	assert.Contains(t, out, "+v1")
}
