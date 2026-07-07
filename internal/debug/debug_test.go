package debug_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/debug"
)

func TestFrom_absent(t *testing.T) {
	t.Parallel()

	cfg := debug.From(context.Background())
	assert.False(t, cfg.Enabled)
	assert.Nil(t, cfg.Writer)
}

func TestWith_roundtrip(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	ctx := debug.With(context.Background(), debug.Config{Enabled: true, Writer: &buf})

	cfg := debug.From(ctx)
	assert.True(t, cfg.Enabled)
	assert.Same(t, &buf, cfg.Writer)
}
