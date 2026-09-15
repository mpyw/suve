package param

import "strings"

// MatchPrefix reports whether name is in scope for the given prefix, using AWS
// Parameter Store PATH-HIERARCHY semantics (the old server-side Path filter):
//
//   - An empty prefix matches everything (the old code applied no Path filter,
//     returning all parameters at any depth).
//   - Otherwise a name matches the prefix iff name == prefix OR it is a
//     descendant, i.e. HasPrefix(name, prefix+"/"). This is hierarchical, so
//     prefix "/app" does NOT match "/application".
//   - Recursive (--recursive) matches any depth under the prefix.
//   - OneLevel (default) matches only immediate children: the segment after
//     "prefix/" must contain no further "/".
//
// A trailing slash on the prefix is normalized so "/app/" behaves like "/app".
func MatchPrefix(name, prefix string, recursive bool) bool {
	if prefix == "" {
		return true
	}

	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" { // prefix was only slashes → treat as root (match everything)
		return true
	}

	if name == prefix {
		return true
	}

	if !strings.HasPrefix(name, prefix+"/") {
		return false
	}

	if recursive {
		return true
	}

	rest := name[len(prefix)+1:] // segment(s) after "prefix/"

	return !strings.Contains(rest, "/")
}
