package secret

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	"github.com/mpyw/suve/internal/provider"
)

// KMSKeyID sets the KMS key (ID or ARN) used to encrypt the secret value. It
// implements provider.WriteOption.
type KMSKeyID struct {
	provider.WriteOptionMarker

	Value string
}

// RotationRules configures automatic rotation for the secret. It is modeled
// minimally (rotation interval in days); it implements provider.WriteOption.
type RotationRules struct {
	provider.WriteOptionMarker

	// AutomaticallyAfterDays is the number of days between automatic rotations.
	// A zero value leaves rotation unconfigured.
	AutomaticallyAfterDays int64
}

// RecoveryWindow sets the number of days AWS retains the secret before
// permanent deletion. It implements provider.DeleteOption.
type RecoveryWindow struct {
	provider.DeleteOptionMarker

	Days int64
}

// Compile-time assertions that the secret options satisfy the markers.
var (
	_ provider.WriteOption  = KMSKeyID{}
	_ provider.WriteOption  = RotationRules{}
	_ provider.DeleteOption = RecoveryWindow{}
)

// applyCreateOptions folds recognized WriteOptions onto a CreateSecretInput.
func applyCreateOptions(input *secretsmanager.CreateSecretInput, opts []provider.WriteOption) {
	for _, opt := range opts {
		if k, ok := opt.(KMSKeyID); ok && k.Value != "" {
			input.KmsKeyId = aws.String(k.Value)
		}
	}
}

// applyUpdateOptions folds recognized WriteOptions onto an UpdateSecretInput.
func applyUpdateOptions(input *secretsmanager.UpdateSecretInput, opts []provider.WriteOption) {
	for _, opt := range opts {
		if k, ok := opt.(KMSKeyID); ok && k.Value != "" {
			input.KmsKeyId = aws.String(k.Value)
		}
	}
}

// rotationOption returns the RotationRules option if one with a non-zero
// interval was provided, so callers can issue a RotateSecret request.
func rotationOption(opts []provider.WriteOption) (RotationRules, bool) {
	for _, opt := range opts {
		if r, ok := opt.(RotationRules); ok && r.AutomaticallyAfterDays > 0 {
			return r, true
		}
	}

	return RotationRules{}, false
}

// applyDeleteOptions folds recognized DeleteOptions onto a DeleteSecretInput.
func applyDeleteOptions(input *secretsmanager.DeleteSecretInput, opts []provider.DeleteOption) {
	for _, opt := range opts {
		switch o := opt.(type) {
		case provider.ForceDelete:
			input.ForceDeleteWithoutRecovery = aws.Bool(true)
		case RecoveryWindow:
			if o.Days > 0 {
				input.RecoveryWindowInDays = aws.Int64(o.Days)
			}
		}
	}
}

// rotationInput builds a RotateSecretInput for the given secret and rules.
func rotationInput(name string, rules RotationRules) *secretsmanager.RotateSecretInput {
	return &secretsmanager.RotateSecretInput{
		SecretId: aws.String(name),
		RotationRules: &types.RotationRulesType{
			AutomaticallyAfterDays: aws.Int64(rules.AutomaticallyAfterDays),
		},
	}
}
