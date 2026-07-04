// Package paramopts builds AWS Parameter Store provider write options from CLI
// flag values, keeping the flag-to-option mapping in one place shared by the
// param create and update commands. It also validates the --tier value.
package paramopts

import (
	"fmt"
	"slices"

	"github.com/mpyw/suve/internal/provider"
	awsparam "github.com/mpyw/suve/internal/provider/aws/param"
)

// SSM parameter tier names accepted by the --tier flag.
const (
	TierStandard           = "Standard"
	TierAdvanced           = "Advanced"
	TierIntelligentTiering = "Intelligent-Tiering"
)

// ValidateTier reports an error if tier is a non-empty, unrecognized value. An
// empty tier is valid (it means "leave the tier unset").
func ValidateTier(tier string) error {
	validTiers := []string{TierStandard, TierAdvanced, TierIntelligentTiering}
	if tier == "" || slices.Contains(validTiers, tier) {
		return nil
	}

	return fmt.Errorf("invalid --tier %q (want one of %s, %s, %s)", tier, TierStandard, TierAdvanced, TierIntelligentTiering)
}

// Values holds the raw flag values for the provider-specific param options.
type Values struct {
	Tier           string
	DataType       string
	AllowedPattern string
	Policies       string
}

// Build converts the set (non-empty) flag values into provider.WriteOptions.
// Empty values contribute no option, so passing an all-empty Values yields nil
// and preserves the exact behavior of the command when no flags are set.
func Build(v Values) []provider.WriteOption {
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
