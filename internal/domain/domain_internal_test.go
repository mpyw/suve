// White-box tests of domain.go.
//declscope:namespace domain

package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntry_Fields(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	created := modified.Add(-time.Hour)

	entry := Entry{
		Name:  "/my/param",
		Value: "hunter2",
		Type:  ValueTypeSecret,
		Version: Version{
			ID:      "v3",
			State:   "enabled",
			Labels:  []string{"AWSCURRENT", "AWSPREVIOUS"},
			Created: &created,
		},
		Description: "example",
		Tags:        []Tag{{Key: "env", Value: "prod"}},
		Modified:    &modified,
	}

	assert.Equal(t, "/my/param", entry.Name)
	assert.Equal(t, "hunter2", entry.Value)
	assert.Equal(t, ValueTypeSecret, entry.Type)
	assert.Equal(t, "v3", entry.Version.ID)
	assert.Equal(t, "enabled", entry.Version.State)
	assert.Equal(t, []string{"AWSCURRENT", "AWSPREVIOUS"}, entry.Version.Labels)
	assert.Equal(t, &created, entry.Version.Created)
	assert.Equal(t, "example", entry.Description)
	assert.Len(t, entry.Tags, 1)
	assert.Equal(t, "env", entry.Tags[0].Key)
	assert.Equal(t, &modified, entry.Modified)
}

func TestEntry_Extra(t *testing.T) {
	t.Parallel()

	entry := Entry{
		Name: "my-secret",
		Extra: []Field{
			{Label: "ARN", Value: "arn:aws:secretsmanager:us-east-1:123:secret:my-secret"},
		},
	}

	require.Len(t, entry.Extra, 1)
	assert.Equal(t, "ARN", entry.Extra[0].Label)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123:secret:my-secret", entry.Extra[0].Value)
}

func TestField_Construction(t *testing.T) {
	t.Parallel()

	f := Field{Label: "Tier", Value: "Advanced"}

	assert.Equal(t, "Tier", f.Label)
	assert.Equal(t, "Advanced", f.Value)
}

func TestValueType_Values(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ValueTypePlaintext, ValueType("plaintext"))
	assert.Equal(t, ValueTypeSecret, ValueType("secret"))
	assert.Equal(t, ValueTypeList, ValueType("list"))
}

func TestTagChange_Fields(t *testing.T) {
	t.Parallel()

	change := tagChange{
		add:    map[string]string{"env": "prod"},
		remove: []string{"stale"},
	}

	assert.Equal(t, "prod", change.add["env"])
	assert.Equal(t, []string{"stale"}, change.remove)
}
