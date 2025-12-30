// Package ssm provides CLI commands for AWS Systems Manager Parameter Store.
package ssm

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

// DescribeParametersAPI is the interface for describing parameters.
type DescribeParametersAPI interface {
	DescribeParameters(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
}

// PutParameterAPI is the interface for putting a parameter.
type PutParameterAPI interface {
	PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

// DeleteParameterAPI is the interface for deleting a parameter.
type DeleteParameterAPI interface {
	DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
}

// ShowClient is the interface for the show command.
type ShowClient interface {
	GetParameterAPI
	GetParameterHistoryAPI
}

// CatClient is the interface for the cat command.
type CatClient interface {
	GetParameterAPI
	GetParameterHistoryAPI
}

// LogClient is the interface for the log command.
type LogClient interface {
	GetParameterHistoryAPI
}

// DiffClient is the interface for the diff command.
type DiffClient interface {
	GetParameterAPI
	GetParameterHistoryAPI
}

// LsClient is the interface for the ls command.
type LsClient interface {
	DescribeParametersAPI
}

// SetClient is the interface for the set command.
type SetClient interface {
	PutParameterAPI
}

// RmClient is the interface for the rm command.
type RmClient interface {
	DeleteParameterAPI
}
