package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// confirmer prompts the user to confirm an action. *confirm.Prompter satisfies
// this interface; it is kept small so the apply flow stays testable.
type confirmer interface {
	Confirm(message string, skip bool) (bool, error)
}

// ApplyRunner applies staged changes via the ApplyUseCase and reports the
// results. RunInteractive wraps Run with the presentation-layer orchestration
// (empty-check, name validation, and interactive confirmation) that the
// `stage <service> apply` command performs before applying.
type ApplyRunner struct {
	UseCase     *stagingusecase.ApplyUseCase
	Store       store.ReadWriteOperator
	Parser      staging.Parser
	Confirmer   confirmer
	SkipConfirm bool
	Stdout      io.Writer
	Stderr      io.Writer
}

// ApplyOptions holds options for the apply command.
type ApplyOptions struct {
	Name            string // Optional: apply only this item, otherwise apply all
	IgnoreConflicts bool   // Skip conflict detection and force apply
}

// RunInteractive performs the command-level apply flow: it lists staged
// entries, short-circuits when nothing is staged, validates an optional target
// name, asks for confirmation, and then delegates to Run. Interactive
// confirmation lives here (presentation layer) rather than in the usecase.
func (r *ApplyRunner) RunInteractive(ctx context.Context, opts ApplyOptions) error {
	service := r.Parser.Service()

	// Get entries and staged tag changes to show what will be applied. Tag-only
	// changes (a staged tag with no staged entry) are valid applies, so both
	// must feed the has-changes check, name validation, and confirmation count.
	entries, err := r.Store.ListEntries(ctx, service)
	if err != nil {
		return err
	}

	tags, err := r.Store.ListTags(ctx, service)
	if err != nil {
		return err
	}

	serviceEntries := entries[service]
	serviceTags := tags[service]

	if len(serviceEntries) == 0 && len(serviceTags) == 0 {
		output.Info(r.Stdout, "No %s changes staged.", r.Parser.ServiceName())

		return nil
	}

	// Validate the target name if specified: staged as an entry OR a tag change.
	// Items are keyed by EntryKey (name, namespace), so match on the key's name —
	// a name may be staged under several App Configuration namespaces.
	if opts.Name != "" {
		entryStaged, tagStaged := false, false

		for key := range serviceEntries {
			if key.Name == opts.Name {
				entryStaged = true

				break
			}
		}

		for key := range serviceTags {
			if key.Name == opts.Name {
				tagStaged = true

				break
			}
		}

		if !entryStaged && !tagStaged {
			return fmt.Errorf("%s is not staged", opts.Name)
		}
	}

	// Confirm apply
	var message string
	if opts.Name != "" {
		message = fmt.Sprintf("Apply staged changes for %s to AWS?", opts.Name)
	} else {
		total := len(serviceEntries) + len(serviceTags)
		message = fmt.Sprintf("Apply %d staged %s change(s) to AWS?", total, r.Parser.ServiceName())
	}

	confirmed, err := r.Confirmer.Confirm(message, r.SkipConfirm)
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	return r.Run(ctx, opts)
}

// Run applies the staged changes via the usecase and reports the results.
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
	for _, name := range maputil.SortedKeys(lo.SliceToMap(result.Conflicts, func(s string) (string, struct{}) { return s, struct{}{} })) {
		output.Warning(r.Stderr, "conflict detected for %s: AWS was modified after staging", name)
	}

	// Handle "nothing staged" case
	if len(result.EntryResults) == 0 && len(result.TagResults) == 0 && err == nil {
		output.Info(r.Stdout, "No %s changes staged.", result.ServiceName)

		return nil
	}

	// Output entry results in sorted order
	for _, name := range maputil.SortedNames(result.EntryResults, func(e stagingusecase.ApplyEntryResult) string { return e.Name }) {
		for _, entry := range result.EntryResults {
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
				case stagingusecase.ApplyResultFailed:
					// Unreachable: when Status is Failed, entry.Error is always non-nil,
					// so the outer if-branch handles this case.
				}

				// The cloud apply succeeded but clearing the staged entry failed:
				// warn so the leftover (which a later apply would re-run) is visible.
				if entry.UnstageError != nil {
					output.Warning(r.Stderr, "failed to clear staging for %s: %v", name, entry.UnstageError)
				}
			}

			break
		}
	}

	// Output tag results in sorted order
	for _, name := range maputil.SortedNames(result.TagResults, func(e stagingusecase.ApplyTagResult) string { return e.Name }) {
		for _, tag := range result.TagResults {
			if tag.Name != name {
				continue
			}

			if tag.Error != nil {
				output.Failed(r.Stderr, name+" (tags)", tag.Error)
			} else {
				output.Success(r.Stdout, "Tagged %s%s", name, FormatTagApplySummary(tag))

				if tag.UnstageError != nil {
					output.Warning(r.Stderr, "failed to clear staging for %s tags: %v", name, tag.UnstageError)
				}
			}

			break
		}
	}

	// Return the original error if any (e.g., from conflict detection or failures)
	return err
}

// FormatTagApplySummary formats a tag apply result as a summary string.
func FormatTagApplySummary(tag stagingusecase.ApplyTagResult) string {
	var parts []string
	if len(tag.AddTags) > 0 {
		parts = append(parts, fmt.Sprintf("+%d", len(tag.AddTags)))
	}

	if tag.RemoveTag.Len() > 0 {
		parts = append(parts, fmt.Sprintf("-%d", tag.RemoveTag.Len()))
	}

	if len(parts) == 0 {
		// Unreachable: TagEntry with empty Add and Remove is unstaged by persistTagState,
		// so ApplyTagResult should always have at least one non-empty field.
		return ""
	}

	return " [" + strings.Join(parts, ", ") + "]"
}
