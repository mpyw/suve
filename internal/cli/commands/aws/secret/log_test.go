package secret_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awssecret "github.com/mpyw/suve/internal/cli/commands/aws/secret"
	genericlog "github.com/mpyw/suve/internal/cli/commands/generic/log"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
)

// logStore builds a mock reader over a two-version secret history, mapping each
// resolved "#<id>" back to its stored value.
func logStore() *providermock.Store {
	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	return &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "new-version-id-long", StagingLabels: []string{"AWSCURRENT"}, Created: &created},
				{ID: "old-version-id-long", StagingLabels: []string{"AWSPREVIOUS"}, Created: &created},
			}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Value: "value-" + ref.ID()}, nil
		},
	}
}

func runLog(
	t *testing.T, presenter genericlog.Presenter, opts genericlog.Options,
) string {
	t.Helper()

	var stdout, stderr bytes.Buffer

	r := &genericlog.Runner{Presenter: presenter, Options: opts, Stdout: &stdout, Stderr: &stderr}
	require.NoError(t, r.Run(t.Context()))

	return stdout.String()
}

// TestLogPresenter_RenderValueNoop drives the default (non-patch) log render,
// which calls RenderHeader followed by RenderValue for each version. Secrets
// Manager's RenderValue is a deliberate no-op (no default value preview), so
// the rendered output carries the version headers but never the stored values.
func TestLogPresenter_RenderValueNoop(t *testing.T) {
	t.Parallel()

	presenter := awssecret.NewLogPresenter(logStore(), genericlog.Request{Name: "my-secret"})
	out := runLog(t, presenter, genericlog.Options{})

	// Truncated version IDs appear in the headers.
	assert.Contains(t, out, "Version new-vers")
	assert.Contains(t, out, "AWSCURRENT")
	assert.Contains(t, out, "AWSPREVIOUS")
	// RenderValue is a no-op: the stored values never leak into the log output.
	assert.NotContains(t, out, "value-new-version-id-long")
	assert.NotContains(t, out, "value-old-version-id-long")
}

// TestLogPresenter_Oneline exercises the compact one-line render path.
func TestLogPresenter_Oneline(t *testing.T) {
	t.Parallel()

	presenter := awssecret.NewLogPresenter(logStore(), genericlog.Request{Name: "my-secret"})
	out := runLog(t, presenter, genericlog.Options{Oneline: true})

	assert.Contains(t, out, "AWSCURRENT")
	assert.Contains(t, out, "AWSPREVIOUS")
}

// TestLogPresenter_JSON exercises the JSON render path, asserting per-version
// IDs, stages, and values.
func TestLogPresenter_JSON(t *testing.T) {
	t.Parallel()

	presenter := awssecret.NewLogPresenter(logStore(), genericlog.Request{Name: "my-secret"})
	out := runLog(t, presenter, genericlog.Options{Output: output.FormatJSON})

	var items []struct {
		VersionID string   `json:"versionId"`
		Stages    []string `json:"stages"`
		Value     *string  `json:"value"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &items))
	require.Len(t, items, 2)
	assert.Equal(t, "new-version-id-long", items[0].VersionID)
	assert.Equal(t, []string{"AWSCURRENT"}, items[0].Stages)
	require.NotNil(t, items[0].Value)
	assert.Equal(t, "value-new-version-id-long", *items[0].Value)
}
