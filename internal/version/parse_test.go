package version_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version"
	"github.com/mpyw/suve/internal/version/internal"
)

// Sentinel errors for testing.
var errTestMustHaveNumber = errors.New("# must be followed by a number")

// TestAbsoluteSpec is a simple test struct for absolute specifiers.
type TestAbsoluteSpec struct {
	Number *int
	Label  string
}

// testParser creates a test parser for testing purposes.
func testParser() version.AbsoluteParser[TestAbsoluteSpec] {
	return version.AbsoluteParser[TestAbsoluteSpec]{
		Parsers: []version.SpecifierParser[TestAbsoluteSpec]{
			{
				PrefixChar: '#',
				IsChar:     internal.IsDigit,
				Error:      errTestMustHaveNumber,
				Duplicated: func(abs TestAbsoluteSpec) bool {
					return abs.Number != nil
				},
				Apply: func(value string, abs TestAbsoluteSpec) (TestAbsoluteSpec, error) {
					n := 0
					for _, c := range value {
						n = n*10 + int(c-'0')
					}
					abs.Number = &n
					return abs, nil
				},
			},
			{
				PrefixChar: ':',
				IsChar:     internal.IsLetter,
				Error:      nil, // No error, treat as part of name if not followed by letter
				Duplicated: func(abs TestAbsoluteSpec) bool {
					return abs.Label != ""
				},
				Apply: func(value string, abs TestAbsoluteSpec) (TestAbsoluteSpec, error) {
					abs.Label = value
					return abs, nil
				},
			},
		},
		Zero: func() TestAbsoluteSpec {
			return TestAbsoluteSpec{}
		},
	}
}

func TestSpec_HasShift(t *testing.T) {
	t.Parallel()

	t.Run("no shift", func(t *testing.T) {
		t.Parallel()
		spec := &version.Spec[TestAbsoluteSpec]{
			Name:  "test",
			Shift: 0,
		}
		assert.False(t, spec.HasShift())
	})

	t.Run("with shift", func(t *testing.T) {
		t.Parallel()
		spec := &version.Spec[TestAbsoluteSpec]{
			Name:  "test",
			Shift: 1,
		}
		assert.True(t, spec.HasShift())
	})

	t.Run("negative shift treated as no shift", func(t *testing.T) {
		t.Parallel()
		spec := &version.Spec[TestAbsoluteSpec]{
			Name:  "test",
			Shift: -1,
		}
		assert.False(t, spec.HasShift())
	})
}

