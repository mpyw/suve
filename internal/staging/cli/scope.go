// The scope/store plumbing (resolveScope, workingStore) is part of the
// package's shared command-building layer.
//declscope:core

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

// workingStore resolves the staging scope via the resolver and
// opens the working store keyed by that scope.
//
//declscope:package // global.go opens each service's own working store for the all-service commands
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
