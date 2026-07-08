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
	// ScopeResolver resolves THIS service's staging scope. It is per-service
	// because a provider's services may live in independent resources with
	// separate staging buckets: Azure App Configuration (param) is keyed by
	// store name, Key Vault (secret) by vault name. AWS keeps one account scope
	// for both. Nil defaults to AWS (AWSScopeResolver).
	ScopeResolver staging.ScopeResolver
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
			{Service: staging.ServiceParam, ParserFactory: paramCfg.ParserFactory, Factory: paramCfg.Factory, ScopeResolver: AWSScopeResolver},
			{Service: staging.ServiceSecret, ParserFactory: secretCfg.ParserFactory, Factory: secretCfg.Factory, ScopeResolver: AWSScopeResolver},
		},
	}
}

// AzureGlobalConfig builds the GlobalConfig for Azure. Unlike AWS, App
// Configuration (param) and Key Vault (secret) are INDEPENDENT resources with
// separate staging buckets, so each service carries its own ScopeResolver. The
// top-level ScopeResolver keys the (single-file) global stash under App
// Configuration; cross-resource stash is tracked separately (#435).
func AzureGlobalConfig(paramCfg, secretCfg CommandConfig) GlobalConfig {
	return GlobalConfig{
		ProviderLabel: "Azure",
		ScopeResolver: paramCfg.ScopeResolver,
		Services: []GlobalServiceSpec{
			{
				Service:       staging.ServiceParam,
				ParserFactory: paramCfg.ParserFactory,
				Factory:       paramCfg.Factory,
				ScopeResolver: paramCfg.ScopeResolver,
			},
			{
				Service:       staging.ServiceSecret,
				ParserFactory: secretCfg.ParserFactory,
				Factory:       secretCfg.Factory,
				ScopeResolver: secretCfg.ScopeResolver,
			},
		},
	}
}
