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
	gcloud := provider.ProviderGoogleCloud
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
			name:          "GoogleCloud only -> secret=GoogleCloud, no param",
			vars:          map[string]string{"GOOGLE_CLOUD_PROJECT": "my-proj"},
			wantSecret:    gcloud,
			wantSecretSet: []provider.Provider{gcloud},
			// param stays empty: GoogleCloud has no parameter store
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
			name:          "AWS + GoogleCloud -> secret ambiguous (none), param=AWS",
			vars:          map[string]string{"AWS_PROFILE": "dev", "GOOGLE_CLOUD_PROJECT": "p"},
			wantParam:     aws,
			wantSecret:    "",
			wantParamSet:  []provider.Provider{aws},
			wantSecretSet: []provider.Provider{aws, gcloud},
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
			name:          "GoogleCloud + Azure App Config -> secret=GoogleCloud, param=Azure (each unique)",
			vars:          map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "AZURE_APPCONFIG_NAME": "ac"},
			wantParam:     az,
			wantSecret:    gcloud,
			wantParamSet:  []provider.Provider{az},
			wantSecretSet: []provider.Provider{gcloud},
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
			name:       "GoogleCloud set with creds file present -> no AWS fallback (env is active)",
			vars:       map[string]string{"GOOGLE_CLOUD_PROJECT": "p"},
			credsExist: true,
			wantSecret: gcloud, wantSecretSet: []provider.Provider{gcloud},
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

			// Staging-capable providers, in stable order (AWS, Google Cloud,
			// Azure): AWS/Google Cloud when active for secret; Azure when active
			// for EITHER service. Stage is the unique active one, or "" for 0/2+.
			var wantStageSet []provider.Provider

			if slices.Contains(tt.wantSecretSet, provider.ProviderAWS) {
				wantStageSet = append(wantStageSet, provider.ProviderAWS)
			}

			if slices.Contains(tt.wantSecretSet, provider.ProviderGoogleCloud) {
				wantStageSet = append(wantStageSet, provider.ProviderGoogleCloud)
			}

			if slices.Contains(tt.wantSecretSet, provider.ProviderAzure) ||
				slices.Contains(tt.wantParamSet, provider.ProviderAzure) {
				wantStageSet = append(wantStageSet, provider.ProviderAzure)
			}

			wantStage := provider.Provider("")
			if len(wantStageSet) == 1 {
				wantStage = wantStageSet[0]
			}

			assert.Equal(t, wantStageSet, got.StageActive, "StageActive")
			assert.Equal(t, wantStage, got.Stage, "Stage")
			assert.Equal(t, wantStage != "", got.FlatStage(), "FlatStage")
		})
	}
}
