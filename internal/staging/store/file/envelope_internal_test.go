// In-package tests of the envelope (the file-name namespace envelopeInternal
// names no unit of its own).
//declscope:namespace envelope

package file

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

// singleParamEnvelopeState builds a single-service (param) state with one create entry.
func singleParamEnvelopeState(name, value string) *staging.State {
	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: name}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new(value),
	}

	return state
}

// encryptedEnvelope builds a v2 envelope whose encrypted payload is bound to its
// own header via AAD, mirroring what WriteEnvelopeFile produces.
func encryptedEnvelope(t *testing.T, passphrase string) Envelope {
	t.Helper()

	env := Envelope{
		Version:  envelopeVersion,
		Provider: "aws",
		Scope:    "aws/123456789012/ap-northeast-1",
		Service:  "param",
	}

	payload, err := encodeEnvelopePayload(singleParamEnvelopeState("/app/config", "secret-value"), passphrase, env.associatedData())
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
		"version":  func(e *Envelope) { e.Version = envelopeVersion + 1 },
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
		Version:  envelopeVersion,
		Provider: "aws",
		Scope:    "aws/1/r",
		Service:  "param",
	}

	blob, err := crypt.EncryptWithAAD([]byte("not json"), "pw", env.associatedData())
	require.NoError(t, err)

	env.Payload = base64.StdEncoding.EncodeToString(blob)

	_, err = env.DecodeState("pw")
	require.ErrorIs(t, err, errInvalidEnvelope)
}

// paramState builds a single-service (param) state with one create entry.
func envelopeParamState(name, value string) *staging.State {
	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: name}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new(value),
	}

	return state
}

// secretState builds a single-service (secret) state with one create entry.
func envelopeSecretState(name, value string) *staging.State {
	state := staging.NewEmptyState()
	state.Entries[staging.ServiceSecret][staging.EntryKey{Name: name}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new(value),
	}

	return state
}

func TestWriteAndReadEnvelope_Encrypted(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "param.json")
	scope := provider.AWSScope("123456789012", "ap-northeast-1")
	state := envelopeParamState("/app/config", "secret-value")

	err := WriteEnvelopeFile(path, scope, staging.ServiceParam, state, "correct horse")
	require.NoError(t, err)

	env, err := ReadEnvelopeFile(path)
	require.NoError(t, err)
	assert.Equal(t, envelopeVersion, env.Version)
	assert.Equal(t, "aws", env.Provider)
	assert.Equal(t, "aws/123456789012/ap-northeast-1", env.Scope)
	assert.Equal(t, "param", env.Service)

	encrypted, err := env.IsEncryptedPayload()
	require.NoError(t, err)
	assert.True(t, encrypted)

	got, err := env.DecodeState("correct horse")
	require.NoError(t, err)
	assert.Equal(t, "secret-value",
		lo.FromPtr(got.Entries[staging.ServiceParam][staging.EntryKey{Name: "/app/config"}].Value))
}

func TestWriteAndReadEnvelope_Plaintext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secret.json")
	scope := provider.AWSScope("123456789012", "us-east-1")
	state := envelopeSecretState("my-secret", "plain-value")

	// Empty passphrase => plaintext payload.
	err := WriteEnvelopeFile(path, scope, staging.ServiceSecret, state, "")
	require.NoError(t, err)

	env, err := ReadEnvelopeFile(path)
	require.NoError(t, err)
	assert.Equal(t, "secret", env.Service)

	encrypted, err := env.IsEncryptedPayload()
	require.NoError(t, err)
	assert.False(t, encrypted)

	// A passphrase is ignored for a plaintext payload.
	got, err := env.DecodeState("ignored")
	require.NoError(t, err)
	assert.Equal(t, "plain-value",
		lo.FromPtr(got.Entries[staging.ServiceSecret][staging.EntryKey{Name: "my-secret"}].Value))
}

