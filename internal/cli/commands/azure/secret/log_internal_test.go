// White-box tests of log.go.
//declscope:namespace log

package secret

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
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
				{ID: "new", State: "enabled", Created: &created},
				{ID: "old", State: "disabled", Created: &created},
			}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			if ref.ID() == "old" {
				return nil, errors.New("disabled version has no value")
			}

			return &domain.Entry{Value: "v-" + ref.ID()}, nil
		},
	}

	presenter := newLogPresenter(store, generic.LogRequest{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))
	assert.Equal(t, 2, presenter.Len())

	var buf bytes.Buffer

	presenter.RenderHeader(&buf, 0)
	out := buf.String()
	assert.Contains(t, out, "Version new")
	assert.Contains(t, out, "enabled")
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

// TestLogPresenter_RenderJSON covers the JSON path, including both the value
// branch (enabled versions) and the error branch (disabled versions) plus tags.
func TestLogPresenter_RenderJSON(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := logStore([]domain.Version{
		{ID: "new", State: "enabled", Created: &created, Tags: []domain.Tag{{Key: "env", Value: "prod"}}},
		{ID: "old", State: "disabled", Created: &created},
	}, "old")

	presenter := newLogPresenter(store, generic.LogRequest{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf bytes.Buffer
	require.NoError(t, presenter.RenderJSON(&buf))

	var items []struct {
		Version string            `json:"version"`
		State   string            `json:"state"`
		Created string            `json:"created"`
		Value   *string           `json:"value"`
		Tags    map[string]string `json:"tags"`
		Error   string            `json:"error"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 2)

	assert.Equal(t, "new", items[0].Version)
	assert.Equal(t, "enabled", items[0].State)
	require.NotNil(t, items[0].Value)
	assert.Equal(t, "v-new", *items[0].Value)
	assert.Equal(t, map[string]string{"env": "prod"}, items[0].Tags)
	assert.Empty(t, items[0].Error)

	assert.Equal(t, "old", items[1].Version)
	assert.Nil(t, items[1].Value)
	assert.Contains(t, items[1].Error, "disabled version")
}

// TestLogPresenter_RenderOneline covers the compact one-line format, including
// the state annotation and the formatted date.
func TestLogPresenter_RenderOneline(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	store := logStore([]domain.Version{
		{ID: "new", State: "enabled", Created: &created},
	})

	presenter := newLogPresenter(store, generic.LogRequest{Name: "my-secret"})
	require.NoError(t, presenter.Fetch(t.Context()))

	var buf bytes.Buffer
	presenter.RenderOneline(&buf, 0, 0)
	// RenderValue is a documented no-op; call it to confirm it stays silent.
	presenter.RenderValue(&buf, 0, 0)

	out := buf.String()
	assert.Contains(t, out, "new")
	assert.Contains(t, out, "enabled")
	assert.Contains(t, out, "2024-05-06")
}

// TestLogPresenter_PatchSkips covers the three RenderPatch skip branches:
//   - the newest entry is disabled, so its value is missing (log.go:154);
//   - the parent entry is disabled, so the old value is missing (log.go:180);
//   - the oldest shown entry is not the initial version because the window was
//     cut by --number, so no all-added creation diff is emitted (log.go:165).
func TestLogPresenter_PatchSkips(t *testing.T) {
	t.Parallel()

	t.Run("disabled newest and disabled parent are skipped", func(t *testing.T) {
		t.Parallel()

		// History: new (enabled) -> old (disabled). Both branches are exercised:
		// i=0 has a disabled parent (old); i=1 is itself disabled.
		store := logStore([]domain.Version{
			{ID: "new", State: "enabled"},
			{ID: "old", State: "disabled"},
		}, "old")

		presenter := newLogPresenter(store, generic.LogRequest{Name: "my-secret"})
		require.NoError(t, presenter.Fetch(t.Context()))

		var buf, errBuf bytes.Buffer

		presenter.RenderPatch(&buf, &errBuf, 0, false, false) // parent "old" is disabled -> skip
		presenter.RenderPatch(&buf, &errBuf, 1, false, false) // "old" itself is disabled -> skip

		assert.Empty(t, buf.String())
	})

	t.Run("windowed oldest is not the initial version", func(t *testing.T) {
		t.Parallel()

		// Full history has two versions but --number=1 shows only the newest, so
		// the oldest shown version is not the genuine initial: no creation diff.
		store := logStore([]domain.Version{
			{ID: "new", State: "enabled"},
			{ID: "old", State: "enabled"},
		})

		presenter := newLogPresenter(store, generic.LogRequest{Name: "my-secret", MaxResults: 1})
		require.NoError(t, presenter.Fetch(t.Context()))
		require.Equal(t, 1, presenter.Len())

		var buf, errBuf bytes.Buffer
		presenter.RenderPatch(&buf, &errBuf, 0, false, false)

		assert.Empty(t, buf.String())
	})
}

// logStore builds a Key Vault log store from a newest-first version history.
// A version listed in errVersions has its value fetch fail (mirroring a disabled
// version, whose value is inaccessible).
func logStore(history []domain.Version, errVersions ...string) *providermock.Store {
	return &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return history, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			if slices.Contains(errVersions, ref.ID()) {
				return nil, errors.New("disabled version has no accessible value")
			}

			return &domain.Entry{Value: "v-" + ref.ID()}, nil
		},
	}
}
