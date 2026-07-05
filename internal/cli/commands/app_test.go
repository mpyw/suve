package commands_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	commands "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

func topLevelNames(app *cli.Command) []string {
	names := make([]string, 0, len(app.Commands))
	for _, c := range app.Commands {
		names = append(names, c.Name)
	}

	return names
}

func TestMakeAppWithDetect_flatAliases(t *testing.T) {
	t.Parallel()

	aws := provider.ProviderAWS
	gcloud := provider.ProviderGoogleCloud
	az := provider.ProviderAzure

	tests := []struct {
		name       string
		det        detect.Result
		wantParam  bool // top-level flat `param` present
		wantSecret bool // top-level flat `secret` present
		wantStage  bool // top-level flat `stage` present (AWS-only)
	}{
		{name: "nothing active", det: detect.Result{}},
		{name: "AWS all", det: detect.Result{Param: aws, Secret: aws, Stage: aws}, wantParam: true, wantSecret: true, wantStage: true},
		{name: "GoogleCloud secret only", det: detect.Result{Secret: gcloud}, wantSecret: true},
		{name: "Azure param only", det: detect.Result{Param: az}, wantParam: true},
		{name: "Azure both", det: detect.Result{Param: az, Secret: az}, wantParam: true, wantSecret: true},
		{name: "GoogleCloud secret + Azure param", det: detect.Result{Param: az, Secret: gcloud}, wantParam: true, wantSecret: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := commands.MakeAppWithDetect(tt.det)
			names := topLevelNames(app)

			// Explicit provider groups are always present, regardless of detection.
			assert.Contains(t, names, "aws")
			assert.Contains(t, names, "gcloud")
			assert.Contains(t, names, "azure")

			assert.Equal(t, tt.wantParam, contains(names, "param"), "flat param alias")
			assert.Equal(t, tt.wantSecret, contains(names, "secret"), "flat secret alias")
			assert.Equal(t, tt.wantStage, contains(names, "stage"), "flat stage alias")
		})
	}
}

func TestMakeAppWithDetect_flatCommandsAreRunnable(t *testing.T) {
	t.Parallel()

	// A flat GoogleCloud secret alias should behave like `gcloud secret`: it must carry
	// the --project flag folded in from the group. `--help` must succeed.
	app := commands.MakeAppWithDetect(detect.Result{Secret: provider.ProviderGoogleCloud})
	err := app.Run(t.Context(), []string{"suve", "secret", "--help"})
	require.NoError(t, err)

	// A flat Azure param alias should carry the base --subscription flag folded
	// in on top of --store-name.
	app = commands.MakeAppWithDetect(detect.Result{Param: provider.ProviderAzure})
	err = app.Run(t.Context(), []string{"suve", "param", "--help"})
	require.NoError(t, err)
}

func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}