func TestWriteAndReadEnvelope_TagsAndNamespace(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "param.json")
	scope := provider.AzureAppConfigScope("mystore")

	state := staging.NewEmptyState()
	key := staging.EntryKey{Name: "k", Namespace: "dev"}
	state.Entries[staging.ServiceParam][key] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("v"),
	}
	state.Tags[staging.ServiceParam][key] = staging.TagEntry{
		Add: map[string]string{"env": "dev"},
	}

	err := WriteEnvelopeFile(path, scope, staging.ServiceParam, state, "pw")
	require.NoError(t, err)

	env, err := ReadEnvelopeFile(path)
	require.NoError(t, err)

	got, err := env.DecodeState("pw")
	require.NoError(t, err)
	assert.Equal(t, "v", lo.FromPtr(got.Entries[staging.ServiceParam][key].Value))
	assert.Equal(t, "dev", got.Tags[staging.ServiceParam][key].Add["env"])
}

func TestDecodeState_WrongPassphrase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "param.json")
	scope := provider.AWSScope("123456789012", "ap-northeast-1")

	err := WriteEnvelopeFile(path, scope, staging.ServiceParam, envelopeParamState("/k", "v"), "right")
	require.NoError(t, err)

	env, err := ReadEnvelopeFile(path)
	require.NoError(t, err)

	_, err = env.DecodeState("wrong")
	require.ErrorIs(t, err, crypt.ErrDecryptionFailed)
}

func TestDecodeState_EncryptedButNoPassphrase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "param.json")
	scope := provider.AWSScope("123456789012", "ap-northeast-1")

	err := WriteEnvelopeFile(path, scope, staging.ServiceParam, envelopeParamState("/k", "v"), "pw")
	require.NoError(t, err)

	env, err := ReadEnvelopeFile(path)
	require.NoError(t, err)

	_, err = env.DecodeState("")
	require.ErrorIs(t, err, crypt.ErrDecryptionFailed)
}

func TestDecodeState_ServiceMismatchDropsForeignData(t *testing.T) {
	t.Parallel()

	// A plaintext payload holding a full multi-service state, but the header
	// declares only "param": DecodeState must return only param entries.
	full := staging.NewEmptyState()
	full.Entries[staging.ServiceParam][staging.EntryKey{Name: "/p"}] = staging.Entry{
		Operation: staging.OperationCreate, Value: new("pv"),
	}
	full.Entries[staging.ServiceSecret][staging.EntryKey{Name: "s"}] = staging.Entry{
		Operation: staging.OperationCreate, Value: new("sv"),
	}

	raw, err := json.Marshal(full) //nolint:errchkjson // State has a custom MarshalJSON
	require.NoError(t, err)

	env := &Envelope{
		Version:  envelopeVersion,
		Provider: "aws",
		Scope:    "aws/1/r",
		Service:  "param",
		Payload:  base64.StdEncoding.EncodeToString(raw),
	}

	got, err := env.DecodeState("")
	require.NoError(t, err)
	assert.Len(t, got.Entries[staging.ServiceParam], 1)
	assert.Empty(t, got.Entries[staging.ServiceSecret], "foreign service must be dropped")
}

func TestDecodeState_RejectsNamespaceForAgnosticProvider(t *testing.T) {
	t.Parallel()

	// An AWS (namespace-agnostic) envelope whose payload smuggles a
	// namespace-bearing param entry must be rejected, not silently kept.
	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: "/p", Namespace: "prod"}] = staging.Entry{
		Operation: staging.OperationCreate, Value: new("v"),
	}

	raw, err := json.Marshal(state) //nolint:errchkjson // State has a custom MarshalJSON
	require.NoError(t, err)

	env := &Envelope{
		Version:  envelopeVersion,
		Provider: "aws",
		Scope:    "aws/1/r",
		Service:  "param",
		Payload:  base64.StdEncoding.EncodeToString(raw),
	}

	_, err = env.DecodeState("")
	require.ErrorIs(t, err, errInvalidEnvelope)
	assert.Contains(t, err.Error(), "namespace")
}

