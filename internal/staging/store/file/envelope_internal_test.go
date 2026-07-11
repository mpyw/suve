package file

import (
	"encoding/base64"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

// singleParamState builds a single-service (param) state with one create entry.
func singleParamState(name, value string) *staging.State {
	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: name}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(value),
	}

	return state
}

// encryptedEnvelope builds a v2 envelope whose encrypted payload is bound to its
// own header via AAD, mirroring what WriteEnvelopeFile produces.
func encryptedEnvelope(t *testing.T, passphrase string) Envelope {
	t.Helper()

	env := Envelope{
		Version:  EnvelopeVersion,
		Provider: "aws",
		Scope:    "aws/123456789012/ap-northeast-1",
		Service:  "param",
	}

	payload, err := encodePayload(singleParamState("/app/config", "secret-value"), passphrase, env.associatedData())
	require.NoError(t, err)

	env.Payload = payload

	return env
}

// TestDecodeState_AADBindsHeaderFields verifies the header (version/provider/
// scope/service) is authenticated with the ciphertext: the untouched envelope
// round-trips, but tampering with any single header field makes decryption fail
// with crypt.ErrDecryptionFailed.
func TestDecodeState_AADBindsHeaderFields(t *testing.T) {
	t.Parallel()

	base := encryptedEnvelope(t, "correct horse")

	// Baseline: an untampered round-trip still works.
	got, err := base.DecodeState("correct horse")
	require.NoError(t, err)
	assert.Equal(t, "secret-value",
		lo.FromPtr(got.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))

	tampers := map[string]func(e *Envelope){
		"version":  func(e *Envelope) { e.Version = EnvelopeVersion + 1 },
		"provider": func(e *Envelope) { e.Provider = "googlecloud" },
		"scope":    func(e *Envelope) { e.Scope = "aws/999999999999/us-east-1" },
		"service":  func(e *Envelope) { e.Service = "secret" },
	}

	for name, tamper := range tampers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := base
			tamper(&env)

			_, err := env.DecodeState("correct horse")
			require.ErrorIs(t, err, crypt.ErrDecryptionFailed)
		})
	}
}

// TestDecodeState_EncryptedDecryptsToInvalidJSON covers the path where the AAD
// matches and decryption succeeds, but the plaintext is not valid state JSON.
func TestDecodeState_EncryptedDecryptsToInvalidJSON(t *testing.T) {
	t.Parallel()

	env := Envelope{
		Version:  EnvelopeVersion,
		Provider: "aws",
		Scope:    "aws/1/r",
		Service:  "param",
	}

	blob, err := crypt.EncryptWithAAD([]byte("not json"), "pw", env.associatedData())
	require.NoError(t, err)

	env.Payload = base64.StdEncoding.EncodeToString(blob)

	_, err = env.DecodeState("pw")
	require.ErrorIs(t, err, ErrInvalidEnvelope)
}
