// Package generic provides the provider-neutral command scaffolding shared by
// every provider: show, log, diff, list, tag and untag. Each file owns one
// command's flow (flags, validation, pager gating, output dispatch) and takes the
// provider-specific parts through a small per-command config and presenter, so a
// provider's CLI package only supplies help text, its version grammar and its
// renderers.
package generic
