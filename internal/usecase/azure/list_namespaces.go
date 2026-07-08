package azure

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
)

// NamespaceLister is the App-Config-specific extension that lists per-(key,
// namespace) rows honoring the store's configured --namespace filter. Only the
// Azure App Configuration store implements it (via ListWithNamespacesScoped);
// callers type-assert the resolved store to reach it. The neutral
// provider.Reader.List contract is untouched.
type NamespaceLister interface {
	ListWithNamespacesScoped(ctx context.Context) ([]appconfig.KeyNamespace, error)
}

// ListNamespacesInput holds input for the namespace-aware list use case. The
// namespace filter itself lives on the store (resolved from --namespace); here
// only the client-side key filters apply.
type ListNamespacesInput struct {
	Prefix    string // Name prefix filter (case-sensitive)
	Filter    string // Regex filter pattern (client-side)
	WithValue bool   // Include values (App Config's list response already carries them)
}

// ListNamespacesEntry is one (key, namespace) row. Value is nil when not
// requested. App Configuration's list response always carries the value, so no
// per-entry error path exists (unlike the value-fetching neutral list).
type ListNamespacesEntry struct {
	Namespace string
	Name      string
	Value     *string
}

// ListNamespacesOutput holds the result of the namespace-aware list use case.
type ListNamespacesOutput struct {
	Entries []ListNamespacesEntry
}

// ListNamespacesUseCase lists App Configuration settings with their namespaces.
type ListNamespacesUseCase struct {
	Lister NamespaceLister
}

// Execute runs the namespace-aware list use case. The store applies the
// namespace (label) filter; the name prefix and client-side regex filter are
// applied here, on the key.
func (u *ListNamespacesUseCase) Execute(ctx context.Context, input ListNamespacesInput) (*ListNamespacesOutput, error) {
	var filterRegex *regexp.Regexp

	if input.Filter != "" {
		var err error

		filterRegex, err = regexp.Compile(input.Filter)
		if err != nil {
			return nil, fmt.Errorf("invalid filter regex: %w", err)
		}
	}

	rows, err := u.Lister.ListWithNamespacesScoped(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	out := &ListNamespacesOutput{}

	for _, row := range rows {
		if input.Prefix != "" && !strings.HasPrefix(row.Key, input.Prefix) {
			continue
		}

		if filterRegex != nil && !filterRegex.MatchString(row.Key) {
			continue
		}

		entry := ListNamespacesEntry{Namespace: row.Namespace, Name: row.Key}
		if input.WithValue {
			entry.Value = lo.ToPtr(row.Value)
		}

		out.Entries = append(out.Entries, entry)
	}

	debug.From(ctx).Logf("azure list (namespaces): provider returned %d rows, %d after filters (prefix=%q, filter=%q)\n",
		len(rows), len(out.Entries), input.Prefix, input.Filter)

	return out, nil
}
