package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/model"
)

func TestTypedSecret_ToBase(t *testing.T) {
	t.Parallel()

	now := time.Now()
	typed := &model.TypedSecret[model.AWSSecretMeta]{
		Name:        "test-secret",
		ARN:         "arn:aws:secretsmanager:us-east-1:123456789012:secret:test-secret",
		Value:       "test-value",
		VersionID:   "v1",
		Description: "test description",
		CreatedDate: &now,
		Tags:        map[string]string{"key": "value"},
		Metadata: model.AWSSecretMeta{
			VersionStages:   []string{"AWSCURRENT"},
			KmsKeyID:        "arn:aws:kms:us-east-1:123456789012:key/test",
			RotationEnabled: true,
		},
	}

	base := typed.ToBase()

	assert.Equal(t, typed.Name, base.Name)
	assert.Equal(t, typed.ARN, base.ARN)
	assert.Equal(t, typed.Value, base.Value)
	assert.Equal(t, typed.VersionID, base.VersionID)
	assert.Equal(t, typed.Description, base.Description)
	assert.Equal(t, typed.CreatedDate, base.CreatedDate)
	assert.Equal(t, typed.Tags, base.Tags)
	assert.IsType(t, model.AWSSecretMeta{}, base.Metadata)
}

func TestTypedSecretMetadata(t *testing.T) {
	t.Parallel()

	t.Run("valid type cast", func(t *testing.T) {
		t.Parallel()

		secret := &model.Secret{
			Name:  "test",
			Value: "value",
			Metadata: model.AWSSecretMeta{
				VersionStages: []string{"AWSCURRENT", "AWSPREVIOUS"},
				KmsKeyID:      "arn:aws:kms:us-east-1:123456789012:key/test",
			},
		}

		meta, ok := model.TypedSecretMetadata[model.AWSSecretMeta](secret)
		assert.True(t, ok)
		assert.Equal(t, []string{"AWSCURRENT", "AWSPREVIOUS"}, meta.VersionStages)
		assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/test", meta.KmsKeyID)
	})

	t.Run("invalid type cast", func(t *testing.T) {
		t.Parallel()

		secret := &model.Secret{
			Name:     "test",
			Value:    "value",
			Metadata: "not a struct",
		}

		_, ok := model.TypedSecretMetadata[model.AWSSecretMeta](secret)
		assert.False(t, ok)
	})
}

func TestTypedSecretVersion_ToBase(t *testing.T) {
	t.Parallel()

	now := time.Now()
	typed := &model.TypedSecretVersion[model.AWSSecretVersionMeta]{
		VersionID:   "v1",
		CreatedDate: &now,
		Metadata: model.AWSSecretVersionMeta{
			VersionStages: []string{"AWSCURRENT"},
		},
	}

	base := typed.ToBase()

	assert.Equal(t, typed.VersionID, base.VersionID)
	assert.Equal(t, typed.CreatedDate, base.CreatedDate)
	assert.IsType(t, model.AWSSecretVersionMeta{}, base.Metadata)
}
