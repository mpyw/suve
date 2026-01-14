package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging/store/agent/daemon/lifecycle"
)

// mockStarter implements Starter for testing.
type mockStarter struct {
	err error
}

func (m *mockStarter) Start(_ context.Context) error {
	return m.err
}

// mockPinger implements Pinger for testing.
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ context.Context) error {
	return m.err
}

func TestExecuteWrite_StartSucceeds_ActionSucceeds(t *testing.T) {
	t.Parallel()

	starter := &mockStarter{err: nil}
	ctx := context.Background()

	result, err := lifecycle.ExecuteWrite(ctx, starter, lifecycle.CmdAdd, func() (string, error) {
		return "success", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestExecuteWrite_StartSucceeds_ActionFails(t *testing.T) {
	t.Parallel()

	starter := &mockStarter{err: nil}
	ctx := context.Background()
	actionErr := errors.New("action failed")

	result, err := lifecycle.ExecuteWrite(ctx, starter, lifecycle.CmdEdit, func() (string, error) {
		return "", actionErr
	})

	require.ErrorIs(t, err, actionErr)
	assert.Empty(t, result)
}

func TestExecuteWrite_StartFails(t *testing.T) {
	t.Parallel()

	startErr := errors.New("start failed")
	starter := &mockStarter{err: startErr}
	ctx := context.Background()

	result, err := lifecycle.ExecuteWrite(ctx, starter, lifecycle.CmdDelete, func() (string, error) {
		t.Fatal("action should not be called when Start fails")

		return "", nil
	})

	require.ErrorIs(t, err, startErr)
	assert.Empty(t, result)
}

func TestExecuteWrite_AllCommands(t *testing.T) {
	t.Parallel()

	// Test that all WriteCommand constants can be used
	commands := []lifecycle.WriteCommand{
		lifecycle.CmdAdd,
		lifecycle.CmdEdit,
		lifecycle.CmdDelete,
		lifecycle.CmdTag,
		lifecycle.CmdUntag,
		lifecycle.CmdResetVersion,
		lifecycle.CmdStashPop,
		lifecycle.CmdAgentStart,
	}

	for _, cmd := range commands {
		starter := &mockStarter{err: nil}
		ctx := context.Background()

		result, err := lifecycle.ExecuteWrite(ctx, starter, cmd, func() (int, error) {
			return 42, nil
		})

		require.NoError(t, err)
		assert.Equal(t, 42, result)
	}
}

func TestExecuteRead_PingFails_ReturnsNothingStaged(t *testing.T) {
	t.Parallel()

	pinger := &mockPinger{err: errors.New("agent not running")}
	ctx := context.Background()

	result, err := lifecycle.ExecuteRead(ctx, pinger, lifecycle.CmdStatus, func() (string, error) {
		t.Fatal("action should not be called when Ping fails")

		return "", nil
	})

	require.NoError(t, err)
	assert.True(t, result.NothingStaged)
	assert.Empty(t, result.Value)
}

func TestExecuteRead_PingSucceeds_ActionSucceeds(t *testing.T) {
	t.Parallel()

	pinger := &mockPinger{err: nil}
	ctx := context.Background()

	result, err := lifecycle.ExecuteRead(ctx, pinger, lifecycle.CmdDiff, func() (string, error) {
		return "diff output", nil
	})

	require.NoError(t, err)
	assert.False(t, result.NothingStaged)
	assert.Equal(t, "diff output", result.Value)
}

func TestExecuteRead_PingSucceeds_ActionFails(t *testing.T) {
	t.Parallel()

	pinger := &mockPinger{err: nil}
	ctx := context.Background()
	actionErr := errors.New("action failed")

	result, err := lifecycle.ExecuteRead(ctx, pinger, lifecycle.CmdApply, func() (string, error) {
		return "", actionErr
	})

	require.ErrorIs(t, err, actionErr)
	assert.False(t, result.NothingStaged)
	assert.Empty(t, result.Value)
}

func TestExecuteRead_AllCommands(t *testing.T) {
	t.Parallel()

	// Test that all ReadCommand constants can be used
	commands := []lifecycle.ReadCommand{
		lifecycle.CmdStatus,
		lifecycle.CmdDiff,
		lifecycle.CmdApply,
		lifecycle.CmdResetAll,
		lifecycle.CmdReset,
		lifecycle.CmdStashPush,
		lifecycle.CmdAgentStop,
	}

	for _, cmd := range commands {
		pinger := &mockPinger{err: nil}
		ctx := context.Background()

		result, err := lifecycle.ExecuteRead(ctx, pinger, cmd, func() (int, error) {
			return 42, nil
		})

		require.NoError(t, err)
		assert.False(t, result.NothingStaged)
		assert.Equal(t, 42, result.Value)
	}
}

func TestExecuteFile_ActionSucceeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	result, err := lifecycle.ExecuteFile(ctx, lifecycle.CmdStashShow, func() (string, error) {
		return "file content", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "file content", result)
}

func TestExecuteFile_ActionFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	actionErr := errors.New("file read failed")

	result, err := lifecycle.ExecuteFile(ctx, lifecycle.CmdStashDrop, func() (string, error) {
		return "", actionErr
	})

	require.ErrorIs(t, err, actionErr)
	assert.Empty(t, result)
}

func TestExecuteFile_AllCommands(t *testing.T) {
	t.Parallel()

	// Test that all FileCommand constants can be used
	commands := []lifecycle.FileCommand{
		lifecycle.CmdStashShow,
		lifecycle.CmdStashDrop,
	}

	for _, cmd := range commands {
		ctx := context.Background()

		result, err := lifecycle.ExecuteFile(ctx, cmd, func() (int, error) {
			return 42, nil
		})

		require.NoError(t, err)
		assert.Equal(t, 42, result)
	}
}

func TestResult_ZeroValue(t *testing.T) {
	t.Parallel()

	// Test that zero value of Result has expected defaults
	var result lifecycle.Result[string]

	assert.Empty(t, result.Value)
	assert.False(t, result.NothingStaged)
}

func TestResult_WithValue(t *testing.T) {
	t.Parallel()

	result := lifecycle.Result[string]{
		Value:         "test",
		NothingStaged: false,
	}

	assert.Equal(t, "test", result.Value)
	assert.False(t, result.NothingStaged)
}

func TestResult_NothingStaged(t *testing.T) {
	t.Parallel()

	result := lifecycle.Result[string]{
		NothingStaged: true,
	}

	assert.Empty(t, result.Value)
	assert.True(t, result.NothingStaged)
}

// Test with complex types to ensure generics work correctly.
func TestExecuteWrite_ComplexType(t *testing.T) {
	t.Parallel()

	type Output struct {
		Name  string
		Count int
	}

	starter := &mockStarter{err: nil}
	ctx := context.Background()

	result, err := lifecycle.ExecuteWrite(ctx, starter, lifecycle.CmdAdd, func() (*Output, error) {
		return &Output{Name: "test", Count: 5}, nil
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 5, result.Count)
}

func TestExecuteRead_ComplexType(t *testing.T) {
	t.Parallel()

	type Output struct {
		Items []string
	}

	pinger := &mockPinger{err: nil}
	ctx := context.Background()

	result, err := lifecycle.ExecuteRead(ctx, pinger, lifecycle.CmdStatus, func() (*Output, error) {
		return &Output{Items: []string{"a", "b", "c"}}, nil
	})

	require.NoError(t, err)
	assert.False(t, result.NothingStaged)
	require.NotNil(t, result.Value)
	assert.Equal(t, []string{"a", "b", "c"}, result.Value.Items)
}

func TestExecuteFile_ComplexType(t *testing.T) {
	t.Parallel()

	type FileContent struct {
		Data []byte
	}

	ctx := context.Background()

	result, err := lifecycle.ExecuteFile(ctx, lifecycle.CmdStashShow, func() (*FileContent, error) {
		return &FileContent{Data: []byte{1, 2, 3}}, nil
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []byte{1, 2, 3}, result.Data)
}

// Tests for Execute*0 variants (no return value).

func TestExecuteWrite0_StartSucceeds_ActionSucceeds(t *testing.T) {
	t.Parallel()

	starter := &mockStarter{err: nil}
	ctx := context.Background()

	err := lifecycle.ExecuteWrite0(ctx, starter, lifecycle.CmdAdd, func() error {
		return nil
	})

	require.NoError(t, err)
}

func TestExecuteWrite0_StartSucceeds_ActionFails(t *testing.T) {
	t.Parallel()

	starter := &mockStarter{err: nil}
	ctx := context.Background()
	actionErr := errors.New("action failed")

	err := lifecycle.ExecuteWrite0(ctx, starter, lifecycle.CmdEdit, func() error {
		return actionErr
	})

	require.ErrorIs(t, err, actionErr)
}

func TestExecuteWrite0_StartFails(t *testing.T) {
	t.Parallel()

	startErr := errors.New("start failed")
	starter := &mockStarter{err: startErr}
	ctx := context.Background()

	err := lifecycle.ExecuteWrite0(ctx, starter, lifecycle.CmdDelete, func() error {
		t.Fatal("action should not be called when Start fails")

		return nil
	})

	require.ErrorIs(t, err, startErr)
}

func TestExecuteRead0_PingFails_ReturnsNothingStaged(t *testing.T) {
	t.Parallel()

	pinger := &mockPinger{err: errors.New("agent not running")}
	ctx := context.Background()

	result, err := lifecycle.ExecuteRead0(ctx, pinger, lifecycle.CmdStatus, func() error {
		t.Fatal("action should not be called when Ping fails")

		return nil
	})

	require.NoError(t, err)
	assert.True(t, result.NothingStaged)
}

func TestExecuteRead0_PingSucceeds_ActionSucceeds(t *testing.T) {
	t.Parallel()

	pinger := &mockPinger{err: nil}
	ctx := context.Background()

	result, err := lifecycle.ExecuteRead0(ctx, pinger, lifecycle.CmdDiff, func() error {
		return nil
	})

	require.NoError(t, err)
	assert.False(t, result.NothingStaged)
}

func TestExecuteRead0_PingSucceeds_ActionFails(t *testing.T) {
	t.Parallel()

	pinger := &mockPinger{err: nil}
	ctx := context.Background()
	actionErr := errors.New("action failed")

	result, err := lifecycle.ExecuteRead0(ctx, pinger, lifecycle.CmdApply, func() error {
		return actionErr
	})

	require.ErrorIs(t, err, actionErr)
	assert.False(t, result.NothingStaged)
}

func TestExecuteFile0_ActionSucceeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	err := lifecycle.ExecuteFile0(ctx, lifecycle.CmdStashShow, func() error {
		return nil
	})

	require.NoError(t, err)
}

func TestExecuteFile0_ActionFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	actionErr := errors.New("file read failed")

	err := lifecycle.ExecuteFile0(ctx, lifecycle.CmdStashDrop, func() error {
		return actionErr
	})

	require.ErrorIs(t, err, actionErr)
}

func TestResult0_ZeroValue(t *testing.T) {
	t.Parallel()

	var result lifecycle.Result0

	assert.False(t, result.NothingStaged)
}
