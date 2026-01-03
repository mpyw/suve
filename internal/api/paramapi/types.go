// Package paramapi provides interfaces and types for AWS Systems Manager Parameter Store.
package paramapi

import (
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Re-exported SSM client and options types.
type (
	Client  = ssm.Client
	Options = ssm.Options
)

// Re-exported SSM input/output types.
type (
	GetParameterInput            = ssm.GetParameterInput
	GetParameterOutput           = ssm.GetParameterOutput
	GetParameterHistoryInput     = ssm.GetParameterHistoryInput
	GetParameterHistoryOutput    = ssm.GetParameterHistoryOutput
	PutParameterInput            = ssm.PutParameterInput
	PutParameterOutput           = ssm.PutParameterOutput
	DeleteParameterInput         = ssm.DeleteParameterInput
	DeleteParameterOutput        = ssm.DeleteParameterOutput
	DescribeParametersInput      = ssm.DescribeParametersInput
	DescribeParametersOutput     = ssm.DescribeParametersOutput
	AddTagsToResourceInput       = ssm.AddTagsToResourceInput
	AddTagsToResourceOutput      = ssm.AddTagsToResourceOutput
	RemoveTagsFromResourceInput  = ssm.RemoveTagsFromResourceInput
	RemoveTagsFromResourceOutput = ssm.RemoveTagsFromResourceOutput
	ListTagsForResourceInput     = ssm.ListTagsForResourceInput
	ListTagsForResourceOutput    = ssm.ListTagsForResourceOutput
)

// Re-exported SSM model types.
type (
	Parameter              = types.Parameter
	ParameterHistory       = types.ParameterHistory
	ParameterMetadata      = types.ParameterMetadata
	ParameterType          = types.ParameterType
	ParameterStringFilter  = types.ParameterStringFilter
	Tag                    = types.Tag
	ResourceTypeForTagging = types.ResourceTypeForTagging
)

// Re-exported SSM constants.
const (
	ParameterTypeString             = types.ParameterTypeString
	ParameterTypeSecureString       = types.ParameterTypeSecureString
	ParameterTypeStringList         = types.ParameterTypeStringList
	ResourceTypeForTaggingParameter = types.ResourceTypeForTaggingParameter
)

// Re-exported SSM error types.
type (
	ParameterNotFound = types.ParameterNotFound
)

// Re-exported SSM functions.
var (
	NewFromConfig                  = ssm.NewFromConfig
	NewDescribeParametersPaginator = ssm.NewDescribeParametersPaginator
)

// Re-exported filter types.
const (
	FilterNameStringTypeName = types.ParametersFilterKeyName
)