func TestParse(t *testing.T) {
	t.Parallel()
	parser := testParser()

	t.Run("name only", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("/my/param", parser)
		require.NoError(t, err)
		assert.Equal(t, "/my/param", spec.Name)
		assert.Nil(t, spec.Absolute.Number)
		assert.Empty(t, spec.Absolute.Label)
		assert.Equal(t, 0, spec.Shift)
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		_, err := version.Parse("", parser)
		assert.ErrorIs(t, err, version.ErrEmptySpec)
	})

	t.Run("whitespace only", func(t *testing.T) {
		t.Parallel()
		_, err := version.Parse("   ", parser)
		assert.ErrorIs(t, err, version.ErrEmptySpec)
	})

	t.Run("empty name with specifier", func(t *testing.T) {
		t.Parallel()
		_, err := version.Parse("#1", parser)
		assert.ErrorIs(t, err, version.ErrEmptyName)
	})

	t.Run("empty name with shift", func(t *testing.T) {
		t.Parallel()
		_, err := version.Parse("~1", parser)
		assert.ErrorIs(t, err, version.ErrEmptyName)
	})

	t.Run("with number specifier", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("/my/param#123", parser)
		require.NoError(t, err)
		assert.Equal(t, "/my/param", spec.Name)
		require.NotNil(t, spec.Absolute.Number)
		assert.Equal(t, 123, *spec.Absolute.Number)
	})

	t.Run("with label specifier", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("secret:CURRENT", parser)
		require.NoError(t, err)
		assert.Equal(t, "secret", spec.Name)
		assert.Equal(t, "CURRENT", spec.Absolute.Label)
	})

	t.Run("with shift only", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("/my/param~2", parser)
		require.NoError(t, err)
		assert.Equal(t, "/my/param", spec.Name)
		assert.Equal(t, 2, spec.Shift)
	})

	t.Run("with number and shift", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("/my/param#5~2", parser)
		require.NoError(t, err)
		assert.Equal(t, "/my/param", spec.Name)
		require.NotNil(t, spec.Absolute.Number)
		assert.Equal(t, 5, *spec.Absolute.Number)
		assert.Equal(t, 2, spec.Shift)
	})

	t.Run("with label and shift", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("secret:CURRENT~1", parser)
		require.NoError(t, err)
		assert.Equal(t, "secret", spec.Name)
		assert.Equal(t, "CURRENT", spec.Absolute.Label)
		assert.Equal(t, 1, spec.Shift)
	})

	t.Run("shift without number", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("/my/param~", parser)
		require.NoError(t, err)
		assert.Equal(t, 1, spec.Shift) // ~ alone = ~1
	})

	t.Run("multiple shifts", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("/my/param~~", parser)
		require.NoError(t, err)
		assert.Equal(t, 2, spec.Shift) // ~~ = ~1~1 = 2
	})

	t.Run("cumulative shifts", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("/my/param~1~2", parser)
		require.NoError(t, err)
		assert.Equal(t, 3, spec.Shift) // ~1~2 = 3
	})

	t.Run("ambiguous tilde", func(t *testing.T) {
		t.Parallel()
		_, err := version.Parse("/my/param~backup", parser)
		assert.ErrorIs(t, err, version.ErrAmbiguousTilde)
	})

	t.Run("invalid specifier - # at end", func(t *testing.T) {
		t.Parallel()
		_, err := version.Parse("/my/param#", parser)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "# must be followed")
	})

	t.Run("# followed by non-digit", func(t *testing.T) {
		t.Parallel()
		_, err := version.Parse("/my/param#abc", parser)
		assert.Error(t, err)
	})

	t.Run("multiple absolute specifiers", func(t *testing.T) {
		t.Parallel()
		_, err := version.Parse("/my/param#1#2", parser)
		assert.ErrorIs(t, err, version.ErrMultipleAbsoluteSpec)
	})

	t.Run("tilde in middle of name - followed by special char", func(t *testing.T) {
		t.Parallel()
		// ~/ is not ambiguous: ~ followed by /, so ~ treated as part of name
		spec, err := version.Parse("/my~/param", parser)
		require.NoError(t, err)
		assert.Equal(t, "/my~/param", spec.Name)
	})

	t.Run("colon not followed by letter - part of name", func(t *testing.T) {
		t.Parallel()
		// : without Error set, not followed by valid char, treated as part of name
		spec, err := version.Parse("secret:123", parser)
		require.NoError(t, err)
		// Since IsChar for : is IsLetter, and 123 starts with digit, : is part of name
		assert.Equal(t, "secret:123", spec.Name)
	})

	t.Run("whitespace trimmed", func(t *testing.T) {
		t.Parallel()
		spec, err := version.Parse("  /my/param  ", parser)
		require.NoError(t, err)
		assert.Equal(t, "/my/param", spec.Name)
	})
}

var errApplyFailed = errors.New("apply failed")

