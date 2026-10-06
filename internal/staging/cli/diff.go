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

// diffRunner executes diff operations using a usecase.
//
//declscope:shared // command.go and global_diff.go build and run it
type diffRunner struct {
	useCase *stagingusecase.DiffUseCase
	stdout  io.Writer
	stderr  io.Writer
	// providerLabel names the remote in diff labels and warnings. Empty uses the
	// strategy's ServiceName (e.g. "App Configuration", "Key Vault").
	providerLabel string
	// keptStagedWarnings renders a DiffEntryWarning as a fetch failure that kept
	// the entry staged. The all-service diff sets it: it never filters by name,
	// so its only warnings are fetch failures.
	keptStagedWarnings bool
}

// remoteLabel names the backing store in diff labels: providerLabel when set,
// otherwise the strategy's ServiceName. This runner serves every provider, so
// the default comes from the strategy.
func (r *diffRunner) remoteLabel() string {
	if r.providerLabel != "" {
		return r.providerLabel
	}

	if r.useCase == nil || r.useCase.Strategy == nil {
		return "remote"
	}

	return r.useCase.Strategy.ServiceName()
}

// diffOptions holds options for the diff command.
//
//declscope:shared // command.go and global_diff.go fill it
type diffOptions struct {
	name      string // Optional: diff only this item, otherwise diff all
	parseJSON bool
	noPager   bool
}

// run executes the diff command.
//
//declscope:shared // command.go runs it
func (r *diffRunner) run(ctx context.Context, opts diffOptions) error {
	result, err := r.useCase.Execute(ctx, stagingusecase.DiffInput{
		Name: opts.name,
	})
	if err != nil {
		return err
	}

	if len(result.Entries) == 0 && len(result.TagEntries) == 0 {
		output.Warning(r.stderr, "no %ss staged", result.ItemName)

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
//declscope:shared // the all-service diff (global_diff.go) renders through it
func (r *diffRunner) outputEntries(opts diffOptions, entries []stagingusecase.DiffEntry, first *bool) {
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
				output.Warning(r.stderr, "could not diff %s (kept staged): %s", diffEntryDisplayName(entry), entry.Warning)
			} else {
				output.Warning(r.stderr, "%s is %s", diffEntryDisplayName(entry), entry.Warning)
			}
		case stagingusecase.DiffEntryAutoUnstaged:
			output.Warning(r.stderr, "unstaged %s: %s", diffEntryDisplayName(entry), entry.Warning)
		case stagingusecase.DiffEntryCreate:
			r.separate(first)
			r.outputDiffCreate(opts, entry)
		case stagingusecase.DiffEntryNormal:
			r.separate(first)
			r.outputDiff(opts, entry)
		}
	}
}

// outputTagEntries renders tag entries sorted by (name, namespace), one block
// per tagged item (like entries, a key tagged under several App Configuration
// namespaces is several distinct items).
//
//declscope:shared // the all-service diff (global_diff.go) renders through it
func (r *diffRunner) outputTagEntries(tagEntries []stagingusecase.DiffTagEntry, first *bool) {
	tagEntries = slices.Clone(tagEntries)
	slices.SortFunc(tagEntries, func(a, b stagingusecase.DiffTagEntry) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}

		return cmp.Compare(a.Namespace, b.Namespace)
	})

	for _, tagEntry := range tagEntries {
		r.separate(first)
		r.outputTagEntry(tagEntry)
	}
}

// separate prints the blank line between two rendered blocks.
func (r *diffRunner) separate(first *bool) {
	if !*first {
		output.Println(r.stdout, "")
	}

	*first = false
}

// outputDiff outputs a diff entry for an existing resource.
func (r *diffRunner) outputDiff(opts diffOptions, entry stagingusecase.DiffEntry) {
	remoteValue := entry.RemoteValue
	stagedValue := entry.StagedValue

	// Format as JSON if enabled
	if opts.parseJSON {
		remoteValue, stagedValue = jsonutil.TryFormatOrWarn2(remoteValue, stagedValue, r.stderr, entry.Name)
	}

	name := diffEntryDisplayName(entry)
	label1 := fmt.Sprintf("%s%s (%s)", name, entry.RemoteIdentifier, r.remoteLabel())
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), name)

	diff := output.Diff(r.stdout, label1, label2, remoteValue, stagedValue)

	// Raw values differ (identical ones were auto-unstaged) but --parse-json
	// renders no textual diff: the staged update only reformats JSON. It stays
	// staged, since the use case decides on raw values.
	if diff == "" && opts.parseJSON {
		output.Warning(r.stderr, "%s: staged value differs from %s only in JSON formatting", entry.Name, r.remoteLabel())

		return
	}

	output.Print(r.stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)
}

// outputDiffCreate outputs a diff entry for a newly created resource.
func (r *diffRunner) outputDiffCreate(opts diffOptions, entry stagingusecase.DiffEntry) {
	stagedValue := entry.StagedValue

	// Format as JSON if enabled
	if opts.parseJSON {
		if formatted, ok := jsonutil.TryFormat(stagedValue); ok {
			stagedValue = formatted
		}
	}

	name := diffEntryDisplayName(entry)
	label1 := fmt.Sprintf("%s (not in %s)", name, r.remoteLabel())
	label2 := fmt.Sprintf("%s (staged for creation)", name)

	diff := output.Diff(r.stdout, label1, label2, "", stagedValue)
	output.Print(r.stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)
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

// outputMetadata outputs metadata for a diff entry.
func (r *diffRunner) outputMetadata(entry stagingusecase.DiffEntry) {
	if desc := lo.FromPtr(entry.Description); desc != "" {
		output.Printf(r.stdout, "%s %s\n", colors.For(r.stdout).FieldLabel("Description:"), desc)
	}
}

// outputTagEntry outputs a tag entry.
func (r *diffRunner) outputTagEntry(tagEntry stagingusecase.DiffTagEntry) {
	output.Printf(r.stdout, "%s %s (staged tag changes)\n", colors.For(r.stdout).Info("Tags:"), diffDisplayName(tagEntry.Name, tagEntry.Namespace))

	if len(tagEntry.Add) > 0 {
		tagPairs := slices.Collect(it.Map(maputil.SortedKeys(tagEntry.Add), func(k string) string {
			return fmt.Sprintf("%s=%s", k, tagEntry.Add[k])
		}))

		output.Printf(r.stdout, "  %s %s\n", colors.For(r.stdout).OpAdd("+"), strings.Join(tagPairs, ", "))
	}

	if len(tagEntry.Remove) > 0 {
		tagPairs := slices.Collect(it.Map(maputil.SortedKeys(tagEntry.Remove), func(k string) string {
			if v := tagEntry.Remove[k]; v != "" {
				return fmt.Sprintf("%s=%s", k, v)
			}

			return k
		}))

		output.Printf(r.stdout, "  %s %s\n", colors.For(r.stdout).OpDelete("-"), strings.Join(tagPairs, ", "))
	}
}
