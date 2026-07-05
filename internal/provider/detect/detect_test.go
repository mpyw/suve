package detect_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

func env(vars map[string]string, credsExist bool) detect.Environment {
	return detect.Environment{
		Getenv:              func(k string) string { return vars[k] },
		AWSCredentialsExist: func() bool { return credsExist },
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	aws := provider.ProviderAWS
	gcp := provider.ProviderGoogleCloud
	az := provider.ProviderAzure

	tests := []struct {
		name           string
		vars           map[string]string
		credsExist     bool
		wantParam      provider.Provider
		wantSecret     provider.Provider
		wantParamSet   []provider.Provider
		wantSecretSet  []provider.Provider
		wantAWSViaFall bool
	}{
		{
			name:       "nothing set, no credentials file",
			vars:       nil,
			credsExist: false,
			// no aliases at all
		},
		{
			name:           "nothing set, credentials file present -> AWS fallback for both",
			vars:           nil,
			credsExist:     true,
			wantParam:      aws,
			wantSecret:     aws,
			wantParamSet:   []provider.Provider{aws},
			wantSecretSet:  []provider.Provider{aws},
			wantAWSViaFall: true,
		},
		{
			name:          "AWS_ACCESS_KEY_ID set -> AWS both, no fallback",
			vars:          map[string]string{"AWS_ACCESS_KEY_ID": "AKIA..."},
			credsExist:    true, // must be ignored: env is active
			wantParam:     aws,
			wantSecret:    aws,
			wantParamSet:  []provider.Provider{aws},
			wantSecretSet: []provider.Provider{aws},
		},
		{
			name:          "AWS_VAULT set -> AWS both",
			vars:          map[string]string{"AWS_VAULT": "myprofile"},
			wantParam:     aws,
			wantSecret:    aws,
			wantParamSet:  []provider.Provider{aws},
			wantSecretSet: []provider.Provider{aws},
		},
		{
			name:          "AWS_PROFILE set -> AWS both",
			vars:          map[string]string{"AWS_PROFILE": "dev"},
			wantParam:     aws,
			wantSecret:    aws,
			wantParamSet:  []provider.Provider{aws},
			wantSecretSet: []provider.Provider{aws},
		},
		{
			name:          "GCP only -> secret=GCP, no param",
			vars:          map[string]string{"GOOGLE_CLOUD_PROJECT": "my-proj"},
			wantSecret:    gcp,
			wantSecretSet: []provider.Provider{gcp},
			// param stays empty: GCP has no parameter store
		},
		{
			name:          "Azure Key Vault only -> secret=Azure, no param",
			vars:          map[string]string{"AZURE_KEYVAULT_NAME": "kv"},
			wantSecret:    az,
			wantSecretSet: []provider.Provider{az},
		},
		{
			name:         "Azure App Config only -> param=Azure, no secret",
			vars:         map[string]string{"AZURE_APPCONFIG_NAME": "ac"},
			wantParam:    az,
			wantParamSet: []provider.Provider{az},
		},
		{
			name:          "Azure both services -> param=Azure, secret=Azure",
			vars:          map[string]string{"AZURE_KEYVAULT_NAME": "kv", "AZURE_APPCONFIG_NAME": "ac"},
			wantParam:     az,
			wantSecret:    az,
			wantParamSet:  []provider.Provider{az},
			wantSecretSet: []provider.Provider{az},
		},
		{
			name:          "AWS + GCP -> secret ambiguous (none), param=AWS",
			vars:          map[string]string{"AWS_PROFILE": "dev", "GOOGLE_CLOUD_PROJECT": "p"},
			wantParam:     aws,
			wantSecret:    "",
			wantParamSet:  []provider.Provider{aws},
			wantSecretSet: []provider.Provider{aws, gcp},
		},
		{
			name:          "AWS + Azure Key Vault -> secret ambiguous, param=AWS",
			vars:          map[string]string{"AWS_PROFILE": "dev", "AZURE_KEYVAULT_NAME": "kv"},
			wantParam:     aws,
			wantSecret:    "",
			wantParamSet:  []provider.Provider{aws},
			wantSecretSet: []provider.Provider{aws, az},
		},
		{
			name:          "GCP + Azure App Config -> secret=GCP, param=Azure (each unique)",
			vars:          map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "AZURE_APPCONFIG_NAME": "ac"},
			wantParam:     az,
			wantSecret:    gcp,
			wantParamSet:  []provider.Provider{az},
			wantSecretSet: []provider.Provider{gcp},
		},
		{
			name:          "AWS + Azure App Config -> param ambiguous, secret=AWS",
			vars:          map[string]string{"AWS_PROFILE": "dev", "AZURE_APPCONFIG_NAME": "ac"},
			wantParam:     "",
			wantSecret:    aws,
			wantParamSet:  []provider.Provider{aws, az},
			wantSecretSet: []provider.Provider{aws},
		},
		{
			name:       "GCP set with creds file present -> no AWS fallback (env is active)",
			vars:       map[string]string{"GOOGLE_CLOUD_PROJECT": "p"},
			credsExist: true,
			wantSecret: gcp, wantSecretSet: []provider.Provider{gcp},
			// AWS must NOT appear: fallback only when nothing is active via env
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := detect.Resolve(env(tt.vars, tt.credsExist))

			assert.Equal(t, tt.wantParam, got.Param, "Param")
			assert.Equal(t, tt.wantSecret, got.Secret, "Secret")
			assert.Equal(t, tt.wantParamSet, got.ParamActive, "ParamActive")
			assert.Equal(t, tt.wantSecretSet, got.SecretActive, "SecretActive")
			assert.Equal(t, tt.wantAWSViaFall, got.AWSViaFallback, "AWSViaFallback")
			assert.Equal(t, tt.wantParam != "", got.FlatParam(), "FlatParam")
			assert.Equal(t, tt.wantSecret != "", got.FlatSecret(), "FlatSecret")

			// Staging is AWS-only: Stage is AWS exactly when AWS is active,
			// i.e. when AWS appears in the (secret) active set.
			wantStage := provider.Provider("")
			if slices.Contains(tt.wantSecretSet, provider.ProviderAWS) {
				wantStage = provider.ProviderAWS
			}

			assert.Equal(t, wantStage, got.Stage, "Stage")
			assert.Equal(t, wantStage != "", got.FlatStage(), "FlatStage")
		})
	}
}
