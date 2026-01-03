// Package paramapi provides interfaces for AWS Systems Manager Parameter Store.
package paramapi

import (
	"context"
)

// GetParameterAPI is the interface for getting a single parameter.
type GetParameterAPI interface {
	GetParameter(ctx context.Context, params *GetParameterInput, optFns ...func(*Options)) (*GetParameterOutput, error)
}

// GetParameterHistoryAPI is the interface for getting parameter history.
type GetParameterHistoryAPI interface {
	GetParameterHistory(ctx context.Context, params *GetParameterHistoryInput, optFns ...func(*Options)) (*GetParameterHistoryOutput, error)
}

// PutParameterAPI is the interface for creating or updating a parameter.
type PutParameterAPI interface {
	PutParameter(ctx context.Context, params *PutParameterInput, optFns ...func(*Options)) (*PutParameterOutput, error)
}

// DeleteParameterAPI is the interface for deleting a parameter.
type DeleteParameterAPI interface {
	DeleteParameter(ctx context.Context, params *DeleteParameterInput, optFns ...func(*Options)) (*DeleteParameterOutput, error)
}

// DescribeParametersAPI is the interface for listing parameters.
type DescribeParametersAPI interface {
	DescribeParameters(ctx context.Context, params *DescribeParametersInput, optFns ...func(*Options)) (*DescribeParametersOutput, error)
}

// AddTagsToResourceAPI is the interface for adding tags to a resource.
type AddTagsToResourceAPI interface {
	AddTagsToResource(ctx context.Context, params *AddTagsToResourceInput, optFns ...func(*Options)) (*AddTagsToResourceOutput, error)
}

// RemoveTagsFromResourceAPI is the interface for removing tags from a resource.
type RemoveTagsFromResourceAPI interface {
	RemoveTagsFromResource(ctx context.Context, params *RemoveTagsFromResourceInput, optFns ...func(*Options)) (*RemoveTagsFromResourceOutput, error)
}

// ListTagsForResourceAPI is the interface for listing tags for a resource.
type ListTagsForResourceAPI interface {
	ListTagsForResource(ctx context.Context, params *ListTagsForResourceInput, optFns ...func(*Options)) (*ListTagsForResourceOutput, error)
}
