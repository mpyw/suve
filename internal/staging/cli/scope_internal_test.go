// These are scope.go's tests.
//declscope:namespace scope

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenScopedWorkingStore_NilResolverFails(t *testing.T) {
	t.Parallel()

	// There is no default provider: a config without a ScopeResolver is a wiring
	// bug and must fail instead of silently keying state under some provider.
	store, _, err := openScopedWorkingStore(t.Context(), nil)
	require.Error(t, err)
	assert.Nil(t, store)
	assert.Contains(t, err.Error(), "staging scope resolver is not configured")
}
