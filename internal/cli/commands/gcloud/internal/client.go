// Package internal holds the Google Cloud project context, store and
// staging-scope wiring that the gcloud command group, its secret package and
// its stage commands share.
package internal

import (
	"context"
	"errors"

	cliinternal "github.com/mpyw/suve/internal/cli/commands/internal"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/binding"
)

// errNoProject is returned when neither --project nor GOOGLE_CLOUD_PROJECT
// named a project.
var errNoProject = errors.New(
	"no Google Cloud project specified: set --project or the GOOGLE_CLOUD_PROJECT environment variable",
)

// projectContextKey keys the resolved Google Cloud project id stored in the
// context by the gcloud command group's Before hook.
type projectContextKey struct{}

// WithProject returns a context carrying the resolved Google Cloud project id.
// The gcloud command group sets it once (from --project or the
// GOOGLE_CLOUD_PROJECT env) so every gcloud subcommand can resolve a store
// without threading the flag through the generic command Config.
func WithProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, projectContextKey{}, project)
}

func projectFromContext(ctx context.Context) string {
	project, _ := ctx.Value(projectContextKey{}).(string)

	return project
}

// SecretStore resolves a provider.Store for the Google Cloud Secret Manager
// service. The project id is read from the context (see WithProject); it
// returns a clear error when no project could be resolved.
func SecretStore(ctx context.Context) (provider.Store, error) {
	project := projectFromContext(ctx)
	if project == "" {
		return nil, errNoProject
	}

	return cliinternal.Store(ctx, provider.GoogleCloudScope(project), provider.KindSecret)
}

// StagingScopeResolver resolves the Google Cloud staging scope from the project
// stashed in the context (see WithProject). It performs no network calls. It
// satisfies staging.ScopeResolver.
func StagingScopeResolver(ctx context.Context) (staging.ResolvedScope, error) {
	project := projectFromContext(ctx)
	if project == "" {
		return staging.ResolvedScope{}, errNoProject
	}

	return binding.StagingScope(ctx, provider.GoogleCloudScope(project), provider.KindSecret, nil)
}
