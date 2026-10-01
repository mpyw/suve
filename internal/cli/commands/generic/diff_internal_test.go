// White-box tests of diff.go.
//declscope:namespace diff

package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version"
)

type diffParamWantSpec struct {
	name    string
	version *int64
	shift   int
}

func diffAssertParamSpec(t *testing.T, label string, got *version.NumericSpec, want *diffParamWantSpec) {
	t.Helper()
	assert.Equal(t, want.name, got.Name, "%s.Name", label)
	assert.Equal(t, want.version, got.Absolute.Version, "%s.Absolute.Version", label)
	assert.Equal(t, want.shift, got.Shift, "%s.Shift", label)
}

func TestParseDiffArgsParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *diffParamWantSpec
		wantSpec2  *diffParamWantSpec
		wantErrMsg string
	}{
		{
			name:      "1 arg: version specified",
			args:      []string{"/app/config#3"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(3)), shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "1 arg: shift specified",
			args:      []string{"/app/config~1"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: nil, shift: 1},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "1 arg: version and shift",
			args:      []string{"/app/config#5~2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(5)), shift: 2},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "1 arg: no version (latest vs latest)",
			args:      []string{"/app/config"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "2 args: name + version spec (partial spec)",
			args:      []string{"/app/config", "#3"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(3)), shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "2 args: full spec + version spec (mixed)",
			args:      []string{"/app/config#1", "#2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(1)), shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: new(int64(2)), shift: 0},
		},
		{
			name:      "2 args: full spec with shift + version spec",
			args:      []string{"/app/config~1", "#2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: nil, shift: 1},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: new(int64(2)), shift: 0},
		},
		{
			name:      "2 args: name + shift spec",
			args:      []string{"/app/config", "~"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: nil, shift: 1},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "2 args: name + shift spec ~2",
			args:      []string{"/app/config", "~2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: nil, shift: 2},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "2 args: full spec x2 same key",
			args:      []string{"/app/config#1", "/app/config#2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(1)), shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: new(int64(2)), shift: 0},
		},
		{
			name:      "2 args: full spec x2 different keys",
			args:      []string{"/app/config#1", "/other/key#2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(1)), shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/other/key", version: new(int64(2)), shift: 0},
		},
		{
			name:      "2 args: first latest, second versioned",
			args:      []string{"/app/config", "/app/config#2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: new(int64(2)), shift: 0},
		},
		{
			name:      "2 args: first versioned, second latest",
			args:      []string{"/app/config#1", "/app/config"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(1)), shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "3 args: partial spec format",
			args:      []string{"/app/config", "#1", "#2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(1)), shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: new(int64(2)), shift: 0},
		},
		{
			name:      "3 args: partial spec with shifts",
			args:      []string{"/app/config", "~1", "~2"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: nil, shift: 1},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 2},
		},
		{
			name:      "3 args: partial spec mixed version and shift",
			args:      []string{"/app/config", "#3", "~"},
			wantSpec1: &diffParamWantSpec{name: "/app/config", version: new(int64(3)), shift: 0},
			wantSpec2: &diffParamWantSpec{name: "/app/config", version: nil, shift: 1},
		},
		{name: "0 args: error", args: []string{}, wantErrMsg: "usage:"},
		{name: "4+ args: error", args: []string{"/app/config", "#1", "#2", "#3"}, wantErrMsg: "usage:"},
		{name: "invalid shift spec in 1 arg", args: []string{"/app/config#3~abc"}, wantErrMsg: "invalid"},
		{name: "invalid shift spec in 2nd arg", args: []string{"/app/config", "#3~abc"}, wantErrMsg: "invalid"},
		{name: "2 args: invalid first arg", args: []string{"#", "/app/config#2"}, wantErrMsg: "invalid first argument"},
		{
			name:       "2 args: invalid second arg (full spec x2)",
			args:       []string{"/app/config#1", "/app/config#"},
			wantErrMsg: "invalid second argument",
		},
		{name: "3 args: invalid version1", args: []string{"/app/config", "#", "#2"}, wantErrMsg: "invalid version1"},
		{name: "3 args: invalid version2", args: []string{"/app/config", "#1", "#"}, wantErrMsg: "invalid version2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec1, spec2, err := ParseDiffArgs(
				tt.args,
				version.AWSParameterStore.Parse,
				func(abs version.NumericAbsolute) bool { return abs.Version != nil },
				"#~",
				"usage: suve param diff <spec1> [spec2] | <name> <version1> [version2]",
			)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			diffAssertParamSpec(t, "spec1", spec1, tt.wantSpec1)
			diffAssertParamSpec(t, "spec2", spec2, tt.wantSpec2)
		})
	}
}

type diffSecretWantSpec struct {
	secretName string
	id         *string
	label      *string
	shift      int
}

func diffAssertSecretSpec(t *testing.T, label string, got *version.OpaqueSpec, want *diffSecretWantSpec) {
	t.Helper()
	assert.Equal(t, want.secretName, got.Name, "%s.Name", label)
	assert.Equal(t, want.id, got.Absolute.ID, "%s.Absolute.ID", label)
	assert.Equal(t, want.label, got.Absolute.Label, "%s.Absolute.Label", label)
	assert.Equal(t, want.shift, got.Shift, "%s.Shift", label)
}

func TestParseDiffArgsSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *diffSecretWantSpec
		wantSpec2  *diffSecretWantSpec
		wantErrMsg string
	}{
		{
			name:      "1 arg: label specified",
			args:      []string{"my-secret:AWSPREVIOUS"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSPREVIOUS"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "1 arg: version ID specified",
			args:      []string{"my-secret#abc123"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", id: new("abc123"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "1 arg: shift specified",
			args:      []string{"my-secret~1"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", shift: 1},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "1 arg: no specifier (AWSCURRENT vs AWSCURRENT)",
			args:      []string{"my-secret"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "2 args: name + label spec (partial spec)",
			args:      []string{"my-secret", ":AWSPREVIOUS"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSPREVIOUS"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "2 args: name + version ID spec",
			args:      []string{"my-secret", "#abc123"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", id: new("abc123"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "2 args: full spec + label spec (mixed)",
			args:      []string{"my-secret:AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSPREVIOUS"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSCURRENT"), shift: 0},
		},
		{
			name:      "2 args: full spec#id + #id spec (mixed)",
			args:      []string{"my-secret#abc123", "#def456"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", id: new("abc123"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", id: new("def456"), shift: 0},
		},
		{
			name:      "2 args: name + shift spec",
			args:      []string{"my-secret", "~"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", shift: 1},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "2 args: full spec x2 same key with labels",
			args:      []string{"my-secret:AWSPREVIOUS", "my-secret:AWSCURRENT"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSPREVIOUS"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSCURRENT"), shift: 0},
		},
		{
			name:      "2 args: full spec x2 same key with IDs",
			args:      []string{"my-secret#abc123", "my-secret#def456"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", id: new("abc123"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", id: new("def456"), shift: 0},
		},
		{
			name:      "2 args: full spec x2 different keys",
			args:      []string{"my-secret#abc123", "other-secret#def456"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", id: new("abc123"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "other-secret", id: new("def456"), shift: 0},
		},
		{
			name:      "2 args: first latest, second versioned",
			args:      []string{"my-secret", "my-secret#abc123"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", id: new("abc123"), shift: 0},
		},
		{
			name:      "3 args: partial spec format with labels",
			args:      []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSPREVIOUS"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSCURRENT"), shift: 0},
		},
		{
			name:      "3 args: partial spec format with IDs",
			args:      []string{"my-secret", "#abc123", "#def456"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", id: new("abc123"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", id: new("def456"), shift: 0},
		},
		{
			name:      "3 args: partial spec mixed label and shift",
			args:      []string{"my-secret", ":AWSPREVIOUS", "~"},
			wantSpec1: &diffSecretWantSpec{secretName: "my-secret", label: new("AWSPREVIOUS"), shift: 0},
			wantSpec2: &diffSecretWantSpec{secretName: "my-secret", shift: 1},
		},
		{name: "0 args: error", args: []string{}, wantErrMsg: "usage:"},
		{
			name:       "4+ args: error",
			args:       []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT", ":extra"},
			wantErrMsg: "usage:",
		},
		{name: "invalid label in 1 arg", args: []string{"my-secret:"}, wantErrMsg: "invalid"},
		{name: "invalid version ID in 2nd arg", args: []string{"my-secret", "#"}, wantErrMsg: "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec1, spec2, err := ParseDiffArgs(
				tt.args,
				version.AWSSecretsManager.Parse,
				func(abs version.OpaqueAbsolute) bool { return abs.ID != nil || abs.Label != nil },
				"#:~",
				"usage: suve secret diff <spec1> [spec2] | <name> <version1> [version2]",
			)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			diffAssertSecretSpec(t, "spec1", spec1, tt.wantSpec1)
			diffAssertSecretSpec(t, "spec2", spec2, tt.wantSpec2)
		})
	}
}

func TestParseDiffArgs_Param(t *testing.T) {
	t.Parallel()

	parse := version.AWSParameterStore.Parse
	hasAbsolute := func(abs version.NumericAbsolute) bool { return abs.Version != nil }
	prefixes := "#~"
	usage := "usage: suve param diff"

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *version.NumericSpec
		wantSpec2  *version.NumericSpec
		wantErrMsg string
	}{
		// Error cases
		{
			name:       "no arguments",
			args:       []string{},
			wantErrMsg: "usage:",
		},
		{
			name:       "too many arguments",
			args:       []string{"/app/param", "#1", "#2", "#3"},
			wantErrMsg: "usage:",
		},

		// 1 arg: full spec format
		{
			name: "one arg with version",
			args: []string{"/app/param#3"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: new(int64(3))},
			},
			wantSpec2: &version.NumericSpec{
				Name: "/app/param",
			},
		},
		{
			name: "one arg with shift",
			args: []string{"/app/param~1"},
			wantSpec1: &version.NumericSpec{
				Name:  "/app/param",
				Shift: 1,
			},
			wantSpec2: &version.NumericSpec{
				Name: "/app/param",
			},
		},
		{
			name:       "one arg invalid spec",
			args:       []string{"/app/param#"},
			wantErrMsg: "invalid version specification",
		},

		// 2 args: full spec x2
		{
			name: "two args both full spec",
			args: []string{"/app/param#1", "/app/param#2"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: new(int64(1))},
			},
			wantSpec2: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: new(int64(2))},
			},
		},
		{
			name: "two args different names",
			args: []string{"/app/config#1", "/app/secrets#2"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/config",
				Absolute: version.NumericAbsolute{Version: new(int64(1))},
			},
			wantSpec2: &version.NumericSpec{
				Name:     "/app/secrets",
				Absolute: version.NumericAbsolute{Version: new(int64(2))},
			},
		},

		// 2 args: mixed format (first has specifier, second is specifier-only)
		{
			name: "two args mixed format",
			args: []string{"/app/param#1", "#2"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: new(int64(1))},
			},
			wantSpec2: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: new(int64(2))},
			},
		},
		{
			name: "two args mixed format with shift",
			args: []string{"/app/param~1", "~2"},
			wantSpec1: &version.NumericSpec{
				Name:  "/app/param",
				Shift: 1,
			},
			wantSpec2: &version.NumericSpec{
				Name:  "/app/param",
				Shift: 2,
			},
		},

		// 2 args: partial spec format (first is name-only, second is specifier-only)
		{
			name: "two args partial spec format",
			args: []string{"/app/param", "#3"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: new(int64(3))},
			},
			wantSpec2: &version.NumericSpec{
				Name: "/app/param",
			},
		},
		{
			name: "two args partial spec with shift",
			args: []string{"/app/param", "~2"},
			wantSpec1: &version.NumericSpec{
				Name:  "/app/param",
				Shift: 2,
			},
			wantSpec2: &version.NumericSpec{
				Name: "/app/param",
			},
		},
		{
			name:       "two args first invalid",
			args:       []string{"/app/param#", "#2"},
			wantErrMsg: "invalid first argument",
		},
		{
			name:       "two args second invalid specifier",
			args:       []string{"/app/param", "#"},
			wantErrMsg: "invalid second argument",
		},
		{
			name:       "two args second invalid full spec",
			args:       []string{"/app/param#1", "/other#"},
			wantErrMsg: "invalid second argument",
		},

		// 3 args: partial spec format
		{
			name: "three args",
			args: []string{"/app/param", "#1", "#2"},
			wantSpec1: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: new(int64(1))},
			},
			wantSpec2: &version.NumericSpec{
				Name:     "/app/param",
				Absolute: version.NumericAbsolute{Version: new(int64(2))},
			},
		},
		{
			name: "three args with shifts",
			args: []string{"/app/param", "~2", "~1"},
			wantSpec1: &version.NumericSpec{
				Name:  "/app/param",
				Shift: 2,
			},
			wantSpec2: &version.NumericSpec{
				Name:  "/app/param",
				Shift: 1,
			},
		},
		{
			name:       "three args invalid version1",
			args:       []string{"/app/param", "#", "#2"},
			wantErrMsg: "invalid version1",
		},
		{
			name:       "three args invalid version2",
			args:       []string{"/app/param", "#1", "#"},
			wantErrMsg: "invalid version2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec1, spec2, err := ParseDiffArgs(tt.args, parse, hasAbsolute, prefixes, usage)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSpec1, spec1)
			assert.Equal(t, tt.wantSpec2, spec2)
		})
	}
}

