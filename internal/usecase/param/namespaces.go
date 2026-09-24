package param

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
)

// ListNamespacesRow is one (key, namespace) row a NamespacesLister returns. An
// empty Namespace is the null (default) namespace. Value is the key's current
// value in that namespace.
type ListNamespacesRow struct {
	Key       string
	Namespace string
	Value     string
}

// NamespacesLister lists per-(key, namespace) rows honoring the store's
// configured namespace filter, sorted by key then namespace. It is an extension
// outside the neutral provider.Reader.List contract: only a service with a
// namespace axis (Azure App Configuration) offers it, through an adapter the
// caller builds over its store.
type NamespacesLister interface {
	ListNamespaces(ctx context.Context) ([]ListNamespacesRow, error)
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
	Lister NamespacesLister
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

	rows, err := u.Lister.ListNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	out := &ListNamespacesOutput{}

	out.Entries = lo.FilterMap(rows, func(row ListNamespacesRow, _ int) (ListNamespacesEntry, bool) {
		if input.Prefix != "" && !strings.HasPrefix(row.Key, input.Prefix) {
			return ListNamespacesEntry{}, false
		}

		if filterRegex != nil && !filterRegex.MatchString(row.Key) {
			return ListNamespacesEntry{}, false
		}

		entry := ListNamespacesEntry{Namespace: row.Namespace, Name: row.Key}
		if input.WithValue {
			entry.Value = lo.ToPtr(row.Value)
		}

		return entry, true
	})

	debug.From(ctx).Logf("param list (namespaces): provider returned %d rows, %d after filters (prefix=%q, filter=%q)\n",
		len(rows), len(out.Entries), input.Prefix, input.Filter)

	return out, nil
}
