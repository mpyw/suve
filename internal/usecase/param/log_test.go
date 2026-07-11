package param_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

// logVer describes a single version for the log-store helper.
type logVer struct {
	ver      int64
	value    string
	typ      domain.ValueType
	modified *time.Time
	getErr   error // when set, Get fails for this version
}

// newLogStore builds a provider mock whose History returns the given versions
// newest-first (input is oldest-first) and whose Resolve/Get fetch a version's
// value/type by id, mirroring the AWS param adapter.
func newLogStore(oldestFirst []logVer) *providermock.Store {
	byID := make(map[string]logVer, len(oldestFirst))

	versionsNewestFirst := make([]domain.Version, 0, len(oldestFirst))

	for _, v := range slices.Backward(oldestFirst) {
		id := strconv.FormatInt(v.ver, 10)
		byID[id] = v
		versionsNewestFirst = append(versionsNewestFirst, domain.Version{ID: id, Created: v.modified})
	}

	return &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return versionsNewestFirst, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			v := byID[ref.ID()]

			if v.getErr != nil {
				return nil, v.getErr
			}

			return &domain.Entry{
				Name:     name,
				Value:    v.value,
				Type:     v.typ,
				Version:  domain.Version{ID: ref.ID(), Created: v.modified},
				Modified: v.modified,
			}, nil
		},
	}
}

func TestLogUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := newLogStore([]logVer{
		{ver: 1, value: "v1", typ: domain.ValueTypePlaintext, modified: lo.ToPtr(now.Add(-2 * time.Hour))},
		{ver: 2, value: "v2", typ: domain.ValueTypePlaintext, modified: lo.ToPtr(now.Add(-1 * time.Hour))},
		{ver: 3, value: "v3", typ: domain.ValueTypePlaintext, modified: lo.ToPtr(now)},
	})

	uc := &param.LogUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Len(t, output.Entries, 3)

	// Newest first (default order)
	assert.Equal(t, int64(3), output.Entries[0].Version)
	assert.Equal(t, int64(2), output.Entries[1].Version)
	assert.Equal(t, int64(1), output.Entries[2].Version)

	// IsCurrent flag
	assert.True(t, output.Entries[0].IsCurrent)
	assert.False(t, output.Entries[1].IsCurrent)
	assert.False(t, output.Entries[2].IsCurrent)
}

func TestLogUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	store := newLogStore(nil)

	uc := &param.LogUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Empty(t, output.Entries)
}

func TestLogUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return nil, errHistoryFailed
		},
	}

	uc := &param.LogUseCase{Reader: store}

	_, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config"})
	require.Error(t, err)
}

// TestLogUseCase_Execute_PartialFetchError records a per-version fetch failure
// on the entry rather than aborting the whole listing.
func TestLogUseCase_Execute_PartialFetchError(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := newLogStore([]logVer{
		{ver: 1, value: "v1", getErr: errAccessDenied, modified: lo.ToPtr(now.Add(-1 * time.Hour))},
		{ver: 2, value: "v2", modified: lo.ToPtr(now)},
	})

	uc := &param.LogUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config"})
	require.NoError(t, err)
	require.Len(t, output.Entries, 2)

	// Newest first: v2 succeeds, v1 records its fetch error.
	require.NoError(t, output.Entries[0].Error)
	assert.Equal(t, "v2", output.Entries[0].Value)
	require.Error(t, output.Entries[1].Error)
	assert.Empty(t, output.Entries[1].Value)
	// IsCurrent stays correct even for a failed entry (derived from version number).
	assert.True(t, output.Entries[0].IsCurrent)
	assert.False(t, output.Entries[1].IsCurrent)
}

func TestLogUseCase_Execute_Reverse(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := newLogStore([]logVer{
		{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-2 * time.Hour))},
		{ver: 2, value: "v2", modified: lo.ToPtr(now.Add(-1 * time.Hour))},
		{ver: 3, value: "v3", modified: lo.ToPtr(now)},
	})

	uc := &param.LogUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config", Reverse: true})
	require.NoError(t, err)

	// Oldest first when Reverse is true
	assert.Equal(t, int64(1), output.Entries[0].Version)
	assert.Equal(t, int64(2), output.Entries[1].Version)
	assert.Equal(t, int64(3), output.Entries[2].Version)
}

