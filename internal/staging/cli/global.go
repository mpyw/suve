package cli

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
)

// GlobalServiceSpec describes one service for the provider-wide (all-service)
// stage commands (status / diff / apply / reset in global_*.go). ParserFactory
// yields a network-free Parser (service name, delete-option support); Factory
// builds a FullStrategy backed by a provider.Store for apply/diff.
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
// iterate param + secret, each service through its own ScopeResolver.
type GlobalConfig struct {
	// ProviderLabel is the human-readable provider name used in prompts and
	// messages (e.g. "AWS", "Azure").
	ProviderLabel string
	// CommandPath is the explicit command path of the provider's stage group,
	// used in help text and usage errors (e.g. "suve aws stage").
	CommandPath string
	// ScopeResolver resolves the one provider staging scope that the all-service
	// export/import (NewGlobalExportCommand / NewGlobalImportCommand) read and
	// write. Required when those commands are wired; status / diff / apply /
	// reset use each service's own ScopeResolver instead.
	ScopeResolver staging.ScopeResolver
	// Services lists the provider's services in stable display order.
	Services []GlobalServiceSpec
}

// globalStoreResolver resolves a service's staging store and scope. It matches
// WorkingStore; tests substitute a fake to exercise the skip-unconfigured and
// store-error paths without touching disk.
type globalStoreResolver func(
	ctx context.Context, resolver staging.ScopeResolver,
) (store.ReadWriteOperator, staging.ResolvedScope, error)

// globalWorkingStore is the production globalStoreResolver.
func globalWorkingStore(
	ctx context.Context, resolver staging.ScopeResolver,
) (store.ReadWriteOperator, staging.ResolvedScope, error) {
	return workingStore(ctx, resolver)
}

// globalStoreFor returns a resolver that hands out override for every service
// when it is set (a test seam), and resolves each service's own working store
// otherwise.
func globalStoreFor(override store.ReadWriteOperator) globalStoreResolver {
	if override == nil {
		return globalWorkingStore
	}

	return func(context.Context, staging.ScopeResolver) (store.ReadWriteOperator, staging.ResolvedScope, error) {
		return override, staging.ResolvedScope{}, nil
	}
}

// globalService is one configured service of an all-service command, with its
// own working store and the number of changes staged in it.
type globalService struct {
	spec   GlobalServiceSpec
	store  store.ReadWriteOperator
	target string
	staged int
}

// gatherGlobalServices resolves each service's OWN working store and counts its
// staged entries and tags, in spec order. A service whose scope is not
// configured is skipped: it can hold no staged state (Azure with only one of
// Key Vault / App Configuration named). Any other resolver or store error is
// returned.
func gatherGlobalServices(
	ctx context.Context, specs []GlobalServiceSpec, resolve globalStoreResolver,
) ([]globalService, error) {
	var services []globalService

	for _, spec := range specs {
		st, resolved, err := resolve(ctx, spec.ScopeResolver)
		if errors.Is(err, staging.ErrServiceNotConfigured) {
			continue
		}

		if err != nil {
			return nil, err
		}

		entries, err := st.ListEntries(ctx, spec.Service)
		if err != nil {
			return nil, err
		}

		tags, err := st.ListTags(ctx, spec.Service)
		if err != nil {
			return nil, err
		}

		services = append(services, globalService{
			spec:   spec,
			store:  st,
			target: resolved.Target.String(),
			staged: len(entries[spec.Service]) + len(tags[spec.Service]),
		})
	}

	return services, nil
}

// globalStagedServices keeps only the services holding staged changes.
func globalStagedServices(services []globalService) []globalService {
	return lo.Filter(services, func(s globalService, _ int) bool { return s.staged > 0 })
}
