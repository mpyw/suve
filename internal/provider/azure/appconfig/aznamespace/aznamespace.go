// Package aznamespace parses the value of the Azure App Configuration
// --namespace / --ns flag (env AZURE_APPCONFIG_NAMESPACE).
//
// suve calls this axis a "namespace"; Azure App Configuration calls it a
// "label". Structurally it is a k8s-style namespace — a flat identity partition
// of the key space, with the null (unlabeled) label as the default namespace —
// not key=value metadata (which suve unifies as tags). See #381 for the naming
// rationale.
//
// The value has two interpretations depending on context:
//
//   - list/read: an App Configuration LabelFilter (see Filter). The filter
//     grammar (`*` wildcard, `,` OR-list, `\` escape) is honored natively by
//     the service, so `*`=all, `dev*`=prefix, `dev,prod`=multi all fall out.
//   - single-item ops (show/set/delete/...): exactly one concrete namespace
//     (see Literal). `\` escapes are decoded; any unescaped `*` or `,` is a
//     usage error because those name all/multiple namespaces.
package aznamespace

import (
	"fmt"
	"strings"
)

// NullLabelFilter is the reserved App Configuration label filter that matches
// ONLY settings with no label — the null (default) namespace. The service
// encodes it as label=%00 (#352).
const NullLabelFilter = "\x00"

// AllNamespacesFilter is the App Configuration label filter that matches every
// namespace (the degenerate `*` wildcard). Cross-namespace enumeration (see
// appconfig.Store.ListWithNamespaces, #425) uses it to deliberately ignore the
// store's configured namespace.
const AllNamespacesFilter = "*"

// NullDisplay is the human-readable rendering of the null (default) namespace,
// used wherever a namespace is shown to a user (the CLI `param list` NAMESPACE
// column, #430) so it is visible rather than a blank. It mirrors the GUI's
// NS_NULL (frontend viewUtils.ts).
const NullDisplay = "(NULL)"

// Filter maps a raw --namespace value to an App Configuration LabelFilter for
// list/read enumeration. An empty value maps to the null-label filter (the
// default namespace); any other value is forwarded verbatim so the service's
// native filter grammar (`*` wildcard, `,` OR-list, `\` escape) applies (#381).
func Filter(raw string) string {
	if raw == "" {
		return NullLabelFilter
	}

	return raw
}

// Literal decodes a raw --namespace value to the single literal namespace a
// single-item operation must target. `\` escapes are decoded (`\*` -> `*`,
// `\,` -> `,`, `\\` -> `\`); an empty value maps to the null (default)
// namespace (""). If any UNESCAPED `*` or `,` remains, the value names
// all/multiple namespaces and a usage error is returned (#381).
func Literal(raw string) (string, error) {
	var b strings.Builder

	escaped := false

	for _, r := range raw {
		if escaped {
			b.WriteRune(r)

			escaped = false

			continue
		}

		switch r {
		case '\\':
			escaped = true
		case '*', ',':
			return "", fmt.Errorf(
				"--namespace %q names all/multiple namespaces; a single-item operation needs one", raw,
			)
		default:
			b.WriteRune(r)
		}
	}

	// A trailing backslash escapes nothing; keep it as a literal backslash.
	if escaped {
		b.WriteRune('\\')
	}

	return b.String(), nil
}
