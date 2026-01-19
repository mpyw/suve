package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/model"
)

func TestTypedParameter_ToBase(t *testing.T) {
	t.Parallel()

	now := time.Now()
	typed := &model.TypedParameter[model.AWSParameterMeta]{
		Name:         "test-param",
		Value:        "test-value",
		Version:      "1",
		Type:         "String",
		Description:  "test description",
		LastModified: &now,
		Tags:         map[string]string{"key": "value"},
		Metadata: model.AWSParameterMeta{
			ARN:  "arn:aws:ssm:us-east-1:123456789012:parameter/test-param",
			Tier: "Standard",
		},
	}

	base := typed.ToBase()

	assert.Equal(t, typed.Name, base.Name)
	assert.Equal(t, typed.Value, base.Value)
	assert.Equal(t, typed.Version, base.Version)
	assert.Equal(t, typed.Type, base.Type)
	assert.Equal(t, typed.Description, base.Description)
	assert.Equal(t, typed.LastModified, base.LastModified)
	assert.Equal(t, typed.Tags, base.Tags)
	assert.IsType(t, model.AWSParameterMeta{}, base.Metadata)
}

func TestTypedMetadata(t *testing.T) {
	t.Parallel()

	t.Run("valid type cast", func(t *testing.T) {
		t.Parallel()

		param := &model.Parameter{
			Name:  "test",
			Value: "value",
			Metadata: model.AWSParameterMeta{
				ARN:  "arn:aws:ssm:us-east-1:123456789012:parameter/test",
				Tier: "Standard",
			},
		}

		meta, ok := model.TypedMetadata[model.AWSParameterMeta](param)
		assert.True(t, ok)
		assert.Equal(t, "arn:aws:ssm:us-east-1:123456789012:parameter/test", meta.ARN)
		assert.Equal(t, "Standard", meta.Tier)
	})

	t.Run("invalid type cast", func(t *testing.T) {
		t.Parallel()

		param := &model.Parameter{
			Name:     "test",
			Value:    "value",
			Metadata: "not a struct",
		}

		_, ok := model.TypedMetadata[model.AWSParameterMeta](param)
		assert.False(t, ok)
	})
}

func TestTypedParameterHistory_ToBase(t *testing.T) {
	t.Parallel()

	now := time.Now()
	history := &model.TypedParameterHistory[model.AWSParameterMeta]{
		Name: "test-param",
		Parameters: []*model.TypedParameter[model.AWSParameterMeta]{
			{
				Name:         "test-param",
				Value:        "value1",
				Version:      "1",
				LastModified: &now,
				Metadata:     model.AWSParameterMeta{Tier: "Standard"},
			},
			{
				Name:         "test-param",
				Value:        "value2",
				Version:      "2",
				LastModified: &now,
				Metadata:     model.AWSParameterMeta{Tier: "Advanced"},
			},
		},
	}

	base := history.ToBase()

	assert.Equal(t, history.Name, base.Name)
	assert.Len(t, base.Parameters, 2)
	assert.Equal(t, "value1", base.Parameters[0].Value)
	assert.Equal(t, "value2", base.Parameters[1].Value)
}
