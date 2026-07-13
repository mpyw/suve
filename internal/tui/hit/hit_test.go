package hit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/hit"
)

// TestAtResolvesRegionAndOffset pins that At returns the ID of the region under a
// point and the point's offset from the region's top-left.
func TestAtResolvesRegionAndOffset(t *testing.T) {
	t.Parallel()

	m := hit.New(
		hit.Region("list", 1, 2, 10, 5),
		hit.Region("history", 20, 8, 30, 4),
	)

	id, dx, dy, ok := m.At(3, 4)
	assert.True(t, ok, "a point inside the list region hits")
	assert.Equal(t, "list", id)
	assert.Equal(t, 2, dx, "dx is the column offset from the region's left")
	assert.Equal(t, 2, dy, "dy is the row offset from the region's top")

	id, _, _, ok = m.At(0, 0)
	assert.False(t, ok, "a point outside every region does not hit")
	assert.Empty(t, id, "a miss returns no ID")
}

// TestAtPrefersHigherZ pins that an overlapping higher-Z region wins the hit, so
// a sub-region placed above its container is resolved first.
func TestAtPrefersHigherZ(t *testing.T) {
	t.Parallel()

	m := hit.New(
		hit.Region("pane", 0, 0, 40, 20),
		hit.Region("band", 0, 10, 40, 4).Z(1),
	)

	id, _, _, ok := m.At(5, 11)
	require.True(t, ok)
	assert.Equal(t, "band", id, "the higher-Z band wins over the pane it sits in")

	id, _, _, ok = m.At(5, 3)
	require.True(t, ok)
	assert.Equal(t, "pane", id, "outside the band, the pane is hit")
}

// TestOriginReturnsLayoutCoordinate pins that Origin returns a region's drawn
// top-left, so tests derive click coordinates from the layout.
func TestOriginReturnsLayoutCoordinate(t *testing.T) {
	t.Parallel()

	m := hit.New(hit.Region("btn", 7, 9, 5, 1))

	x, y, ok := m.Origin("btn")
	assert.True(t, ok)
	assert.Equal(t, 7, x)
	assert.Equal(t, 9, y)

	_, _, ok = m.Origin("missing")
	assert.False(t, ok)
}

// TestNilMapNeverHits pins that a nil Map is safe: it hits nothing and has no
// origins, so a page that has not rendered yet needs no guard.
func TestNilMapNeverHits(t *testing.T) {
	t.Parallel()

	var m *hit.Map

	id, _, _, ok := m.At(1, 1)
	assert.False(t, ok)
	assert.Empty(t, id)

	_, _, ok = m.Origin("x")
	assert.False(t, ok)
}
