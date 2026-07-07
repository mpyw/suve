package azure

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/debug"
)

// TestEnableDebugLogging covers both branches. The azcore logger is
// process-global and guarded by a sync.Once, so this test only asserts the call
// is safe (no panic) with and without debug active.
//
//nolint:paralleltest // configures the process-global azcore logger
func TestEnableDebugLogging(t *testing.T) {
	assert.NotPanics(t, func() {
		enableDebugLogging(context.Background())
	})

	assert.NotPanics(t, func() {
		enableDebugLogging(debug.With(context.Background(), debug.Config{Enabled: true, Writer: &bytes.Buffer{}}))
	})
}