func TestLogUseCase_Execute_SinceFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := newLogStore([]logVer{
		{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-3 * time.Hour))},
		{ver: 2, value: "v2", modified: lo.ToPtr(now.Add(-1 * time.Hour))},
		{ver: 3, value: "v3", modified: lo.ToPtr(now)},
	})

	uc := &param.LogUseCase{Reader: store}

	since := now.Add(-2 * time.Hour)
	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config", Since: &since})
	require.NoError(t, err)

	assert.Len(t, output.Entries, 2)
	assert.Equal(t, int64(3), output.Entries[0].Version)
	assert.Equal(t, int64(2), output.Entries[1].Version)
}

func TestLogUseCase_Execute_UntilFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := newLogStore([]logVer{
		{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-3 * time.Hour))},
		{ver: 2, value: "v2", modified: lo.ToPtr(now.Add(-1 * time.Hour))},
		{ver: 3, value: "v3", modified: lo.ToPtr(now)},
	})

	uc := &param.LogUseCase{Reader: store}

	until := now.Add(-30 * time.Minute)
	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config", Until: &until})
	require.NoError(t, err)

	assert.Len(t, output.Entries, 2)
	assert.Equal(t, int64(2), output.Entries[0].Version)
	assert.Equal(t, int64(1), output.Entries[1].Version)
}

func TestLogUseCase_Execute_DateRangeFilter(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := newLogStore([]logVer{
		{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-4 * time.Hour))},
		{ver: 2, value: "v2", modified: lo.ToPtr(now.Add(-2 * time.Hour))},
		{ver: 3, value: "v3", modified: lo.ToPtr(now)},
	})

	uc := &param.LogUseCase{Reader: store}

	since := now.Add(-3 * time.Hour)
	until := now.Add(-1 * time.Hour)
	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config", Since: &since, Until: &until})
	require.NoError(t, err)

	assert.Len(t, output.Entries, 1)
	assert.Equal(t, int64(2), output.Entries[0].Version)
}

// TestLogUseCase_Execute_FilterBeforeCount asserts date filters run BEFORE the
// count cap: -n must yield up to N versions that match the filter, not N newest
// then filtered down to fewer (#351). Here only the two oldest versions match
// --until, and the old count-first order would have truncated them all away.
func TestLogUseCase_Execute_FilterBeforeCount(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := newLogStore([]logVer{
		{ver: 1, value: "v1", modified: lo.ToPtr(now.Add(-3 * time.Hour))},
		{ver: 2, value: "v2", modified: lo.ToPtr(now.Add(-2 * time.Hour))},
		{ver: 3, value: "v3", modified: lo.ToPtr(now)},
	})

	uc := &param.LogUseCase{Reader: store}

	until := now.Add(-1 * time.Hour)
	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config", MaxResults: 1, Until: &until})
	require.NoError(t, err)

	// Only v1 and v2 predate --until; capping to 1 yields the newest of those (v2),
	// not an empty result from truncating to v3 first.
	require.Len(t, output.Entries, 1)
	assert.Equal(t, int64(2), output.Entries[0].Version)
}

func TestLogUseCase_Execute_NoLastModifiedDate(t *testing.T) {
	t.Parallel()

	store := newLogStore([]logVer{
		{ver: 1, value: "v1", modified: nil},
	})

	uc := &param.LogUseCase{Reader: store}

	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Nil(t, output.Entries[0].LastModified)
}

func TestLogUseCase_Execute_FilterWithNilLastModifiedDate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := newLogStore([]logVer{
		{ver: 1, value: "v1", modified: nil},
		{ver: 2, value: "v2", modified: lo.ToPtr(now)},
	})

	uc := &param.LogUseCase{Reader: store}

	since := now.Add(-1 * time.Hour)
	output, err := uc.Execute(t.Context(), param.LogInput{Name: "/app/config", Since: &since})
	require.NoError(t, err)

	// v1 has nil timestamp, so it is skipped when a date filter is applied; only v2 remains.
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, int64(2), output.Entries[0].Version)
}
