// Package detect resolves which cloud provider should back the flat `param` /
// `secret` command aliases and a bare `suve --tui` / `suve --gui` launch, and
// hydrates a launch scope's resource fields, based purely on environment
// variables. It performs no network calls and no
// credential-chain resolution, so it is safe to run on every process start
// (including on the shell-completion path, where speed matters).
//
// The rules (per service, param and secret decided independently):
//
//   - A provider is "active via env" when one of its address/identity/ambient
//     env vars is set:
//     AWS — AWS_ACCESS_KEY_ID | AWS_VAULT | AWS_PROFILE, or an ambient-credential
//     marker set by AWS-managed compute: AWS_CONTAINER_CREDENTIALS_FULL_URI
//     (CloudShell, App Runner, EKS Pod Identity), AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
//     (ECS), or AWS_WEB_IDENTITY_TOKEN_FILE (IRSA / EKS). This is what makes the
//     flat aliases work in AWS CloudShell, where none of the classic vars are set.
//     GoogleCloud — GOOGLE_CLOUD_PROJECT (secret only)
//     Azure — AZURE_KEYVAULT_NAME (secret) / AZURE_APPCONFIG_NAME (param)
//   - A flat alias is exposed for a service only when exactly ONE provider is
//     active for it. Zero or two-plus active means no alias — the user must use
//     the explicit group (e.g. `suve aws secret`). There is no priority order.
//   - AWS-only final fallback: when NO provider is active via env for any
//     service, AWS is accepted via ~/.aws/credentials so the common "plain AWS"
//     setup keeps working. If that file is absent too, nothing is aliased.
//
// Deliberately NOT detected: an EC2 instance profile with no credentials env var
// and no shared file delivers credentials only through IMDS, which has no env
// marker and would require a network probe. Running suve from a bare EC2 host is
// rare; there the explicit `suve aws ...` group still works.
package detect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/samber/lo"

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
	// secret), Google Cloud (secret), and Azure (Key Vault secret / App
	// Configuration param).
	Stage provider.Provider

	// ParamActive and SecretActive list every provider active for that service,
	// in stable order (AWS, GoogleCloud, Azure).
	ParamActive  []provider.Provider
	SecretActive []provider.Provider
	// StageActive lists every staging-capable provider active, in stable order
	// (AWS, GoogleCloud, Azure).
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
		getenv("AWS_PROFILE") != "" ||
		// Ambient credentials from AWS-managed compute (no classic env var):
		// CloudShell / App Runner / EKS Pod Identity, ECS, and IRSA / EKS.
		getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" ||
		getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
		getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != ""
	gcloudSecret := getenv("GOOGLE_CLOUD_PROJECT") != ""
	azureSecret := getenv("AZURE_KEYVAULT_NAME") != ""
	azureParam := getenv("AZURE_APPCONFIG_NAME") != ""

	anyEnv := awsEnv || gcloudSecret || azureSecret || azureParam

	var res Result

	awsActive := awsEnv
	if !anyEnv && env.AWSCredentialsExist != nil && env.AWSCredentialsExist() {
		awsActive = true
		res.AWSViaFallback = true
	}

	// Secret candidates in stable order: AWS, GoogleCloud, Azure (Key Vault).
	if awsActive {
		res.SecretActive = append(res.SecretActive, provider.ProviderAWS)
	}

	if gcloudSecret {
		res.SecretActive = append(res.SecretActive, provider.ProviderGoogleCloud)
	}

	if azureSecret {
		res.SecretActive = append(res.SecretActive, provider.ProviderAzure)
	}

	// Param candidates in stable order: AWS, Azure (App Configuration). GoogleCloud has
	// no parameter store.
	if awsActive {
		res.ParamActive = append(res.ParamActive, provider.ProviderAWS)
	}

	if azureParam {
		res.ParamActive = append(res.ParamActive, provider.ProviderAzure)
	}

	// Staging-capable providers in stable order: AWS (param + secret), Google
	// Cloud (secret), Azure (Key Vault secret and/or App Configuration param).
	if awsActive {
		res.StageActive = append(res.StageActive, provider.ProviderAWS)
	}

	if gcloudSecret {
		res.StageActive = append(res.StageActive, provider.ProviderGoogleCloud)
	}

	if azureSecret || azureParam {
		res.StageActive = append(res.StageActive, provider.ProviderAzure)
	}

	res.Secret = unique(res.SecretActive)
	res.Param = unique(res.ParamActive)
	res.Stage = unique(res.StageActive)

	return res
}

// ActiveProviders lists every provider active on any service axis (param,
// secret, or stage), deduplicated, in provider.Providers order.
func (r Result) ActiveProviders() []provider.Provider {
	return lo.Filter(
		provider.Providers(),
		func(p provider.Provider, _ int) bool {
			return slices.Contains(r.ParamActive, p) ||
				slices.Contains(r.SecretActive, p) ||
				slices.Contains(r.StageActive, p)
		},
	)
}

// UniqueProvider returns the sole provider active on any service axis, or ""
// when zero or two-plus are active. It is the "exactly one active provider"
// rule for a bare UI launch (`suve --tui`, `suve --gui`).
func (r Result) UniqueProvider() provider.Provider {
	return unique(r.ActiveProviders())
}

// HydrateScope fills the empty resource fields of a launch scope from the
// environment, so a flag-supplied value wins and an unset one falls back to
// GOOGLE_CLOUD_PROJECT / AZURE_KEYVAULT_NAME / AZURE_APPCONFIG_NAME /
// AZURE_APPCONFIG_NAMESPACE. AWS carries no resource field (the region comes
// from the ambient AWS config). An unknown provider is an error.
func HydrateScope(env Environment, s provider.Scope) (provider.Scope, error) {
	getenv := env.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	fill := func(field *string, name string) {
		if *field == "" {
			*field = getenv(name)
		}
	}

	switch s.Provider {
	case provider.ProviderGoogleCloud:
		fill(&s.ProjectID, "GOOGLE_CLOUD_PROJECT")
	case provider.ProviderAzure:
		fill(&s.VaultName, "AZURE_KEYVAULT_NAME")
		fill(&s.StoreName, "AZURE_APPCONFIG_NAME")
		fill(&s.AppConfigNamespace, "AZURE_APPCONFIG_NAMESPACE")
	case provider.ProviderAWS:
		// The region comes from the ambient AWS config; nothing to hydrate.
	default:
		return provider.Scope{}, fmt.Errorf("%w %q", ErrUnknownProvider, s.Provider)
	}

	return s, nil
}

// ErrUnknownProvider is returned by HydrateScope for an unknown or empty
// provider.
var ErrUnknownProvider = errors.New("unknown provider")

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
