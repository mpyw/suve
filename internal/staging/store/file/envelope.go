package file

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file/internal/crypt"
)

// EnvelopeVersion is the current export/import envelope schema version.
//
// v2 binds the plaintext header (version/provider/scope/service) to the
// encrypted payload as AES-GCM associated data (AAD). This is a breaking
// change: v1 files (no AAD binding) are rejected on import and must be
// re-created with `stage export`.
const EnvelopeVersion = 2

// Envelope is the on-disk format for `stage export` / `stage import` files.
//
// It is a plaintext JSON object; only Payload carries secret data (the staged
// State, optionally encrypted, base64-encoded). Provider/Scope/Service stay in
// the clear so a file can be identified and validated without a passphrase.
//
// Exactly one service lives in a single envelope file (per-service files mirror
// the working store's param.json / secret.json split).
type Envelope struct {
	// Version is the envelope schema version (EnvelopeVersion).
	Version int `json:"version"`
	// Provider is the scope provider string, e.g. "aws"/"googlecloud"/"azure".
	Provider string `json:"provider"`
	// Scope is the scope key (provider.Scope.Key()), used to validate on import.
	Scope string `json:"scope"`
	// Service is the staging service the payload holds ("param" or "secret").
	Service string `json:"service"`
	// Payload is base64(std) of the marshaled single-service State. When the
	// state was exported with a passphrase it is the encrypted (v1) blob;
	// otherwise it is the plaintext state JSON.
	//
	// WARNING: base64 is NOT encryption. An empty passphrase stores the secret
	// values in cleartext (base64 only). Callers exporting with an empty
	// passphrase must warn the user before writing such a file.
	Payload string `json:"payload"`
}

var (
	// ErrInvalidEnvelope is returned when a file is not a valid export envelope
	// (bad JSON, missing required fields, or corrupted base64 payload).
	ErrInvalidEnvelope = errors.New("invalid export file")
	// ErrUnsupportedEnvelopeVersion is returned for an unknown envelope version.
	ErrUnsupportedEnvelopeVersion = errors.New("unsupported export file version")
)

// aadDomain is a domain-separation prefix for the envelope AAD, so the bound
// bytes can never be confused with any other AES-GCM associated data.
const aadDomain = "suve/staging/export-envelope\x00"

// associatedData returns the canonical AES-GCM associated data that binds the
// envelope header (version/provider/scope/service) to the encrypted payload.
//
// The encoding is a fixed-order, length-prefixed concatenation so that no field
// boundary is ambiguous: tampering with any single header field yields
// different AAD, which makes DecryptWithAAD fail with crypt.ErrDecryptionFailed.
func (e *Envelope) associatedData() []byte {
	var buf bytes.Buffer

	buf.WriteString(aadDomain)

	var num [8]byte

	binary.BigEndian.PutUint64(num[:], uint64(e.Version)) //nolint:gosec // version is a small non-negative schema constant
	buf.Write(num[:])

	writeField := func(s string) {
		binary.BigEndian.PutUint64(num[:], uint64(len(s)))
		buf.Write(num[:])
		buf.WriteString(s)
	}

	writeField(e.Provider)
	writeField(e.Scope)
	writeField(e.Service)

	return buf.Bytes()
}

// encodePayload marshals a single-service state into the base64 payload. With an
// empty passphrase the state JSON is stored as plaintext (base64 only, no
// encryption); otherwise it is encrypted with the passphrase-based (v1) format,
// binding aad (the canonical envelope header) to the ciphertext.
func encodePayload(state *staging.State, passphrase string, aad []byte) (string, error) {
	// staging.State implements json.Marshaler (MarshalJSON), emitting its
	// EntryKey-keyed maps as arrays of (name, namespace) records, so the static
	// errchkjson "unsupported map key" warning is a false positive here.
	data, err := json.Marshal(state) //nolint:errchkjson // State has a custom MarshalJSON
	if err != nil {
		return "", fmt.Errorf("failed to marshal state: %w", err)
	}

	if passphrase != "" {
		data, err = crypt.EncryptWithAAD(data, passphrase, aad)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt payload: %w", err)
		}
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// WriteEnvelopeFile writes state to path as an export envelope, overwriting any
// existing file wholesale. Only svc's entries are written: the state is scoped
// to svc defensively so a caller passing a multi-service state can never leak
// another service's secrets into a file labeled for svc. The parent directory
// is created if needed and the write is atomic.
//
// WARNING: an empty passphrase writes the secret values UNENCRYPTED (base64
// only); callers must warn the user first.
func WriteEnvelopeFile(path string, scope provider.Scope, svc staging.Service, state *staging.State, passphrase string) error {
	env := Envelope{
		Version:  EnvelopeVersion,
		Provider: string(scope.Provider),
		Scope:    scope.Key(),
		Service:  string(svc),
	}

	// The header is bound to the ciphertext as AAD, so it must be filled in
	// before the payload is encoded.
	payload, err := encodePayload(state.ExtractService(svc), passphrase, env.associatedData())
	if err != nil {
		return err
	}

	env.Payload = payload

	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal export envelope: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:mnd // owner-only directory permissions
		return fmt.Errorf("failed to create export directory: %w", err)
	}

	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("failed to write export file: %w", err)
	}

	return nil
}

