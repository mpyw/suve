package staging

import (
	"io"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/cli/output"
)

// maxValueDisplayLength is the maximum length of a value shown in status output.
const maxValueDisplayLength = 100

// EntryPrinter prints staged entries to the given writer.
type EntryPrinter struct {
	Writer io.Writer
}

// PrintEntry prints a single staged entry.
// If verbose is true, shows detailed information including timestamp and value.
// If showDeleteOptions is true, shows delete options (Force/RecoveryWindow) for delete operations.
func (p *EntryPrinter) PrintEntry(name string, entry Entry, verbose, showDeleteOptions bool) {
	var opColor string

	switch entry.Operation {
	case OperationCreate:
		opColor = colors.OpAdd("A")
	case OperationUpdate:
		opColor = colors.OpModify("M")
	case OperationDelete:
		opColor = colors.OpDelete("D")
	}

	if !verbose {
		output.Printf(p.Writer, "  %s %s\n", opColor, name)

		return
	}

	output.Printf(p.Writer, "\n%s %s\n", opColor, name)
	output.Printf(p.Writer, "  %s %s\n", colors.FieldLabel("Staged:"), entry.StagedAt.Format("2006-01-02 15:04:05"))

	switch entry.Operation {
	case OperationCreate, OperationUpdate:
		if entry.Value != nil {
			value := lo.FromPtr(entry.Value)
			if len(value) > maxValueDisplayLength {
				value = value[:maxValueDisplayLength] + "..."
			}

			output.Printf(p.Writer, "  %s %s\n", colors.FieldLabel("Value:"), value)
		}
	case OperationDelete:
		if showDeleteOptions && entry.DeleteOptions != nil {
			switch {
			case entry.DeleteOptions.Force:
				output.Printf(p.Writer, "  %s force (immediate, no recovery)\n", colors.FieldLabel("Delete:"))
			case entry.DeleteOptions.RecoveryWindow > 0:
				output.Printf(p.Writer, "  %s %d days recovery window\n", colors.FieldLabel("Delete:"), entry.DeleteOptions.RecoveryWindow)
			}
		}
	}
}
