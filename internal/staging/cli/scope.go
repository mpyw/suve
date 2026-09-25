package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// errNoScopeResolver is returned when a command config carries no
// ScopeResolver. Every provider must set one: there is no default provider.
var errNoScopeResolver = errors.New("staging scope resolver is not configured")

// resolveScope runs the resolver. A nil resolver is a wiring bug and fails with
// errNoScopeResolver rather than falling back to any provider.
//
//declscope:package // export.go resolves the scope without opening the working store
func resolveScope(ctx context.Context, resolver staging.ScopeResolver) (staging.ResolvedScope, error) {
	if resolver == nil {
		return staging.ResolvedScope{}, errNoScopeResolver
	}

	return resolver(ctx)
}

// openScopedWorkingStore resolves the staging scope via the resolver and
// opens the working store keyed by that scope.
//
//declscope:package // global.go opens each service's own working store for the all-service commands
func openScopedWorkingStore(ctx context.Context, resolver staging.ScopeResolver) (*file.Store, staging.ResolvedScope, error) {
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

// sharedScopeID tells the resolvers built by SharedScopeResolver apart. It is
// not zero-sized, so each one gets its own address.
type sharedScopeID struct{ _ byte }

// sharedScopeResult is one memoized SharedScopeResolver run.
type sharedScopeResult struct {
	resolved staging.ResolvedScope
	err      error
}

// sharedScopeMemoKey is the context key of the per-command memo that
// withSharedScopeMemo installs.
type sharedScopeMemoKey struct{}

// SharedScopeResolver wraps a resolver that several services of one provider
// share (AWS Parameter Store and Secrets Manager resolve one account scope from
// the STS caller identity). Within one all-service command it runs once, and
// the other services reuse its result (or error). Outside those commands it
// just calls resolver.
func SharedScopeResolver(resolver staging.ScopeResolver) staging.ScopeResolver {
	id := &sharedScopeID{}

	return func(ctx context.Context) (staging.ResolvedScope, error) {
		memo, ok := ctx.Value(sharedScopeMemoKey{}).(map[*sharedScopeID]sharedScopeResult)
		if !ok {
			return resolver(ctx)
		}

		if r, ok := memo[id]; ok {
			return r.resolved, r.err
		}

		resolved, err := resolver(ctx)
		memo[id] = sharedScopeResult{resolved: resolved, err: err}

		return resolved, err
	}
}

// withSharedScopeMemo returns ctx carrying a fresh memo for the
// SharedScopeResolver resolvers, so each one runs once in that context. The
// memo is not safe for concurrent use: the services are resolved in turn.
//
//declscope:package // global.go resolves every service of an all-service command under one memo
func withSharedScopeMemo(ctx context.Context) context.Context {
	return context.WithValue(ctx, sharedScopeMemoKey{}, map[*sharedScopeID]sharedScopeResult{})
}
