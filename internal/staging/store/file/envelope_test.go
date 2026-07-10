package file_test

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
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

// paramState builds a single-service (param) state with one create entry.
func paramState(name, value string) *staging.State {
	state := staging.NewEmptyState()
	state.Entries[staging.ServiceParam][staging.EntryKey{Name: name}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(value),
	}

	return state
}

// secretState builds a single-service (secret) state with one create entry.
func secretState(name, value string) *staging.State {
	state := staging.NewEmptyState()
	state.Entries[staging.ServiceSecret][staging.EntryKey{Name: name}] = staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(value),
	}

	return state
}

func TestWriteAndReadEnvelope_Encrypted(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "param.json")
	scope := provider.AWSScope("123456789012", "ap-northeast-1")
	state := paramState("/app/config", "secret-value")

	err := file.WriteEnvelopeFile(path, scope, staging.ServiceParam, state, "correct horse")
	require.NoError(t, err)

	env, err := file.ReadEnvelopeFile(path)
	require.NoError(t, err)
	assert.Equal(t, file.EnvelopeVersion, env.Version)
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
	state := secretState("my-secret", "plain-value")

	// Empty passphrase => plaintext payload.
	err := file.WriteEnvelopeFile(path, scope, staging.ServiceSecret, state, "")
	require.NoError(t, err)

	env, err := file.ReadEnvelopeFile(path)
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
		Value:     lo.ToPtr("v"),
	}
	state.Tags[staging.ServiceParam][key] = staging.TagEntry{
		Add: map[string]string{"env": "dev"},
	}

	err := file.WriteEnvelopeFile(path, scope, staging.ServiceParam, state, "pw")
	require.NoError(t, err)

	env, err := file.ReadEnvelopeFile(path)
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

	err := file.WriteEnvelopeFile(path, scope, staging.ServiceParam, paramState("/k", "v"), "right")
	require.NoError(t, err)

	env, err := file.ReadEnvelopeFile(path)
	require.NoError(t, err)

	_, err = env.DecodeState("wrong")
	require.ErrorIs(t, err, crypt.ErrDecryptionFailed)
}

func TestDecodeState_EncryptedButNoPassphrase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "param.json")
	scope := provider.AWSScope("123456789012", "ap-northeast-1")

	err := file.WriteEnvelopeFile(path, scope, staging.ServiceParam, paramState("/k", "v"), "pw")
	require.NoError(t, err)

	env, err := file.ReadEnvelopeFile(path)
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
		Operation: staging.OperationCreate, Value: lo.ToPtr("pv"),
	}
	full.Entries[staging.ServiceSecret][staging.EntryKey{Name: "s"}] = staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("sv"),
	}

	raw, err := json.Marshal(full) //nolint:errchkjson // State has a custom MarshalJSON
	require.NoError(t, err)

	env := &file.Envelope{
		Version:  file.EnvelopeVersion,
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

func TestWriteEnvelopeFile_WriteErrors(t *testing.T) {
	t.Parallel()

	scope := provider.AWSScope("1", "r")
	state := paramState("/k", "v")

	t.Run("mkdir fails when a parent path component is a file", func(t *testing.T) {
		t.Parallel()

		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

		// The parent directory cannot be created because "blocker" is a file.
		err := file.WriteEnvelopeFile(filepath.Join(blocker, "sub", "param.json"), scope, staging.ServiceParam, state, "")
		require.Error(t, err)
	})

	t.Run("atomic write fails when the target path is a directory", func(t *testing.T) {
		t.Parallel()

		target := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, os.Mkdir(target, 0o700))

		// Renaming the temp file over an existing directory fails.
		err := file.WriteEnvelopeFile(target, scope, staging.ServiceParam, state, "")
		require.Error(t, err)
	})
}

