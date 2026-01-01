package tagging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name         string
		tags         []string
		untags       []string
		wantAdd      map[string]string
		wantRemove   []string
		wantWarnings []string
		wantErr      string
	}{
		{
			name:       "empty",
			tags:       nil,
			untags:     nil,
			wantAdd:    map[string]string{},
			wantRemove: []string{},
		},
		{
			name:       "add single tag",
			tags:       []string{"env=prod"},
			wantAdd:    map[string]string{"env": "prod"},
			wantRemove: []string{},
		},
		{
			name:       "add multiple tags",
			tags:       []string{"env=prod", "team=platform"},
			wantAdd:    map[string]string{"env": "prod", "team": "platform"},
			wantRemove: []string{},
		},
		{
			name:       "remove single tag",
			untags:     []string{"env"},
			wantAdd:    map[string]string{},
			wantRemove: []string{"env"},
		},
		{
			name:       "remove multiple tags",
			untags:     []string{"env", "team"},
			wantAdd:    map[string]string{},
			wantRemove: []string{"env", "team"},
		},
		{
			name:       "add and remove different tags",
			tags:       []string{"env=prod"},
			untags:     []string{"deprecated"},
			wantAdd:    map[string]string{"env": "prod"},
			wantRemove: []string{"deprecated"},
		},
		{
			name:         "conflict - untag wins over tag",
			tags:         []string{"env=prod"},
			untags:       []string{"env"},
			wantAdd:      map[string]string{},
			wantRemove:   []string{"env"},
			wantWarnings: []string{`tag "env": --untag env overrides --tag env=prod`},
		},
		{
			name:       "tag with equals in value",
			tags:       []string{"config=key=value"},
			wantAdd:    map[string]string{"config": "key=value"},
			wantRemove: []string{},
		},
		{
			name:       "tag with empty value",
			tags:       []string{"empty="},
			wantAdd:    map[string]string{"empty": ""},
			wantRemove: []string{},
		},
		{
			name:    "invalid tag format - no equals",
			tags:    []string{"invalid"},
			wantErr: `invalid tag format "invalid": expected key=value`,
		},
		{
			name:    "invalid tag format - empty key",
			tags:    []string{"=value"},
			wantErr: `invalid tag format "=value": key cannot be empty`,
		},
		{
			name:    "invalid untag - empty key",
			untags:  []string{""},
			wantErr: "invalid untag: key cannot be empty",
		},
		{
			name:         "duplicate tag - last wins",
			tags:         []string{"env=dev", "env=prod"},
			wantAdd:      map[string]string{"env": "prod"},
			wantRemove:   []string{},
			wantWarnings: []string{`tag "env": --tag env=prod overrides --tag env=dev`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFlags(tt.tags, tt.untags)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantAdd, result.Change.Add)
			assert.ElementsMatch(t, tt.wantRemove, result.Change.Remove)
			if tt.wantWarnings == nil {
				assert.Empty(t, result.Warnings)
			} else {
				assert.Equal(t, tt.wantWarnings, result.Warnings)
			}
		})
	}
}

func TestChange_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		change *Change
		want   bool
	}{
		{
			name:   "empty",
			change: &Change{Add: map[string]string{}, Remove: []string{}},
			want:   true,
		},
		{
			name:   "has add",
			change: &Change{Add: map[string]string{"k": "v"}, Remove: []string{}},
			want:   false,
		},
		{
			name:   "has remove",
			change: &Change{Add: map[string]string{}, Remove: []string{"k"}},
			want:   false,
		},
		{
			name:   "has both",
			change: &Change{Add: map[string]string{"k": "v"}, Remove: []string{"x"}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.change.IsEmpty())
		})
	}
}
