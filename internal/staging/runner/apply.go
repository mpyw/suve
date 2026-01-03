// Package runner provides shared runners and command builders for stage commands.
package runner

import (
	"context"
	"io"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/maputil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// ApplyRunner executes apply operations using a usecase.
type ApplyRunner struct {
	UseCase *stagingusecase.ApplyUseCase
	Stdout  io.Writer
	Stderr  io.Writer
}

// ApplyOptions holds options for the apply command.
type ApplyOptions struct {
	Name            string // Optional: apply only this item, otherwise apply all
	IgnoreConflicts bool   // Skip conflict detection and force apply
}

// Run executes the apply command.
func (r *ApplyRunner) Run(ctx context.Context, opts ApplyOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.ApplyInput{
		Name:            opts.Name,
		IgnoreConflicts: opts.IgnoreConflicts,
	})

	// Handle nil result (shouldn't happen but be safe)
	if result == nil {
		return err
	}

	// Output conflicts if any
	for _, name := range maputil.SortedKeys(sliceToMap(result.Conflicts)) {
		output.Warning(r.Stderr, "conflict detected for %s: AWS was modified after staging", name)
	}

	// Handle "nothing staged" case
	if len(result.Results) == 0 && err == nil {
		output.Info(r.Stdout, "No %s changes staged.", result.ServiceName)
		return nil
	}

	// Output results in sorted order
	for _, name := range r.sortedResultNames(result.Results) {
		for _, entry := range result.Results {
			if entry.Name != name {
				continue
			}
			if entry.Error != nil {
				output.Failed(r.Stderr, name, entry.Error)
			} else {
				switch entry.Status {
				case stagingusecase.ApplyResultCreated:
					output.Success(r.Stdout, "Created %s", name)
				case stagingusecase.ApplyResultUpdated:
					output.Success(r.Stdout, "Updated %s", name)
				case stagingusecase.ApplyResultDeleted:
					output.Success(r.Stdout, "Deleted %s", name)
				}
			}
			break
		}
	}

	// Return the original error if any (e.g., from conflict detection or failures)
	return err
}

func (r *ApplyRunner) sortedResultNames(results []stagingusecase.ApplyResultEntry) []string {
	names := make(map[string]struct{})
	for _, e := range results {
		names[e.Name] = struct{}{}
	}
	return maputil.SortedKeys(names)
}

func sliceToMap(slice []string) map[string]struct{} {
	m := make(map[string]struct{})
	for _, s := range slice {
		m[s] = struct{}{}
	}
	return m
}
