//go:build production || dev

package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/provider"
)

// TestApp_StagingBindings_RejectNonAWS asserts that every staging binding
// returns errStagingNonAWS under a non-AWS scope. Because the guard runs before
// any infra.GetAWSIdentity call, the returned error is the sentinel (not an STS
// failure): if GetAWSIdentity were reached, an unauthenticated test environment
// would surface a different error (or block on the network) instead.
func TestApp_StagingBindings_RejectNonAWS(t *testing.T) {
	t.Parallel()

	// Each binding invoked with harmless arguments; only the error is checked.
	bindings := map[string]func(a *App) error{
		"StagingStatus": func(a *App) error {
			_, err := a.StagingStatus()

			return err
		},
		"StagingApply": func(a *App) error {
			_, err := a.StagingApply("param", false)

			return err
		},
		"StagingReset": func(a *App) error {
			_, err := a.StagingReset("param")

			return err
		},
		"StagingAdd": func(a *App) error {
			_, err := a.StagingAdd("param", "n", "v")

			return err
		},
		"StagingEdit": func(a *App) error {
			_, err := a.StagingEdit("param", "n", "v")

			return err
		},
		"StagingDelete": func(a *App) error {
			_, err := a.StagingDelete("param", "n", false, 0)

			return err
		},
		"StagingUnstage": func(a *App) error {
			_, err := a.StagingUnstage("param", "n")

			return err
		},
		"StagingAddTag": func(a *App) error {
			_, err := a.StagingAddTag("param", "n", "k", "v")

			return err
		},
		"StagingRemoveTag": func(a *App) error {
			_, err := a.StagingRemoveTag("param", "n", "k")

			return err
		},
		"StagingCancelAddTag": func(a *App) error {
			_, err := a.StagingCancelAddTag("param", "n", "k")

			return err
		},
		"StagingCancelRemoveTag": func(a *App) error {
			_, err := a.StagingCancelRemoveTag("param", "n", "k")

			return err
		},
		"StagingCheckStatus": func(a *App) error {
			_, err := a.StagingCheckStatus("param", "n")

			return err
		},
		"StagingDiff": func(a *App) error {
			_, err := a.StagingDiff("param", "n")

			return err
		},
		"StagingFileStatus": func(a *App) error {
			_, err := a.StagingFileStatus()

			return err
		},
		"StagingDrain": func(a *App) error {
			_, err := a.StagingDrain("param", "", false, "merge")

			return err
		},
		"StagingPersist": func(a *App) error {
			_, err := a.StagingPersist("param", "", false, "overwrite")

			return err
		},
		"StagingDrop": func(a *App) error {
			_, err := a.StagingDrop()

			return err
		},
	}

	scopes := map[string]provider.Scope{
		"googlecloud": {Provider: provider.ProviderGoogleCloud, ProjectID: "p"},
		"azure":       {Provider: provider.ProviderAzure, VaultName: "v"},
	}

	for scopeName, scope := range scopes {
		t.Run(scopeName, func(t *testing.T) {
			t.Parallel()

			for bindingName, call := range bindings {
				t.Run(bindingName, func(t *testing.T) {
					t.Parallel()

					app := NewApp(provider.ProviderAWS)
					app.scope = scope

					err := call(app)
					assert.ErrorIs(t, err, errStagingNonAWS)
				})
			}
		})
	}
}
