// writeoption.go builds AWS Parameter Store provider write options from CLI flag
// values, keeping the flag-to-option mapping in one place shared by the create
// and update commands. It also validates the --tier value.

package param

import (
	"fmt"
	"slices"

	"github.com/mpyw/suve/internal/provider"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
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
//declscope:package // create.go and update.go validate --tier with it
func validateWriteOptionTier(tier string) error {
	validTiers := []string{writeOptionTierStandard, writeOptionTierAdvanced, writeOptionTierIntelligentTiering}
	if tier == "" || slices.Contains(validTiers, tier) {
		return nil
	}

	return fmt.Errorf("invalid --tier %q (want one of %s, %s, %s)",
		tier, writeOptionTierStandard, writeOptionTierAdvanced, writeOptionTierIntelligentTiering)
}

// WriteOptionFlags holds the raw flag values for the provider-specific param options.
type WriteOptionFlags struct {
	Tier           string
	DataType       string
	AllowedPattern string
	Policies       string
}

// buildWriteOptions converts the set (non-empty) flag values into
// provider.WriteOptions. Empty values contribute no option, so passing an
// all-empty WriteOptionFlags yields nil and preserves the exact behavior of the
// command when no flags are set.
//
//declscope:package // create.go and update.go build their write options with it
func buildWriteOptions(v WriteOptionFlags) []provider.WriteOption {
	var opts []provider.WriteOption

	if v.Tier != "" {
		opts = append(opts, awsparam.Tier{Value: v.Tier})
	}

	if v.DataType != "" {
		opts = append(opts, awsparam.DataType{Value: v.DataType})
	}

	if v.AllowedPattern != "" {
		opts = append(opts, awsparam.AllowedPattern{Value: v.AllowedPattern})
	}

	if v.Policies != "" {
		opts = append(opts, awsparam.Policies{JSON: v.Policies})
	}

	return opts
}