// TestParseArgs_ThreeArgRequiresSpecifier covers the fix for #482: in the 3-arg
// form each version argument must start with a specifier prefix so that a bare
// token (e.g. "3") is rejected instead of being concatenated onto the name.
func TestParseDiffArgs_ThreeArgRequiresSpecifier(t *testing.T) {
	t.Parallel()

	parse := version.AWSParameterStore.Parse
	hasAbsolute := func(abs version.NumericAbsolute) bool { return abs.Version != nil }
	prefixes := "#~"
	usage := "usage: suve param diff"

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *version.NumericSpec
		wantSpec2  *version.NumericSpec
		wantErrMsg string
	}{
		{
			name:       "bare version1 rejected",
			args:       []string{"/p", "3", "1"},
			wantErrMsg: "must start with a version specifier",
		},
		{
			name:       "bare version2 rejected",
			args:       []string{"/p", "#3", "1"},
			wantErrMsg: "must start with a version specifier",
		},
		{
			name: "hash specifiers keep name",
			args: []string{"/p", "#3", "#1"},
			wantSpec1: &version.NumericSpec{
				Name:     "/p",
				Absolute: version.NumericAbsolute{Version: new(int64(3))},
			},
			wantSpec2: &version.NumericSpec{
				Name:     "/p",
				Absolute: version.NumericAbsolute{Version: new(int64(1))},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec1, spec2, err := ParseDiffArgs(tt.args, parse, hasAbsolute, prefixes, usage)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSpec1, spec1)
			assert.Equal(t, tt.wantSpec2, spec2)
		})
	}
}

