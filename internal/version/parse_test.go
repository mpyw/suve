package version_test

import (
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version"
	"github.com/mpyw/suve/internal/version/internal"
)

// testAbsoluteSpec is a simple test type for absolute spec.
type testAbsoluteSpec struct {
	Version *int64
	Label   *string
}

var errInvalidVersion = errors.New("# must be followed by a version number")
var errInvalidLabel = errors.New(": must be followed by a label")

// testParser is a test parser with both version and label specifiers.
var testParser = version.AbsoluteParser[testAbsoluteSpec]{
	Parsers: []version.SpecifierParser[testAbsoluteSpec]{
		{
			PrefixChar: '#',
			IsChar:     internal.IsDigit,
			Error:      errInvalidVersion,
			Duplicated: func(abs testAbsoluteSpec) bool {
				return abs.Version != nil
			},
			Apply: func(value string, abs testAbsoluteSpec) (testAbsoluteSpec, error) {
				var v int64
				for _, c := range value {
					v = v*10 + int64(c-'0')
				}
				abs.Version = lo.ToPtr(v)
				return abs, nil
			},
		},
		{
			PrefixChar: ':',
			IsChar:     func(c byte) bool { return internal.IsLetter(c) || internal.IsDigit(c) },
			Error:      errInvalidLabel,
			Duplicated: func(abs testAbsoluteSpec) bool {
				return abs.Label != nil
			},
			Apply: func(value string, abs testAbsoluteSpec) (testAbsoluteSpec, error) {
				abs.Label = lo.ToPtr(value)
				return abs, nil
			},
		},
	},
	Zero: func() testAbsoluteSpec {
		return testAbsoluteSpec{}
	},
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantSpec   *version.Spec[testAbsoluteSpec]
		wantErrMsg string
	}{
		// Basic name only
		{
			name:  "name only",
			input: "/my/param",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name: "/my/param",
			},
		},
		{
			name:  "name with whitespace",
			input: "  /my/param  ",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name: "/my/param",
			},
		},

		// Version specifier
		{
			name:  "with version",
			input: "/my/param#3",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:     "/my/param",
				Absolute: testAbsoluteSpec{Version: lo.ToPtr(int64(3))},
			},
		},
		{
			name:  "with multi-digit version",
			input: "/my/param#123",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:     "/my/param",
				Absolute: testAbsoluteSpec{Version: lo.ToPtr(int64(123))},
			},
		},

		// Label specifier
		{
			name:  "with label",
			input: "my-secret:AWSCURRENT",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:     "my-secret",
				Absolute: testAbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")},
			},
		},

		// Shift specifier
		{
			name:  "with shift",
			input: "/my/param~1",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:  "/my/param",
				Shift: 1,
			},
		},
		{
			name:  "with double tilde",
			input: "/my/param~~",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:  "/my/param",
				Shift: 2,
			},
		},
		{
			name:  "with tilde only",
			input: "/my/param~",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:  "/my/param",
				Shift: 1,
			},
		},

		// Combined specifiers
		{
			name:  "version and shift",
			input: "/my/param#5~2",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:     "/my/param",
				Absolute: testAbsoluteSpec{Version: lo.ToPtr(int64(5))},
				Shift:    2,
			},
		},
		{
			name:  "label and shift",
			input: "my-secret:AWSCURRENT~1",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:     "my-secret",
				Absolute: testAbsoluteSpec{Label: lo.ToPtr("AWSCURRENT")},
				Shift:    1,
			},
		},

		// Error cases
		{
			name:       "empty input",
			input:      "",
			wantErrMsg: "empty specification",
		},
		{
			name:       "whitespace only",
			input:      "   ",
			wantErrMsg: "empty specification",
		},
		{
			name:       "empty name with version",
			input:      "#3",
			wantErrMsg: "empty name",
		},
		{
			name:       "empty name with shift",
			input:      "~1",
			wantErrMsg: "empty name",
		},
		{
			name:       "invalid version specifier at end",
			input:      "/my/param#",
			wantErrMsg: "# must be followed by",
		},
		{
			name:       "invalid label specifier at end",
			input:      "my-secret:",
			wantErrMsg: ": must be followed by",
		},
		{
			name:       "ambiguous tilde",
			input:      "/my/param~backup",
			wantErrMsg: "ambiguous tilde",
		},
		{
			name:       "duplicate version specifier",
			input:      "/my/param#1#2",
			wantErrMsg: "multiple absolute version specifiers",
		},
		{
			name:       "duplicate label specifier",
			input:      "my-secret:PREV:CURR",
			wantErrMsg: "multiple absolute version specifiers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec, err := version.Parse(tt.input, testParser)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSpec, spec)
		})
	}
}

