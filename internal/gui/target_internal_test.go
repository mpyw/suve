//go:build production || dev

// In-package tests of the sidebar scope target bindings in target.go.
//declscope:namespace target

package gui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
)

func TestApp_GetScopeTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		scope provider.Scope
		want  ScopeTarget
	}{
		{
			name:  "aws is pending until STS answers",
			scope: provider.Scope{Provider: provider.ProviderAWS},
			want: ScopeTarget{Segments: []ScopeTargetSegment{
				{Label: "profile"}, {Label: "account"}, {Label: "region"},
			}, Pending: true},
		},
		{
			name:  "google cloud shows the project",
			scope: provider.GoogleCloudScope("proj"),
			want:  ScopeTarget{Segments: []ScopeTargetSegment{{Label: "project", Value: "proj"}}},
		},
		{
			name: "azure leaves the namespace to the sidebar filter",
			scope: provider.Scope{
				Provider: provider.ProviderAzure, StoreName: "store", AppConfigNamespace: "dev",
			},
			want: ScopeTarget{Segments: []ScopeTargetSegment{
				{Label: "vault"}, {Label: "store", Value: "store"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp(t, tt.scope, "")
			assert.Equal(t, tt.want, app.GetScopeTarget())
		})
	}
}

func TestApp_ResolveScopeTarget_SelfDescribing(t *testing.T) {
	t.Parallel()

	app := newTestApp(t, provider.GoogleCloudScope("proj"), "")

	got, err := app.ResolveScopeTarget()
	require.NoError(t, err)
	assert.Equal(t, ScopeTarget{Segments: []ScopeTargetSegment{{Label: "project", Value: "proj"}}}, got)
}
