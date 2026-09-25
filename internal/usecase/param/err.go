package param

import (
	"errors"

	"github.com/samber/lo"
)

var (
	// ErrNotFound is returned (wrapped with the item noun and name, e.g.
	// "setting not found: key") when the item to update does not exist.
	ErrNotFound = errors.New("not found")
)

// errItemNoun returns the noun that names one item in error messages: the
// caller's ItemNoun ("parameter", "setting"), or the neutral "entry" when unset.
//
//declscope:package // create, update and delete word their errors with it
func errItemNoun(noun string) string {
	return lo.CoalesceOrEmpty(noun, "entry")
}
