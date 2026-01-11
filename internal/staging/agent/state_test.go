package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
)

func TestSecureState_GetSetEmpty(t *testing.T) {
	state := newSecureState()
	defer state.destroy()

	// Get should return empty state for non-existent key
	result, err := state.get("account1", "us-east-1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Version)
	assert.Empty(t, result.Entries[staging.ServiceParam])
	assert.Empty(t, result.Entries[staging.ServiceSecret])
}

func TestSecureState_SetGet(t *testing.T) {
	state := newSecureState()
	defer state.destroy()

	// Create a state with entries
	testState := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/my/param": {
					Operation: staging.OperationCreate,
					Value:     strPtr("test-value"),
					StagedAt:  time.Now(),
				},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	// Set the state
	err := state.set("account1", "us-east-1", testState)
	require.NoError(t, err)

	// Get should return the state
	result, err := state.get("account1", "us-east-1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Entries[staging.ServiceParam], 1)
	assert.Equal(t, "test-value", *result.Entries[staging.ServiceParam]["/my/param"].Value)
}

func TestSecureState_SetOverwrite(t *testing.T) {
	state := newSecureState()
	defer state.destroy()

	// Create initial state
	initialState := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/my/param": {
					Operation: staging.OperationCreate,
					Value:     strPtr("initial"),
					StagedAt:  time.Now(),
				},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	err := state.set("account1", "us-east-1", initialState)
	require.NoError(t, err)

	// Overwrite with new state
	newState := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/my/param": {
					Operation: staging.OperationUpdate,
					Value:     strPtr("updated"),
					StagedAt:  time.Now(),
				},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	err = state.set("account1", "us-east-1", newState)
	require.NoError(t, err)

	// Get should return the new state
	result, err := state.get("account1", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "updated", *result.Entries[staging.ServiceParam]["/my/param"].Value)
}

func TestSecureState_SetEmptyDeletes(t *testing.T) {
	state := newSecureState()
	defer state.destroy()

	// Create state with entries
	testState := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/my/param": {
					Operation: staging.OperationCreate,
					Value:     strPtr("test"),
					StagedAt:  time.Now(),
				},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	err := state.set("account1", "us-east-1", testState)
	require.NoError(t, err)
	assert.False(t, state.isEmpty())

	// Set empty state
	emptyState := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	err = state.set("account1", "us-east-1", emptyState)
	require.NoError(t, err)
	assert.True(t, state.isEmpty())
}

func TestSecureState_IsEmpty(t *testing.T) {
	state := newSecureState()
	defer state.destroy()

	// Initially empty
	assert.True(t, state.isEmpty())

	// Add state
	testState := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/my/param": {
					Operation: staging.OperationCreate,
					Value:     strPtr("test"),
					StagedAt:  time.Now(),
				},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	err := state.set("account1", "us-east-1", testState)
	require.NoError(t, err)
	assert.False(t, state.isEmpty())
}

func TestSecureState_MultipleAccounts(t *testing.T) {
	state := newSecureState()
	defer state.destroy()

	// Set state for account1
	state1 := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/param1": {Operation: staging.OperationCreate, Value: strPtr("value1"), StagedAt: time.Now()},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}
	err := state.set("account1", "us-east-1", state1)
	require.NoError(t, err)

	// Set state for account2
	state2 := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/param2": {Operation: staging.OperationUpdate, Value: strPtr("value2"), StagedAt: time.Now()},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}
	err = state.set("account2", "us-west-2", state2)
	require.NoError(t, err)

	// Verify account1
	result1, err := state.get("account1", "us-east-1")
	require.NoError(t, err)
	assert.Len(t, result1.Entries[staging.ServiceParam], 1)
	assert.Equal(t, "value1", *result1.Entries[staging.ServiceParam]["/param1"].Value)

	// Verify account2
	result2, err := state.get("account2", "us-west-2")
	require.NoError(t, err)
	assert.Len(t, result2.Entries[staging.ServiceParam], 1)
	assert.Equal(t, "value2", *result2.Entries[staging.ServiceParam]["/param2"].Value)
}

func TestSecureState_Destroy(t *testing.T) {
	state := newSecureState()

	// Add some state
	testState := &staging.State{
		Version: 1,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/my/param": {Operation: staging.OperationCreate, Value: strPtr("test"), StagedAt: time.Now()},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}
	err := state.set("account1", "us-east-1", testState)
	require.NoError(t, err)
	assert.False(t, state.isEmpty())

	// Destroy
	state.destroy()
	assert.True(t, state.isEmpty())
}

func TestIsStateEmpty(t *testing.T) {
	tests := []struct {
		name     string
		state    *staging.State
		expected bool
	}{
		{
			name:     "nil state",
			state:    nil,
			expected: true,
		},
		{
			name: "empty entries and tags",
			state: &staging.State{
				Entries: map[staging.Service]map[string]staging.Entry{
					staging.ServiceParam:  {},
					staging.ServiceSecret: {},
				},
				Tags: map[staging.Service]map[string]staging.TagEntry{
					staging.ServiceParam:  {},
					staging.ServiceSecret: {},
				},
			},
			expected: true,
		},
		{
			name: "with param entry",
			state: &staging.State{
				Entries: map[staging.Service]map[string]staging.Entry{
					staging.ServiceParam: {
						"/test": {Operation: staging.OperationCreate},
					},
					staging.ServiceSecret: {},
				},
				Tags: map[staging.Service]map[string]staging.TagEntry{
					staging.ServiceParam:  {},
					staging.ServiceSecret: {},
				},
			},
			expected: false,
		},
		{
			name: "with secret entry",
			state: &staging.State{
				Entries: map[staging.Service]map[string]staging.Entry{
					staging.ServiceParam: {},
					staging.ServiceSecret: {
						"test-secret": {Operation: staging.OperationUpdate},
					},
				},
				Tags: map[staging.Service]map[string]staging.TagEntry{
					staging.ServiceParam:  {},
					staging.ServiceSecret: {},
				},
			},
			expected: false,
		},
		{
			name: "with tag entry",
			state: &staging.State{
				Entries: map[staging.Service]map[string]staging.Entry{
					staging.ServiceParam:  {},
					staging.ServiceSecret: {},
				},
				Tags: map[staging.Service]map[string]staging.TagEntry{
					staging.ServiceParam: {
						"/test": {Add: map[string]string{"key": "value"}},
					},
					staging.ServiceSecret: {},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.state.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func strPtr(s string) *string {
	return &s
}
