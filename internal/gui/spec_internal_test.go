//go:build production || dev

// These are spec.go's in-package tests.
//declscope:namespace spec

package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version"
)

func TestApp_parseParamSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   provider.Provider
		input      string
		wantName   string
		wantSuffix string
		wantErr    error
	}{
		{
			name: "aws bare name", provider: provider.ProviderAWS,
			input: "/my/param", wantName: "/my/param",
		},
		{
			name: "aws with version", provider: provider.ProviderAWS,
			input: "/my/param#3", wantName: "/my/param", wantSuffix: "#3",
		},
		{
			name: "aws with shift", provider: provider.ProviderAWS,
			input: "/my/param~~", wantName: "/my/param", wantSuffix: "~2",
		},
		{
			name: "azure bare key", provider: provider.ProviderAzure,
			input: "my-key", wantName: "my-key",
		},
		{
			name: "azure key with slashes", provider: provider.ProviderAzure,
			input: "app/config/timeout", wantName: "app/config/timeout",
		},
		{
			// The #353 regression: '#' in an App Configuration key must NOT be
			// split into name+version (as the AWS grammar would); the whole
			// argument is the key.
			name: "azure key containing hash is kept whole", provider: provider.ProviderAzure,
			input: "my-key#3", wantName: "my-key#3",
		},
		{
			name: "azure key with tilde is kept whole", provider: provider.ProviderAzure,
			input: "my-key~1", wantName: "my-key~1",
		},
		{
			name: "azure ASP.NET-style colon key is kept whole", provider: provider.ProviderAzure,
			input: "Logging:LogLevel:Default", wantName: "Logging:LogLevel:Default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appWithProvider(tt.provider)

			name, suffix, err := app.parseParamSpec(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantSuffix, suffix)
		})
	}
}

func TestApp_parseSecretSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   provider.Provider
		input      string
		wantName   string
		wantSuffix string
		wantErr    error
	}{
		{
			name: "aws version id", provider: provider.ProviderAWS,
			input: "sec#abc123", wantName: "sec", wantSuffix: "#abc123",
		},
		{
			name: "aws staging label", provider: provider.ProviderAWS,
			input: "sec:AWSCURRENT", wantName: "sec", wantSuffix: ":AWSCURRENT",
		},
		{
			// The Secret Manager adapter re-parses "#3" as integer version 3.
			name: "google cloud integer version", provider: provider.ProviderGoogleCloud,
			input: "sec#3", wantName: "sec", wantSuffix: "#3",
		},
		{
			name: "google cloud with shift", provider: provider.ProviderGoogleCloud,
			input: "sec#5~2", wantName: "sec", wantSuffix: "#5~2",
		},
		{
			// Google Cloud has no staging labels: a colon specifier must be
			// rejected, not folded into the name (which the AWS grammar accepts).
			name: "google cloud label rejected", provider: provider.ProviderGoogleCloud,
			input: "sec:prod", wantErr: version.ErrSecretManagerLabelUnsupported,
		},
		{
			name: "azure key vault opaque id", provider: provider.ProviderAzure,
			input: "sec#deadbeef01", wantName: "sec", wantSuffix: "#deadbeef01",
		},
		{
			name: "azure with shift", provider: provider.ProviderAzure,
			input: "sec~", wantName: "sec", wantSuffix: "~1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := appWithProvider(tt.provider)

			name, suffix, err := app.parseSecretSpec(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantSuffix, suffix)
		})
	}
}
