package aws_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	awsprovider "github.com/mpyw/suve/internal/provider/aws"
)

func TestFactory_Param(t *testing.T) {
	t.Parallel()

	store, err := awsprovider.Factory{}.Store(t.Context(), provider.AWSScope("123456789012", "us-east-1"), provider.KindParam)
	require.NoError(t, err)
	require.NotNil(t, store)
}

func TestFactory_Secret(t *testing.T) {
	t.Parallel()

	store, err := awsprovider.Factory{}.Store(t.Context(), provider.AWSScope("123456789012", "us-east-1"), provider.KindSecret)
	require.NoError(t, err)
	require.NotNil(t, store)
}

func TestFactory_UnsupportedKind(t *testing.T) {
	t.Parallel()

	_, err := awsprovider.Factory{}.Store(t.Context(), provider.AWSScope("123456789012", "us-east-1"), provider.Kind("bogus"))
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrUnsupportedKind)
}

func TestNewRegistry_ResolvesAWS(t *testing.T) {
	t.Parallel()

	reg := awsprovider.NewRegistry()

	store, err := reg.Store(t.Context(), provider.AWSScope("123456789012", "eu-west-1"), provider.KindParam)
	require.NoError(t, err)
	require.NotNil(t, store)
}

func TestRegister_AddsFactory(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	awsprovider.Register(reg)

	store, err := reg.Store(t.Context(), provider.AWSScope("123456789012", "ap-northeast-1"), provider.KindSecret)
	require.NoError(t, err)
	require.NotNil(t, store)
}
