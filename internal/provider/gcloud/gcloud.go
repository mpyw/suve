// Package gcloud wires the Google Cloud Secret Manager adapter into a
// provider.Factory / provider.Registry. It builds a Secret Manager client from
// Application Default Credentials and hands it to the secret subpackage.
//
// Google Cloud offers no parameter store, so the factory returns
// provider.ErrUnsupportedKind for KindParam.
package gcloud

import (
	"context"
	"fmt"
	"os"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/gcloud/secret"
)

// EmulatorEnvVar is the environment variable that, when set (e.g. to
// "localhost:9090"), points the Secret Manager client at a local emulator over
// plaintext gRPC with no authentication. It is a testing seam only: in normal
// use it is unset and the client uses Application Default Credentials over TLS.
// Never set this in production.
const EmulatorEnvVar = "SUVE_GCLOUD_SECRETMANAGER_ENDPOINT"

// grpcStatus renders a unary call's outcome for a debug line without printing
// the reply message (an AccessSecretVersion reply carries the secret payload).
func grpcStatus(err error) string {
	if err != nil {
		return fmt.Sprintf("failed: %v", err)
	}

	return "ok"
}

// resourceHint extracts a safe, non-secret resource identifier from a request
// message: the resource name (Get/Access/Delete/...) or the parent
// (List/Create). Both are `projects/...` paths — never payloads — and they are
// exactly what a user needs to spot a wrong project ID. Unknown shapes yield "".
func resourceHint(req any) string {
	if n, ok := req.(interface{ GetName() string }); ok && n.GetName() != "" {
		return n.GetName()
	}

	if p, ok := req.(interface{ GetParent() string }); ok && p.GetParent() != "" {
		return p.GetParent()
	}

	return ""
}

// debugUnaryInterceptor logs each unary gRPC call's method, resource, target,
// duration and status via cfg. It deliberately never logs the request or reply
// messages themselves — only the resource name/parent (metadata) is printed.
func debugUnaryInterceptor(cfg debug.Config) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)

		hint := resourceHint(req)
		if hint != "" {
			hint = " " + hint
		}

		cfg.Logf("gcloud grpc: %s%s (%s) %s in %s\n", method, hint, cc.Target(), grpcStatus(err), time.Since(start))

		return err
	}
}

// debugGRPCDialOptions returns the raw gRPC dial options that enable request
// logging when debug is active on ctx, or nil otherwise. Both the normal
// (self-dialed) client and the emulator connection use them, so e2e runs
// against the emulator exercise the same debug path as production.
func debugGRPCDialOptions(ctx context.Context) []grpc.DialOption {
	d := debug.From(ctx)
	if !d.Enabled {
		return nil
	}

	return []grpc.DialOption{grpc.WithChainUnaryInterceptor(debugUnaryInterceptor(d))}
}

// debugDialOptions wraps debugGRPCDialOptions for the self-dialing client path.
func debugDialOptions(ctx context.Context) []option.ClientOption {
	grpcOpts := debugGRPCDialOptions(ctx)
	if len(grpcOpts) == 0 {
		return nil
	}

	clientOpts := make([]option.ClientOption, 0, len(grpcOpts))
	for _, opt := range grpcOpts {
		clientOpts = append(clientOpts, option.WithGRPCDialOption(opt))
	}

	return clientOpts
}

// newSecretManagerClient builds the Secret Manager client, honoring the
// emulator seam (EmulatorEnvVar) when set.
func newSecretManagerClient(ctx context.Context) (*secretmanager.Client, error) {
	endpoint := os.Getenv(EmulatorEnvVar)
	if endpoint == "" {
		return secretmanager.NewClient(ctx, debugDialOptions(ctx)...)
	}

	// Emulator: dial plaintext gRPC and skip authentication entirely.
	dialOpts := append(
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		debugGRPCDialOptions(ctx)...,
	)

	conn, err := grpc.NewClient(endpoint, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial Google Cloud Secret Manager emulator at %s: %w", endpoint, err)
	}

	return secretmanager.NewClient(ctx, option.WithGRPCConn(conn), option.WithoutAuthentication())
}

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
		client, err := newSecretManagerClient(ctx)
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
