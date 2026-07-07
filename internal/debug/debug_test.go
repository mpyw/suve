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

func TestConfig_Logf_enabled(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	cfg := debug.Config{Enabled: true, Writer: &buf}
	cfg.Logf("hello %s\n", "world")

	// Every line carries the unified prefix with a wall-clock timestamp.
	assert.Regexp(t, `^\[suve debug \d{2}:\d{2}:\d{2}\.\d{3}\] hello world\n$`, buf.String())
}

func TestConfig_Logf_noop(t *testing.T) {
	t.Parallel()

	// Disabled: nothing is written.
	var buf bytes.Buffer

	(debug.Config{Enabled: false, Writer: &buf}).Logf("nope")
	assert.Empty(t, buf.String())

	// Enabled but no writer: must not panic.
	assert.NotPanics(t, func() {
		(debug.Config{Enabled: true}).Logf("nope")
	})
}
