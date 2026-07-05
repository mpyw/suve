package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

func TestAWSGlobalConfig(t *testing.T) {
	t.Parallel()

	param := stgcli.CommandConfig{Factory: nil, ParserFactory: staging.ParamParserFactory}
	secret := stgcli.CommandConfig{Factory: nil, ParserFactory: staging.SecretParserFactory}

	cfg := stgcli.AWSGlobalConfig(param, secret)

	assert.Equal(t, "AWS", cfg.ProviderLabel)
	assert.NotNil(t, cfg.ScopeResolver)
	assert.Len(t, cfg.Services, 2)
	assert.Equal(t, staging.ServiceParam, cfg.Services[0].Service)
	assert.Equal(t, staging.ServiceSecret, cfg.Services[1].Service)
	// Parser factories are carried through and are network-free.
	assert.Equal(t, "SSM Parameter Store", cfg.Services[0].ParserFactory().ServiceName())
	assert.Equal(t, "Secrets Manager", cfg.Services[1].ParserFactory().ServiceName())
}
