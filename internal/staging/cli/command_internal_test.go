// In-package tests of the core CommandConfig plumbing.
//declscope:core

package cli

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/staging"
)

// hasDescriptionFlag reports whether the built command registers a --description
// flag.
func hasDescriptionFlag(cmd *cli.Command) bool {
	return slices.ContainsFunc(cmd.Flags, func(f cli.Flag) bool {
		return slices.Contains(f.Names(), "description")
	})
}

// TestCommandConfig_DescriptionFlagGating pins that add/edit register a
// --description flag ONLY when the service honors a description (HasDescription).
// A provider that does not (Azure Key Vault / App Configuration) must reject
// --description as an unknown flag rather than accept it and drop it on apply
// (#666 — the previously silent no-op).
func TestCommandConfig_DescriptionFlagGating(t *testing.T) {
	t.Parallel()

	supported := CommandConfig{CommandName: "secret", ItemName: "secret", HasDescription: true}
	assert.True(t, hasDescriptionFlag(NewAddCommand(supported)), "add registers --description when supported")
	assert.True(t, hasDescriptionFlag(NewEditCommand(supported)), "edit registers --description when supported")

	unsupported := CommandConfig{CommandName: "secret", ItemName: "secret", HasDescription: false}
	assert.False(t, hasDescriptionFlag(NewAddCommand(unsupported)), "add omits --description when unsupported")
	assert.False(t, hasDescriptionFlag(NewEditCommand(unsupported)), "edit omits --description when unsupported")

	// Behavioral: an unsupported provider rejects --description at parse time,
	// before the action runs (so no store is needed).
	err := NewAddCommand(unsupported).Run(t.Context(), []string{"add", "my-secret", "value", "--description", "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description", "the rejection names the offending flag")
}

// TestCommandConfig_Description pins the flag reader: it returns "" when the flag
// is not registered (unsupported provider) and never panics.
func TestCommandConfig_Description(t *testing.T) {
	t.Parallel()

	unsupported := CommandConfig{HasDescription: false}
	assert.Empty(t, unsupported.description(&cli.Command{}))
}

func TestCommandConfig_NamespaceFor(t *testing.T) {
	t.Parallel()

	// Nil resolver (providers without a namespace axis) yields the empty
	// namespace regardless of context.
	var noNS CommandConfig
	assert.Empty(t, noNS.namespaceFor(t.Context()))

	withNS := CommandConfig{
		Namespace: func(context.Context) string { return "dev" },
	}
	assert.Equal(t, "dev", withNS.namespaceFor(t.Context()))
}

func TestStrategyForAdapters(t *testing.T) {
	t.Parallel()

	// Nil StrategyForNamespace => nil resolvers, so the use cases fall back to
	// their single Strategy (the namespace-agnostic providers).
	assert.Nil(t, diffStrategyFor(t.Context(), nil))
	assert.Nil(t, applyStrategyFor(t.Context(), nil))

	sentinel := errors.New("resolved")
	forNamespace := func(_ context.Context, namespace string) (staging.FullStrategy, error) {
		assert.Equal(t, "dev", namespace)

		return nil, sentinel
	}

	diff := diffStrategyFor(t.Context(), forNamespace)
	require.NotNil(t, diff)
	_, err := diff("dev")
	require.ErrorIs(t, err, sentinel)

	apply := applyStrategyFor(t.Context(), forNamespace)
	require.NotNil(t, apply)
	_, err = apply("dev")
	require.ErrorIs(t, err, sentinel)
}

func TestWorkingStore_NilResolverFails(t *testing.T) {
	t.Parallel()

	// There is no default provider: a config without a ScopeResolver is a wiring
	// bug and must fail instead of silently keying state under some provider.
	store, _, err := workingStore(t.Context(), nil)
	require.Error(t, err)
	assert.Nil(t, store)
	assert.Contains(t, err.Error(), "staging scope resolver is not configured")
}
