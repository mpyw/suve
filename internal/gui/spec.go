//go:build production || dev

package gui

import (
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version"
)

// Per-provider version-spec parsing.
//
// The grammar that is legal depends on the active provider: parsing every input
// with the AWS grammar would, for example, split an Azure App Configuration key
// that legally contains '#' or '~' into a bogus name+version. These helpers pick
// the provider's grammar (mirroring the CLI's per-provider parsers) and return
// the name plus the rebuilt version suffix the neutral usecases hand to
// provider.Reader.Resolve, which re-parses it with the same grammar.

// parseParamSpec splits a parameter version spec into its name and version
// suffix with the grammar of the active provider.
//
//   - AWS   -> version.ParameterStore (name#N~shift).
//   - Azure -> version.AppConfiguration: App Configuration is unversioned, so
//     the whole argument is the key and a key containing '#'/'~' is not
//     mis-split.
//
//declscope:package // shared with the param namespace
func (a *App) parseParamSpec(specStr string) (name, suffix string, err error) {
	switch a.currentScope().Provider {
	case provider.ProviderAzure:
		spec, err := version.AppConfiguration.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, "", nil
	default:
		spec, err := version.ParameterStore.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, version.ParameterStore.Suffix(spec), nil
	}
}

// parseSecretSpec splits a secret version spec into its name and version suffix
// with the grammar of the active provider.
//
//   - AWS          -> version.SecretsManager (name#id | :label, plus ~shift).
//   - Google Cloud -> version.SecretManager (integer #N, ~shift; ':' labels rejected).
//   - Azure        -> version.KeyVault (opaque #id, ~shift; ':' labels rejected).
//
//declscope:package // shared with the secret namespace
func (a *App) parseSecretSpec(specStr string) (name, suffix string, err error) {
	switch a.currentScope().Provider {
	case provider.ProviderGoogleCloud:
		spec, err := version.SecretManager.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, version.SecretManager.Suffix(spec), nil
	case provider.ProviderAzure:
		spec, err := version.KeyVault.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, version.KeyVault.Suffix(spec), nil
	default:
		spec, err := version.SecretsManager.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, version.SecretsManager.Suffix(spec), nil
	}
}
