//go:build production || dev

package gui

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

// clearDetectEnv makes provider detection hermetic: it blanks every env var the
// resolver reads and points the AWS shared-credentials path at a non-existent
// file, so the ~/.aws/credentials fallback stays off regardless of the ambient
// machine. A subtest then sets only the vars its case needs.
func clearDetectEnv(t *testing.T) {
	t.Helper()

	for _, k := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_VAULT", "AWS_PROFILE",
		"GOOGLE_CLOUD_PROJECT", "AZURE_KEYVAULT_NAME", "AZURE_APPCONFIG_NAME",
	} {
		t.Setenv(k, "")
	}

	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "no-such-credentials"))
}

func TestUniqueActiveProvider(t *testing.T) {
	t.Parallel()

	aws := provider.ProviderAWS
	gcloud := provider.ProviderGoogleCloud
	az := provider.ProviderAzure

	tests := []struct {
		name   string
		result detect.Result
		want   provider.Provider
	}{
		{
			name:   "none active -> empty",
			result: detect.Result{},
			want:   "",
		},
		{
			name:   "single provider (secret only) -> that provider",
			result: detect.Result{SecretActive: []provider.Provider{gcloud}},
			want:   gcloud,
		},
		{
			name:   "single provider (param only) -> that provider",
			result: detect.Result{ParamActive: []provider.Provider{az}},
			want:   az,
		},
		{
			name: "same provider across both services -> that provider",
			result: detect.Result{
				ParamActive:  []provider.Provider{aws},
				SecretActive: []provider.Provider{aws},
			},
			want: aws,
		},
		{
			name: "two distinct providers -> empty (ambiguous)",
			result: detect.Result{
				ParamActive:  []provider.Provider{aws},
				SecretActive: []provider.Provider{gcloud},
			},
			want: "",
		},
		{
			name:   "two active in one service -> empty (ambiguous)",
			result: detect.Result{SecretActive: []provider.Provider{aws, az}},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := uniqueActiveProvider(tt.result); got != tt.want {
				t.Errorf("uniqueActiveProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDetectProviders covers the DetectProviders binding (and providerStrings):
// it projects detect.Resolve over the ambient env into the frontend DTO — the
// uniquely-active provider per service (empty when 0 or 2+ are active) plus the
// full active sets, in the resolver's stable order.
func TestDetectProviders(t *testing.T) {
	const (
		aws    = "aws"
		gcloud = "googlecloud"
		az     = "azure"
	)

	tests := []struct {
		name string
		env  map[string]string
		want DetectResult
	}{
		{
			name: "aws via env: param+secret+stage all aws",
			env:  map[string]string{"AWS_ACCESS_KEY_ID": "AKIA"},
			want: DetectResult{
				Param: aws, Secret: aws, Stage: aws,
				ParamActive: []string{aws}, SecretActive: []string{aws}, StageActive: []string{aws},
			},
		},
		{
			name: "google cloud: secret+stage only (no param store)",
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "proj"},
			want: DetectResult{
				Secret: gcloud, Stage: gcloud,
				ParamActive: []string{}, SecretActive: []string{gcloud}, StageActive: []string{gcloud},
			},
		},
		{
			name: "azure app config: param+stage only (no key vault)",
			env:  map[string]string{"AZURE_APPCONFIG_NAME": "store"},
			want: DetectResult{
				Param: az, Stage: az,
				ParamActive: []string{az}, SecretActive: []string{}, StageActive: []string{az},
			},
		},
		{
			name: "two param providers -> ambiguous param, unique secret stays aws",
			env:  map[string]string{"AWS_ACCESS_KEY_ID": "AKIA", "AZURE_APPCONFIG_NAME": "store"},
			want: DetectResult{
				Param: "", Secret: aws, Stage: "",
				ParamActive: []string{aws, az}, SecretActive: []string{aws}, StageActive: []string{aws, az},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearDetectEnv(t)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			app := &App{}
			got := app.DetectProviders()
			require.NotNil(t, got)
			assert.Equal(t, &tt.want, got)
		})
	}
}

// TestInitialProviderFromEnv covers the standalone env-derived initial provider:
// the sole provider active across services, or "" when zero or two-plus are
// active (the frontend then shows the selector).
func TestInitialProviderFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want provider.Provider
	}{
		{
			name: "nothing active -> empty",
			env:  nil,
			want: "",
		},
		{
			name: "single provider (google cloud) -> that provider",
			env:  map[string]string{"GOOGLE_CLOUD_PROJECT": "proj"},
			want: provider.ProviderGoogleCloud,
		},
		{
			name: "aws across both services -> aws",
			env:  map[string]string{"AWS_ACCESS_KEY_ID": "AKIA"},
			want: provider.ProviderAWS,
		},
		{
			name: "two distinct providers -> empty (ambiguous)",
			env:  map[string]string{"AWS_ACCESS_KEY_ID": "AKIA", "GOOGLE_CLOUD_PROJECT": "proj"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearDetectEnv(t)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			assert.Equal(t, tt.want, InitialProviderFromEnv())
		})
	}
}

func TestProviderStrings(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		[]string{"aws", "azure"},
		providerStrings([]provider.Provider{provider.ProviderAWS, provider.ProviderAzure}))
	assert.Empty(t, providerStrings(nil))
}

// TestApp_InitialProvider covers the launch-provider accessor: the provider the
// GUI was launched with, surfaced verbatim for the frontend's initial selection
// (empty when no explicit provider was chosen).
func TestApp_InitialProvider(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "azure", (&App{initialProvider: provider.ProviderAzure}).InitialProvider())
	assert.Empty(t, (&App{}).InitialProvider())
}
