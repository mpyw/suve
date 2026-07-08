package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
)

// peekStub is a stashPeeker double for stashExistsMessage.
type peekStub struct {
	encrypted   bool
	encErr      error
	state       *staging.State
	drainErr    error
	drainCalled bool
}

func (p *peekStub) IsEncrypted() (bool, error) { return p.encrypted, p.encErr }

func (p *peekStub) Drain(_ context.Context, _ staging.Service, _ bool) (*staging.State, error) {
	p.drainCalled = true

	return p.state, p.drainErr
}

func TestStashExistsMessage(t *testing.T) {
	t.Parallel()

	t.Run("encrypted stash is announced without decrypting (regression #328)", func(t *testing.T) {
		t.Parallel()

		// Drain would fail on this passphrase-less store; it must NOT be called.
		p := &peekStub{encrypted: true, drainErr: errors.New("decryption failed")}

		msg, err := stashExistsMessage(t.Context(), p)
		require.NoError(t, err)
		assert.Contains(t, msg, "encrypted")
		assert.False(t, p.drainCalled, "must not attempt to decrypt an encrypted stash for the pre-count")
	})

	t.Run("plaintext stash is counted", func(t *testing.T) {
		t.Parallel()

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/a"] = staging.Entry{Operation: staging.OperationCreate}
		state.Entries[staging.ServiceParam]["/b"] = staging.Entry{Operation: staging.OperationCreate}

		p := &peekStub{encrypted: false, state: state}

		msg, err := stashExistsMessage(t.Context(), p)
		require.NoError(t, err)
		assert.True(t, p.drainCalled)
		assert.Contains(t, msg, "2 item(s)")
	})

	t.Run("IsEncrypted error is surfaced", func(t *testing.T) {
		t.Parallel()

		p := &peekStub{encErr: errors.New("stat failed")}

		_, err := stashExistsMessage(t.Context(), p)
		require.Error(t, err)
		assert.False(t, p.drainCalled)
	})
}
