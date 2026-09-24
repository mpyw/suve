package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/samber/lo"
	"github.com/samber/lo/it"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// DiffRunner executes diff operations using a usecase.
type DiffRunner struct {
	UseCase *stagingusecase.DiffUseCase
	Stdout  io.Writer
	Stderr  io.Writer
	// RemoteLabel names the remote in diff labels and warnings. Empty uses the
	// strategy's ServiceName (e.g. "App Configuration", "Key Vault").
	RemoteLabel string
	// keptStagedWarnings renders a DiffEntryWarning as a fetch failure that kept
	// the entry staged. The all-service diff sets it: it never filters by name,
	// so its only warnings are fetch failures.
	//
	//declscope:package // set by the all-service diff (global_diff.go)
	keptStagedWarnings bool
}

// remoteLabel names the backing store in diff labels: RemoteLabel when set,
// otherwise the strategy's ServiceName. This runner serves every provider, so
// the default comes from the strategy.
func (r *DiffRunner) remoteLabel() string {
	if r.RemoteLabel != "" {
		return r.RemoteLabel
	}

	if r.UseCase == nil || r.UseCase.Strategy == nil {
		return "remote"
	}

	return r.UseCase.Strategy.ServiceName()
}

// DiffOptions holds options for the diff command.
type DiffOptions struct {
	Name      string // Optional: diff only this item, otherwise diff all
	ParseJSON bool
	NoPager   bool
}

// Run executes the diff command.
func (r *DiffRunner) Run(ctx context.Context, opts DiffOptions) error {
	result, err := r.UseCase.Execute(ctx, stagingusecase.DiffInput{
		Name: opts.Name,
	})
	if err != nil {
		return err
	}

	if len(result.Entries) == 0 && len(result.TagEntries) == 0 {
		output.Warning(r.Stderr, "no %ss staged", result.ItemName)

		return nil
	}

	first := true

	r.outputEntries(opts, result.Entries, &first)
	r.outputTagEntries(result.TagEntries, &first)

	return nil
}

// outputEntries renders value entries sorted by (name, namespace): the same App
// Configuration key staged under several namespaces is several distinct
// entries, so each is printed (deduping by name would drop all but one,
// order-dependently). first tracks whether a blank separator line is due.
//
//declscope:package // the all-service diff (global_diff.go) renders through it
func (r *DiffRunner) outputEntries(opts DiffOptions, entries []stagingusecase.DiffEntry, first *bool) {
	entries = slices.Clone(entries)
	slices.SortFunc(entries, func(a, b stagingusecase.DiffEntry) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}

		return cmp.Compare(a.Namespace, b.Namespace)
	})

	for _, entry := range entries {
		switch entry.Type {
		case stagingusecase.DiffEntryWarning:
			if r.keptStagedWarnings {
				output.Warning(r.Stderr, "could not diff %s (kept staged): %s", entry.Name, entry.Warning)
			} else {
				output.Warning(r.Stderr, "%s is %s", entry.Name, entry.Warning)
			}
		case stagingusecase.DiffEntryAutoUnstaged:
			output.Warning(r.Stderr, "unstaged %s: %s", entry.Name, entry.Warning)
		case stagingusecase.DiffEntryCreate:
			r.separate(first)
			r.OutputDiffCreate(opts, entry)
		case stagingusecase.DiffEntryNormal:
			r.separate(first)
			r.OutputDiff(opts, entry)
		}
	}
}

// outputTagEntries renders tag entries sorted by (name, namespace), one block
// per tagged item (like entries, a key tagged under several App Configuration
// namespaces is several distinct items).
//
//declscope:package // the all-service diff (global_diff.go) renders through it
func (r *DiffRunner) outputTagEntries(tagEntries []stagingusecase.DiffTagEntry, first *bool) {
	tagEntries = slices.Clone(tagEntries)
	slices.SortFunc(tagEntries, func(a, b stagingusecase.DiffTagEntry) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}

		return cmp.Compare(a.Namespace, b.Namespace)
	})

	for _, tagEntry := range tagEntries {
		r.separate(first)
		r.OutputTagEntry(tagEntry)
	}
}

// separate prints the blank line between two rendered blocks.
func (r *DiffRunner) separate(first *bool) {
	if !*first {
		output.Println(r.Stdout, "")
	}

	*first = false
}

