//go:build production || dev

package gui

import (
	"strconv"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version/azureappconfigversion"
	"github.com/mpyw/suve/internal/version/azurekvversion"
	"github.com/mpyw/suve/internal/version/gcloudversion"
	"github.com/mpyw/suve/internal/version/paramversion"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// Per-provider version-spec parsing.
//
// The param/secret usecases the GUI drives are typed to *paramversion.Spec /
// *secretversion.Spec, but the grammar that is legal depends on the active
// provider. Parsing every input with the AWS grammar mis-handles other
// providers — most visibly it splits an Azure App Configuration key that
// legally contains '#' or '~' into a bogus name+version. These helpers select
// the correct grammar (mirroring the CLI's per-provider parsers) and then adapt
// the result back into the usecase's spec type.
//
// The adaptation round-trips through the neutral name+suffix contract: the
// usecase reconstructs a suffix string from the returned spec and hands
// name+suffix to provider.Reader.Resolve, which re-parses it with the
// provider's OWN grammar (see each adapter's Resolve). So carrying the parsed
// name and an equivalent absolute/shift in a paramversion/secretversion spec is
// sufficient — the suffix bytes are grammar-agnostic (#id / :label / ~N).

// parseParamSpec parses a parameter version spec with the grammar of the active
// provider and adapts it to the *paramversion.Spec the param usecase expects.
//
//   - AWS   -> paramversion (name#N~shift).
//   - Azure -> App Configuration is unversioned; azureappconfigversion accepts a
//     bare name only and rejects any specifier, so a key containing '#'/'~' gets
//     a clean error instead of a mis-split.
func (a *App) parseParamSpec(specStr string) (*paramversion.Spec, error) {
	switch a.currentScope().Provider {
	case provider.ProviderAzure:
		spec, err := azureappconfigversion.Parse(specStr)
		if err != nil {
			return nil, err
		}

		// App Configuration has no absolute/shift specifier, so the equivalent
		// paramversion spec is a bare name (empty suffix).
		return &paramversion.Spec{Name: spec.Name}, nil
	default:
		return paramversion.Parse(specStr)
	}
}

// parseSecretSpec parses a secret version spec with the grammar of the active
// provider and adapts it to the *secretversion.Spec the secret usecase expects.
//
//   - AWS          -> secretversion (name#id | :label, plus ~shift).
//   - Google Cloud -> gcloudversion (integer #N, ~shift; ':' labels rejected).
//   - Azure        -> azurekvversion (opaque #id, ~shift; ':' labels rejected).
func (a *App) parseSecretSpec(specStr string) (*secretversion.Spec, error) {
	switch a.currentScope().Provider {
	case provider.ProviderGoogleCloud:
		spec, err := gcloudversion.Parse(specStr)
		if err != nil {
			return nil, err
		}

		out := &secretversion.Spec{Name: spec.Name, Shift: spec.Shift}
		if spec.Absolute.Version != nil {
			out.Absolute.ID = lo.ToPtr(strconv.FormatInt(*spec.Absolute.Version, 10))
		}

		return out, nil
	case provider.ProviderAzure:
		spec, err := azurekvversion.Parse(specStr)
		if err != nil {
			return nil, err
		}

		return &secretversion.Spec{
			Name:     spec.Name,
			Absolute: secretversion.AbsoluteSpec{ID: spec.Absolute.ID},
			Shift:    spec.Shift,
		}, nil
	default:
		return secretversion.Parse(specStr)
	}
}
