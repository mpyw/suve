package staging

import (
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
)

// UntagInput holds input for the untag staging use case. Key identifies the
// resource by name and (Azure App Configuration) namespace.
type UntagInput struct {
	Key     staging.EntryKey
	TagKeys maputil.Set[string]
}

// UntagOutput holds the result of the untag staging use case.
type UntagOutput struct {
	Name string
}
