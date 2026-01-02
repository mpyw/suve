// Package paramapi provides interfaces for AWS Systems Manager Parameter Store.
package paramapi

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// GetParameterAPI is the interface for getting a single parameter.
type GetParameterAPI interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// GetParameterHistoryAPI is the interface for getting parameter history.
type GetParameterHistoryAPI interface {
	GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

// PutParameterAPI is the interface for creating or updating a parameter.
type PutParameterAPI interface {
	PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

// DeleteParameterAPI is the interface for deleting a parameter.
type DeleteParameterAPI interface {
	DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
}

// DescribeParametersAPI is the interface for listing parameters.
type DescribeParametersAPI interface {
	DescribeParameters(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
}

// AddTagsToResourceAPI is the interface for adding tags to a resource.
type AddTagsToResourceAPI interface {
	AddTagsToResource(ctx context.Context, params *ssm.AddTagsToResourceInput, optFns ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error)
}

// RemoveTagsFromResourceAPI is the interface for removing tags from a resource.
type RemoveTagsFromResourceAPI interface {
	RemoveTagsFromResource(ctx context.Context, params *ssm.RemoveTagsFromResourceInput, optFns ...func(*ssm.Options)) (*ssm.RemoveTagsFromResourceOutput, error)
}
