//go:build production || dev

package gui

import (
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/awsparamversion"
	"github.com/mpyw/suve/internal/version/awssecretversion"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
	"github.com/mpyw/suve/internal/version/azurekvversion"
	"github.com/mpyw/suve/internal/version/gcloudversion"
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
//   - AWS   -> awsparamversion (name#N~shift).
//   - Azure -> App Configuration is unversioned; azureappconfigversion accepts a
//     bare name only and rejects any specifier, so a key containing '#'/'~' gets
//     a clean error instead of a mis-split.
//
//declscope:package // shared with the param namespace
func (a *App) parseParamSpec(specStr string) (name, suffix string, err error) {
	switch a.currentScope().Provider {
	case provider.ProviderAzure:
		spec, err := azureappconfigversion.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, "", nil
	default:
		spec, err := awsparamversion.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, awsparamversion.Suffix(spec), nil
	}
}

// parseSecretSpec splits a secret version spec into its name and version suffix
// with the grammar of the active provider.
//
//   - AWS          -> awssecretversion (name#id | :label, plus ~shift).
//   - Google Cloud -> gcloudversion (integer #N, ~shift; ':' labels rejected).
//   - Azure        -> azurekvversion (opaque #id, ~shift; ':' labels rejected).
//
//declscope:package // shared with the secret namespace
func (a *App) parseSecretSpec(specStr string) (name, suffix string, err error) {
	switch a.currentScope().Provider {
	case provider.ProviderGoogleCloud:
		spec, err := gcloudversion.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, gcloudversion.Suffix(spec), nil
	case provider.ProviderAzure:
		spec, err := azurekvversion.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, azurekvversion.Suffix(spec), nil
	default:
		spec, err := awssecretversion.Parse(specStr)
		if err != nil {
			return "", "", err
		}

		return spec.Name, awssecretversion.Suffix(spec), nil
	}
}