func TestWriteEnvelope_ScopesToService(t *testing.T) {
	t.Parallel()

	// A multi-service state written as "param" must not leak secret entries.
	full := staging.NewEmptyState()
	full.Entries[staging.ServiceParam][staging.EntryKey{Name: "/p"}] = staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("pv"),
	}
	full.Entries[staging.ServiceSecret][staging.EntryKey{Name: "s"}] = staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("super-secret"),
	}

	path := filepath.Join(t.TempDir(), "param.json")
	err := file.WriteEnvelopeFile(path, provider.AWSScope("1", "r"), staging.ServiceParam, full, "")
	require.NoError(t, err)

	env, err := file.ReadEnvelopeFile(path)
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
		require.NoError(t, file.WriteEnvelopeFile(path, scope, staging.ServiceParam, paramState("/k", "cleartext-secret"), ""))

		env, err := file.ReadEnvelopeFile(path)
		require.NoError(t, err)
		decoded, err := base64.StdEncoding.DecodeString(env.Payload)
		require.NoError(t, err)
		assert.Contains(t, string(decoded), "cleartext-secret")
	})

	t.Run("encrypted payload hides the secret", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "param.json")
		require.NoError(t, file.WriteEnvelopeFile(path, scope, staging.ServiceParam, paramState("/k", "hidden-secret"), "pw"))

		env, err := file.ReadEnvelopeFile(path)
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

		_, err := file.ReadEnvelopeFile(filepath.Join(t.TempDir(), "nope.json"))
		require.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "bad.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

		_, err := file.ReadEnvelopeFile(path)
		require.ErrorIs(t, err, file.ErrInvalidEnvelope)
	})

	t.Run("unsupported version", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "v99.json")
		data, err := json.Marshal(file.Envelope{
			Version:  99,
			Provider: "aws",
			Scope:    "aws/1/r",
			Service:  "param",
			Payload:  base64.StdEncoding.EncodeToString([]byte("{}")),
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))

		_, err = file.ReadEnvelopeFile(path)
		require.ErrorIs(t, err, file.ErrUnsupportedEnvelopeVersion)
	})

	t.Run("missing required fields", func(t *testing.T) {
		t.Parallel()

		cases := map[string]file.Envelope{
			"all empty":       {Version: file.EnvelopeVersion},
			"missing scope":   {Version: file.EnvelopeVersion, Provider: "aws", Service: "param", Payload: "eyJ9"},
			"missing payload": {Version: file.EnvelopeVersion, Provider: "aws", Scope: "aws/1/r", Service: "param"},
		}

		for name, env := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				path := filepath.Join(t.TempDir(), "e.json")
				data, err := json.Marshal(env)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, data, 0o600))

				_, err = file.ReadEnvelopeFile(path)
				require.ErrorIs(t, err, file.ErrInvalidEnvelope)
			})
		}
	})
}

func TestDecodeState_CorruptedPayload(t *testing.T) {
	t.Parallel()

	base := file.Envelope{
		Version:  file.EnvelopeVersion,
		Provider: "aws",
		Scope:    "aws/1/r",
		Service:  "param",
	}

	t.Run("bad base64", func(t *testing.T) {
		t.Parallel()

		env := base
		env.Payload = "!!!not-base64!!!"

		_, err := env.DecodeState("")
		require.ErrorIs(t, err, file.ErrInvalidEnvelope)
	})

	t.Run("plaintext payload with invalid json", func(t *testing.T) {
		t.Parallel()

		env := base
		env.Payload = base64.StdEncoding.EncodeToString([]byte("not json"))

		_, err := env.DecodeState("")
		require.ErrorIs(t, err, file.ErrInvalidEnvelope)
	})

	t.Run("encrypted payload decrypts to invalid json", func(t *testing.T) {
		t.Parallel()

		blob, err := crypt.Encrypt([]byte("not json"), "pw")
		require.NoError(t, err)

		env := base
		env.Payload = base64.StdEncoding.EncodeToString(blob)

		_, err = env.DecodeState("pw")
		require.ErrorIs(t, err, file.ErrInvalidEnvelope)
	})
}

func TestIsEncryptedPayload_BadBase64(t *testing.T) {
	t.Parallel()

	env := &file.Envelope{Payload: "!!!not-base64!!!"}

	_, err := env.IsEncryptedPayload()
	require.ErrorIs(t, err, file.ErrInvalidEnvelope)
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
				state = paramState("/k", "v")
			} else {
				state = secretState("k", "v")
			}

			path := filepath.Join(t.TempDir(), string(tt.svc)+".json")
			err := file.WriteEnvelopeFile(path, tt.scope, tt.svc, state, "")
			require.NoError(t, err)

			env, err := file.ReadEnvelopeFile(path)
			require.NoError(t, err)
			assert.Equal(t, tt.wantProvider, env.Provider)
			assert.Equal(t, tt.wantScope, env.Scope)
			assert.Equal(t, string(tt.svc), env.Service)
		})
	}
}
