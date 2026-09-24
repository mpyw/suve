package staging

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/secretsmanager"
	"github.com/mpyw/suve/internal/version"
)

// AWSSecretStrategy implements ServiceStrategy for Secrets Manager over a
// provider.Store. Secrets Manager specifics:
//
//   - Versions are ids or staging labels, parsed with version.AWSSecretsManager
//     (#ID, :LABEL, ~SHIFT); ids render truncated.
//   - Delete takes force / recovery-window options.
//   - A staged string edit refuses to overwrite a binary secret.
//
// The zero value (no store) is a parser-only strategy (ParseName/ParseSpec).
type AWSSecretStrategy struct {
	versionedStrategy[awsSecretHooks]
}

// NewAWSSecretStrategy creates a new Secrets Manager strategy over the given
// provider store. A nil store is allowed for parser-only use.
func NewAWSSecretStrategy(store provider.Store) *AWSSecretStrategy {
	return &AWSSecretStrategy{versionedStrategy[awsSecretHooks]{store: store}}
}

// AWSSecretParserFactory creates a Parser without provider access.
// Use this for operations that don't need AWS access (e.g., status, parsing).
func AWSSecretParserFactory() Parser {
	return NewAWSSecretStrategy(nil)
}

// awsSecretHooks supplies the Secrets Manager specifics.
type awsSecretHooks struct {
	versionedSecretHooks
}

func (awsSecretHooks) traits() versionedTraits {
	return versionedTraits{
		service:          ServiceSecret,
		serviceName:      "Secrets Manager",
		itemName:         secretServiceItemName,
		tagsFetchError:   "failed to describe secret",
		hasDeleteOptions: true,
	}
}

func (awsSecretHooks) parse(input string) (name, suffix string, err error) {
	return version.AWSSecretsManager.Split(input)
}

func (awsSecretHooks) versionLabel(id string) string {
	return "#" + secretsmanager.TruncateVersionID(id)
}

// deleteOptions translates staged delete options into provider delete options.
func (awsSecretHooks) deleteOptions(o *DeleteOptions) []provider.DeleteOption {
	if o == nil {
		return nil
	}

	switch {
	case o.Force:
		return []provider.DeleteOption{provider.ForceDelete{}}
	case o.RecoveryWindow > 0:
		return []provider.DeleteOption{secretsmanager.RecoveryWindow{Days: int64(o.RecoveryWindow)}}
	default:
		return nil
	}
}

func (h awsSecretHooks) update(ctx context.Context, store provider.Store, name string, entry Entry) error {
	if entry.Value == nil {
		return nil
	}

	// Overwrite guard: applying a staged string edit issues UpdateSecret with
	// SecretString, which silently drops a current SecretBinary value. Probe the
	// current value and refuse when it is binary (#469). Any other probe failure
	// is left for Put to surface, so the guard never turns a transient read error
	// into a spurious apply failure.
	if _, err := store.Get(ctx, name, provider.VersionRef{}); errors.Is(err, provider.ErrBinaryValue) {
		return fmt.Errorf("refusing to overwrite binary secret %q with a string value: %w", name, err)
	}

	return h.versionedSecretHooks.update(ctx, store, name, entry)
}
