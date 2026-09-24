// White-box tests of binding.go.
//declscope:namespace binding

package binding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAWSTarget(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "dev (123456789012 / us-east-1)", awsTarget("dev", "123456789012", "us-east-1"))
	assert.Equal(t, "123456789012 / us-east-1", awsTarget("", "123456789012", "us-east-1"))
	assert.Empty(t, awsTarget("dev", "", "us-east-1"))
	assert.Empty(t, awsTarget("dev", "123456789012", ""))
}
