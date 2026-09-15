// These are diff.go's tests: diffEntryDisplayName is private to the diff
// namespace.
//declscope:namespace diff

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestDiffEntryDisplayName(t *testing.T) {
	t.Parallel()

	// The null/default namespace yields the bare name.
	assert.Equal(t, "app/k", diffEntryDisplayName(stagingusecase.DiffEntry{Name: "app/k"}))

	// A named namespace is appended so a key staged under several namespaces is
	// unambiguous in the diff.
	assert.Equal(t, "app/k [dev]", diffEntryDisplayName(stagingusecase.DiffEntry{Name: "app/k", Namespace: "dev"}))
}
