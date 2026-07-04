package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/domain"
)

func TestEntry_Fields(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	created := modified.Add(-time.Hour)

	entry := domain.Entry{
		Name:  "/my/param",
		Value: "hunter2",
		Type:  domain.ValueTypeSecret,
		Version: domain.Version{
			ID:      "v3",
			Label:   "current",
			Created: &created,
		},
		Description: "example",
		Tags:        []domain.Tag{{Key: "env", Value: "prod"}},
		Modified:    &modified,
	}

	assert.Equal(t, "/my/param", entry.Name)
	assert.Equal(t, "hunter2", entry.Value)
	assert.Equal(t, domain.ValueTypeSecret, entry.Type)
	assert.Equal(t, "v3", entry.Version.ID)
	assert.Equal(t, "current", entry.Version.Label)
	assert.Equal(t, &created, entry.Version.Created)
	assert.Equal(t, "example", entry.Description)
	assert.Len(t, entry.Tags, 1)
	assert.Equal(t, "env", entry.Tags[0].Key)
	assert.Equal(t, &modified, entry.Modified)
}

func TestValueType_Values(t *testing.T) {
	t.Parallel()

	assert.Equal(t, domain.ValueTypePlaintext, domain.ValueType("plaintext"))
	assert.Equal(t, domain.ValueTypeSecret, domain.ValueType("secret"))
	assert.Equal(t, domain.ValueTypeList, domain.ValueType("list"))
}

func TestTagChange_Fields(t *testing.T) {
	t.Parallel()

	change := domain.TagChange{
		Add:    map[string]string{"env": "prod"},
		Remove: []string{"stale"},
	}

	assert.Equal(t, "prod", change.Add["env"])
	assert.Equal(t, []string{"stale"}, change.Remove)
}
