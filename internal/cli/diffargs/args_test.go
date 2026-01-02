package diffargs_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/diffargs"
	"github.com/mpyw/suve/internal/version/smversion"
	"github.com/mpyw/suve/internal/version/ssmversion"
)

func TestParseArgs_SSM(t *testing.T) {
	t.Parallel()

	parse := ssmversion.Parse
	hasAbsolute := func(abs ssmversion.AbsoluteSpec) bool { return abs.Version != nil }
	prefixes := "#~"
	usage := "usage: suve ssm diff"

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *ssmversion.Spec
		wantSpec2  *ssmversion.Spec
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
			wantSpec1: &ssmversion.Spec{
				Name:     "/app/param",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(3))},
			},
			wantSpec2: &ssmversion.Spec{
				Name: "/app/param",
			},
		},
		{
			name: "one arg with shift",
			args: []string{"/app/param~1"},
			wantSpec1: &ssmversion.Spec{
				Name:  "/app/param",
				Shift: 1,
			},
			wantSpec2: &ssmversion.Spec{
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
			wantSpec1: &ssmversion.Spec{
				Name:     "/app/param",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))},
			},
			wantSpec2: &ssmversion.Spec{
				Name:     "/app/param",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))},
			},
		},
		{
			name: "two args different names",
			args: []string{"/app/config#1", "/app/secrets#2"},
			wantSpec1: &ssmversion.Spec{
				Name:     "/app/config",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))},
			},
			wantSpec2: &ssmversion.Spec{
				Name:     "/app/secrets",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))},
			},
		},

		// 2 args: mixed format (first has specifier, second is specifier-only)
		{
			name: "two args mixed format",
			args: []string{"/app/param#1", "#2"},
			wantSpec1: &ssmversion.Spec{
				Name:     "/app/param",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))},
			},
			wantSpec2: &ssmversion.Spec{
				Name:     "/app/param",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))},
			},
		},
		{
			name: "two args mixed format with shift",
			args: []string{"/app/param~1", "~2"},
			wantSpec1: &ssmversion.Spec{
				Name:  "/app/param",
				Shift: 1,
			},
			wantSpec2: &ssmversion.Spec{
				Name:  "/app/param",
				Shift: 2,
			},
		},

		// 2 args: partial spec format (first is name-only, second is specifier-only)
		{
			name: "two args partial spec format",
			args: []string{"/app/param", "#3"},
			wantSpec1: &ssmversion.Spec{
				Name:     "/app/param",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(3))},
			},
			wantSpec2: &ssmversion.Spec{
				Name: "/app/param",
			},
		},
		{
			name: "two args partial spec with shift",
			args: []string{"/app/param", "~2"},
			wantSpec1: &ssmversion.Spec{
				Name:  "/app/param",
				Shift: 2,
			},
			wantSpec2: &ssmversion.Spec{
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
			wantSpec1: &ssmversion.Spec{
				Name:     "/app/param",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(1))},
			},
			wantSpec2: &ssmversion.Spec{
				Name:     "/app/param",
				Absolute: ssmversion.AbsoluteSpec{Version: lo.ToPtr(int64(2))},
			},
		},
		{
			name: "three args with shifts",
			args: []string{"/app/param", "~2", "~1"},
			wantSpec1: &ssmversion.Spec{
				Name:  "/app/param",
				Shift: 2,
			},
			wantSpec2: &ssmversion.Spec{
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
			spec1, spec2, err := diffargs.ParseArgs(tt.args, parse, hasAbsolute, prefixes, usage)

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

func TestParseArgs_SM(t *testing.T) {
	t.Parallel()

	parse := smversion.Parse
	hasAbsolute := func(abs smversion.AbsoluteSpec) bool { return abs.ID != nil || abs.Label != nil }
	prefixes := "#:~"
	usage := "usage: suve sm diff"

	tests := []struct {
		name       string
		args       []string
		wantSpec1  *smversion.Spec
		wantSpec2  *smversion.Spec
		wantErrMsg string
	}{
		// 1 arg with label
		{
			name: "one arg with label",
			args: []string{"my-secret:AWSPREVIOUS"},
			wantSpec1: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")},
			},
			wantSpec2: &smversion.Spec{
				Name: "my-secret",
			},
		},

		// 2 args with labels
		{
			name: "two args mixed with labels",
			args: []string{"my-secret:AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")},
			},
			wantSpec2: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")},
			},
		},

		// 2 args with version ID
		{
			name: "two args with version ID",
			args: []string{"my-secret#abc123", "#def456"},
			wantSpec1: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{ID: lo.ToPtr("abc123")},
			},
			wantSpec2: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{ID: lo.ToPtr("def456")},
			},
		},

		// 3 args
		{
			name: "three args with labels",
			args: []string{"my-secret", ":AWSPREVIOUS", ":AWSCURRENT"},
			wantSpec1: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{Label: lo.ToPtr("AWSPREVIOUS")},
			},
			wantSpec2: &smversion.Spec{
				Name:     "my-secret",
				Absolute: smversion.AbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec1, spec2, err := diffargs.ParseArgs(tt.args, parse, hasAbsolute, prefixes, usage)

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
