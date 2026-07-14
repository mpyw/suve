package diff_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmdparam "github.com/mpyw/suve/internal/cli/commands/aws/param"
	cmdsecret "github.com/mpyw/suve/internal/cli/commands/aws/secret"
	genericdiff "github.com/mpyw/suve/internal/cli/commands/generic/diff"
	"github.com/mpyw/suve/internal/cli/commands/internal/apptest"
	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/version/awsparamversion"
	"github.com/mpyw/suve/internal/version/awssecretversion"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"param missing arguments", []string{"suve", "param", "diff"}, "usage:"},
		{"param invalid version spec", []string{"suve", "param", "diff", "/app/param#"}, "must be followed by"},
		{"param too many arguments", []string{"suve", "param", "diff", "/a", "#1", "#2", "#3"}, "usage:"},
		{"secret missing arguments", []string{"suve", "secret", "diff"}, "usage:"},
		{"secret invalid version spec", []string{"suve", "secret", "diff", "my-secret#"}, "must be followed by"},
		{"secret invalid label spec", []string{"suve", "secret", "diff", "my-secret:"}, "must be followed by"},
		{
			"secret too many arguments",
			[]string{"suve", "secret", "diff", "my-secret", ":AWSPREVIOUS", ":AWSCURRENT", ":extra"},
			"usage:",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := apptest.AWSApp()
			err := app.Run(t.Context(), tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

// === param diff-arg parsing ===

type paramWantSpec struct {
	name    string
	version *int64
	shift   int
}

func assertParamSpec(t *testing.T, label string, got *awsparamversion.Spec, want *paramWantSpec) {
	t.Helper()
	assert.Equal(t, want.name, got.Name, "%s.Name", label)
	assert.Equal(t, want.version, got.Absolute.Version, "%s.Absolute.Version", label)
	assert.Equal(t, want.shift, got.Shift, "%s.Shift", label)
}

func TestParseArgsParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *paramWantSpec
		wantSpec2  *paramWantSpec
		wantErrMsg string
	}{
		{
			name:      "1 arg: version specified",
			args:      []string{"/app/config#3"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(3)), shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "1 arg: shift specified",
			args:      []string{"/app/config~1"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: nil, shift: 1},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "1 arg: version and shift",
			args:      []string{"/app/config#5~2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(5)), shift: 2},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "1 arg: no version (latest vs latest)",
			args:      []string{"/app/config"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "2 args: name + version spec (partial spec)",
			args:      []string{"/app/config", "#3"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(3)), shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "2 args: full spec + version spec (mixed)",
			args:      []string{"/app/config#1", "#2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(1)), shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(2)), shift: 0},
		},
		{
			name:      "2 args: full spec with shift + version spec",
			args:      []string{"/app/config~1", "#2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: nil, shift: 1},
			wantSpec2: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(2)), shift: 0},
		},
		{
			name:      "2 args: name + shift spec",
			args:      []string{"/app/config", "~"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: nil, shift: 1},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "2 args: name + shift spec ~2",
			args:      []string{"/app/config", "~2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: nil, shift: 2},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "2 args: full spec x2 same key",
			args:      []string{"/app/config#1", "/app/config#2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(1)), shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(2)), shift: 0},
		},
		{
			name:      "2 args: full spec x2 different keys",
			args:      []string{"/app/config#1", "/other/key#2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(1)), shift: 0},
			wantSpec2: &paramWantSpec{name: "/other/key", version: lo.ToPtr(int64(2)), shift: 0},
		},
		{
			name:      "2 args: first latest, second versioned",
			args:      []string{"/app/config", "/app/config#2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(2)), shift: 0},
		},
		{
			name:      "2 args: first versioned, second latest",
			args:      []string{"/app/config#1", "/app/config"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(1)), shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 0},
		},
		{
			name:      "3 args: partial spec format",
			args:      []string{"/app/config", "#1", "#2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(1)), shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(2)), shift: 0},
		},
		{
			name:      "3 args: partial spec with shifts",
			args:      []string{"/app/config", "~1", "~2"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: nil, shift: 1},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 2},
		},
		{
			name:      "3 args: partial spec mixed version and shift",
			args:      []string{"/app/config", "#3", "~"},
			wantSpec1: &paramWantSpec{name: "/app/config", version: lo.ToPtr(int64(3)), shift: 0},
			wantSpec2: &paramWantSpec{name: "/app/config", version: nil, shift: 1},
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

			spec1, spec2, err := diffargs.ParseArgs(
				tt.args,
				awsparamversion.Parse,
				func(abs awsparamversion.AbsoluteSpec) bool { return abs.Version != nil },
				"#~",
				"usage: suve param diff <spec1> [spec2] | <name> <version1> [version2]",
			)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			assertParamSpec(t, "spec1", spec1, tt.wantSpec1)
			assertParamSpec(t, "spec2", spec2, tt.wantSpec2)
		})
	}
}

// === secret diff-arg parsing ===

type secretWantSpec struct {
	secretName string
	id         *string
	label      *string
	shift      int
}

func assertSecretSpec(t *testing.T, label string, got *awssecretversion.Spec, want *secretWantSpec) {
	t.Helper()
	assert.Equal(t, want.secretName, got.Name, "%s.Name", label)
	assert.Equal(t, want.id, got.Absolute.ID, "%s.Absolute.ID", label)
	assert.Equal(t, want.label, got.Absolute.Label, "%s.Absolute.Label", label)
	assert.Equal(t, want.shift, got.Shift, "%s.Shift", label)
}

func TestParseArgsSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *secretWantSpec
		wantSpec2  *secretWantSpec
		wantErrMsg string
	}{
		{
			name:      "1 arg: label specified",
			args:      []string{"my-secret:AWSPREVIOUS"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSPREVIOUS"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "1 arg: version ID specified",
			args:      []string{"my-secret#abc123"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("abc123"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "1 arg: shift specified",
			args:      []string{"my-secret~1"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", shift: 1},
			wantSpec2: &secretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "1 arg: no specifier (AWSCURRENT vs AWSCURRENT)",
			args:      []string{"my-secret"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "2 args: name + label spec (partial spec)",
			args:      []string{"my-secret", ":AWSPREVIOUS"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSPREVIOUS"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "2 args: name + version ID spec",
			args:      []string{"my-secret", "#abc123"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("abc123"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "2 args: full spec + label spec (mixed)",
			args:      []string{"my-secret:AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSPREVIOUS"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSCURRENT"), shift: 0},
		},
		{
			name:      "2 args: full spec#id + #id spec (mixed)",
			args:      []string{"my-secret#abc123", "#def456"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("abc123"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("def456"), shift: 0},
		},
		{
			name:      "2 args: name + shift spec",
			args:      []string{"my-secret", "~"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", shift: 1},
			wantSpec2: &secretWantSpec{secretName: "my-secret", shift: 0},
		},
		{
			name:      "2 args: full spec x2 same key with labels",
			args:      []string{"my-secret:AWSPREVIOUS", "my-secret:AWSCURRENT"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSPREVIOUS"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSCURRENT"), shift: 0},
		},
		{
			name:      "2 args: full spec x2 same key with IDs",
			args:      []string{"my-secret#abc123", "my-secret#def456"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("abc123"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("def456"), shift: 0},
		},
		{
			name:      "2 args: full spec x2 different keys",
			args:      []string{"my-secret#abc123", "other-secret#def456"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("abc123"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "other-secret", id: lo.ToPtr("def456"), shift: 0},
		},
		{
			name:      "2 args: first latest, second versioned",
			args:      []string{"my-secret", "my-secret#abc123"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("abc123"), shift: 0},
		},
		{
			name:      "3 args: partial spec format with labels",
			args:      []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSPREVIOUS"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSCURRENT"), shift: 0},
		},
		{
			name:      "3 args: partial spec format with IDs",
			args:      []string{"my-secret", "#abc123", "#def456"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("abc123"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", id: lo.ToPtr("def456"), shift: 0},
		},
		{
			name:      "3 args: partial spec mixed label and shift",
			args:      []string{"my-secret", ":AWSPREVIOUS", "~"},
			wantSpec1: &secretWantSpec{secretName: "my-secret", label: lo.ToPtr("AWSPREVIOUS"), shift: 0},
			wantSpec2: &secretWantSpec{secretName: "my-secret", shift: 1},
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

			spec1, spec2, err := diffargs.ParseArgs(
				tt.args,
				awssecretversion.Parse,
				func(abs awssecretversion.AbsoluteSpec) bool { return abs.ID != nil || abs.Label != nil },
				"#:~",
				"usage: suve secret diff <spec1> [spec2] | <name> <version1> [version2]",
			)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			assertSecretSpec(t, "spec1", spec1, tt.wantSpec1)
			assertSecretSpec(t, "spec2", spec2, tt.wantSpec2)
		})
	}
}

// === rendering ===

func run(
	t *testing.T, presenter genericdiff.Presenter, opts genericdiff.Options,
) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	r := &genericdiff.Runner{Presenter: presenter, Options: opts, Stdout: &stdout, Stderr: &stderr}
	err := r.Run(t.Context())

	return stdout.String(), stderr.String(), err
}

// paramDiffStore resolves a version-spec suffix ("#N" or "" for latest) to a ref
// and returns the mapped entry, erroring for unmapped refs (simulating not-found).
func paramDiffStore(byRef map[string]*domain.Entry) *providermock.Store {
	return &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			entry, ok := byRef[ref.ID()]
			if !ok {
				return nil, fmt.Errorf("version not found")
			}

			return entry, nil
		},
	}
}

func paramVersionSpec(v int64) *awsparamversion.Spec {
	return &awsparamversion.Spec{Name: "/app/param", Absolute: awsparamversion.AbsoluteSpec{Version: lo.ToPtr(v)}}
}

func TestRunParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec1   *awsparamversion.Spec
		spec2   *awsparamversion.Spec
		opts    genericdiff.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name:  "diff between two versions",
			spec1: paramVersionSpec(1), spec2: paramVersionSpec(2),
			store: paramDiffStore(map[string]*domain.Entry{
				"1": {Name: "/app/param", Value: "old-value", Version: domain.Version{ID: "1"}},
				"2": {Name: "/app/param", Value: "new-value", Version: domain.Version{ID: "2"}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-old-value")
				assert.Contains(t, output, "+new-value")
			},
		},
		{
			name:  "no diff when same content",
			spec1: paramVersionSpec(1), spec2: paramVersionSpec(2),
			store: paramDiffStore(map[string]*domain.Entry{
				"1": {Name: "/app/param", Value: "same-value", Version: domain.Version{ID: "1"}},
				"2": {Name: "/app/param", Value: "same-value", Version: domain.Version{ID: "2"}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.NotContains(t, output, "-same-value")
			},
		},
		{
			name:  "error getting first version",
			spec1: paramVersionSpec(1), spec2: paramVersionSpec(2),
			store: paramDiffStore(map[string]*domain.Entry{
				"2": {Name: "/app/param", Value: "value", Version: domain.Version{ID: "2"}},
			}),
			wantErr: true,
		},
		{
			name:  "error getting second version",
			spec1: paramVersionSpec(1), spec2: paramVersionSpec(2),
			store: paramDiffStore(map[string]*domain.Entry{
				"1": {Name: "/app/param", Value: "value", Version: domain.Version{ID: "1"}},
			}),
			wantErr: true,
		},
		{
			name:  "json format with valid JSON values",
			spec1: paramVersionSpec(1), spec2: paramVersionSpec(2),
			opts: genericdiff.Options{ParseJSON: true},
			store: paramDiffStore(map[string]*domain.Entry{
				"1": {Name: "/app/param", Value: `{"key":"old"}`, Version: domain.Version{ID: "1"}},
				"2": {Name: "/app/param", Value: `{"key":"new"}`, Version: domain.Version{ID: "2"}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-")
				assert.Contains(t, output, "+")
			},
		},
		{
			name:  "json format with non-JSON values warns",
			spec1: paramVersionSpec(1), spec2: paramVersionSpec(2),
			opts: genericdiff.Options{ParseJSON: true},
			store: paramDiffStore(map[string]*domain.Entry{
				"1": {Name: "/app/param", Value: "not json", Version: domain.Version{ID: "1"}},
				"2": {Name: "/app/param", Value: "also not json", Version: domain.Version{ID: "2"}},
			}),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-not json")
				assert.Contains(t, output, "+also not json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			presenter := cmdparam.NewDiffPresenter(tt.store, tt.spec1, tt.spec2)
			out, _, err := run(t, presenter, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestParamIdenticalWarning(t *testing.T) {
	t.Parallel()

	store := paramDiffStore(map[string]*domain.Entry{
		"": {Name: "/app/param", Value: "same-value", Version: domain.Version{ID: "1"}},
	})

	spec := &awsparamversion.Spec{Name: "/app/param", Absolute: awsparamversion.AbsoluteSpec{}}
	presenter := cmdparam.NewDiffPresenter(store, spec, spec)
	stdout, stderr, err := run(t, presenter, genericdiff.Options{})
	require.NoError(t, err)

	// stdout should be empty (no diff output)
	assert.Empty(t, stdout)

	// stderr should contain warning and hint
	assert.Contains(t, stderr, "Warning:")
	assert.Contains(t, stderr, "comparing identical versions")
	assert.Contains(t, stderr, "Hint:")
	assert.Contains(t, stderr, "/app/param~1")
}

// TestDiff_DistinctVersionsSameContent verifies that comparing two DISTINCT
// versions whose values happen to match reports the content as identical (not
// "comparing identical versions") and omits the self-comparison hint (#335).
func TestDiff_DistinctVersionsSameContent(t *testing.T) {
	t.Parallel()

	store := paramDiffStore(map[string]*domain.Entry{
		"1": {Name: "/app/param", Value: "same-value", Version: domain.Version{ID: "1"}},
		"3": {Name: "/app/param", Value: "same-value", Version: domain.Version{ID: "3"}},
	})

	spec1 := &awsparamversion.Spec{Name: "/app/param", Absolute: awsparamversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}}
	spec3 := &awsparamversion.Spec{Name: "/app/param", Absolute: awsparamversion.AbsoluteSpec{Version: lo.ToPtr(int64(3))}}

	stdout, stderr, err := run(t, cmdparam.NewDiffPresenter(store, spec1, spec3), genericdiff.Options{})
	require.NoError(t, err)

	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "versions differ but content is identical")
	assert.NotContains(t, stderr, "comparing identical versions")
	// The self-comparison hint ("use ~1") does not apply to two distinct specs.
	assert.NotContains(t, stderr, "Hint:")
}

// TestDiff_FormattingOnlyDifference verifies that with --parse-json, two values
// that differ only in JSON formatting (key order / whitespace) are reported as
// formatting-only rather than being masked as identical (#344).
func TestDiff_FormattingOnlyDifference(t *testing.T) {
	t.Parallel()

	store := paramDiffStore(map[string]*domain.Entry{
		"1": {Name: "/app/param", Value: `{"a":1,"b":2}`, Version: domain.Version{ID: "1"}},
		"3": {Name: "/app/param", Value: `{"b":2,"a":1}`, Version: domain.Version{ID: "3"}},
	})

	spec1 := &awsparamversion.Spec{Name: "/app/param", Absolute: awsparamversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}}
	spec3 := &awsparamversion.Spec{Name: "/app/param", Absolute: awsparamversion.AbsoluteSpec{Version: lo.ToPtr(int64(3))}}

	stdout, stderr, err := run(t, cmdparam.NewDiffPresenter(store, spec1, spec3), genericdiff.Options{ParseJSON: true})
	require.NoError(t, err)

	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "values differ only in JSON formatting")
	assert.NotContains(t, stderr, "identical")
}

// TestDiff_JSONOutputIdenticalOnRawValues verifies --output=json reports
// identical based on the raw stored values, so a formatting-only difference is
// not masked as identical:true (#344).
func TestDiff_JSONOutputIdenticalOnRawValues(t *testing.T) {
	t.Parallel()

	store := paramDiffStore(map[string]*domain.Entry{
		"1": {Name: "/app/param", Value: `{"a":1,"b":2}`, Version: domain.Version{ID: "1"}},
		"3": {Name: "/app/param", Value: `{"b":2,"a":1}`, Version: domain.Version{ID: "3"}},
	})

	spec1 := &awsparamversion.Spec{Name: "/app/param", Absolute: awsparamversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))}}
	spec3 := &awsparamversion.Spec{Name: "/app/param", Absolute: awsparamversion.AbsoluteSpec{Version: lo.ToPtr(int64(3))}}

	stdout, _, err := run(t, cmdparam.NewDiffPresenter(store, spec1, spec3),
		genericdiff.Options{ParseJSON: true, Output: output.FormatJSON})
	require.NoError(t, err)
	assert.Contains(t, stdout, `"identical": false`)
}

// secretDiffStore builds a mock reader keyed by the resolved spec suffix
// (":AWSPREVIOUS", ":AWSCURRENT", or "" for current), returning per-spec entries
// or errors.
func secretDiffStore(entries map[string]*domain.Entry, errs map[string]error) *providermock.Store {
	return &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(spec), nil
		},
		GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
			id := ref.ID()
			if errs != nil {
				if err, ok := errs[id]; ok {
					return nil, err
				}
			}

			return entries[id], nil
		},
	}
}

func prevCurrSpecs() (*awssecretversion.Spec, *awssecretversion.Spec) {
	return &awssecretversion.Spec{Name: "my-secret", Absolute: awssecretversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")}},
		&awssecretversion.Spec{Name: "my-secret", Absolute: awssecretversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")}}
}

func TestRunSecret(t *testing.T) {
	t.Parallel()

	spec1, spec2 := prevCurrSpecs()

	tests := []struct {
		name    string
		opts    genericdiff.Options
		store   *providermock.Store
		wantErr bool
		check   func(t *testing.T, output string)
	}{
		{
			name: "diff between two versions",
			store: secretDiffStore(map[string]*domain.Entry{
				":AWSPREVIOUS": {Name: "my-secret", Value: "old-secret", Version: domain.Version{ID: "prev-version-id-long"}},
				":AWSCURRENT":  {Name: "my-secret", Value: "new-secret", Version: domain.Version{ID: "curr-version-id-long"}},
			}, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-old-secret")
				assert.Contains(t, output, "+new-secret")
			},
		},
		{
			name: "short version IDs not truncated",
			store: secretDiffStore(map[string]*domain.Entry{
				":AWSPREVIOUS": {Name: "my-secret", Value: "old", Version: domain.Version{ID: "v1"}},
				":AWSCURRENT":  {Name: "my-secret", Value: "new", Version: domain.Version{ID: "v2"}},
			}, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-old")
				assert.Contains(t, output, "+new")
			},
		},
		{
			name:    "error getting first version",
			store:   secretDiffStore(nil, map[string]error{":AWSPREVIOUS": fmt.Errorf("version not found")}),
			wantErr: true,
		},
		{
			name: "error getting second version",
			store: secretDiffStore(map[string]*domain.Entry{
				":AWSPREVIOUS": {Name: "my-secret", Value: "secret", Version: domain.Version{ID: "v1"}},
			}, map[string]error{":AWSCURRENT": fmt.Errorf("version not found")}),
			wantErr: true,
		},
		{
			name: "json format with valid JSON values",
			opts: genericdiff.Options{ParseJSON: true},
			store: secretDiffStore(map[string]*domain.Entry{
				":AWSPREVIOUS": {Name: "my-secret", Value: `{"key":"old"}`, Version: domain.Version{ID: "v1-longer-id"}},
				":AWSCURRENT":  {Name: "my-secret", Value: `{"key":"new"}`, Version: domain.Version{ID: "v2-longer-id"}},
			}, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-")
				assert.Contains(t, output, "+")
			},
		},
		{
			name: "json format with non-JSON values warns",
			opts: genericdiff.Options{ParseJSON: true},
			store: secretDiffStore(map[string]*domain.Entry{
				":AWSPREVIOUS": {Name: "my-secret", Value: "not json", Version: domain.Version{ID: "v1-longer-id"}},
				":AWSCURRENT":  {Name: "my-secret", Value: "also not json", Version: domain.Version{ID: "v2-longer-id"}},
			}, nil),
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "-not json")
				assert.Contains(t, output, "+also not json")
			},
		},
		{
			// --output=json serializes the full (untruncated) version IDs and the
			// identical flag via aws/secret diffPresenter.RenderJSON.
			name: "json output serializes version IDs (differing)",
			opts: genericdiff.Options{Output: output.FormatJSON},
			store: secretDiffStore(map[string]*domain.Entry{
				":AWSPREVIOUS": {Name: "my-secret", Value: "old-secret", Version: domain.Version{ID: "prev-version-id-long"}},
				":AWSCURRENT":  {Name: "my-secret", Value: "new-secret", Version: domain.Version{ID: "curr-version-id-long"}},
			}, nil),
			check: func(t *testing.T, out string) {
				t.Helper()

				var diffOut struct {
					OldVersionID string `json:"oldVersionId"`
					NewVersionID string `json:"newVersionId"`
					Identical    bool   `json:"identical"`
					Diff         string `json:"diff"`
				}
				require.NoError(t, json.Unmarshal([]byte(out), &diffOut))
				assert.Equal(t, "prev-version-id-long", diffOut.OldVersionID)
				assert.Equal(t, "curr-version-id-long", diffOut.NewVersionID)
				assert.False(t, diffOut.Identical)
				assert.NotEmpty(t, diffOut.Diff)
			},
		},
		{
			// Same content across two distinct versions: identical is true and the
			// diff field is omitted, but both version IDs still serialize.
			name: "json output serializes version IDs (identical)",
			opts: genericdiff.Options{Output: output.FormatJSON},
			store: secretDiffStore(map[string]*domain.Entry{
				":AWSPREVIOUS": {Name: "my-secret", Value: "same-secret", Version: domain.Version{ID: "prev-version-id-long"}},
				":AWSCURRENT":  {Name: "my-secret", Value: "same-secret", Version: domain.Version{ID: "curr-version-id-long"}},
			}, nil),
			check: func(t *testing.T, out string) {
				t.Helper()

				var diffOut struct {
					OldVersionID string `json:"oldVersionId"`
					NewVersionID string `json:"newVersionId"`
					Identical    bool   `json:"identical"`
					Diff         string `json:"diff"`
				}
				require.NoError(t, json.Unmarshal([]byte(out), &diffOut))
				assert.Equal(t, "prev-version-id-long", diffOut.OldVersionID)
				assert.Equal(t, "curr-version-id-long", diffOut.NewVersionID)
				assert.True(t, diffOut.Identical)
				assert.Empty(t, diffOut.Diff)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			presenter := cmdsecret.NewDiffPresenter(tt.store, spec1, spec2)
			out, _, err := run(t, presenter, tt.opts)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func TestSecretIdenticalWarning(t *testing.T) {
	t.Parallel()

	store := secretDiffStore(map[string]*domain.Entry{
		"": {Name: "my-secret", Value: "same-content", Version: domain.Version{ID: "version-id"}},
	}, nil)

	spec := &awssecretversion.Spec{Name: "my-secret", Absolute: awssecretversion.AbsoluteSpec{}}
	presenter := cmdsecret.NewDiffPresenter(store, spec, spec)
	stdout, stderr, err := run(t, presenter, genericdiff.Options{})
	require.NoError(t, err)

	// stdout should be empty (no diff output)
	assert.Empty(t, stdout)

	// stderr should contain warning and hints
	assert.Contains(t, stderr, "Warning:")
	assert.Contains(t, stderr, "comparing identical versions")
	assert.Contains(t, stderr, "Hint:")
	assert.Contains(t, stderr, "my-secret~1")
	assert.Contains(t, stderr, "my-secret:AWSPREVIOUS")
}