func TestWriteEnvelopeFile_WriteErrors(t *testing.T) {
	t.Parallel()

	scope := provider.AWSScope("1", "r")
	state := envelopeParamState("/k", "v")

	t.Run("mkdir fails when a parent path component is a file", func(t *testing.T) {
		t.Parallel()

		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

		// The parent directory cannot be created because "blocker" is a file.
		err := WriteEnvelopeFile(filepath.Join(blocker, "sub", "param.json"), scope, staging.ServiceParam, state, "")
		require.Error(t, err)
	})

	t.Run("atomic write fails when the target path is a directory", func(t *testing.T) {
		t.Parallel()

		target := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, os.Mkdir(target, 0o700))

		// Renaming the temp file over an existing directory fails.
		err := WriteEnvelopeFile(target, scope, staging.ServiceParam, state, "")
		require.Error(t, err)
	})
}

func TestWriteEnvelope_ScopesToService(t *testing.T) {
	t.Parallel()

	// A multi-service state written as "param" must not leak secret entries.
	full := staging.NewEmptyState()
	full.Entries[staging.ServiceParam][staging.EntryKey{Name: "/p"}] = staging.Entry{
		Operation: staging.OperationCreate, Value: new("pv"),
	}
	full.Entries[staging.ServiceSecret][staging.EntryKey{Name: "s"}] = staging.Entry{
		Operation: staging.OperationCreate, Value: new("super-secret"),
	}

	path := filepath.Join(t.TempDir(), "param.json")
	err := WriteEnvelopeFile(path, provider.AWSScope("1", "r"), staging.ServiceParam, full, "")
	require.NoError(t, err)

	env, err := ReadEnvelopeFile(path)
	require.NoError(t, err)

	got, err := env.DecodeState("")
	require.NoError(t, err)
	assert.Len(t, got.Entries[staging.ServiceParam], 1)
	assert.Empty(t, got.Entries[staging.ServiceSecret])

	// The other service's secret must not be present anywhere in the file.
	fileBytes, err := os.ReadFile(path) //nolint:gosec // path is a file just written to t.TempDir()
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(env.Payload)
	require.NoError(t, err)
	assert.NotContains(t, string(decoded), "super-secret")
	assert.NotContains(t, string(fileBytes), "super-secret")
}

func TestPayloadBytes_PlaintextVsEncrypted(t *testing.T) {
	t.Parallel()

	scope := provider.AWSScope("1", "r")

	t.Run("plaintext payload contains the secret", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, WriteEnvelopeFile(path, scope, staging.ServiceParam, envelopeParamState("/k", "cleartext-secret"), ""))

		env, err := ReadEnvelopeFile(path)
		require.NoError(t, err)
		decoded, err := base64.StdEncoding.DecodeString(env.Payload)
		require.NoError(t, err)
		assert.Contains(t, string(decoded), "cleartext-secret")
	})

	t.Run("encrypted payload hides the secret", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, WriteEnvelopeFile(path, scope, staging.ServiceParam, envelopeParamState("/k", "hidden-secret"), "pw"))

		env, err := ReadEnvelopeFile(path)
		require.NoError(t, err)
		decoded, err := base64.StdEncoding.DecodeString(env.Payload)
		require.NoError(t, err)
		assert.NotContains(t, string(decoded), "hidden-secret")
		assert.True(t, crypt.IsEncrypted(decoded))
	})
}

