// Package aws wires the AWS parameter and secret adapters into a
// provider.Factory / provider.Registry. It builds SSM and Secrets Manager
// clients from the AWS config (honoring the scope's region) and hands them to
// the per-service adapters in the param and secret subpackages.
//
// This package is additive: it makes the AWS provider available behind the
// #199 interfaces without rewiring existing commands (that migration is #204).
package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/infra"
	"github.com/mpyw/suve/internal/provider/aws/param"
	"github.com/mpyw/suve/internal/provider/aws/secret"
)

// Factory builds AWS-backed provider.Store values for a scope + kind.
type Factory struct{}

// Compile-time assertion that Factory implements provider.Factory.
var _ provider.Factory = Factory{}

// Store builds a Store for the given scope and kind. It returns
// provider.ErrUnsupportedKind for kinds AWS does not offer.
func (Factory) Store(ctx context.Context, scope provider.Scope, kind provider.Kind) (provider.Store, error) {
	cfg, err := infra.LoadConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	if scope.Region != "" {
		cfg.Region = scope.Region
	}

	switch kind {
	case provider.KindParam:
		return param.New(ssm.NewFromConfig(cfg)), nil
	case provider.KindSecret:
		return secret.New(secretsmanager.NewFromConfig(cfg)), nil
	default:
		return nil, fmt.Errorf("%w: %s", provider.ErrUnsupportedKind, kind)
	}
}

// Register associates the AWS Factory with provider.ProviderAWS in reg.
func Register(reg *provider.Registry) {
	reg.Register(provider.ProviderAWS, Factory{})
}

// NewRegistry returns a provider.Registry with the AWS provider registered.
func NewRegistry() *provider.Registry {
	reg := provider.NewRegistry()
	Register(reg)

	return reg
}
