// Package detect resolves which cloud provider should back the flat `param` /
// `secret` command aliases (and, later, the GUI's initial provider selection),
// based purely on environment variables. It performs no network calls and no
// credential-chain resolution, so it is safe to run on every process start.
//
// The rules (per service, param and secret decided independently):
//
//   - A provider is "active via env" when its address/identity env var is set:
//     AWS     — AWS_ACCESS_KEY_ID | AWS_VAULT | AWS_PROFILE (both services)
//     GCP     — GOOGLE_CLOUD_PROJECT (secret only)
//     Azure   — AZURE_KEYVAULT_NAME (secret) / AZURE_APPCONFIG_NAME (param)
//   - A flat alias is exposed for a service only when exactly ONE provider is
//     active for it. Zero or two-plus active means no alias — the user must use
//     the explicit group (e.g. `suve aws secret`). There is no priority order.
//   - AWS-only final fallback: when NO provider is active via env for any
//     service, AWS is accepted via ~/.aws/credentials so the common "plain AWS"
//     setup keeps working. If that file is absent too, nothing is aliased.
package detect

import (
	"os"
	"path/filepath"

	"github.com/mpyw/suve/internal/provider"
)

// Environment abstracts the inputs the resolver reads, so it can be tested
// without mutating the real process environment or filesystem.
type Environment struct {
	// Getenv reads an environment variable (os.Getenv in production).
	Getenv func(string) string
	// AWSCredentialsExist reports whether the AWS shared credentials file is
	// present. It is only consulted for the final fallback, never eagerly.
	AWSCredentialsExist func() bool
}

// OSEnvironment returns an Environment backed by the real OS.
func OSEnvironment() Environment {
	return Environment{
		Getenv:              os.Getenv,
		AWSCredentialsExist: awsCredentialsExist,
	}
}

// Result holds the resolved flat-alias target for each service plus the full
// active sets (useful for help text and the GUI).
type Result struct {
	// Param and Secret name the single active provider for that service, or an
	// empty Provider ("") when the service is not uniquely resolvable (0 or 2+
	// active) — meaning no flat alias should be exposed for it.
	Param  provider.Provider
	Secret provider.Provider
	// Stage names the single active provider for the staging workflow, or an
	// empty Provider ("") when staging is not uniquely resolvable (0 or 2+
	// staging-capable providers active). Staging is supported for AWS (param +
	// secret) and Google Cloud (secret); Azure is not yet staging-capable.
	Stage provider.Provider

	// ParamActive and SecretActive list every provider active for that service,
	// in stable order (AWS, GCP, Azure).
	ParamActive  []provider.Provider
	SecretActive []provider.Provider
	// StageActive lists every staging-capable provider active, in stable order
	// (AWS, GCP).
	StageActive []provider.Provider

	// AWSViaFallback is true when AWS became active only through the
	// ~/.aws/credentials fallback (no provider was active via env).
	AWSViaFallback bool
}

// FlatParam reports whether a top-level `param` alias should be exposed.
func (r Result) FlatParam() bool { return r.Param != "" }

// FlatSecret reports whether a top-level `secret` alias should be exposed.
func (r Result) FlatSecret() bool { return r.Secret != "" }

// FlatStage reports whether a top-level `stage` alias should be exposed.
func (r Result) FlatStage() bool { return r.Stage != "" }

// Resolve computes the alias targets from the given environment.
func Resolve(env Environment) Result {
	getenv := env.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	awsEnv := getenv("AWS_ACCESS_KEY_ID") != "" ||
		getenv("AWS_VAULT") != "" ||
		getenv("AWS_PROFILE") != ""
	gcpSecret := getenv("GOOGLE_CLOUD_PROJECT") != ""
	azureSecret := getenv("AZURE_KEYVAULT_NAME") != ""
	azureParam := getenv("AZURE_APPCONFIG_NAME") != ""

	anyEnv := awsEnv || gcpSecret || azureSecret || azureParam

	var res Result

	awsActive := awsEnv
	if !anyEnv && env.AWSCredentialsExist != nil && env.AWSCredentialsExist() {
		awsActive = true
		res.AWSViaFallback = true
	}

	// Secret candidates in stable order: AWS, GCP, Azure (Key Vault).
	if awsActive {
		res.SecretActive = append(res.SecretActive, provider.ProviderAWS)
	}

	if gcpSecret {
		res.SecretActive = append(res.SecretActive, provider.ProviderGoogleCloud)
	}

	if azureSecret {
		res.SecretActive = append(res.SecretActive, provider.ProviderAzure)
	}

	// Param candidates in stable order: AWS, Azure (App Configuration). GCP has
	// no parameter store.
	if awsActive {
		res.ParamActive = append(res.ParamActive, provider.ProviderAWS)
	}

	if azureParam {
		res.ParamActive = append(res.ParamActive, provider.ProviderAzure)
	}

	// Staging-capable providers in stable order: AWS (param + secret), Google
	// Cloud (secret). Azure is not yet staging-capable.
	if awsActive {
		res.StageActive = append(res.StageActive, provider.ProviderAWS)
	}

	if gcpSecret {
		res.StageActive = append(res.StageActive, provider.ProviderGoogleCloud)
	}

	res.Secret = unique(res.SecretActive)
	res.Param = unique(res.ParamActive)
	res.Stage = unique(res.StageActive)

	return res
}

// unique returns the sole element of ps, or "" when ps has zero or 2+ elements.
func unique(ps []provider.Provider) provider.Provider {
	if len(ps) == 1 {
		return ps[0]
	}

	return ""
}

// awsCredentialsExist reports whether the AWS shared credentials file is
// present, honoring AWS_SHARED_CREDENTIALS_FILE and falling back to
// ~/.aws/credentials. Existence only — the file is not parsed.
func awsCredentialsExist() bool {
	path := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}

		path = filepath.Join(home, ".aws", "credentials")
	}

	// Path is the user's own AWS credentials location (AWS_SHARED_CREDENTIALS_FILE
	// or ~/.aws/credentials); an existence check on it is intentional.
	info, err := os.Stat(path) //nolint:gosec // user-controlled AWS credentials path by design

	return err == nil && !info.IsDir()
}
