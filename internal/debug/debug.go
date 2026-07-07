// Package debug carries an opt-in, SDK-neutral debug switch through the request
// context. The root --debug flag (and the SUVE_DEBUG environment variable) turns
// it on; provider adapters read it to enable their cloud SDK's request/response
// logging, writing to the supplied writer (stderr in normal use).
//
// Keeping this package free of any cloud SDK is deliberate: both the CLI layer
// (which sets the switch) and every provider adapter (which acts on it) can
// depend on it without violating the provider seam that confines each cloud SDK
// to its own provider package.
package debug

import (
	"context"
	"fmt"
	"io"
)

// ctxKey is the unexported context key under which Config is stored.
type ctxKey struct{}

// Config describes the active debug settings pulled from a context.
type Config struct {
	// Enabled reports whether verbose debug logging was requested.
	Enabled bool
	// Writer is where debug output should go (typically stderr). Callers that
	// set Enabled must supply a non-nil Writer.
	Writer io.Writer
}

// With returns a child context carrying cfg.
func With(ctx context.Context, cfg Config) context.Context {
	return context.WithValue(ctx, ctxKey{}, cfg)
}

// From returns the Config carried by ctx, or the zero value (disabled) when
// none is present.
func From(ctx context.Context) Config {
	cfg, _ := ctx.Value(ctxKey{}).(Config)

	return cfg
}

// Logf writes a single formatted debug line to the configured Writer. It is a
// no-op when debug is disabled or no Writer is set, so callers can invoke it
// unconditionally. Centralizing the write here keeps provider adapters free of
// direct fmt.Fprint* usage (see the output package for the same rationale).
func (c Config) Logf(format string, args ...any) {
	if !c.Enabled || c.Writer == nil {
		return
	}

	_, _ = fmt.Fprintf(c.Writer, format, args...)
}
