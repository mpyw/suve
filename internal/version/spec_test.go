package version_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/version"
)

func TestSpec_HasShift(t *testing.T) {
	t.Parallel()

	t.Run("no shift", func(t *testing.T) {
		t.Parallel()

		spec := &version.Spec[version.BareAbsolute]{
			Name:  "test",
			Shift: 0,
		}
		assert.False(t, spec.HasShift())
	})

	t.Run("with shift", func(t *testing.T) {
		t.Parallel()

		spec := &version.Spec[version.BareAbsolute]{
			Name:  "test",
			Shift: 1,
		}
		assert.True(t, spec.HasShift())
	})

	t.Run("negative shift treated as no shift", func(t *testing.T) {
		t.Parallel()

		spec := &version.Spec[version.BareAbsolute]{
			Name:  "test",
			Shift: -1,
		}
		assert.False(t, spec.HasShift())
	})
}
