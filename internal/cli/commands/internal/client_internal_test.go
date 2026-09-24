// These are client.go's tests, so they share its core namespace.
//declscope:core

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAWSStagingTarget(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "dev (123456789012 / us-east-1)", awsStagingTarget("dev", "123456789012", "us-east-1"))
	assert.Equal(t, "123456789012 / us-east-1", awsStagingTarget("", "123456789012", "us-east-1"))
	assert.Empty(t, awsStagingTarget("dev", "", "us-east-1"))
	assert.Empty(t, awsStagingTarget("dev", "123456789012", ""))
}
