package tui

import (
	"context"
	"sync"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/tui/data"
)

// sourceFactory builds the read-path data sources and staging probes for the
// launched scope, resolving provider.Stores through the registry exactly as the
// CLI/GUI do. It caches staging stores per scope key so two stores never race
// the keychain data key (the staging invariant). It is separate from the App so
// the App can be built (with the factory's method as its source seam) before the
// program runs, and so tests can substitute a providermock-backed factory.
type sourceFactory struct {
	ctx   context.Context //nolint:containedctx // the TUI resolves stores lazily against the Run context
	scope provider.Scope

	mu            sync.Mutex
	stagingStores map[string]store.ReadWriteOperator
}

// newSourceFactory builds a factory for a launched scope and Run context.
func newSourceFactory(ctx context.Context, scope provider.Scope) *sourceFactory {
	return &sourceFactory{ctx: ctx, scope: scope, stagingStores: map[string]store.ReadWriteOperator{}}
}

// sourceFor returns the read source and (best-effort) staging probe for a
// service tab, or (nil, nil) when the service is unavailable for the scope.
func (f *sourceFactory) sourceFor(service string) (data.Source, data.StagingProbe) {
	svcCap, ok := capabilityFor(f.scope.Provider, service)
	if !ok {
		return nil, nil
	}

	switch service {
	case string(staging.ServiceParam):
		src := data.NewParamSource(svcCap, f.paramResolver())

		return src, f.stagingProbe(provider.KindParam, service)
	case string(staging.ServiceSecret):
		store, err := registry.Store(f.ctx, f.scope, provider.KindSecret)
		if err != nil {
			return nil, nil
		}

		return data.NewSecretSource(svcCap, store), f.stagingProbe(provider.KindSecret, service)
	default:
		return nil, nil
	}
}

// mutatorFor returns the write-path Mutator for a service tab, or nil when the
// service is unavailable for the scope. It pairs the immediate param/secret use
// cases with the staged-write strategy and the per-scope-cached staging store,
// mirroring the GUI's serviceStrategyScoped discipline.
func (f *sourceFactory) mutatorFor(service string) data.Mutator {
	svcCap, ok := capabilityFor(f.scope.Provider, service)
	if !ok {
		return nil
	}

	switch service {
	case string(staging.ServiceParam):
		return data.NewParamMutator(svcCap, f.paramResolver(), f.paramStrategyBuilder(), f.stagingStoreResolver(svcCap, provider.KindParam))
	case string(staging.ServiceSecret):
		store, err := registry.Store(f.ctx, f.scope, provider.KindSecret)
		if err != nil {
			return nil
		}

		return data.NewSecretMutator(svcCap, store, f.secretStrategyBuilder(), f.stagingStoreResolver(svcCap, provider.KindSecret))
	default:
		return nil
	}
}

// stagingStoreResolver returns a lazy resolver for the service's cached staging
// store, or nil when the service has no staging workflow (so a staged write is
// never offered). Deferring the build keeps dialog open off the keychain.
func (f *sourceFactory) stagingStoreResolver(svcCap capability.ServiceCapability, kind provider.Kind) data.StagingStoreResolver {
	if !svcCap.HasStaging {
		return nil
	}

	return func() (store.ReadWriteOperator, error) {
		return f.stagingStore(kind)
	}
}

// paramStrategyBuilder builds the provider-specific param staging strategy over a
// resolved store (Azure App Configuration vs AWS SSM), mirroring the GUI's
// serviceStrategyScoped.
func (f *sourceFactory) paramStrategyBuilder() data.StrategyBuilder {
	return func(s provider.Store) staging.FullStrategy {
		if f.scope.Provider == provider.ProviderAzure {
			return staging.NewAzureAppConfigParamStrategy(s)
		}

		return staging.NewAWSParamStrategy(s)
	}
}

// secretStrategyBuilder builds the provider-specific secret staging strategy over
// a resolved store (Google Cloud / Azure Key Vault / AWS Secrets Manager).
func (f *sourceFactory) secretStrategyBuilder() data.StrategyBuilder {
	return func(s provider.Store) staging.FullStrategy {
		switch f.scope.Provider {
		case provider.ProviderGoogleCloud:
			return staging.NewGoogleCloudSecretStrategy(s)
		case provider.ProviderAzure:
			return staging.NewAzureKeyVaultSecretStrategy(s)
		default:
			return staging.NewAWSSecretStrategy(s)
		}
	}
}

// paramResolver resolves the param store for an App Configuration namespace,
// mirroring the GUI's paramStoreForNamespace (the namespace is harmless for
// other providers).
func (f *sourceFactory) paramResolver() data.StoreResolver {
	return func(ctx context.Context, namespace string) (provider.Store, error) {
		sc := f.scope
		if sc.Provider == provider.ProviderAzure && sc.StoreName != "" {
			sc.AppConfigNamespace = namespace
		}

		return registry.Store(ctx, sc, provider.KindParam)
	}
}

