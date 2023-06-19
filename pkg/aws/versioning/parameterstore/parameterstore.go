package parameterstore

import (
	"strconv"

	"github.com/mpyw/suve/pkg/aws/versioning/internal"
	"github.com/mpyw/suve/pkg/core/versioning"
)

var VersionParser versioning.VersionParserFunc = parseVersion

func parseVersion(version string) (versioning.VersionRequirement, error) {
	result, err := internal.ParseNumberOfShift(version)
	if err != nil {
		return nil, err
	}
	var absolute versioning.AbsoluteVersionRequirement
	if n, err := strconv.Atoi(result.BaseVersion); err == nil {
		absolute = versioning.AbsoluteVersionRequirement{
			Type: versioning.VersionRequirementTypeCanonical,
			CanonicalValue: versioning.CanonicalVersion{
				Type:        versioning.CanonicalVersionTypeNumber,
				NumberValue: int64(n),
			},
		}
	} else {
		absolute = versioning.AbsoluteVersionRequirement{
			Type:              versioning.VersionRequirementTypeStageOrLabel,
			StageOrLabelValue: result.BaseVersion,
		}
	}
	if result.NumberOfShift < 1 {
		return absolute, nil
	}
	return versioning.RelativeVersionRequirement{
		AbsoluteVersionRequirement: absolute,
		NumberOfShift:              result.NumberOfShift,
	}, nil
}
