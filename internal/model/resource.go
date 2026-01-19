package model

import "time"

// ResourceKind identifies the type of resource.
type ResourceKind string

const (
	// KindParameter represents an SSM Parameter Store parameter.
	KindParameter ResourceKind = "parameter"
	// KindSecret represents a Secrets Manager secret.
	KindSecret ResourceKind = "secret"
)

// Resource is a unified versioned resource (parameter or secret) for the UseCase layer.
// This type abstracts common fields from Parameter and Secret to enable unified usecase logic.
type Resource struct {
	// Kind identifies whether this is a parameter or secret.
	Kind ResourceKind
	// Name is the resource name/identifier.
	Name string
	// ARN is the resource ARN (primarily for secrets, empty for parameters).
	ARN string
	// Value is the resource value/content.
	Value string
	// Version is the version identifier (param.Version or secret.VersionID).
	Version string
	// Type is the resource type (e.g., String, SecureString for parameters).
	// This is empty for secrets which don't have a type concept.
	Type string
	// Description is the resource description.
	Description string
	// ModifiedAt is when the resource was last modified (LastModified or CreatedDate).
	ModifiedAt *time.Time
	// Tags are key-value tags associated with the resource.
	Tags map[string]string
	// Metadata contains provider-specific metadata (e.g., AWSParameterMeta, AWSSecretMeta).
	Metadata any
}

// ToResource converts a Parameter to a unified Resource.
func (p *Parameter) ToResource() *Resource {
	return &Resource{
		Kind:        KindParameter,
		Name:        p.Name,
		Value:       p.Value,
		Version:     p.Version,
		Type:        p.Type,
		Description: p.Description,
		ModifiedAt:  p.LastModified,
		Tags:        p.Tags,
		Metadata:    p.Metadata,
	}
}

// ToResource converts a Secret to a unified Resource.
func (s *Secret) ToResource() *Resource {
	return &Resource{
		Kind:        KindSecret,
		Name:        s.Name,
		ARN:         s.ARN,
		Value:       s.Value,
		Version:     s.VersionID,
		Description: s.Description,
		ModifiedAt:  s.CreatedDate,
		Tags:        s.Tags,
		Metadata:    s.Metadata,
	}
}
