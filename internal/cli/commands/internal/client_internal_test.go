// These are client.go's tests, so they share its core namespace.
//declscope:core

package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
)

func TestStrategyFactory(t *testing.T) {
	t.Parallel()

	factory := StrategyFactory(provider.ProviderGoogleCloud, provider.KindSecret,
		func(context.Context) (provider.Store, error) { return &providermock.Store{}, nil })
	strategy, err := factory(t.Context())
	require.NoError(t, err)
	assert.IsType(t, &staging.GoogleCloudSecretStrategy{}, strategy)

	errStore := errors.New("no store")
	failing := StrategyFactory(provider.ProviderAWS, provider.KindParam,
		func(context.Context) (provider.Store, error) { return nil, errStore })
	_, err = failing(t.Context())
	require.ErrorIs(t, err, errStore)

	assert.IsType(t, &staging.AzureAppConfigParamStrategy{}, ParserFactory(provider.ProviderAzure, provider.KindParam)())
}

func TestStrategyFactory_PanicsWithoutBinding(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { ParserFactory(provider.ProviderGoogleCloud, provider.KindParam) })
	assert.Panics(t, func() { StrategyFactory(provider.Provider("oracle"), provider.KindSecret, nil) })
}