// ReadEnvelopeFile reads and validates the envelope at path without decoding the
// payload, so scope/service can be checked before a passphrase is prompted.
func ReadEnvelopeFile(path string) (*Envelope, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is user-supplied import target
	if err != nil {
		return nil, fmt.Errorf("failed to read export file: %w", err)
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidEnvelope, err.Error())
	}

	if env.Version != EnvelopeVersion {
		return nil, fmt.Errorf(
			"%w: file is version %d, but this build only reads version %d; re-create it with `stage export`",
			ErrUnsupportedEnvelopeVersion, env.Version, EnvelopeVersion,
		)
	}

	if env.Provider == "" || env.Scope == "" || env.Service == "" || env.Payload == "" {
		return nil, ErrInvalidEnvelope
	}

	return &env, nil
}

// decodedPayload base64-decodes the payload into its raw (possibly encrypted)
// bytes.
func (e *Envelope) decodedPayload() ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("%w: corrupted payload encoding", ErrInvalidEnvelope)
	}

	return raw, nil
}

// IsEncryptedPayload reports whether the payload is passphrase-encrypted. It
// only inspects the header, so no passphrase is required.
func (e *Envelope) IsEncryptedPayload() (bool, error) {
	raw, err := e.decodedPayload()
	if err != nil {
		return false, err
	}

	return crypt.IsEncrypted(raw), nil
}

// DecodeState decodes (and decrypts, when encrypted) the payload into a State.
// An empty passphrase is valid only for a plaintext payload.
func (e *Envelope) DecodeState(passphrase string) (*staging.State, error) {
	raw, err := e.decodedPayload()
	if err != nil {
		return nil, err
	}

	if crypt.IsEncrypted(raw) {
		if passphrase == "" {
			return nil, crypt.ErrDecryptionFailed
		}

		// The header is bound to the ciphertext as AAD: any tampering with
		// version/provider/scope/service makes GCM authentication fail here.
		raw, err = crypt.DecryptWithAAD(raw, passphrase, e.associatedData())
		if err != nil {
			return nil, err
		}
	}

	var state staging.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidEnvelope, err.Error())
	}

	// The payload is untrusted input, so keep only the service the (plaintext)
	// header declares: a mismatched or hostile payload carrying another service's
	// data is dropped rather than trusted. ExtractService returns a state with
	// fully initialized maps.
	scoped := state.ExtractService(staging.Service(e.Service))

	// A namespace (Azure App Configuration label) axis exists only for App
	// Configuration; AWS, Google Cloud and Azure Key Vault are namespace-agnostic.
	// Reject an envelope for such a provider that still carries namespace-bearing
	// entries, mirroring the provider-mismatch guard: applying them would push a
	// namespaced item to a provider that ignores namespaces.
	if !e.namespaceAllowed() {
		if key, found := firstNamespacedKey(scoped); found {
			return nil, fmt.Errorf(
				"%w: provider %q is namespace-agnostic but the payload carries item %q under namespace %q",
				ErrInvalidEnvelope, e.Provider, key.Name, key.Namespace)
		}
	}

	return scoped, nil
}

// namespaceAllowed reports whether entries in this envelope may carry a non-empty
// Namespace. Only Azure App Configuration (provider "azure", param service) has
// a namespace (label) axis.
func (e *Envelope) namespaceAllowed() bool {
	return e.Provider == string(provider.ProviderAzure) && e.Service == string(staging.ServiceParam)
}

// firstNamespacedKey returns the first entry or tag key in state that carries a
// non-empty namespace, for reporting a namespace-agnostic violation.
func firstNamespacedKey(state *staging.State) (staging.EntryKey, bool) {
	for _, entries := range state.Entries {
		for key := range entries {
			if key.Namespace != "" {
				return key, true
			}
		}
	}

	for _, tags := range state.Tags {
		for key := range tags {
			if key.Namespace != "" {
				return key, true
			}
		}
	}

	return staging.EntryKey{}, false
}
