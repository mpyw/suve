package cli

import (
	"context"

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
	// for both. Required.
	ScopeResolver staging.ScopeResolver
	// StrategyForNamespace, when set, builds a strategy scoped to a given
	// namespace so apply/diff act on each staged entry under its own namespace
	// (Azure App Configuration keeps all namespaces in one staging store). Nil
	// for services without a namespace axis — the single Factory strategy is used.
	StrategyForNamespace func(ctx context.Context, namespace string) (staging.FullStrategy, error)
}

// GlobalConfig configures the provider-wide stage commands so a single set of
// implementations serves every multi-service provider: AWS and Azure each
// iterate param + secret. The ScopeResolver keys on-disk staging state for
// the active provider.
type GlobalConfig struct {
	// ProviderLabel is the human-readable provider name used in prompts and
	// messages (e.g. "AWS", "Azure").
	ProviderLabel string
	// CommandPath is the explicit command path of the provider's stage group,
	// used in help text and usage errors (e.g. "suve aws stage").
	CommandPath string
	// ScopeResolver resolves the provider staging scope. Required.
	ScopeResolver staging.ScopeResolver
	// Services lists the provider's services in stable display order.
	Services []GlobalServiceSpec
}
