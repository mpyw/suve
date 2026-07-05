package cli

import (
	"github.com/mpyw/suve/internal/staging"
)

// GlobalServiceSpec describes one service for the provider-wide (all-service)
// stage commands (status / diff / apply / reset). ParserFactory yields a
// network-free Parser (service name, delete-option support); Factory builds a
// FullStrategy backed by a provider.Store for apply/diff.
type GlobalServiceSpec struct {
	// Service is the staging service (param or secret).
	Service staging.Service
	// ParserFactory builds a network-free parser for this service.
	ParserFactory staging.ParserFactory
	// Factory builds a provider-backed strategy for this service.
	Factory staging.StrategyFactory
}

// GlobalConfig configures the provider-wide stage commands so a single set of
// implementations serves every provider: AWS iterates param + secret, Google
// Cloud iterates secret only. The ScopeResolver keys on-disk staging state for
// the active provider (nil defaults to AWS).
type GlobalConfig struct {
	// ProviderLabel is the human-readable provider name used in prompts and
	// messages (e.g. "AWS", "Google Cloud").
	ProviderLabel string
	// ScopeResolver resolves the provider staging scope. Nil defaults to AWS.
	ScopeResolver staging.ScopeResolver
	// Services lists the provider's services in stable display order.
	Services []GlobalServiceSpec
}

// AWSGlobalConfig builds the GlobalConfig for AWS (param + secret) from the
// given service factories. It preserves the historical AWS behavior and wording.
func AWSGlobalConfig(paramCfg, secretCfg CommandConfig) GlobalConfig {
	return GlobalConfig{
		ProviderLabel: "AWS",
		ScopeResolver: AWSScopeResolver,
		Services: []GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: paramCfg.ParserFactory, Factory: paramCfg.Factory},
			{Service: staging.ServiceSecret, ParserFactory: secretCfg.ParserFactory, Factory: secretCfg.Factory},
		},
	}
}
