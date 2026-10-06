package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// applyConfirmer prompts the user to confirm an action. *confirm.Prompter satisfies
// this interface; it is kept small so the apply flow stays testable.
type applyConfirmer interface {
	Confirm(message string, skip bool) (bool, error)
}

// applyRunner applies staged changes via the ApplyUseCase and reports the
// results. runInteractive wraps Run with the presentation-layer orchestration
// (empty-check, name validation, and interactive confirmation) that the
// `stage <service> apply` command performs before applying.
//
//declscope:shared // command.go builds and runs it
type applyRunner struct {
	useCase *stagingusecase.ApplyUseCase
	store   store.ReadWriteOperator
	parser  staging.Parser
	// providerLabel names the remote store in prompts and conflict warnings
	// (e.g. "AWS"); empty renders as "remote".
	providerLabel string
	confirmer     applyConfirmer
	skipConfirm   bool
	stdout        io.Writer
	stderr        io.Writer
}

// applyOptions holds options for the apply command.
//
//declscope:shared // command.go fills it from the flags
type applyOptions struct {
	name            string // Optional: apply only this item, otherwise apply all
	ignoreConflicts bool   // Skip conflict detection and force apply
}

// runInteractive performs the command-level apply flow: it lists staged
// entries, short-circuits when nothing is staged, validates an optional target
// name, asks for confirmation, and then delegates to Run. Interactive
// confirmation lives here (presentation layer) rather than in the usecase.
//
//declscope:shared // command.go runs it
func (r *applyRunner) runInteractive(ctx context.Context, opts applyOptions) error {
	service := r.parser.Service()

	// Get entries and staged tag changes to show what will be applied. Tag-only
	// changes (a staged tag with no staged entry) are valid applies, so both
	// must feed the has-changes check, name validation, and confirmation count.
	entries, err := r.store.ListEntries(ctx, service)
	if err != nil {
		return err
	}

	tags, err := r.store.ListTags(ctx, service)
	if err != nil {
		return err
	}

	serviceEntries := entries[service]
	serviceTags := tags[service]

	if len(serviceEntries) == 0 && len(serviceTags) == 0 {
		output.Info(r.stdout, "No %s changes staged.", r.parser.ServiceName())

		return nil
	}

	// Validate the target name if specified: staged as an entry OR a tag change.
	// Items are keyed by EntryKey (name, namespace), so match on the key's name —
	// a name may be staged under several App Configuration namespaces.
	if opts.name != "" {
		entryStaged, tagStaged := false, false

		for key := range serviceEntries {
			if key.Name == opts.name {
				entryStaged = true

				break
			}
		}

		for key := range serviceTags {
			if key.Name == opts.name {
				tagStaged = true

				break
			}
		}

		if !entryStaged && !tagStaged {
			return fmt.Errorf("%s is not staged", opts.name)
		}
	}

	// Confirm apply
	var message string
	if opts.name != "" {
		message = fmt.Sprintf("Apply staged changes for %s to %s?", opts.name, remoteName(r.providerLabel))
	} else {
		total := len(serviceEntries) + len(serviceTags)
		message = fmt.Sprintf("Apply %d staged %s change(s) to %s?", total, r.parser.ServiceName(), remoteName(r.providerLabel))
	}

	confirmed, err := r.confirmer.Confirm(message, r.skipConfirm)
	if err != nil {
		return err
	}

	if !confirmed {
		return nil
	}

	return r.run(ctx, opts)
}

// run applies the staged changes via the usecase and reports the results.
//
//declscope:shared // command.go runs it
func (r *applyRunner) run(ctx context.Context, opts applyOptions) error {
	result, err := r.useCase.Execute(ctx, stagingusecase.ApplyInput{
		Name:            opts.name,
		IgnoreConflicts: opts.ignoreConflicts,
	})

	// Handle nil result (shouldn't happen but be safe)
	if result == nil {
		return err
	}

	// Output conflicts if any. result.Conflicts is already sorted by (name,
	// namespace); render each with the namespace badge (bare name when empty).
	for _, key := range result.Conflicts {
		output.Warning(r.stderr, "conflict detected for %s: %s was modified after staging", key.Label(), remoteName(r.providerLabel))
	}

	// Handle "nothing staged" case
	if len(result.EntryResults) == 0 && len(result.TagResults) == 0 && err == nil {
		output.Info(r.stdout, "No %s changes staged.", result.ServiceName)

		return nil
	}

	// Output entry results in the usecase's (name, namespace) order. The same
	// name may be applied under several App Configuration namespaces; each is
	// its own line, labeled with its namespace badge (bare name for the empty
	// namespace).
	for _, entry := range result.EntryResults {
		label := staging.EntryKey{Name: entry.Name, Namespace: entry.Namespace}.Label()

		if entry.Error != nil {
			output.Failed(r.stderr, label, entry.Error)

			continue
		}

		switch entry.Status {
		case stagingusecase.ApplyResultCreated:
			output.Success(r.stdout, "Created %s", label)
		case stagingusecase.ApplyResultUpdated:
			output.Success(r.stdout, "Updated %s", label)
		case stagingusecase.ApplyResultDeleted:
			output.Success(r.stdout, "Deleted %s", label)
		case stagingusecase.ApplyResultFailed:
			// Unreachable: when Status is Failed, entry.Error is always non-nil,
			// so the branch above handles this case.
		}

		// The cloud apply succeeded but clearing the staged entry failed:
		// warn so the leftover (which a later apply would re-run) is visible.
		if entry.UnstageError != nil {
			output.Warning(r.stderr, "failed to clear staging for %s: %v", label, entry.UnstageError)
		}
	}

	// Output tag results in the same order, one line per namespace.
	for _, tag := range result.TagResults {
		label := staging.EntryKey{Name: tag.Name, Namespace: tag.Namespace}.Label()

		if tag.Error != nil {
			output.Failed(r.stderr, label+" (tags)", tag.Error)

			continue
		}

		output.Success(r.stdout, "Tagged %s%s", label, formatTagApplySummary(tag))

		if tag.UnstageError != nil {
			output.Warning(r.stderr, "failed to clear staging for %s tags: %v", label, tag.UnstageError)
		}
	}

	// Return the original error if any (e.g., from conflict detection or failures)
	return err
}

// formatTagApplySummary formats a tag apply result as a summary string.
//
//declscope:shared // global_apply.go summarizes tag results with it
func formatTagApplySummary(tag stagingusecase.ApplyTagResult) string {
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
