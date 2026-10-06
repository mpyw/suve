// writeoption.go builds AWS Parameter Store provider write options from CLI flag
// values, keeping the flag-to-option mapping in one place shared by the create
// and update commands. It also validates the --tier value.

package param

import (
	"fmt"
	"slices"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/parameterstore"
)

// SSM parameter tier names accepted by the --tier flag.
const (
	writeOptionTierStandard           = "Standard"
	writeOptionTierAdvanced           = "Advanced"
	writeOptionTierIntelligentTiering = "Intelligent-Tiering"
)

// validateWriteOptionTier reports an error if tier is a non-empty,
// unrecognized value. An empty tier is valid (it means "leave the tier unset").
//
//declscope:shared // create.go and update.go validate --tier with it
func validateWriteOptionTier(tier string) error {
	validTiers := []string{writeOptionTierStandard, writeOptionTierAdvanced, writeOptionTierIntelligentTiering}
	if tier == "" || slices.Contains(validTiers, tier) {
		return nil
	}

	return fmt.Errorf("invalid --tier %q (want one of %s, %s, %s)",
		tier, writeOptionTierStandard, writeOptionTierAdvanced, writeOptionTierIntelligentTiering)
}

// writeOptionFlags holds the raw flag values for the provider-specific param options.
//
//declscope:shared // create.go and update.go fill it from the flags
type writeOptionFlags struct {
	tier           string
	dataType       string
	allowedPattern string
	policies       string
}

// buildWriteOptions converts the set (non-empty) flag values into
// provider.WriteOptions. Empty values contribute no option, so passing an
// all-empty writeOptionFlags yields nil and preserves the exact behavior of the
// command when no flags are set.
//
//declscope:shared // create.go and update.go build their write options with it
func buildWriteOptions(v writeOptionFlags) []provider.WriteOption {
	var opts []provider.WriteOption

	if v.tier != "" {
		opts = append(opts, parameterstore.Tier{Value: v.tier})
	}

	if v.dataType != "" {
		opts = append(opts, parameterstore.DataType{Value: v.dataType})
	}

	if v.allowedPattern != "" {
		opts = append(opts, parameterstore.AllowedPattern{Value: v.allowedPattern})
	}

	if v.policies != "" {
		opts = append(opts, parameterstore.Policies{JSON: v.policies})
	}

	return opts
}
