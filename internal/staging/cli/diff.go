package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"

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
}

// remoteLabel names the backing store in diff labels (e.g. "App Configuration",
// "Key Vault", "Secret Manager") via the strategy's ServiceName. This runner
// serves every non-AWS provider, so the label must not be hard-coded to "AWS".
func (r *DiffRunner) remoteLabel() string {
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

	// Output results in sorted order
	first := true

	for _, name := range maputil.SortedNames(result.Entries, func(e stagingusecase.DiffEntry) string { return e.Name }) {
		for _, entry := range result.Entries {
			if entry.Name != name {
				continue
			}

			switch entry.Type {
			case stagingusecase.DiffEntryWarning:
				output.Warning(r.Stderr, "%s is %s", entry.Name, entry.Warning)
			case stagingusecase.DiffEntryAutoUnstaged:
				output.Warning(r.Stderr, "unstaged %s: %s", entry.Name, entry.Warning)
			case stagingusecase.DiffEntryCreate:
				if !first {
					output.Println(r.Stdout, "")
				}

				first = false

				r.OutputDiffCreate(opts, entry)
			case stagingusecase.DiffEntryNormal:
				if !first {
					output.Println(r.Stdout, "")
				}

				first = false

				r.OutputDiff(opts, entry)
			}

			break
		}
	}

	// Output tag entries
	for _, name := range maputil.SortedNames(result.TagEntries, func(e stagingusecase.DiffTagEntry) string { return e.Name }) {
		for _, tagEntry := range result.TagEntries {
			if tagEntry.Name != name {
				continue
			}

			if !first {
				output.Println(r.Stdout, "")
			}

			first = false

			r.OutputTagEntry(tagEntry)

			break
		}
	}

	return nil
}

// OutputDiff outputs a diff entry for an existing resource.
func (r *DiffRunner) OutputDiff(opts DiffOptions, entry stagingusecase.DiffEntry) {
	awsValue := entry.AWSValue
	stagedValue := entry.StagedValue

	// Format as JSON if enabled
	if opts.ParseJSON {
		awsValue, stagedValue = jsonutil.TryFormatOrWarn2(awsValue, stagedValue, r.Stderr, entry.Name)
	}

	name := diffEntryDisplayName(entry)
	label1 := fmt.Sprintf("%s%s (%s)", name, entry.AWSIdentifier, r.remoteLabel())
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), name)

	diff := output.Diff(r.Stdout, label1, label2, awsValue, stagedValue)
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
	if entry.Namespace == "" {
		return entry.Name
	}

	return fmt.Sprintf("%s [%s]", entry.Name, entry.Namespace)
}

// OutputMetadata outputs metadata for a diff entry.
func (r *DiffRunner) OutputMetadata(entry stagingusecase.DiffEntry) {
	if desc := lo.FromPtr(entry.Description); desc != "" {
		output.Printf(r.Stdout, "%s %s\n", colors.For(r.Stdout).FieldLabel("Description:"), desc)
	}
}

// OutputTagEntry outputs a tag entry.
func (r *DiffRunner) OutputTagEntry(tagEntry stagingusecase.DiffTagEntry) {
	output.Printf(r.Stdout, "%s %s (staged tag changes)\n", colors.For(r.Stdout).Info("Tags:"), tagEntry.Name)

	if len(tagEntry.Add) > 0 {
		tagPairs := make([]string, 0, len(tagEntry.Add))
		for _, k := range maputil.SortedKeys(tagEntry.Add) {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, tagEntry.Add[k]))
		}

		output.Printf(r.Stdout, "  %s %s\n", colors.For(r.Stdout).OpAdd("+"), strings.Join(tagPairs, ", "))
	}

	if len(tagEntry.Remove) > 0 {
		tagPairs := make([]string, 0, len(tagEntry.Remove))
		for _, k := range maputil.SortedKeys(tagEntry.Remove) {
			if v := tagEntry.Remove[k]; v != "" {
				tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, v))
			} else {
				tagPairs = append(tagPairs, k)
			}
		}

		output.Printf(r.Stdout, "  %s %s\n", colors.For(r.Stdout).OpDelete("-"), strings.Join(tagPairs, ", "))
	}
}
