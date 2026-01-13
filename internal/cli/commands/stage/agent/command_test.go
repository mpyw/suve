package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := Command()

	require.NotNil(t, cmd)
	assert.Equal(t, "agent", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.Len(t, cmd.Commands, 2)
}

func TestStartCommand(t *testing.T) {
	t.Parallel()

	cmd := startCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "start", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.Action)
}

func TestStopCommand(t *testing.T) {
	t.Parallel()

	cmd := stopCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "stop", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.Action)
}
