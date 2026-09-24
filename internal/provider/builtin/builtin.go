// Package builtin composes the provider.Registry every entry point (CLI, GUI,
// TUI) resolves stores through, with each built-in cloud adapter registered on
// equal footing: AWS (param + secret), Google Cloud (secret), and Azure (Key
// Vault secret + App Configuration param).
package builtin

import (
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws"
	"github.com/mpyw/suve/internal/provider/azure"
	"github.com/mpyw/suve/internal/provider/gcloud"
)

// NewRegistry returns a provider.Registry with every built-in provider
// registered.
func NewRegistry() *provider.Registry {
	reg := provider.NewRegistry()
	aws.Register(reg)
	gcloud.Register(reg)
	azure.Register(reg)

	return reg
}
