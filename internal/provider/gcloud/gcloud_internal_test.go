package gcloud

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/mpyw/suve/internal/debug"
)

func TestGRPCStatus(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "ok", grpcStatus(nil))
	assert.Equal(t, "failed: boom", grpcStatus(errors.New("boom")))
}

func TestResourceHint(t *testing.T) {
	t.Parallel()

	// Real request shapes: name-addressed (Access/Get) and parent-addressed
	// (List/Create). Both carry only resource paths, never payloads.
	assert.Equal(t, "projects/p/secrets/s/versions/1",
		resourceHint(&secretmanagerpb.AccessSecretVersionRequest{Name: "projects/p/secrets/s/versions/1"}))
	assert.Equal(t, "projects/p",
		resourceHint(&secretmanagerpb.ListSecretsRequest{Parent: "projects/p"}))
	assert.Empty(t, resourceHint(&secretmanagerpb.ListSecretsRequest{}))
	assert.Empty(t, resourceHint(struct{}{}))
}

func TestDebugDialOptions_disabled(t *testing.T) {
	t.Parallel()

	assert.Nil(t, debugDialOptions(context.Background()))
}

func TestDebugDialOptions_enabled(t *testing.T) {
	t.Parallel()

	ctx := debug.With(context.Background(), debug.Config{Enabled: true, Writer: &bytes.Buffer{}})
	assert.Len(t, debugDialOptions(ctx), 1)
}

func TestDebugUnaryInterceptor(t *testing.T) {
	t.Parallel()

	// A lazily-created client connection: grpc.NewClient does not dial, so this
	// works offline and only exists to give the interceptor a real cc.Target().
	cc, err := grpc.NewClient("passthrough:///unit-test", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })

	tests := []struct {
		name       string
		req        any
		invokerErr error
		wantSubstr []string
	}{
		{
			name:       "ok with resource hint",
			req:        &secretmanagerpb.ListSecretsRequest{Parent: "projects/p"},
			invokerErr: nil,
			wantSubstr: []string{"gcloud grpc: /pkg.Svc/Method projects/p (", "ok in"},
		},
		{
			name:       "error without hint",
			req:        nil,
			invokerErr: errors.New("boom"),
			wantSubstr: []string{"gcloud grpc: /pkg.Svc/Method (", "failed: boom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			interceptor := debugUnaryInterceptor(debug.Config{Enabled: true, Writer: &buf})
			invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
				return tt.invokerErr
			}
			err := interceptor(context.Background(), "/pkg.Svc/Method", tt.req, nil, cc, invoker)

			assert.Equal(t, tt.invokerErr, err)

			for _, want := range tt.wantSubstr {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestDebugGRPCDialOptions(t *testing.T) {
	t.Parallel()

	// Disabled: nil, so the emulator path adds nothing.
	assert.Nil(t, debugGRPCDialOptions(context.Background()))

	// Enabled: one chained-interceptor option, shared by both client paths.
	ctx := debug.With(context.Background(), debug.Config{Enabled: true, Writer: &bytes.Buffer{}})
	assert.Len(t, debugGRPCDialOptions(ctx), 1)
}