func TestParseDiffArgs_Secret(t *testing.T) {
	t.Parallel()

	parse := version.AWSSecretsManager.Parse
	hasAbsolute := func(abs version.OpaqueAbsolute) bool { return abs.ID != nil || abs.Label != nil }
	prefixes := "#:~"
	usage := "usage: suve secret diff"

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *version.OpaqueSpec
		wantSpec2  *version.OpaqueSpec
		wantErrMsg string
	}{
		// 1 arg with label
		{
			name: "one arg with label",
			args: []string{"my-secret:AWSPREVIOUS"},
			wantSpec1: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{Label: new("AWSPREVIOUS")},
			},
			wantSpec2: &version.OpaqueSpec{
				Name: "my-secret",
			},
		},

		// 2 args with labels
		{
			name: "two args mixed with labels",
			args: []string{"my-secret:AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{Label: new("AWSPREVIOUS")},
			},
			wantSpec2: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{Label: new("AWSCURRENT")},
			},
		},

		// 2 args with version ID
		{
			name: "two args with version ID",
			args: []string{"my-secret#abc123", "#def456"},
			wantSpec1: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{ID: new("abc123")},
			},
			wantSpec2: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{ID: new("def456")},
			},
		},

		// 3 args
		{
			name: "three args with labels",
			args: []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{Label: new("AWSPREVIOUS")},
			},
			wantSpec2: &version.OpaqueSpec{
				Name:     "my-secret",
				Absolute: version.OpaqueAbsolute{Label: new("AWSCURRENT")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec1, spec2, err := ParseDiffArgs(tt.args, parse, hasAbsolute, prefixes, usage)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSpec1, spec1)
			assert.Equal(t, tt.wantSpec2, spec2)
		})
	}
}
