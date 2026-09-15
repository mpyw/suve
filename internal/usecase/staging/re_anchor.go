package staging

import (
	"github.com/mpyw/suve/internal/staging"
)

// ReAnchorResolver resolves the ApplyStrategy that can fetch a resource's
// current LastModified in the TARGET (current) scope, for a service and
// namespace. It mirrors the apply/conflict resolver so a cross-scope import can
// re-base each staged item's conflict-detection timestamp against the scope it
// is imported INTO rather than the foreign scope it was exported FROM. For
// namespace-agnostic providers the namespace is always empty.
type ReAnchorResolver func(svc staging.Service, namespace string) (staging.ApplyStrategy, error)
