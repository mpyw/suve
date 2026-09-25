// Package binding is the single per-(provider, service kind) lookup that the
// CLI, GUI and TUI share. For each provider and kind it owns:
//
//   - the version grammar that splits "name#VERSION~SHIFT" into the name and
//     the suffix the use cases take (SplitSpec),
//   - the store-less parser (status, reset, export/import parsing),
//   - the staging strategy built over a resolved provider.Store,
//   - the scope that keys on-disk staging state, derived from the selected
//     scope (Azure keys its two services in separate buckets; AWS keys by the
//     caller identity, which needs a network lookup),
//   - the namespace override for a service with a namespace axis inside one
//     store (Azure App Configuration).
//
// An unknown provider is an error (ErrUnknownProvider). A known provider that
// does not offer a kind is provider.ErrUnsupportedKind. There is no default
// provider.
//
// The package imports no cloud SDK directly. The AWS identity lookup goes
// through aws.LoadIdentity (internal/provider/aws), which loads the AWS config
// and calls STS.
package binding

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/version"
)

// ErrUnknownProvider is returned for a provider that has no staging binding
// (an unknown or unselected provider).
var ErrUnknownProvider = errors.New("unknown provider")

// IdentityLookup resolves the account-level staging scope for a provider whose
// selected scope does not carry it (AWS: the STS caller identity). Callers may
// pass their own to memoize or stub the lookup.
type IdentityLookup func(ctx context.Context) (staging.ResolvedScope, error)

// Binding is the staging binding for one (provider, kind) pair. Get one from
// Lookup.
type Binding struct {
	// SplitSpec splits a version spec into its name and the version suffix the
	// use cases take, with the product's grammar (a version grammar's Split).
	// An unversioned service (Azure App Configuration) keeps the whole argument
	// as the name, so a key that contains '#' or '~' is not split. It is a
	// field rather than a method because only the build-tagged GUI calls it,
	// and the default-build dead-code gate cannot see that caller.
	SplitSpec func(input string) (name, suffix string, err error)

	parser   staging.ParserFactory
	strategy func(provider.Store) staging.FullStrategy
	// namespaced is set for a service with a namespace axis inside one store
	// (Azure App Configuration).
	namespaced bool
}

// Parser returns a store-less parser for the service.
func (b Binding) Parser() staging.Parser {
	return b.parser()
}

// ParserFactory returns the parser constructor, for configs that take a
// staging.ParserFactory.
func (b Binding) ParserFactory() staging.ParserFactory {
	return b.parser
}

// Strategy builds the staging strategy over a resolved provider store.
func (b Binding) Strategy(store provider.Store) staging.FullStrategy {
	return b.strategy(store)
}

// Namespaced reports whether the service has a namespace axis for the selected
// scope, so staged entries must apply under their own namespace.
func (b Binding) Namespaced(sc provider.Scope) bool {
	return b.namespaced && sc.StoreName != ""
}

// NamespaceScope returns sc with its namespace set to ns when the service has a
// namespace axis. For every other service it returns sc unchanged.
func (b Binding) NamespaceScope(sc provider.Scope, ns string) provider.Scope {
	if b.Namespaced(sc) {
		sc.AppConfigNamespace = ns
	}

	return sc
}

// descriptor holds the bindings of one provider.
type descriptor struct {
	kinds map[provider.Kind]Binding
	// stagingScope derives the staging scope for kind from the selected scope.
	// It returns false when the selected scope cannot key staging by itself and
	// the identity lookup must run.
	stagingScope func(sc provider.Scope, kind provider.Kind) (staging.ResolvedScope, bool)
	// identity is the default network lookup. It is nil when stagingScope never
	// needs one.
	identity IdentityLookup
}

