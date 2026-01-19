package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/model"
)

func TestParameter_ToResource(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	meta := model.AWSParameterMeta{ARN: "arn:aws:ssm:us-east-1:123456789012:parameter/test"}

	param := &model.Parameter{
		Name:         "/app/config",
		Value:        "secret-value",
		Version:      "5",
		Type:         "SecureString",
		Description:  "Test parameter",
		LastModified: &modified,
		Tags:         map[string]string{"env": "prod"},
		Metadata:     meta,
	}

	resource := param.ToResource()

	assert.Equal(t, model.KindParameter, resource.Kind)
	assert.Equal(t, param.Name, resource.Name)
	assert.Equal(t, param.Value, resource.Value)
	assert.Equal(t, param.Version, resource.Version)
	assert.Equal(t, param.Type, resource.Type)
	assert.Equal(t, param.Description, resource.Description)
	assert.Equal(t, param.LastModified, resource.ModifiedAt)
	assert.Equal(t, param.Tags, resource.Tags)
	assert.Equal(t, meta, resource.Metadata)
}

func TestSecret_ToResource(t *testing.T) {
	t.Parallel()

	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	meta := model.AWSSecretMeta{VersionStages: []string{"AWSCURRENT"}}

	secret := &model.Secret{
		Name:        "my-secret",
		ARN:         "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
		Value:       "secret-value",
		VersionID:   "abc123",
		Description: "Test secret",
		CreatedDate: &created,
		Tags:        map[string]string{"env": "prod"},
		Metadata:    meta,
	}

	resource := secret.ToResource()

	assert.Equal(t, model.KindSecret, resource.Kind)
	assert.Equal(t, secret.Name, resource.Name)
	assert.Equal(t, secret.ARN, resource.ARN)
	assert.Equal(t, secret.Value, resource.Value)
	assert.Equal(t, secret.VersionID, resource.Version)
	assert.Equal(t, secret.Description, resource.Description)
	assert.Equal(t, secret.CreatedDate, resource.ModifiedAt)
	assert.Equal(t, secret.Tags, resource.Tags)
	assert.Equal(t, meta, resource.Metadata)
}
