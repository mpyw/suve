// Package model provides provider-agnostic domain types for parameters and secrets.
package model

import "time"

// ============================================================================
// Generic Parameter (Provider Layer)
// ============================================================================

// TypedParameter is a parameter with provider-specific metadata.
// This type is used at the Provider layer for type-safe access to metadata.
type TypedParameter[M any] struct {
	Name        string
	Value       string
	Version     string
	Description string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Tags        map[string]string
	Metadata    M
}

// ToBase converts to a UseCase layer type.
func (p *TypedParameter[M]) ToBase() *Parameter {
	return &Parameter{
		Name:        p.Name,
		Value:       p.Value,
		Version:     p.Version,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		Tags:        p.Tags,
		Metadata:    p.Metadata,
	}
}

// ============================================================================
// Base Parameter (UseCase Layer)
// ============================================================================

// Parameter is a provider-agnostic parameter for the UseCase layer.
type Parameter struct {
	Name        string
	Value       string
	Version     string
	Description string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Tags        map[string]string
	Metadata    any // Provider-specific metadata (e.g., AWSParameterMeta)
}

// AWSMeta returns the AWS-specific metadata if available.
func (p *Parameter) AWSMeta() *AWSParameterMeta {
	if meta, ok := p.Metadata.(AWSParameterMeta); ok {
		return &meta
	}

	if meta, ok := p.Metadata.(*AWSParameterMeta); ok {
		return meta
	}

	return nil
}

// TypedMetadata casts Metadata to a specific type.
func TypedMetadata[M any](p *Parameter) (M, bool) {
	m, ok := p.Metadata.(M)

	return m, ok
}

// ============================================================================
// Provider-Specific Metadata
// ============================================================================

// AWSParameterMeta contains AWS SSM Parameter Store-specific metadata.
type AWSParameterMeta struct {
	// Type is the parameter type (e.g., String, SecureString, StringList).
	Type string
	// ARN is the Amazon Resource Name of the parameter.
	ARN string
	// Tier is the parameter tier (Standard, Advanced, Intelligent-Tiering).
	Tier string
	// DataType is the data type for validation (e.g., text, aws:ec2:image).
	DataType string
	// AllowedPattern is a regex pattern for validation.
	AllowedPattern string
	// Policies contains JSON policy document for parameter policies.
	Policies string
}

// AzureAppConfigMeta contains Azure App Configuration-specific metadata.
type AzureAppConfigMeta struct {
	// ContentType is the content type of the value.
	ContentType string
	// Label is the label associated with the key.
	Label string
	// Locked indicates if the key-value is locked.
	Locked bool
	// Etag is the entity tag for optimistic concurrency.
	Etag string
}

// ============================================================================
// Type Aliases
// ============================================================================

// AWSParameter is a Parameter with AWS-specific metadata.
type AWSParameter = TypedParameter[AWSParameterMeta]

// AzureParameter is a Parameter with Azure-specific metadata.
type AzureParameter = TypedParameter[AzureAppConfigMeta]

// ============================================================================
// History Types
// ============================================================================

// TypedParameterHistory contains version history for a typed parameter.
type TypedParameterHistory[M any] struct {
	Name       string
	Parameters []*TypedParameter[M]
}

// ToBase converts to a UseCase layer type.
func (h *TypedParameterHistory[M]) ToBase() *ParameterHistory {
	params := make([]*Parameter, len(h.Parameters))
	for i, p := range h.Parameters {
		params[i] = p.ToBase()
	}

	return &ParameterHistory{
		Name:       h.Name,
		Parameters: params,
	}
}

// ParameterHistory contains version history for a parameter.
type ParameterHistory struct {
	Name       string
	Parameters []*Parameter
}

// AWSParameterHistory is a ParameterHistory with AWS-specific metadata.
type AWSParameterHistory = TypedParameterHistory[AWSParameterMeta]

// ============================================================================
// List Types
// ============================================================================

// ParameterListItem represents a parameter in a list (without value).
type ParameterListItem struct {
	Name        string
	Description string
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
	Tags        map[string]string
	Metadata    any // Provider-specific metadata (e.g., AWSParameterListItemMeta)
}

// AWSMeta returns the AWS-specific metadata if available.
func (p *ParameterListItem) AWSMeta() *AWSParameterListItemMeta {
	if meta, ok := p.Metadata.(AWSParameterListItemMeta); ok {
		return &meta
	}

	if meta, ok := p.Metadata.(*AWSParameterListItemMeta); ok {
		return meta
	}

	return nil
}

// AWSParameterListItemMeta contains AWS SSM-specific metadata for list items.
type AWSParameterListItemMeta struct {
	// Type is the parameter type (e.g., String, SecureString, StringList).
	Type string
}

// ============================================================================
// Write Result Types
// ============================================================================

// ParameterWriteResult contains the result of a parameter write operation.
type ParameterWriteResult struct {
	Name    string
	Version string
}
