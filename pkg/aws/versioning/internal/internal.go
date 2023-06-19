package internal

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/mpyw/suve/pkg/core/versioning"
	"go.uber.org/multierr"
)

type NumberOfShiftParseResult struct {
	BaseVersion   string
	NumberOfShift int64
}

var parseNumberOfShiftRegex = regexp.MustCompile(`^(.+)[~^](\d+)$`)

func ParseNumberOfShift(version string) (NumberOfShiftParseResult, error) {
	components := parseNumberOfShiftRegex.FindStringSubmatch(version)
	if components == nil {
		return NumberOfShiftParseResult{BaseVersion: version}, nil
	}
	n, err := strconv.Atoi(components[2])
	if err != nil {
		return NumberOfShiftParseResult{}, multierr.Combine(
			fmt.Errorf("%w: %s", versioning.ErrInvalidNumberOfVersionsBack, version),
			err,
		)
	}
	if n < 0 {
		return NumberOfShiftParseResult{}, fmt.Errorf(
			"%w: number of versions back cannot be negative: %s",
			versioning.ErrInvalidNumberOfVersionsBack,
			version,
		)
	}
	return NumberOfShiftParseResult{BaseVersion: components[1], NumberOfShift: int64(n)}, nil
}
