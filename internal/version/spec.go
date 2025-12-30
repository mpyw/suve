// Package version handles version specification parsing.
// Supports git-like version syntax: name[@version][~shift][:label]
package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Spec represents a parsed version specification.
type Spec struct {
	Name    string  // Parameter/Secret name
	Version *int64  // Explicit version number (@N)
	Shift   int     // Number of versions to go back (~N)
	Label   *string // Staging label (Secrets Manager only, :LABEL)
}

// parseRegex matches: name[@version][~shift][:label]
// Examples:
//   - /my/param
//   - /my/param@3
//   - /my/param~1
//   - /my/param@5~2
//   - my-secret:AWSCURRENT
//   - my-secret:AWSPREVIOUS
var parseRegex = regexp.MustCompile(`^(.+?)(?:@(\d+))?(?:~(\d+))?(?::([A-Za-z0-9_-]+))?$`)

// Parse parses a version specification string.
func Parse(input string) (*Spec, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty version specification")
	}

	matches := parseRegex.FindStringSubmatch(input)
	if matches == nil {
		return nil, fmt.Errorf("invalid version specification: %s", input)
	}

	spec := &Spec{
		Name: matches[1],
	}

	// Parse @version
	if matches[2] != "" {
		v, err := strconv.ParseInt(matches[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid version number: %s", matches[2])
		}
		spec.Version = &v
	}

	// Parse ~shift
	if matches[3] != "" {
		s, err := strconv.Atoi(matches[3])
		if err != nil {
			return nil, fmt.Errorf("invalid shift number: %s", matches[3])
		}
		spec.Shift = s
	}

	// Parse :label
	if matches[4] != "" {
		label := matches[4]
		spec.Label = &label
	}

	return spec, nil
}

// HasShift returns true if a shift is specified.
func (s *Spec) HasShift() bool {
	return s.Shift > 0
}
