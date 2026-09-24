// products.go binds each cloud product to the grammar it uses. These values
// are the package's API, so the file is core: AWSParameterStore does not have to
// become ProductAWSParameterStore.
//declscope:core

package version

import (
	"errors"
)

// Product-specific errors.
var (
	// ErrInvalidAWSSecretsManagerID is returned when # is not followed by an AWS
	// Secrets Manager version ID.
	ErrInvalidAWSSecretsManagerID = errors.New("# must be followed by a version ID")
	// ErrInvalidAWSSecretsManagerLabel is returned when : is not followed by an AWS
	// Secrets Manager staging label.
	ErrInvalidAWSSecretsManagerLabel = errors.New(": must be followed by a label")
	// ErrGoogleCloudSecretManagerLabelUnsupported is returned when a :LABEL specifier is
	// used for Google Cloud Secret Manager, which has no staging labels.
	ErrGoogleCloudSecretManagerLabelUnsupported = errors.New(
		": staging labels are not supported for Google Cloud Secret Manager " +
			"(versions are integers or \"latest\")",
	)
	// ErrInvalidAzureKeyVaultID is returned when # is not followed by an Azure Key
	// Vault version id.
	ErrInvalidAzureKeyVaultID = errors.New("# must be followed by a version id")
	// ErrAzureKeyVaultLabelUnsupported is returned when a :LABEL specifier is used
	// for Azure Key Vault, which has no staging labels.
	ErrAzureKeyVaultLabelUnsupported = errors.New(
		": staging labels are not supported for Azure Key Vault " +
			"(versions are opaque ids or the current version)",
	)
)

// The grammar of each cloud product.
//
//nolint:gochecknoglobals // stateless grammar configuration
var (
	// AWSParameterStore is the AWS Systems Manager Parameter Store grammar:
	// integer versions, name#VERSION~SHIFT.
	AWSParameterStore = NumericGrammar{}

	// AWSSecretsManager is the AWS Secrets Manager grammar: opaque version ids
	// plus staging labels, name#VERSION / name:LABEL, then ~SHIFT.
	AWSSecretsManager = OpaqueGrammar{
		IsIDChar:       isAWSSecretsManagerIDChar,
		InvalidIDError: ErrInvalidAWSSecretsManagerID,
		Labels:         true,
		LabelError:     ErrInvalidAWSSecretsManagerLabel,
	}

	// GoogleCloudSecretManager is the Google Cloud Secret Manager grammar: integer
	// versions ("latest" is the zero spec) and no staging labels, so a ':'
	// specifier is rejected before any API call.
	GoogleCloudSecretManager = NumericGrammar{LabelError: ErrGoogleCloudSecretManagerLabelUnsupported}

	// AzureKeyVault is the Azure Key Vault grammar: opaque 32-character hex version
	// ids and no staging labels, so a ':' specifier is rejected before any API
	// call.
	AzureKeyVault = OpaqueGrammar{
		IsIDChar:       isAzureKeyVaultIDChar,
		InvalidIDError: ErrInvalidAzureKeyVaultID,
		LabelError:     ErrAzureKeyVaultLabelUnsupported,
	}

	// AzureAppConfiguration is the Azure App Configuration grammar. The service is
	// unversioned and a key may contain ':' (the ASP.NET hierarchy separator),
	// '#' and '~', so the whole argument is the key.
	AzureAppConfiguration = BareGrammar{}
)

// isAWSSecretsManagerIDChar reports whether c is valid within a Secrets Manager
// version id. Version ids are ClientRequestTokens (not just console UUIDs): a
// token created via the API may contain '_' and '.' as well, so accept them
// alongside letters, digits and '-'. Excludes the specifier characters '#',
// ':', '~'.
func isAWSSecretsManagerIDChar(c byte) bool {
	return isLetterChar(c) || isDigitChar(c) || c == '-' || c == '_' || c == '.'
}

// isAzureKeyVaultIDChar reports whether c is valid within a Key Vault version id
// (hex-like: letters, digits, and dashes).
func isAzureKeyVaultIDChar(c byte) bool {
	return isLetterChar(c) || isDigitChar(c) || c == '-'
}
