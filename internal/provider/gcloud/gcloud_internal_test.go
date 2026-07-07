package gcloud

import (
	"bytes"
	"context"
	"errors"
	"testing"

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
		invokerErr error
		wantSubstr string
	}{
		{name: "ok", invokerErr: nil, wantSubstr: "ok in"},
		{name: "error", invokerErr: errors.New("boom"), wantSubstr: "failed: boom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			interceptor := debugUnaryInterceptor(debug.Config{Enabled: true, Writer: &buf})
			invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
				return tt.invokerErr
			}
			err := interceptor(context.Background(), "/pkg.Svc/Method", nil, nil, cc, invoker)

			assert.Equal(t, tt.invokerErr, err)
			assert.Contains(t, buf.String(), "gRPC /pkg.Svc/Method")
			assert.Contains(t, buf.String(), tt.wantSubstr)
		})
	}
}
