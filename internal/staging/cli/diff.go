// Package cli provides shared runners and command builders for stage commands.
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
				r.outputDiffCreate(opts, entry)
			case stagingusecase.DiffEntryNormal:
				if !first {
					output.Println(r.Stdout, "")
				}
				first = false
				r.outputDiff(opts, entry)
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
			r.outputTagEntry(tagEntry)
			break
		}
	}

	return nil
}

func (r *DiffRunner) outputDiff(opts DiffOptions, entry stagingusecase.DiffEntry) {
	awsValue := entry.AWSValue
	stagedValue := entry.StagedValue

	// Format as JSON if enabled
	if opts.ParseJSON {
		awsValue, stagedValue = jsonutil.TryFormatOrWarn2(awsValue, stagedValue, r.Stderr, entry.Name)
	}

	label1 := fmt.Sprintf("%s%s (AWS)", entry.Name, entry.AWSIdentifier)
	label2 := fmt.Sprintf(lo.Ternary(
		entry.Operation == staging.OperationDelete,
		"%s (staged for deletion)",
		"%s (staged)",
	), entry.Name)

	diff := output.Diff(label1, label2, awsValue, stagedValue)
	output.Print(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)
}

func (r *DiffRunner) outputDiffCreate(opts DiffOptions, entry stagingusecase.DiffEntry) {
	stagedValue := entry.StagedValue

	// Format as JSON if enabled
	if opts.ParseJSON {
		if formatted, ok := jsonutil.TryFormat(stagedValue); ok {
			stagedValue = formatted
		}
	}

	label1 := fmt.Sprintf("%s (not in AWS)", entry.Name)
	label2 := fmt.Sprintf("%s (staged for creation)", entry.Name)

	diff := output.Diff(label1, label2, "", stagedValue)
	output.Print(r.Stdout, diff)

	// Show staged metadata
	r.outputMetadata(entry)
}

func (r *DiffRunner) outputMetadata(entry stagingusecase.DiffEntry) {
	if desc := lo.FromPtr(entry.Description); desc != "" {
		output.Printf(r.Stdout, "%s %s\n", colors.FieldLabel("Description:"), desc)
	}
}

func (r *DiffRunner) outputTagEntry(tagEntry stagingusecase.DiffTagEntry) {
	output.Printf(r.Stdout, "%s %s (staged tag changes)\n", colors.Info("Tags:"), tagEntry.Name)

	if len(tagEntry.Add) > 0 {
		var tagPairs []string
		for _, k := range maputil.SortedKeys(tagEntry.Add) {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, tagEntry.Add[k]))
		}
		output.Printf(r.Stdout, "  %s %s\n", colors.OpAdd("+"), strings.Join(tagPairs, ", "))
	}

	if tagEntry.Remove.Len() > 0 {
		output.Printf(r.Stdout, "  %s %s\n", colors.OpDelete("-"), strings.Join(tagEntry.Remove.Values(), ", "))
	}
}