// OutputDiff outputs a diff entry for an existing resource.
func (r *DiffRunner) OutputDiff(opts DiffOptions, entry stagingusecase.DiffEntry) {
	remoteValue := entry.RemoteValue
	stagedValue := entry.StagedValue

	// Format as JSON if enabled
	if opts.ParseJSON {
		remoteValue, stagedValue = jsonutil.TryFormatOrWarn2(remoteValue, stagedValue, r.Stderr, entry.Name)
	}

	name := diffEntryDisplayName(entry)
	label1 := fmt.Sprintf("%s%s (%s)", name, entry.RemoteIdentifier, r.remoteLabel())
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), name)

	diff := output.Diff(r.Stdout, label1, label2, remoteValue, stagedValue)

	// Raw values differ (identical ones were auto-unstaged) but --parse-json
	// renders no textual diff: the staged update only reformats JSON. It stays
	// staged, since the use case decides on raw values.
	if diff == "" && opts.ParseJSON {
		output.Warning(r.Stderr, "%s: staged value differs from %s only in JSON formatting", entry.Name, r.remoteLabel())

		return
	}

	output.Print(r.Stdout, diff)

	// Show staged metadata
	r.OutputMetadata(entry)
}

// OutputDiffCreate outputs a diff entry for a newly created resource.
func (r *DiffRunner) OutputDiffCreate(opts DiffOptions, entry stagingusecase.DiffEntry) {
	stagedValue := entry.StagedValue

	// Format as JSON if enabled
	if opts.ParseJSON {
		if formatted, ok := jsonutil.TryFormat(stagedValue); ok {
			stagedValue = formatted
		}
	}

	name := diffEntryDisplayName(entry)
	label1 := fmt.Sprintf("%s (not in %s)", name, r.remoteLabel())
	label2 := fmt.Sprintf("%s (staged for creation)", name)

	diff := output.Diff(r.Stdout, label1, label2, "", stagedValue)
	output.Print(r.Stdout, diff)

	// Show staged metadata
	r.OutputMetadata(entry)
}

// diffEntryDisplayName qualifies the entry name with its Azure App Configuration
// namespace (the label axis) when present, so a key staged under several
// namespaces is unambiguous in the diff. Empty namespace (the null/default, and
// every other provider) yields the bare name.
func diffEntryDisplayName(entry stagingusecase.DiffEntry) string {
	return diffDisplayName(entry.Name, entry.Namespace)
}

// diffDisplayName renders a name with its App Configuration namespace badge, or
// the bare name for the empty namespace.
func diffDisplayName(name, namespace string) string {
	if namespace == "" {
		return name
	}

	return fmt.Sprintf("%s [%s]", name, namespace)
}

// OutputMetadata outputs metadata for a diff entry.
func (r *DiffRunner) OutputMetadata(entry stagingusecase.DiffEntry) {
	if desc := lo.FromPtr(entry.Description); desc != "" {
		output.Printf(r.Stdout, "%s %s\n", colors.For(r.Stdout).FieldLabel("Description:"), desc)
	}
}

// OutputTagEntry outputs a tag entry.
func (r *DiffRunner) OutputTagEntry(tagEntry stagingusecase.DiffTagEntry) {
	output.Printf(r.Stdout, "%s %s (staged tag changes)\n", colors.For(r.Stdout).Info("Tags:"), diffDisplayName(tagEntry.Name, tagEntry.Namespace))

	if len(tagEntry.Add) > 0 {
		tagPairs := slices.Collect(it.Map(maputil.SortedKeys(tagEntry.Add), func(k string) string {
			return fmt.Sprintf("%s=%s", k, tagEntry.Add[k])
		}))

		output.Printf(r.Stdout, "  %s %s\n", colors.For(r.Stdout).OpAdd("+"), strings.Join(tagPairs, ", "))
	}

	if len(tagEntry.Remove) > 0 {
		tagPairs := slices.Collect(it.Map(maputil.SortedKeys(tagEntry.Remove), func(k string) string {
			if v := tagEntry.Remove[k]; v != "" {
				return fmt.Sprintf("%s=%s", k, v)
			}

			return k
		}))

		output.Printf(r.Stdout, "  %s %s\n", colors.For(r.Stdout).OpDelete("-"), strings.Join(tagPairs, ", "))
	}
}
