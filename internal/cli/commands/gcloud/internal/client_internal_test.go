package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
)

func TestStagingScopeResolver(t *testing.T) {
	t.Parallel()

	_, err := StagingScopeResolver(t.Context())
	require.ErrorContains(t, err, "--project")

	got, err := StagingScopeResolver(WithProject(t.Context(), "proj"))
	require.NoError(t, err)
	assert.Equal(t, staging.ResolvedScope{Scope: provider.GoogleCloudScope("proj"), Target: provider.GoogleCloudScope("proj").Target()}, got)
}

func TestConfirmTarget(t *testing.T) {
	t.Parallel()

	assert.Empty(t, ConfirmTarget(t.Context()))
	assert.Equal(t, "project proj", ConfirmTarget(WithProject(t.Context(), "proj")))
}
