package provider_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/provider"
)

func TestVersionRef_ZeroValueIsLatest(t *testing.T) {
	t.Parallel()

	var ref provider.VersionRef

	assert.True(t, ref.IsLatest())
	assert.Empty(t, ref.ID())
}

func TestVersionRef_NewVersionRef(t *testing.T) {
	t.Parallel()

	ref := provider.NewVersionRef("abc123")

	assert.False(t, ref.IsLatest())
	assert.Equal(t, "abc123", ref.ID())
}

func TestNewVersionRef_EmptyIDIsLatest(t *testing.T) {
	t.Parallel()

	ref := provider.NewVersionRef("")

	assert.True(t, ref.IsLatest())
	assert.Empty(t, ref.ID())
}
