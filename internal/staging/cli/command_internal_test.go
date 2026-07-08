package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

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
