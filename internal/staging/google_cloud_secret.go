package staging

import (
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/version"
)

// GoogleCloudSecretStrategy implements the staging strategies for Google Cloud
// Secret Manager over a provider.Store. Secret Manager specifics:
//
//   - Versions are immutable integers, parsed with version.GoogleCloudSecretManager (#N, ~SHIFT); a
//     staged "edit" applies as a new version via Put.
//   - There are no delete options (no force / recovery window), so
//     HasDeleteOptions reports false and Delete ignores staged DeleteOptions.
//   - There are no staging labels (:LABEL).
//
// The zero value (no store) is a parser-only strategy (ParseName/ParseSpec).
type GoogleCloudSecretStrategy struct {
	versionedStrategy[googleCloudSecretHooks]
}

// NewGoogleCloudSecretStrategy creates a Google Cloud Secret Manager staging strategy
// over the given provider store. A nil store is allowed for parser-only use.
func NewGoogleCloudSecretStrategy(store provider.Store) *GoogleCloudSecretStrategy {
	return &GoogleCloudSecretStrategy{versionedStrategy[googleCloudSecretHooks]{store: store}}
}

// GoogleCloudSecretParserFactory creates a Parser without provider access, for
// operations that don't need Google Cloud access (e.g. status, parsing).
func GoogleCloudSecretParserFactory() Parser {
	return NewGoogleCloudSecretStrategy(nil)
}

// googleCloudSecretHooks supplies the Secret Manager specifics.
type googleCloudSecretHooks struct {
	versionedSecretHooks
}

func (googleCloudSecretHooks) traits() versionedTraits {
	return versionedTraits{
		service:        ServiceSecret,
		serviceName:    "Secret Manager",
		itemName:       itemNameSecret,
		tagsFetchError: "failed to get secret",
	}
}

func (googleCloudSecretHooks) parse(input string) (name, suffix string, err error) {
	return version.GoogleCloudSecretManager.Split(input)
}
