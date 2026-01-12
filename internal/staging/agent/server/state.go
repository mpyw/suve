package server

import (
	"encoding/json"
	"sync"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/agent/server/secmem"
)

// stateKey uniquely identifies a staging state by account and region.
type stateKey struct {
	AccountID string
	Region    string
}

// secureState holds the staging state in secure memory.
type secureState struct {
	mu     sync.RWMutex
	states map[stateKey]*secmem.Buffer
}

// newSecureState creates a new secure state store.
func newSecureState() *secureState {
	return &secureState{
		states: make(map[stateKey]*secmem.Buffer),
	}
}

// get retrieves the state for the given account/region.
func (s *secureState) get(accountID, region string) (*staging.State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := stateKey{AccountID: accountID, Region: region}
	buf, ok := s.states[key]
	if !ok || buf.IsEmpty() {
		return newEmptyState(), nil
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

// set stores the state for the given account/region.
func (s *secureState) set(accountID, region string, state *staging.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := stateKey{AccountID: accountID, Region: region}

	// Destroy old buffer if exists
	if old, ok := s.states[key]; ok {
		old.Destroy()
	}

	// Check if state is empty
	if state.IsEmpty() {
		delete(s.states, key)
		return nil
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	// NewBuffer zeros the input data
	s.states[key] = secmem.NewBuffer(data)
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
	s.states = make(map[stateKey]*secmem.Buffer)
}

// newEmptyState creates a new empty staging state.
func newEmptyState() *staging.State {
	return &staging.State{
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
}

// zeroBytes securely zeros a byte slice.
func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
