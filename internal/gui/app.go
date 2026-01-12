//go:build production || dev

package gui

import (
	"context"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/infra"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/runner"
)

// =============================================================================
// Client Interfaces
// =============================================================================

// ParamClient is the combined interface for all SSM Parameter Store operations.
type ParamClient interface {
	// For staging
	staging.ParamClient
	// For usecases (additional methods not in staging.ParamClient)
	paramapi.DescribeParametersAPI
	paramapi.GetParametersAPI
	paramapi.ListTagsForResourceAPI
}

// SecretClient is the combined interface for all Secrets Manager operations.
type SecretClient interface {
	// For staging
	staging.SecretClient
	// For usecases (additional methods not in staging.SecretClient)
	secretapi.ListSecretsAPI
	secretapi.DescribeSecretAPI
	secretapi.RestoreSecretAPI
}

// =============================================================================
// App Struct
// =============================================================================

// App struct holds application state and dependencies.
type App struct {
	ctx context.Context

	// AWS clients (lazily initialized)
	paramClient  ParamClient
	secretClient SecretClient

	// Staging store
	stagingStore staging.StoreReadWriteOperator
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// Startup is called when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// =============================================================================
// Errors
// =============================================================================

// errInvalidService is returned when an invalid service is specified.
var errInvalidService = errorString("invalid service: must be 'param' or 'secret'")

type errorString string

func (e errorString) Error() string { return string(e) }

// =============================================================================
// Helper Methods
// =============================================================================

func (a *App) getParamClient() (ParamClient, error) {
	if a.paramClient != nil {
		return a.paramClient, nil
	}

	client, err := infra.NewParamClient(a.ctx)
	if err != nil {
		return nil, err
	}
	a.paramClient = client
	return client, nil
}

func (a *App) getSecretClient() (SecretClient, error) {
	if a.secretClient != nil {
		return a.secretClient, nil
	}

	client, err := infra.NewSecretClient(a.ctx)
	if err != nil {
		return nil, err
	}
	a.secretClient = client
	return client, nil
}

func (a *App) getStagingStore() (staging.StoreReadWriteOperator, error) {
	if a.stagingStore != nil {
		return a.stagingStore, nil
	}

	identity, err := infra.GetAWSIdentity(a.ctx)
	if err != nil {
		return nil, err
	}

	store := runner.NewStore(identity.AccountID, identity.Region)
	a.stagingStore = store
	return store, nil
}

func (a *App) getService(service string) (staging.Service, error) {
	switch service {
	case string(staging.ServiceParam):
		return staging.ServiceParam, nil
	case string(staging.ServiceSecret):
		return staging.ServiceSecret, nil
	default:
		return "", errInvalidService
	}
}

func (a *App) getParser(service string) (staging.Parser, error) {
	switch service {
	case string(staging.ServiceParam):
		return &staging.ParamStrategy{}, nil
	case string(staging.ServiceSecret):
		return &staging.SecretStrategy{}, nil
	default:
		return nil, errInvalidService
	}
}

func (a *App) getEditStrategy(service string) (staging.EditStrategy, error) {
	switch service {
	case string(staging.ServiceParam):
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		return staging.NewParamStrategy(client), nil
	case string(staging.ServiceSecret):
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		return staging.NewSecretStrategy(client), nil
	default:
		return nil, errInvalidService
	}
}

func (a *App) getDeleteStrategy(service string) (staging.DeleteStrategy, error) {
	switch service {
	case string(staging.ServiceParam):
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		return staging.NewParamStrategy(client), nil
	case string(staging.ServiceSecret):
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		return staging.NewSecretStrategy(client), nil
	default:
		return nil, errInvalidService
	}
}

func (a *App) getApplyStrategy(service string) (staging.ApplyStrategy, error) {
	switch service {
	case string(staging.ServiceParam):
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		return staging.NewParamStrategy(client), nil
	case string(staging.ServiceSecret):
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		return staging.NewSecretStrategy(client), nil
	default:
		return nil, errInvalidService
	}
}

func (a *App) getDiffStrategy(service string) (staging.DiffStrategy, error) {
	switch service {
	case string(staging.ServiceParam):
		client, err := a.getParamClient()
		if err != nil {
			return nil, err
		}
		return staging.NewParamStrategy(client), nil
	case string(staging.ServiceSecret):
		client, err := a.getSecretClient()
		if err != nil {
			return nil, err
		}
		return staging.NewSecretStrategy(client), nil
	default:
		return nil, errInvalidService
	}
}

// =============================================================================
// AWS Identity
// =============================================================================

// AWSIdentityResult contains AWS account ID, region, and profile for frontend display.
type AWSIdentityResult struct {
	AccountID string `json:"accountId"`
	Region    string `json:"region"`
	Profile   string `json:"profile"`
}

// GetAWSIdentity returns the current AWS account ID, region, and profile.
func (a *App) GetAWSIdentity() (*AWSIdentityResult, error) {
	identity, err := infra.GetAWSIdentity(a.ctx)
	if err != nil {
		return nil, err
	}
	return &AWSIdentityResult{
		AccountID: identity.AccountID,
		Region:    identity.Region,
		Profile:   identity.Profile,
	}, nil
}