// descriptors is the provider table. Adding a provider means adding one entry.
//
//nolint:gochecknoglobals // static provider table
var descriptors = map[provider.Provider]descriptor{
	provider.ProviderAWS: {
		kinds: map[provider.Kind]Binding{
			provider.KindParam: {
				SplitSpec: version.AWSParameterStore.Split,
				parser:    staging.AWSParamParserFactory,
				strategy:  func(s provider.Store) staging.FullStrategy { return staging.NewAWSParamStrategy(s) },
			},
			provider.KindSecret: {
				SplitSpec: version.AWSSecretsManager.Split,
				parser:    staging.AWSSecretParserFactory,
				strategy:  func(s provider.Store) staging.FullStrategy { return staging.NewAWSSecretStrategy(s) },
			},
		},
		// Both AWS services share the account scope. A scope that already
		// carries account and region needs no STS call.
		stagingScope: func(sc provider.Scope, _ provider.Kind) (staging.ResolvedScope, bool) {
			if sc.AccountID == "" || sc.Region == "" {
				return staging.ResolvedScope{}, false
			}

			return staging.ResolvedScope{Scope: sc, Target: sc.Target()}, true
		},
		identity: awsIdentity,
	},
	provider.ProviderGoogleCloud: {
		kinds: map[provider.Kind]Binding{
			provider.KindSecret: {
				SplitSpec: version.GoogleCloudSecretManager.Split,
				parser:    staging.GoogleCloudSecretParserFactory,
				strategy:  func(s provider.Store) staging.FullStrategy { return staging.NewGoogleCloudSecretStrategy(s) },
			},
		},
		stagingScope: func(sc provider.Scope, _ provider.Kind) (staging.ResolvedScope, bool) {
			return staging.ResolvedScope{Scope: sc, Target: sc.Target()}, true
		},
	},
	provider.ProviderAzure: {
		kinds: map[provider.Kind]Binding{
			provider.KindParam: {
				SplitSpec:  version.AzureAppConfiguration.Split,
				parser:     staging.AzureParamParserFactory,
				strategy:   func(s provider.Store) staging.FullStrategy { return staging.NewAzureParamStrategy(s) },
				namespaced: true,
			},
			provider.KindSecret: {
				SplitSpec: version.AzureKeyVault.Split,
				parser:    staging.AzureSecretParserFactory,
				strategy:  func(s provider.Store) staging.FullStrategy { return staging.NewAzureSecretStrategy(s) },
			},
		},
		// Key Vault and App Configuration are separate resources with separate
		// staging buckets. A combined scope's Key() resolves to the Key Vault
		// key, so each kind gets its own service-specific scope.
		stagingScope: func(sc provider.Scope, kind provider.Kind) (staging.ResolvedScope, bool) {
			if kind == provider.KindParam {
				// The bucket is per store: every namespace's entries share it, and
				// apply pushes them all. The target therefore names the store
				// alone, so a prompt never implies that one namespace is applied.
				store := provider.AzureAppConfigScope(sc.StoreName)
				scope := store
				scope.AppConfigNamespace = sc.AppConfigNamespace

				return staging.ResolvedScope{Scope: scope, Target: store.Target()}, true
			}

			scope := provider.AzureKeyVaultScope(sc.VaultName)

			return staging.ResolvedScope{Scope: scope, Target: scope.Target()}, true
		},
	},
}

// Lookup returns the staging binding for a provider and kind.
func Lookup(p provider.Provider, kind provider.Kind) (Binding, error) {
	d, ok := descriptors[p]
	if !ok {
		return Binding{}, fmt.Errorf("%w %q", ErrUnknownProvider, p)
	}

	b, ok := d.kinds[kind]
	if !ok {
		return Binding{}, fmt.Errorf("%w: provider %q has no %s service", provider.ErrUnsupportedKind, p, kind)
	}

	return b, nil
}

// StagingScope resolves the scope that keys kind's staging state for the
// selected scope sc, plus the confirmation target. It fails only for an
// unknown provider: a known provider keys a kind it does not offer under the
// selected scope, and the parser or strategy lookup rejects that kind instead.
//
// lookup replaces the provider's default identity lookup (for memoizing or
// stubbing it); nil uses the default. It runs only when sc cannot key staging by
// itself.
func StagingScope(ctx context.Context, sc provider.Scope, kind provider.Kind, lookup IdentityLookup) (staging.ResolvedScope, error) {
	d, ok := descriptors[sc.Provider]
	if !ok {
		return staging.ResolvedScope{}, fmt.Errorf("%w %q", ErrUnknownProvider, sc.Provider)
	}

	if resolved, ok := d.stagingScope(sc, kind); ok {
		return resolved, nil
	}

	if lookup == nil {
		lookup = d.identity
	}

	return lookup(ctx)
}

// DefaultIdentity returns the provider's default identity lookup, for a caller
// that wraps it (for example to memoize it). It returns a lookup that fails for
// an unknown provider or one that never needs a lookup.
func DefaultIdentity(p provider.Provider) IdentityLookup {
	d, ok := descriptors[p]
	if !ok || d.identity == nil {
		return func(context.Context) (staging.ResolvedScope, error) {
			return staging.ResolvedScope{}, fmt.Errorf("%w %q: no identity lookup", ErrUnknownProvider, p)
		}
	}

	return d.identity
}

// ResolveTarget describes what the selected scope sc points at, running the
// provider's identity lookup when sc cannot describe itself (AWS: the STS
// caller identity fills the profile, account and region). lookup replaces the
// default lookup, as in StagingScope, so a caller can share one memoized lookup
// between staging and display.
func ResolveTarget(ctx context.Context, sc provider.Scope, lookup IdentityLookup) (provider.Target, error) {
	target := sc.Target()
	if !target.Pending {
		return target, nil
	}

	if lookup == nil {
		lookup = DefaultIdentity(sc.Provider)
	}

	resolved, err := lookup(ctx)
	if err != nil {
		return provider.Target{}, err
	}

	return resolved.Target, nil
}

// awsIdentity resolves the AWS staging scope (account and region) from the STS
// caller identity.
func awsIdentity(ctx context.Context) (staging.ResolvedScope, error) {
	identity, err := aws.LoadIdentity(ctx)
	if err != nil {
		return staging.ResolvedScope{}, fmt.Errorf("failed to get AWS identity: %w", err)
	}

	return staging.ResolvedScope{
		Scope:  provider.AWSScope(identity.AccountID, identity.Region),
		Target: provider.AWSTarget(identity.Profile, identity.AccountID, identity.Region),
	}, nil
}
