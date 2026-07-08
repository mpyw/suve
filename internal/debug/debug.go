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
	"time"
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
	// NoRedaction, when true, tells provider adapters to log full request and
	// response bodies and to stop masking sensitive headers, so the debug output
	// includes secret values and live credentials. It is opt-in via
	// --no-redaction and only meaningful when Enabled; the zero value keeps the
	// safe, redacted default.
	NoRedaction bool
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

// Logf writes a single formatted debug line to the configured Writer, prefixed
// with "[suve debug <wall clock>] " so every provider's output shares one
// grep-able, time-correlatable format. It is a no-op when debug is disabled or
// no Writer is set, so callers can invoke it unconditionally. Centralizing the
// write here keeps provider adapters free of direct fmt.Fprint* usage (see the
// output package for the same rationale).
//
// Callers supply the format WITHOUT a prefix or timestamp; multi-line payloads
// (e.g. HTTP dumps) are prefixed on their first line only.
func (c Config) Logf(format string, args ...any) {
	if !c.Enabled || c.Writer == nil {
		return
	}

	_, _ = fmt.Fprintf(c.Writer, "[suve debug %s] "+format,
		append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}
