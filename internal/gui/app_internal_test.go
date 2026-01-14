//go:build production || dev

package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringError_Error(t *testing.T) {
	t.Parallel()

	err := stringError("test error message")
	assert.Equal(t, "test error message", err.Error())
}

func TestErrInvalidService(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "invalid service: must be 'param' or 'secret'", errInvalidService.Error())
}

func TestNewApp(t *testing.T) {
	t.Parallel()

	app := NewApp()
	assert.NotNil(t, app)
	// Verify fields are nil (lazy initialization)
	assert.Nil(t, app.paramClient)
	assert.Nil(t, app.secretClient)
	assert.Nil(t, app.stagingFactory)
}

func TestApp_Startup(t *testing.T) {
	t.Parallel()

	app := NewApp()
	assert.Nil(t, app.ctx)

	app.Startup(t.Context())
	assert.Equal(t, t.Context(), app.ctx)
}

func TestApp_getService_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		service     string
		expectError bool
	}{
		{
			name:        "uppercase PARAM",
			service:     "PARAM",
			expectError: true,
		},
		{
			name:        "mixed case Secret",
			service:     "Secret",
			expectError: true,
		},
		{
			name:        "with whitespace",
			service:     " param",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := &App{}

			_, err := app.getService(tt.service)
			if tt.expectError {
				assert.ErrorIs(t, err, errInvalidService)
			}
		})
	}
}

func TestApp_getParser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		service     string
		expectError bool
	}{
		{
			name:        "param parser",
			service:     "param",
			expectError: false,
		},
		{
			name:        "secret parser",
			service:     "secret",
			expectError: false,
		},
		{
			name:        "invalid service",
			service:     "invalid",
			expectError: true,
		},
		{
			name:        "empty service",
			service:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := &App{}

			parser, err := app.getParser(tt.service)
			if tt.expectError {
				require.ErrorIs(t, err, errInvalidService)
				assert.Nil(t, parser)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, parser)
			}
		})
	}
}
