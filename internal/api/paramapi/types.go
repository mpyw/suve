package paramapi

import (
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Client is a re-exported SSM client type for dependency injection.
type Client = ssm.Client

// Options is a re-exported SSM options type.
type Options = ssm.Options

// GetParameterInput is a re-exported SSM input type.
type GetParameterInput = ssm.GetParameterInput

// GetParameterOutput is a re-exported SSM output type.
type GetParameterOutput = ssm.GetParameterOutput

// GetParametersInput is a re-exported SSM input type.
type GetParametersInput = ssm.GetParametersInput

// GetParametersOutput is a re-exported SSM output type.
type GetParametersOutput = ssm.GetParametersOutput

// GetParameterHistoryInput is a re-exported SSM input type.
type GetParameterHistoryInput = ssm.GetParameterHistoryInput

// GetParameterHistoryOutput is a re-exported SSM output type.
type GetParameterHistoryOutput = ssm.GetParameterHistoryOutput

// PutParameterInput is a re-exported SSM input type.
type PutParameterInput = ssm.PutParameterInput

// PutParameterOutput is a re-exported SSM output type.
type PutParameterOutput = ssm.PutParameterOutput

// DeleteParameterInput is a re-exported SSM input type.
type DeleteParameterInput = ssm.DeleteParameterInput

// DeleteParameterOutput is a re-exported SSM output type.
type DeleteParameterOutput = ssm.DeleteParameterOutput

// DescribeParametersInput is a re-exported SSM input type.
type DescribeParametersInput = ssm.DescribeParametersInput

// DescribeParametersOutput is a re-exported SSM output type.
type DescribeParametersOutput = ssm.DescribeParametersOutput

// AddTagsToResourceInput is a re-exported SSM input type.
type AddTagsToResourceInput = ssm.AddTagsToResourceInput

// AddTagsToResourceOutput is a re-exported SSM output type.
type AddTagsToResourceOutput = ssm.AddTagsToResourceOutput

// RemoveTagsFromResourceInput is a re-exported SSM input type.
type RemoveTagsFromResourceInput = ssm.RemoveTagsFromResourceInput

// RemoveTagsFromResourceOutput is a re-exported SSM output type.
type RemoveTagsFromResourceOutput = ssm.RemoveTagsFromResourceOutput

// ListTagsForResourceInput is a re-exported SSM input type.
type ListTagsForResourceInput = ssm.ListTagsForResourceInput

// ListTagsForResourceOutput is a re-exported SSM output type.
type ListTagsForResourceOutput = ssm.ListTagsForResourceOutput

// Parameter is a re-exported SSM model type.
type Parameter = types.Parameter

// ParameterHistory is a re-exported SSM model type.
type ParameterHistory = types.ParameterHistory

// ParameterMetadata is a re-exported SSM model type.
type ParameterMetadata = types.ParameterMetadata

// ParameterType is a re-exported SSM model type.
type ParameterType = types.ParameterType

// ParameterStringFilter is a re-exported SSM model type.
type ParameterStringFilter = types.ParameterStringFilter

// Tag is a re-exported SSM model type.
type Tag = types.Tag

// ResourceTypeForTagging is a re-exported SSM model type.
type ResourceTypeForTagging = types.ResourceTypeForTagging

// Re-exported SSM constants.
const (
	ParameterTypeString             = types.ParameterTypeString
	ParameterTypeSecureString       = types.ParameterTypeSecureString
	ParameterTypeStringList         = types.ParameterTypeStringList
	ResourceTypeForTaggingParameter = types.ResourceTypeForTaggingParameter
)

// ParameterNotFound is a re-exported SSM error type.
//
//nolint:errname // Type alias to AWS SDK type, preserving original name for consistency
type ParameterNotFound = types.ParameterNotFound

// ParameterAlreadyExists is a re-exported SSM error type.
//
//nolint:errname // Type alias to AWS SDK type, preserving original name for consistency
type ParameterAlreadyExists = types.ParameterAlreadyExists

// NewFromConfig is a re-exported SSM factory function for dependency injection.
//
//nolint:gochecknoglobals // Re-export of AWS SDK factory function for dependency injection
var NewFromConfig = ssm.NewFromConfig

// NewDescribeParametersPaginator is a re-exported SSM factory function for dependency injection.
//
//nolint:gochecknoglobals // Re-export of AWS SDK factory function for dependency injection
var NewDescribeParametersPaginator = ssm.NewDescribeParametersPaginator

// Re-exported filter types.
const (
	FilterNameStringTypeName = types.ParametersFilterKeyName
)
