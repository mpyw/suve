// aws.go is this package's subject: the AWS provider.Factory and its
// registration. Core, so that aws.Factory does not have to become aws.AwsFactory.
//declscope:core

// Package aws wires the AWS Parameter Store and Secrets Manager adapters into a
// provider.Factory / provider.Registry. It loads the AWS config (config.go, with
// the SDK debug logger in debug.go), resolves the caller identity (identity.go,
// with the ~/.aws/config profile lookup in profile.go), builds SSM and Secrets
// Manager clients from that config (honoring the scope's region), and hands them
// to the per-product adapters in the parameterstore and secretsmanager
// subpackages.
package aws

import (
	"context"
	"fmt"

	secretsmanagersdk "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/parameterstore"
	"github.com/mpyw/suve/internal/provider/aws/secretsmanager"
)

// Factory builds AWS-backed provider.Store values for a scope + kind.
type Factory struct{}

// Compile-time assertion that Factory implements provider.Factory.
var _ provider.Factory = Factory{}

// Store builds a Store for the given scope and kind. It returns
// provider.ErrUnsupportedKind for kinds AWS does not offer.
func (Factory) Store(ctx context.Context, scope provider.Scope, kind provider.Kind) (provider.Store, error) {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	if scope.Region != "" {
		cfg.Region = scope.Region
	}

	switch kind {
	case provider.KindParam:
		return parameterstore.New(ssm.NewFromConfig(cfg)), nil
	case provider.KindSecret:
		return secretsmanager.New(secretsmanagersdk.NewFromConfig(cfg)), nil
	default:
		return nil, fmt.Errorf("%w: %s", provider.ErrUnsupportedKind, kind)
	}
}

// Register associates the AWS Factory with provider.ProviderAWS in reg.
func Register(reg *provider.Registry) {
	reg.Register(provider.ProviderAWS, Factory{})
}
