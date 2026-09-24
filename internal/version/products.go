// products.go binds each cloud product to the grammar it uses. These values
// are the package's API, so the file is core: ParameterStore does not have to
// become ProductParameterStore.
//declscope:core

package version

import (
	"errors"

	"github.com/mpyw/suve/internal/version/internal"
)

// Product-specific errors.
var (
	// ErrInvalidSecretsManagerID is returned when # is not followed by an AWS
	// Secrets Manager version ID.
	ErrInvalidSecretsManagerID = errors.New("# must be followed by a version ID")
	// ErrInvalidSecretsManagerLabel is returned when : is not followed by an AWS
	// Secrets Manager staging label.
	ErrInvalidSecretsManagerLabel = errors.New(": must be followed by a label")
	// ErrSecretManagerLabelUnsupported is returned when a :LABEL specifier is
	// used for Google Cloud Secret Manager, which has no staging labels.
	ErrSecretManagerLabelUnsupported = errors.New(
		": staging labels are not supported for Google Cloud Secret Manager " +
			"(versions are integers or \"latest\")",
	)
	// ErrInvalidKeyVaultID is returned when # is not followed by an Azure Key
	// Vault version id.
	ErrInvalidKeyVaultID = errors.New("# must be followed by a version id")
	// ErrKeyVaultLabelUnsupported is returned when a :LABEL specifier is used
	// for Azure Key Vault, which has no staging labels.
	ErrKeyVaultLabelUnsupported = errors.New(
		": staging labels are not supported for Azure Key Vault " +
			"(versions are opaque ids or the current version)",
	)
)

// The grammar of each cloud product.
//
//nolint:gochecknoglobals // stateless grammar configuration
var (
	// ParameterStore is the AWS Systems Manager Parameter Store grammar:
	// integer versions, name#VERSION~SHIFT.
	ParameterStore = NumericGrammar{}

	// SecretsManager is the AWS Secrets Manager grammar: opaque version ids
	// plus staging labels, name#VERSION / name:LABEL, then ~SHIFT.
	SecretsManager = OpaqueGrammar{
		IsIDChar:       isSecretsManagerIDChar,
		InvalidIDError: ErrInvalidSecretsManagerID,
		Labels:         true,
		LabelError:     ErrInvalidSecretsManagerLabel,
	}

	// SecretManager is the Google Cloud Secret Manager grammar: integer
	// versions ("latest" is the zero spec) and no staging labels, so a ':'
	// specifier is rejected before any API call.
	SecretManager = NumericGrammar{LabelError: ErrSecretManagerLabelUnsupported}

	// KeyVault is the Azure Key Vault grammar: opaque 32-character hex version
	// ids and no staging labels, so a ':' specifier is rejected before any API
	// call.
	KeyVault = OpaqueGrammar{
		IsIDChar:       isKeyVaultIDChar,
		InvalidIDError: ErrInvalidKeyVaultID,
		LabelError:     ErrKeyVaultLabelUnsupported,
	}

	// AppConfiguration is the Azure App Configuration grammar. The service is
	// unversioned and a key may contain ':' (the ASP.NET hierarchy separator),
	// '#' and '~', so the whole argument is the key.
	AppConfiguration = BareGrammar{}
)

// isSecretsManagerIDChar reports whether c is valid within a Secrets Manager
// version id. Version ids are ClientRequestTokens (not just console UUIDs): a
// token created via the API may contain '_' and '.' as well, so accept them
// alongside letters, digits and '-'. Excludes the specifier characters '#',
// ':', '~'.
func isSecretsManagerIDChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-' || c == '_' || c == '.'
}

// isKeyVaultIDChar reports whether c is valid within a Key Vault version id
// (hex-like: letters, digits, and dashes).
func isKeyVaultIDChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-'
}
