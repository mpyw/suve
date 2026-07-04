package param

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/mpyw/suve/internal/provider"
)

// Tier sets the SSM parameter tier (Standard, Advanced, or
// Intelligent-Tiering). It implements provider.WriteOption.
type Tier struct {
	provider.WriteOptionMarker

	Value string
}

// DataType sets the SSM parameter data type (e.g. "text" or "aws:ec2:image").
// It implements provider.WriteOption.
type DataType struct {
	provider.WriteOptionMarker

	Value string
}

// AllowedPattern sets a regular expression the parameter value must match. It
// implements provider.WriteOption.
type AllowedPattern struct {
	provider.WriteOptionMarker

	Value string
}

// Policies sets the parameter policies as a JSON document (expiration,
// no-change notification, etc.). It implements provider.WriteOption.
type Policies struct {
	provider.WriteOptionMarker

	JSON string
}

// Compile-time assertions that the param write options satisfy the marker.
var (
	_ provider.WriteOption = Tier{}
	_ provider.WriteOption = DataType{}
	_ provider.WriteOption = AllowedPattern{}
	_ provider.WriteOption = Policies{}
)

// applyWriteOptions folds the recognized WriteOptions onto a PutParameterInput.
// Unknown options are ignored, per the provider.WriteOption pass-through
// contract. Empty option values are treated as unset.
func applyWriteOptions(input *ssm.PutParameterInput, opts []provider.WriteOption) {
	for _, opt := range opts {
		switch o := opt.(type) {
		case Tier:
			if o.Value != "" {
				input.Tier = types.ParameterTier(o.Value)
			}
		case DataType:
			if o.Value != "" {
				input.DataType = aws.String(o.Value)
			}
		case AllowedPattern:
			if o.Value != "" {
				input.AllowedPattern = aws.String(o.Value)
			}
		case Policies:
			if o.JSON != "" {
				input.Policies = aws.String(o.JSON)
			}
		}
	}
}