// stagingProbe returns a lazy, cached staging probe for a service, or nil when
// the service has no staging workflow. Building the on-disk store (which may
// touch the keychain) is deferred to the first probe, off the update loop.
func (f *sourceFactory) stagingProbe(kind provider.Kind, service string) data.StagingProbe {
	svcCap, ok := capabilityFor(f.scope.Provider, service)
	if !ok || !svcCap.HasStaging {
		return nil
	}

	return &lazyStagingProbe{build: func() (data.StagingProbe, error) {
		st, err := f.stagingStore(kind)
		if err != nil {
			return nil, err
		}

		parser, err := parserFor(f.scope.Provider, service)
		if err != nil {
			return nil, err
		}

		return data.NewStagingProbe(parser, st), nil
	}}
}

// stagingStore resolves (and caches) the on-disk staging store for a service
// kind, keyed by the service-specific scope so the key matches the CLI/GUI
// layout and two stores never race the keychain.
func (f *sourceFactory) stagingStore(kind provider.Kind) (store.ReadWriteOperator, error) {
	scope, err := f.stagingScope(kind)
	if err != nil {
		return nil, err
	}

	key := scope.Key()

	f.mu.Lock()
	defer f.mu.Unlock()

	if s := f.stagingStores[key]; s != nil {
		return s, nil
	}

	s, err := file.NewWorkingStore(scope)
	if err != nil {
		return nil, err
	}

	f.stagingStores[key] = s

	return s, nil
}

// stagingScope resolves the service-specific scope that keys staging state,
// mirroring the GUI's stagingScopeForKind: Azure's two services live in separate
// buckets, and AWS is keyed by the STS caller identity.
func (f *sourceFactory) stagingScope(kind provider.Kind) (provider.Scope, error) {
	if f.scope.Provider == provider.ProviderAzure {
		if kind == provider.KindParam {
			scope := provider.AzureAppConfigScope(f.scope.StoreName)
			scope.AppConfigNamespace = f.scope.AppConfigNamespace

			return scope, nil
		}

		return provider.AzureKeyVaultScope(f.scope.VaultName), nil
	}

	if f.scope.Provider != provider.ProviderAWS {
		return f.scope, nil
	}

	if f.scope.AccountID != "" && f.scope.Region != "" {
		return f.scope, nil
	}

	identity, err := infra.GetAWSIdentity(f.ctx)
	if err != nil {
		return provider.Scope{}, err
	}

	return provider.AWSScope(identity.AccountID, identity.Region), nil
}

// lazyStagingProbe defers building the real probe (and thus the on-disk store)
// to the first StagedKeys call, so page construction never blocks on the
// keychain.
type lazyStagingProbe struct {
	build func() (data.StagingProbe, error)
}

func (p *lazyStagingProbe) StagedKeys(ctx context.Context) (map[data.StagedKey]struct{}, error) {
	probe, err := p.build()
	if err != nil {
		return nil, err
	}

	return probe.StagedKeys(ctx)
}

// capabilityFor looks up the ServiceCapability for a provider+service in the
// neutral matrix, so every gate reads one source of truth.
func capabilityFor(prov provider.Provider, service string) (capability.ServiceCapability, bool) {
	for _, pc := range capability.All() {
		if pc.Provider != string(prov) {
			continue
		}

		for _, sc := range pc.Services {
			if sc.Service == service {
				return sc, true
			}
		}
	}

	return capability.ServiceCapability{}, false
}

// parserFor returns the store-less staging strategy (parser) for a
// provider+service, mirroring the GUI's getParserScoped.
func parserFor(prov provider.Provider, service string) (staging.Parser, error) {
	switch service {
	case string(staging.ServiceParam):
		if prov == provider.ProviderAzure {
			return &staging.AzureAppConfigParamStrategy{}, nil
		}

		return &staging.AWSParamStrategy{}, nil
	case string(staging.ServiceSecret):
		switch prov {
		case provider.ProviderGoogleCloud:
			return &staging.GoogleCloudSecretStrategy{}, nil
		case provider.ProviderAzure:
			return &staging.AzureKeyVaultSecretStrategy{}, nil
		default:
			return &staging.AWSSecretStrategy{}, nil
		}
	default:
		return nil, errUnknownService
	}
}

// errUnknownService is returned by parserFor for an unrecognized service.
var errUnknownService = stringError("tui: unknown staging service")

// stringError is a small sentinel error type.
type stringError string

func (e stringError) Error() string { return string(e) }
