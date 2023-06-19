package versioning

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

//go:generate stringer -type=CanonicalVersionType,VersionRequirementType -output=versioning_string.gen.go
type (
	CanonicalVersionType   int
	VersionRequirementType int
)

var (
	_ fmt.Stringer       = CanonicalVersionType(0)
	_ fmt.Stringer       = VersionRequirementType(0)
	_ VersionRequirement = AbsoluteVersionRequirement{}
	_ VersionRequirement = RelativeVersionRequirement{}
)

const (
	VersionRequirementTypeUnknown VersionRequirementType = iota
	VersionRequirementTypeCanonical
	VersionRequirementTypeStageOrLabel
)

const (
	CanonicalVersionTypeUnknown CanonicalVersionType = iota
	// CanonicalVersionTypeNumber is used for Parameter Store records.
	CanonicalVersionTypeNumber
	// CanonicalVersionTypeUUID is used for Secrets Manager records.
	CanonicalVersionTypeUUID
)

var (
	ErrUnsupportedCanonicalVersion   = errors.New("unsupported canonical version")
	ErrUnsupportedVersionRequirement = errors.New("unsupported version requirement")
	ErrVersionNotFound               = errors.New("version not found")
	ErrInvalidNumberOfVersionsBack   = errors.New("invalid number of versions back")
)

// VersionRequirement represents the target version to be retrieved.
type VersionRequirement interface {
	GetType() VersionRequirementType
	// GetCanonicalValue is valid when GetType() returns VersionRequirementTypeCanonical.
	GetCanonicalValue() CanonicalVersion
	// GetStageOrLabelValue is valid when GetType() returns VersionRequirementTypeStageOrLabel.
	GetStageOrLabelValue() string
	// GetNumberOfShift represents the number of versions back, as it were, git-style HEAD~<shift> format.
	GetNumberOfShift() int64
	// WithoutShift drops information about version shifting.
	WithoutShift() AbsoluteVersionRequirement
}

// VersionParser parses version representation in string.
type VersionParser interface {
	Parse(version string) (VersionRequirement, error)
}

type VersionParserFunc func(version string) (VersionRequirement, error)

func (fn VersionParserFunc) Parse(version string) (VersionRequirement, error) {
	return fn(version)
}

type AbsoluteVersionRequirement struct {
	Type VersionRequirementType
	// CanonicalValue is valid when Type == VersionRequirementTypeCanonical.
	CanonicalValue CanonicalVersion
	// StageOrLabelValue is valid when Type == VersionRequirementTypeStageOrLabel.
	StageOrLabelValue string
}

func (r AbsoluteVersionRequirement) GetType() VersionRequirementType {
	return r.Type
}

func (r AbsoluteVersionRequirement) GetCanonicalValue() CanonicalVersion {
	return r.CanonicalValue
}

func (r AbsoluteVersionRequirement) GetStageOrLabelValue() string {
	return r.StageOrLabelValue
}

func (r AbsoluteVersionRequirement) GetNumberOfShift() int64 {
	return 0
}

func (r AbsoluteVersionRequirement) WithoutShift() AbsoluteVersionRequirement {
	return r
}

type RelativeVersionRequirement struct {
	AbsoluteVersionRequirement
	// NumberOfShift represents the number of versions back, as it were, git-style HEAD~<shift> format.
	NumberOfShift int64
}

func (r RelativeVersionRequirement) GetNumberOfShift() int64 {
	return r.NumberOfShift
}

type Version struct {
	CanonicalVersion
	StagesOrLabels []string
}

func (v Version) String() string {
	if len(v.StagesOrLabels) == 0 {
		return v.CanonicalVersion.String()
	}
	return v.CanonicalVersion.String() + " (" + strings.Join(v.StagesOrLabels, ", ") + ")"
}

type CanonicalVersion struct {
	Type CanonicalVersionType
	// NumberValue is valid when Type == CanonicalVersionTypeNumber.
	NumberValue int64
	// UUIDValue is valid when Type == CanonicalVersionTypeUUID.
	UUIDValue uuid.UUID
}

func (v CanonicalVersion) String() string {
	switch v.Type {
	case CanonicalVersionTypeNumber:
		return strconv.Itoa(int(v.NumberValue))
	case CanonicalVersionTypeUUID:
		return v.UUIDValue.String()
	default:
		return "unknown"
	}
}

func (v CanonicalVersion) AsRequirement() AbsoluteVersionRequirement {
	return AbsoluteVersionRequirement{
		Type:           VersionRequirementTypeCanonical,
		CanonicalValue: v,
	}
}
