package server

import (
	"encoding/json"
	"sync"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/agent/internal/server/security"
)

// secureState holds the staging state in secure memory.
type secureState struct {
	mu     sync.RWMutex
	states map[staging.Scope]*security.Buffer
}

// newSecureState creates a new secure state store.
func newSecureState() *secureState {
	return &secureState{
		states: make(map[staging.Scope]*security.Buffer),
	}
}

// get retrieves the state for the given scope.
func (s *secureState) get(scope staging.Scope) (*staging.State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	buf, ok := s.states[scope]
	if !ok || buf.IsEmpty() {
		return staging.NewEmptyState(), nil
	}

	data, err := buf.Bytes()
	if err != nil {
		return nil, err
	}
	defer zeroBytes(data)

	var state staging.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// set stores the state for the given scope.
func (s *secureState) set(scope staging.Scope, state *staging.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Destroy old buffer if exists
	if old, ok := s.states[scope]; ok {
		old.Destroy()
	}

	// Check if state is empty
	if state.IsEmpty() {
		delete(s.states, scope)

		return nil
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	// NewBuffer zeros the input data
	s.states[scope] = security.NewBuffer(data)

	return nil
}

// isEmpty checks if all states are empty.
func (s *secureState) isEmpty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.states) == 0
}

// destroy securely destroys all state data.
func (s *secureState) destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, buf := range s.states {
		buf.Destroy()
	}

	s.states = make(map[staging.Scope]*security.Buffer)
}

// zeroBytes securely zeros a byte slice.
func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
