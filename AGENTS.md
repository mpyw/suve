# Repository instructions

suve is a multi-cloud secret and parameter CLI with TUI and GUI frontends. The provider-neutral model, provider seams, staging rules, and build matrix are in [implementation notes](design/implementation.md). Read the relevant section before changing a provider, staging, version grammar, or a UI. Use the matching skill under `.agents/skills/` for substantial work in its area; the vendored `declscope-adoption` skill is for adopting that linter and is overwritten on update.

Keep cloud SDK imports inside their provider adapters and use opaque version references outside them. Commands write to injected `io.Writer`s. After a root dependency change, tidy the separate `gui/` module too. Run `mise test` for changes, `mise lint` for the full gate, and the relevant emulator or GUI tests for user-visible behavior.
