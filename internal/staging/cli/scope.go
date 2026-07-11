package cli

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// AWSScopeResolver resolves the AWS staging scope from the STS caller identity.
// It is the default resolver used when a CommandConfig / GlobalConfig does not
// specify one, preserving the original AWS-only staging behavior.
func AWSScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	identity, err := infra.GetAWSIdentity(ctx)
	if err != nil {
		return staging.ResolvedScope{}, fmt.Errorf("failed to get AWS identity: %w", err)
	}

	return staging.ResolvedScope{
		Scope:  provider.AWSScope(identity.AccountID, identity.Region),
		Target: awsTarget(identity.Profile, identity.AccountID, identity.Region),
	}, nil
}

// awsTarget formats the AWS confirmation target line to match the original
// confirm.Prompter output ("profile (account / region)" or "account / region").
func awsTarget(profile, accountID, region string) string {
	if accountID == "" || region == "" {
		return ""
	}

	if profile != "" {
		return fmt.Sprintf("%s (%s / %s)", profile, accountID, region)
	}

	return fmt.Sprintf("%s / %s", accountID, region)
}

// resolveScope runs the resolver, defaulting to AWS when nil.
func resolveScope(ctx context.Context, resolver staging.ScopeResolver) (staging.ResolvedScope, error) {
	if resolver == nil {
		resolver = AWSScopeResolver
	}

	return resolver(ctx)
}

// workingStore resolves the staging scope via the resolver (default AWS) and
// opens the working store keyed by that scope.
func workingStore(ctx context.Context, resolver staging.ScopeResolver) (*file.Store, staging.ResolvedScope, error) {
	resolved, err := resolveScope(ctx, resolver)
	if err != nil {
		return nil, staging.ResolvedScope{}, err
	}

	store, err := file.NewWorkingStore(resolved.Scope)
	if err != nil {
		return nil, staging.ResolvedScope{}, fmt.Errorf("failed to create staging store: %w", err)
	}

	return store, resolved, nil
}

// WorkingStore resolves the staging scope via the resolver (default AWS) and
// opens the working store keyed by that scope. It is the exported entry point
// used by the provider-wide (all-service) stage commands.
func WorkingStore(ctx context.Context, resolver staging.ScopeResolver) (*file.Store, staging.ResolvedScope, error) {
	return workingStore(ctx, resolver)
}
