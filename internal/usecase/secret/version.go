package secret

import "slices"

// versionLabels returns a deterministically sorted copy of a version's labels
// for stable output. An empty slice yields nil so that callers omit the field
// entirely.
//
//declscope:package // show and log format the version labels with it
func versionLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}

	out := slices.Clone(labels)
	slices.Sort(out)

	return out
}
