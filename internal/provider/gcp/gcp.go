// Package gcp wires the Google Cloud Secret Manager adapter into a
// provider.Factory / provider.Registry. It builds a Secret Manager client from
// Application Default Credentials and hands it to the secret subpackage.
//
// Google Cloud offers no parameter store, so the factory returns
// provider.ErrUnsupportedKind for KindParam.
package gcp

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/gcp/secret"
)

// Factory builds Google Cloud-backed provider.Store values for a scope + kind.
type Factory struct{}

// Compile-time assertion that Factory implements provider.Factory.
var _ provider.Factory = Factory{}

// Store builds a Store for the given scope and kind. Google Cloud supports only
// KindSecret; KindParam yields provider.ErrUnsupportedKind. The Secret Manager
// client authenticates via Application Default Credentials.
func (Factory) Store(ctx context.Context, scope provider.Scope, kind provider.Kind) (provider.Store, error) {
	switch kind {
	case provider.KindSecret:
		client, err := secretmanager.NewClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create Google Cloud Secret Manager client: %w", err)
		}

		return secret.New(secret.Wrap(client), scope.ProjectID), nil
	case provider.KindParam:
		return nil, fmt.Errorf("%w: %s (Google Cloud has no parameter store)", provider.ErrUnsupportedKind, kind)
	default:
		return nil, fmt.Errorf("%w: %s", provider.ErrUnsupportedKind, kind)
	}
}

// Register associates the Google Cloud Factory with provider.ProviderGoogleCloud in reg.
func Register(reg *provider.Registry) {
	reg.Register(provider.ProviderGoogleCloud, Factory{})
}

// NewRegistry returns a provider.Registry with the Google Cloud provider registered.
func NewRegistry() *provider.Registry {
	reg := provider.NewRegistry()
	Register(reg)

	return reg
}