func TestReadEnvelopeFile_Errors(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		_, err := ReadEnvelopeFile(filepath.Join(t.TempDir(), "nope.json"))
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "bad.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

		_, err := ReadEnvelopeFile(path)
		require.ErrorIs(t, err, errInvalidEnvelope)
	})

	t.Run("unsupported version", func(t *testing.T) {
		t.Parallel()

		// An older version (e.g. the pre-AAD v1 format) is guided to re-export;
		// a newer version is guided to upgrade suve.
		cases := []struct {
			ver  int
			want string
		}{
			{ver: 1, want: "stage export"},
			{ver: 99, want: "upgrade suve"},
		}
		for _, tc := range cases {
			path := filepath.Join(t.TempDir(), "old.json")
			data, err := json.Marshal(Envelope{
				Version:  tc.ver,
				Provider: "aws",
				Scope:    "aws/1/r",
				Service:  "param",
				Payload:  base64.StdEncoding.EncodeToString([]byte("{}")),
			})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o600))

			_, err = ReadEnvelopeFile(path)
			require.ErrorIs(t, err, errUnsupportedEnvelopeVersion)
			// The error must guide the user to the right remedy for the direction.
			assert.Contains(t, err.Error(), tc.want)
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		t.Parallel()

		cases := map[string]Envelope{
			"all empty":       {Version: envelopeVersion},
			"missing scope":   {Version: envelopeVersion, Provider: "aws", Service: "param", Payload: "eyJ9"},
			"missing payload": {Version: envelopeVersion, Provider: "aws", Scope: "aws/1/r", Service: "param"},
		}

		for name, env := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				path := filepath.Join(t.TempDir(), "e.json")
				data, err := json.Marshal(env)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, data, 0o600))

				_, err = ReadEnvelopeFile(path)
				require.ErrorIs(t, err, errInvalidEnvelope)
			})
		}
	})
}

func TestDecodeState_CorruptedPayload(t *testing.T) {
	t.Parallel()

	base := Envelope{
		Version:  envelopeVersion,
		Provider: "aws",
		Scope:    "aws/1/r",
		Service:  "param",
	}

	t.Run("bad base64", func(t *testing.T) {
		t.Parallel()

		env := base
		env.Payload = "!!!not-base64!!!"

		_, err := env.DecodeState("")
		require.ErrorIs(t, err, errInvalidEnvelope)
	})

	t.Run("plaintext payload with invalid json", func(t *testing.T) {
		t.Parallel()

		env := base
		env.Payload = base64.StdEncoding.EncodeToString([]byte("not json"))

		_, err := env.DecodeState("")
		require.ErrorIs(t, err, errInvalidEnvelope)
	})

	// The "encrypted payload decrypts to invalid json" case needs the exact AAD
	// the header binds, so it lives in envelope_internal_test.go.
}

func TestIsEncryptedPayload_BadBase64(t *testing.T) {
	t.Parallel()

	env := &Envelope{Payload: "!!!not-base64!!!"}

	_, err := env.IsEncryptedPayload()
	require.ErrorIs(t, err, errInvalidEnvelope)
}

func TestWriteEnvelope_ProviderScopeFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		scope        provider.Scope
		svc          staging.Service
		wantProvider string
		wantScope    string
	}{
		{
			name:         "aws param",
			scope:        provider.AWSScope("123456789012", "ap-northeast-1"),
			svc:          staging.ServiceParam,
			wantProvider: "aws",
			wantScope:    "aws/123456789012/ap-northeast-1",
		},
		{
			name:         "gcloud secret",
			scope:        provider.GoogleCloudScope("my-project"),
			svc:          staging.ServiceSecret,
			wantProvider: "googlecloud",
			wantScope:    "googlecloud/my-project",
		},
		{
			name:         "azure keyvault secret",
			scope:        provider.AzureKeyVaultScope("myvault"),
			svc:          staging.ServiceSecret,
			wantProvider: "azure",
			wantScope:    "azure/keyvault/myvault",
		},
		{
			name:         "azure appconfig param",
			scope:        provider.AzureAppConfigScope("mystore"),
			svc:          staging.ServiceParam,
			wantProvider: "azure",
			wantScope:    "azure/appconfig/mystore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var state *staging.State
			if tt.svc == staging.ServiceParam {
				state = envelopeParamState("/k", "v")
			} else {
				state = envelopeSecretState("k", "v")
			}

			path := filepath.Join(t.TempDir(), string(tt.svc)+".json")
			err := WriteEnvelopeFile(path, tt.scope, tt.svc, state, "")
			require.NoError(t, err)

			env, err := ReadEnvelopeFile(path)
			require.NoError(t, err)
			assert.Equal(t, tt.wantProvider, env.Provider)
			assert.Equal(t, tt.wantScope, env.Scope)
			assert.Equal(t, string(tt.svc), env.Service)
		})
	}
}
