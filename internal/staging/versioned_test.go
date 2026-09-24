package staging_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
)

// TestVersionedStrategy_ZeroValueParser pins that each versioned strategy's
// zero value (no store), which the GUI and TUI use as a store-less parser,
// reports its provider's traits and parses with its provider's grammar.
func TestVersionedStrategy_ZeroValueParser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		parser           staging.Parser
		service          staging.Service
		serviceName      string
		itemName         string
		hasDeleteOptions bool
		versioned        string
	}{
		{"aws param", &staging.AWSParamStrategy{}, staging.ServiceParam, "Parameter Store", "parameter", false, "/app/key#3"},
		{"aws secret", &staging.AWSSecretStrategy{}, staging.ServiceSecret, "Secrets Manager", "secret", true, "app-key:AWSPREVIOUS"},
		{"google cloud secret", &staging.GoogleCloudSecretStrategy{}, staging.ServiceSecret, "Secret Manager", "secret", false, "app-key#3"},
		{"azure key vault secret", &staging.AzureSecretStrategy{}, staging.ServiceSecret, "Key Vault", "secret", false, "app-key~1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.service, tt.parser.Service())
			assert.Equal(t, tt.serviceName, tt.parser.ServiceName())
			assert.Equal(t, tt.itemName, tt.parser.ItemName())
			assert.Equal(t, tt.hasDeleteOptions, tt.parser.HasDeleteOptions())

			name, hasVersion, err := tt.parser.ParseSpec(tt.versioned)
			require.NoError(t, err)
			assert.True(t, hasVersion)
			assert.NotEmpty(t, name)

			_, err = tt.parser.ParseName(tt.versioned)
			require.ErrorContains(t, err, tt.itemName+" name must not contain a version specifier")
		})
	}
}
