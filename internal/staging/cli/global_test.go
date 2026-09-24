// Fixtures shared by the all-service command tests (global_status_test.go,
// global_reset_test.go, global_diff_test.go, global_apply_test.go), which join
// this file's namespace.

package cli_test

import (
	"context"
	"fmt"

	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

// globalAWSServices returns the AWS service specs (param + secret) the
// all-service commands iterate.
func globalAWSServices() []stgcli.GlobalServiceSpec {
	return []stgcli.GlobalServiceSpec{
		{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory},
		{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory},
	}
}

// globalAWSConfig is an AWS-labelled GlobalConfig for the help-text tests.
func globalAWSConfig() stgcli.GlobalConfig {
	return stgcli.GlobalConfig{
		ProviderLabel: "AWS",
		CommandPath:   "suve aws stage",
		Services:      globalAWSServices(),
	}
}

// globalNotConfiguredResolver mimics an Azure scope resolver whose resource is
// not named (e.g. no --vault-name), signalling the service should be skipped.
func globalNotConfiguredResolver(_ context.Context) (staging.ResolvedScope, error) {
	return staging.ResolvedScope{}, fmt.Errorf("%w: no resource", staging.ErrServiceNotConfigured)
}
