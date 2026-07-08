package secret_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

// logStore builds a mock reader whose History returns the given versions
// (as-is) and whose Get returns per-version-id values/errors.
func logStore(versions []domain.Version, values map[string]string, errs map[string]error) *providermock.Store {
	return &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return versions, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			id := ref.ID()
			if errs != nil {
				if err, ok := errs[id]; ok {
					return nil, err
				}
			}

			return &domain.Entry{Value: values[id], Version: domain.Version{ID: id}}, nil
		},
	}
}

func TestLogUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		// Unsorted, multi-valued staging labels: all must surface, sorted (#419).
		{ID: "v3", StagingLabels: []string{"custom", "AWSCURRENT"}, Created: &now},
		{ID: "v2", StagingLabels: []string{"AWSPREVIOUS"}, Created: tp(now, -time.Hour)},
		{ID: "v1", Created: tp(now, -2*time.Hour)},
	}
	values := map[string]string{"v1": "value1", "v2": "value2", "v3": "value3"}

	uc := &secret.LogUseCase{Reader: logStore(versions, values, nil)}

	output, err := uc.Execute(t.Context(), secret.LogInput{Name: "my-secret", MaxResults: 10})
	require.NoError(t, err)
	require.Len(t, output.Entries, 3)
	// Newest first.
	assert.Equal(t, "v3", output.Entries[0].VersionID)
	// IsCurrent is a membership test: AWSCURRENT alongside a custom label.
	assert.True(t, output.Entries[0].IsCurrent)
	assert.Equal(t, []string{"AWSCURRENT", "custom"}, output.Entries[0].VersionStage)
	// AWS Secrets Manager carries staging labels, not a per-version state (#419).
	assert.Empty(t, output.Entries[0].State)
	assert.Equal(t, "value3", output.Entries[0].Value)
	assert.False(t, output.Entries[2].IsCurrent)
}

// TestLogUseCase_Execute_State asserts that a GCloud/Key Vault-style history
// (per-version State set, no staging labels) surfaces State on each entry and
// leaves VersionStage empty — the two concepts must not be conflated (#419).
func TestLogUseCase_Execute_State(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "2", State: "enabled", Created: &now},
		{ID: "1", State: "disabled", Created: tp(now, -time.Hour)},
	}
	values := map[string]string{"1": "value1", "2": "value2"}

	uc := &secret.LogUseCase{Reader: logStore(versions, values, nil)}

	output, err := uc.Execute(t.Context(), secret.LogInput{Name: "my-secret", MaxResults: 10})
	require.NoError(t, err)
	require.Len(t, output.Entries, 2)
	assert.Equal(t, "enabled", output.Entries[0].State)
	assert.Empty(t, output.Entries[0].VersionStage)
	assert.Equal(t, "disabled", output.Entries[1].State)
	assert.Empty(t, output.Entries[1].VersionStage)
}

func TestLogUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	uc := &secret.LogUseCase{Reader: logStore(nil, nil, nil)}

	output, err := uc.Execute(t.Context(), secret.LogInput{Name: "my-secret", MaxResults: 10})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestLogUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return nil, errors.New("aws error")
		},
	}

	uc := &secret.LogUseCase{Reader: store}

	_, err := uc.Execute(t.Context(), secret.LogInput{Name: "my-secret", MaxResults: 10})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secret versions")
}

func TestLogUseCase_Execute_Reverse(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "v2", StagingLabels: []string{"AWSCURRENT"}, Created: &now},
		{ID: "v1", StagingLabels: []string{"AWSPREVIOUS"}, Created: tp(now, -time.Hour)},
	}

	uc := &secret.LogUseCase{Reader: logStore(versions, map[string]string{}, nil)}

	output, err := uc.Execute(t.Context(), secret.LogInput{Name: "my-secret", MaxResults: 10, Reverse: true})
	require.NoError(t, err)
	require.Len(t, output.Entries, 2)
	assert.Equal(t, "v1", output.Entries[0].VersionID)
	assert.Equal(t, "v2", output.Entries[1].VersionID)
}

// TestLogUseCase_Execute_MaxResults caps to the newest N versions.
func TestLogUseCase_Execute_MaxResults(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "v3", Created: &now},
		{ID: "v2", Created: tp(now, -time.Hour)},
		{ID: "v1", Created: tp(now, -2*time.Hour)},
	}

	uc := &secret.LogUseCase{Reader: logStore(versions, map[string]string{}, nil)}

	output, err := uc.Execute(t.Context(), secret.LogInput{Name: "my-secret", MaxResults: 2})
	require.NoError(t, err)
	require.Len(t, output.Entries, 2)
	assert.Equal(t, "v3", output.Entries[0].VersionID)
	assert.Equal(t, "v2", output.Entries[1].VersionID)
}

func TestLogUseCase_Execute_SinceUntil(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "newest", Created: &now},
		{ID: "middle", Created: tp(now, -time.Hour)},
		{ID: "oldest", Created: tp(now, -3*time.Hour)},
	}

	uc := &secret.LogUseCase{Reader: logStore(versions, map[string]string{}, nil)}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name:       "my-secret",
		MaxResults: 10,
		Since:      tp(now, -90*time.Minute),
		Until:      tp(now, -30*time.Minute),
	})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)
	assert.Equal(t, "middle", output.Entries[0].VersionID)
}

// TestLogUseCase_Execute_FilterBeforeCount asserts date filters run BEFORE the
// count cap: -n must yield up to N versions that match the filter, not N newest
// then filtered down to fewer (#351). Here only the two oldest versions match
// --until, and the old count-first order would have truncated them all away.
func TestLogUseCase_Execute_FilterBeforeCount(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "newest", Created: &now},
		{ID: "middle", Created: tp(now, -2*time.Hour)},
		{ID: "oldest", Created: tp(now, -3*time.Hour)},
	}

	uc := &secret.LogUseCase{Reader: logStore(versions, map[string]string{}, nil)}

	output, err := uc.Execute(t.Context(), secret.LogInput{
		Name:       "my-secret",
		MaxResults: 1,
		Until:      tp(now, -time.Hour),
	})
	require.NoError(t, err)
	require.Len(t, output.Entries, 1)
	assert.Equal(t, "middle", output.Entries[0].VersionID)
}

// TestLogUseCase_Execute_PartialFetchError records a per-version fetch failure
// on the entry rather than aborting the whole listing.
func TestLogUseCase_Execute_PartialFetchError(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "v2", Created: &now},
		{ID: "v1", Created: tp(now, -time.Hour)},
	}

	uc := &secret.LogUseCase{Reader: logStore(
		versions,
		map[string]string{"v2": "value2"},
		map[string]error{"v1": errors.New("access denied")},
	)}

	output, err := uc.Execute(t.Context(), secret.LogInput{Name: "my-secret", MaxResults: 10})
	require.NoError(t, err)
	require.Len(t, output.Entries, 2)
	require.NoError(t, output.Entries[0].Error)
	assert.Equal(t, "value2", output.Entries[0].Value)
	require.Error(t, output.Entries[1].Error)
}

// lo returns a pointer to now+delta, a small local time-pointer helper.
func tp(now time.Time, delta time.Duration) *time.Time {
	t := now.Add(delta)

	return &t
}
