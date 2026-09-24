// Package internal holds the AWS store and staging-scope wiring that the aws
// param, secret and stage command packages share.
package internal

import (
	"context"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
)

// ParamStore resolves a provider.Store for AWS SSM Parameter Store via the
// registry. The AWS factory builds its client from the ambient AWS config
// (region from env/profile), so the scope carries only the provider.
func ParamStore(ctx context.Context) (provider.Store, error) {
	return cliinternal.Store(ctx, provider.Scope{Provider: provider.ProviderAWS}, provider.KindParam)
}

// SecretStore resolves a provider.Store for AWS Secrets Manager via the
// registry. The AWS factory builds its client from the ambient AWS config
// (region from env/profile), so the scope carries only the provider.
func SecretStore(ctx context.Context) (provider.Store, error) {
	return cliinternal.Store(ctx, provider.Scope{Provider: provider.ProviderAWS}, provider.KindSecret)
}

// StagingScopeResolver resolves the AWS staging scope (account + region) from
// the STS caller identity, through the shared staging binding. Both AWS
// services share it. It satisfies staging.ScopeResolver.
func StagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	return binding.StagingScope(ctx, provider.Scope{Provider: provider.ProviderAWS}, provider.KindParam, nil)
}

// ConfirmTarget describes the AWS target (profile, account, region) for a
// confirmation prompt, through the same resolver the TUI and GUI use. It
// returns "" when the caller identity cannot be resolved, so the prompt shows
// no target line.
func ConfirmTarget(ctx context.Context) string {
	target, err := binding.ResolveTarget(ctx, provider.Scope{Provider: provider.ProviderAWS}, nil)
	if err != nil {
		return ""
	}

	return target.String()
}
