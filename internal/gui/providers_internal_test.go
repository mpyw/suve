//go:build production || dev

package gui

import (
	"testing"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

func TestUniqueActiveProvider(t *testing.T) {
	t.Parallel()

	aws := provider.ProviderAWS
	gcloud := provider.ProviderGoogleCloud
	az := provider.ProviderAzure

	tests := []struct {
		name   string
		result detect.Result
		want   provider.Provider
	}{
		{
			name:   "none active -> empty",
			result: detect.Result{},
			want:   "",
		},
		{
			name:   "single provider (secret only) -> that provider",
			result: detect.Result{SecretActive: []provider.Provider{gcloud}},
			want:   gcloud,
		},
		{
			name:   "single provider (param only) -> that provider",
			result: detect.Result{ParamActive: []provider.Provider{az}},
			want:   az,
		},
		{
			name: "same provider across both services -> that provider",
			result: detect.Result{
				ParamActive:  []provider.Provider{aws},
				SecretActive: []provider.Provider{aws},
			},
			want: aws,
		},
		{
			name: "two distinct providers -> empty (ambiguous)",
			result: detect.Result{
				ParamActive:  []provider.Provider{aws},
				SecretActive: []provider.Provider{gcloud},
			},
			want: "",
		},
		{
			name:   "two active in one service -> empty (ambiguous)",
			result: detect.Result{SecretActive: []provider.Provider{aws, az}},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := uniqueActiveProvider(tt.result); got != tt.want {
				t.Errorf("uniqueActiveProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}