func TestParse_ApplyError(t *testing.T) {
	t.Parallel()

	// Create parser with Apply that returns error
	parser := version.AbsoluteParser[TestAbsoluteSpec]{
		Parsers: []version.SpecifierParser[TestAbsoluteSpec]{
			{
				PrefixChar: '#',
				IsChar:     internal.IsDigit,
				Error:      errTestMustHaveNumber,
				Apply: func(_ string, _ TestAbsoluteSpec) (TestAbsoluteSpec, error) {
					return TestAbsoluteSpec{}, errApplyFailed
				},
			},
		},
		Zero: func() TestAbsoluteSpec {
			return TestAbsoluteSpec{}
		},
	}

	_, err := version.Parse("name#123", parser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid specifier value")
}

func TestParse_NoDuplicatedCheck(t *testing.T) {
	t.Parallel()

	// Create parser with Duplicated as nil
	parser := version.AbsoluteParser[TestAbsoluteSpec]{
		Parsers: []version.SpecifierParser[TestAbsoluteSpec]{
			{
				PrefixChar: '#',
				IsChar:     internal.IsDigit,
				Error:      nil,
				Duplicated: nil, // No duplicate check
				Apply: func(value string, abs TestAbsoluteSpec) (TestAbsoluteSpec, error) {
					n := 0
					for _, c := range value {
						n = n*10 + int(c-'0')
					}
					abs.Number = &n
					return abs, nil
				},
			},
		},
		Zero: func() TestAbsoluteSpec {
			return TestAbsoluteSpec{}
		},
	}

	// With no duplicate check, multiple # should work (last one wins)
	spec, err := version.Parse("name#1#2", parser)
	require.NoError(t, err)
	require.NotNil(t, spec.Absolute.Number)
	assert.Equal(t, 2, *spec.Absolute.Number)
}

func TestParse_InvalidShiftAfterAbsolute(t *testing.T) {
	t.Parallel()
	parser := testParser()

	// Test case where parseShift returns an error after a valid absolute specifier
	// ~! is invalid shift syntax (~ followed by non-digit, non-~, non-end)
	_, err := version.Parse("name#1~!", parser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid shift")
}

func TestParse_UnknownCharAfterAbsolute(t *testing.T) {
	t.Parallel()

	// Create a parser that only accepts # followed by digits
	parser := version.AbsoluteParser[TestAbsoluteSpec]{
		Parsers: []version.SpecifierParser[TestAbsoluteSpec]{
			{
				PrefixChar: '#',
				IsChar:     internal.IsDigit,
				Error:      errTestMustHaveNumber,
				Duplicated: func(abs TestAbsoluteSpec) bool {
					return abs.Number != nil
				},
				Apply: func(value string, abs TestAbsoluteSpec) (TestAbsoluteSpec, error) {
					n := 0
					for _, c := range value {
						n = n*10 + int(c-'0')
					}
					abs.Number = &n
					return abs, nil
				},
			},
		},
		Zero: func() TestAbsoluteSpec {
			return TestAbsoluteSpec{}
		},
	}

	// Test case where an unknown character appears after a valid absolute specifier
	// The '@' character is not handled by any parser, so parseAbsolute should hit
	// the break statement (matchParser returns !ok)
	_, err := version.Parse("name#1@extra", parser)
	require.Error(t, err)
	// The '@' character will cause parseShift to fail since it's unexpected
	assert.Contains(t, err.Error(), "unexpected characters")
}

func TestParse_EmptyParsers(t *testing.T) {
	t.Parallel()

	// Create a parser with no specifier parsers
	parser := version.AbsoluteParser[TestAbsoluteSpec]{
		Parsers: []version.SpecifierParser[TestAbsoluteSpec]{},
		Zero: func() TestAbsoluteSpec {
			return TestAbsoluteSpec{}
		},
	}

	// With no parsers, everything after name should be treated as shift
	// This tests matchParser returning false
	spec, err := version.Parse("name~1", parser)
	require.NoError(t, err)
	assert.Equal(t, "name", spec.Name)
	assert.Equal(t, 1, spec.Shift)
}

func TestParse_MatchParserNoMatch(t *testing.T) {
	t.Parallel()

	// Parser only handles '#', test with a character it doesn't know
	parser := version.AbsoluteParser[TestAbsoluteSpec]{
		Parsers: []version.SpecifierParser[TestAbsoluteSpec]{
			{
				PrefixChar: '#',
				IsChar:     internal.IsDigit,
				Error:      errTestMustHaveNumber,
				Apply: func(value string, abs TestAbsoluteSpec) (TestAbsoluteSpec, error) {
					n := 0
					for _, c := range value {
						n = n*10 + int(c-'0')
					}
					abs.Number = &n
					return abs, nil
				},
			},
		},
		Zero: func() TestAbsoluteSpec {
			return TestAbsoluteSpec{}
		},
	}

	// The '$' character should hit matchParser's "return false" path
	// but in findNameEnd, since '$' is not recognized, the whole thing is name
	spec, err := version.Parse("name$value", parser)
	require.NoError(t, err)
	// Since $ is not a recognized prefix, it's part of the name
	assert.Equal(t, "name$value", spec.Name)
}
