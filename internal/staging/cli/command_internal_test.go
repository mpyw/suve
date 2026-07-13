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
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
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

func TestCommandConfig_StrategyForAdapters(t *testing.T) {
	t.Parallel()

	// Nil StrategyForNamespace => nil resolvers, so the use cases fall back to
	// their single Strategy (the namespace-agnostic providers).
	var none CommandConfig
	assert.Nil(t, none.diffStrategyFor(t.Context()))
	assert.Nil(t, none.applyStrategyFor(t.Context()))

	sentinel := errors.New("resolved")
	cfg := CommandConfig{
		StrategyForNamespace: func(_ context.Context, namespace string) (staging.FullStrategy, error) {
			assert.Equal(t, "dev", namespace)

			return nil, sentinel
		},
	}

	diff := cfg.diffStrategyFor(t.Context())
	require.NotNil(t, diff)
	_, err := diff("dev")
	require.ErrorIs(t, err, sentinel)

	apply := cfg.applyStrategyFor(t.Context())
	require.NotNil(t, apply)
	_, err = apply("dev")
	require.ErrorIs(t, err, sentinel)
}

func TestDiffEntryDisplayName(t *testing.T) {
	t.Parallel()

	// The null/default namespace yields the bare name.
	assert.Equal(t, "app/k", diffEntryDisplayName(stagingusecase.DiffEntry{Name: "app/k"}))

	// A named namespace is appended so a key staged under several namespaces is
	// unambiguous in the diff.
	assert.Equal(t, "app/k [dev]", diffEntryDisplayName(stagingusecase.DiffEntry{Name: "app/k", Namespace: "dev"}))
}
