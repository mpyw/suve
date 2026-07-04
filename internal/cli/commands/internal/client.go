package internal

import (
	"context"
	"errors"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/provider/gcp"
	"github.com/mpyw/suve/internal/staging"
)

// registry is the provider registry reachable by every CLI command. It is the
// single composition point where cloud backends are wired in: AWS (param +
// secret) and Google Cloud (secret only) are registered here. Top-level command
// groups build their own provider.Scope and resolve stores through this same
// registry.
//
//nolint:gochecknoglobals // process-wide provider registry, built once
var registry = func() *provider.Registry {
	reg := aws.NewRegistry()
	gcp.Register(reg)

	return reg
}()

// gcpProjectContextKey keys the resolved Google Cloud project id stored in the
// context by the gcloud command group's Before hook.
type gcpProjectContextKey struct{}

// WithGCPProject returns a context carrying the resolved Google Cloud project
// id. The gcloud command group sets it once (from --project or the
// GOOGLE_CLOUD_PROJECT env) so every gcloud subcommand can resolve a store
// without threading the flag through the generic command Config.
func WithGCPProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, gcpProjectContextKey{}, project)
}

func gcpProjectFromContext(ctx context.Context) string {
	project, _ := ctx.Value(gcpProjectContextKey{}).(string)

	return project
}

// storeScope is the provider selector for read/write commands. Only the
// Provider field is needed: the AWS factory builds its SSM/Secrets Manager
// client from the ambient AWS config (region from env/profile), so no
// account/region lookup — and therefore no STS GetCallerIdentity call — is
// required here. (Account/region only matter for staging-state file keying,
// which the staging commands build from the AWS identity separately.)
//
//nolint:gochecknoglobals // immutable provider selector for read/write commands
var storeScope = provider.Scope{Provider: provider.ProviderAWS}

// ParamStore resolves a provider.Store for the parameter service via the
// registry (AWS by default).
func ParamStore(ctx context.Context) (provider.Store, error) {
	return storeForKind(ctx, provider.KindParam)
}

// SecretStore resolves a provider.Store for the secret service via the
// registry (AWS by default).
func SecretStore(ctx context.Context) (provider.Store, error) {
	return storeForKind(ctx, provider.KindSecret)
}

// GCPSecretStore resolves a provider.Store for the Google Cloud Secret Manager
// service. The project id is read from the context (see WithGCPProject); it
// returns a clear error when no project could be resolved.
func GCPSecretStore(ctx context.Context) (provider.Store, error) {
	project := gcpProjectFromContext(ctx)
	if project == "" {
		return nil, errors.New(
			"no Google Cloud project specified: set --project or the GOOGLE_CLOUD_PROJECT environment variable",
		)
	}

	return registry.Store(ctx, provider.GoogleCloudScope(project), provider.KindSecret)
}

func storeForKind(ctx context.Context, kind provider.Kind) (provider.Store, error) {
	store, err := registry.Store(ctx, storeScope, kind)
	if err != nil {
		return nil, err
	}

	return store, nil
}

// ParamStrategyFactory builds a staging FullStrategy for the parameter service,
// wrapping a provider.Store resolved through the registry. It satisfies
// staging.StrategyFactory.
func ParamStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := ParamStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewParamStrategy(store), nil
}

// SecretStrategyFactory builds a staging FullStrategy for the secret service,
// wrapping a provider.Store resolved through the registry. It satisfies
// staging.StrategyFactory.
func SecretStrategyFactory(ctx context.Context) (staging.FullStrategy, error) {
	store, err := SecretStore(ctx)
	if err != nil {
		return nil, err
	}

	return staging.NewSecretStrategy(store), nil
}