func TestSpec_HasShift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		spec     *version.Spec[testAbsoluteSpec]
		expected bool
	}{
		{
			name:     "no shift",
			spec:     &version.Spec[testAbsoluteSpec]{Name: "test", Shift: 0},
			expected: false,
		},
		{
			name:     "with shift",
			spec:     &version.Spec[testAbsoluteSpec]{Name: "test", Shift: 1},
			expected: true,
		},
		{
			name:     "with large shift",
			spec:     &version.Spec[testAbsoluteSpec]{Name: "test", Shift: 10},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.spec.HasShift())
		})
	}
}

// testParserNoError tests parser without Error set (PrefixChar becomes part of name).
var testParserNoError = version.AbsoluteParser[testAbsoluteSpec]{
	Parsers: []version.SpecifierParser[testAbsoluteSpec]{
		{
			PrefixChar: '@',
			IsChar:     internal.IsDigit,
			Error:      nil, // No error - treat as part of name
			Apply: func(value string, abs testAbsoluteSpec) (testAbsoluteSpec, error) {
				var v int64
				for _, c := range value {
					v = v*10 + int64(c-'0')
				}
				abs.Version = lo.ToPtr(v)
				return abs, nil
			},
		},
	},
	Zero: func() testAbsoluteSpec {
		return testAbsoluteSpec{}
	},
}

func TestParse_NoErrorParser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantSpec *version.Spec[testAbsoluteSpec]
	}{
		{
			name:  "@ at end treated as part of name",
			input: "user@example.com",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name: "user@example.com",
			},
		},
		{
			name:  "@ followed by letter treated as part of name",
			input: "user@domain",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name: "user@domain",
			},
		},
		{
			name:  "@ followed by digit is specifier",
			input: "param@123",
			wantSpec: &version.Spec[testAbsoluteSpec]{
				Name:     "param",
				Absolute: testAbsoluteSpec{Version: lo.ToPtr(int64(123))},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec, err := version.Parse(tt.input, testParserNoError)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSpec, spec)
		})
	}
}

// testParserWithApplyError tests parser that returns error from Apply.
var testParserWithApplyError = version.AbsoluteParser[testAbsoluteSpec]{
	Parsers: []version.SpecifierParser[testAbsoluteSpec]{
		{
			PrefixChar: '#',
			IsChar:     internal.IsDigit,
			Error:      errInvalidVersion,
			Apply: func(value string, abs testAbsoluteSpec) (testAbsoluteSpec, error) {
				// Simulate overflow or invalid value error
				if value == "999" {
					return abs, errors.New("value too large")
				}
				return abs, nil
			},
		},
	},
	Zero: func() testAbsoluteSpec {
		return testAbsoluteSpec{}
	},
}

func TestParse_ApplyError(t *testing.T) {
	t.Parallel()

	spec, err := version.Parse("param#999", testParserWithApplyError)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid specifier value")
	assert.Contains(t, err.Error(), "999")
	assert.Nil(t, spec)
}

// Test tilde in middle of name (not followed by digit or tilde)
func TestParse_TildeInName(t *testing.T) {
	t.Parallel()

	// Tilde followed by special char should be treated as part of name
	spec, err := version.Parse("/my/param~-suffix", testParser)
	require.NoError(t, err)
	assert.Equal(t, "/my/param~-suffix", spec.Name)
}

// Test unknown character after absolute specifier causes shift parse error
func TestParse_UnknownCharAfterAbsolute(t *testing.T) {
	t.Parallel()

	// After parsing #3, there's an unknown char '$' which breaks parseAbsolute
	// Then shift.Parse tries to parse "$foo" and fails
	spec, err := version.Parse("/my/param#3$foo", testParser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected characters")
	assert.Nil(t, spec)
}

// testParserNoDuplicateCheck tests parser without Duplicated check
var testParserNoDuplicateCheck = version.AbsoluteParser[testAbsoluteSpec]{
	Parsers: []version.SpecifierParser[testAbsoluteSpec]{
		{
			PrefixChar: '#',
			IsChar:     internal.IsDigit,
			Error:      errInvalidVersion,
			Duplicated: nil, // No duplicate check
			Apply: func(value string, abs testAbsoluteSpec) (testAbsoluteSpec, error) {
				var v int64
				for _, c := range value {
					v = v*10 + int64(c-'0')
				}
				abs.Version = lo.ToPtr(v)
				return abs, nil
			},
		},
	},
	Zero: func() testAbsoluteSpec {
		return testAbsoluteSpec{}
	},
}

func TestParse_NoDuplicateCheck(t *testing.T) {
	t.Parallel()

	// Without duplicate check, multiple specifiers just overwrite
	spec, err := version.Parse("param#1#2", testParserNoDuplicateCheck)
	require.NoError(t, err)
	// Last value wins
	assert.Equal(t, lo.ToPtr(int64(2)), spec.Absolute.Version)
}
