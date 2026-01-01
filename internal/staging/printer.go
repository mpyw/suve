package staging

import (
	"fmt"
	"io"

	"github.com/fatih/color"
)

// EntryPrinter prints staged entries to the given writer.
type EntryPrinter struct {
	Writer io.Writer
}

// PrintEntry prints a single staged entry.
// If verbose is true, shows detailed information including timestamp and value.
// If showDeleteOptions is true, shows delete options (Force/RecoveryWindow) for delete operations.
func (p *EntryPrinter) PrintEntry(name string, entry Entry, verbose, showDeleteOptions bool) {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	var opColor string
	switch entry.Operation {
	case OperationCreate:
		opColor = green("A")
	case OperationUpdate:
		opColor = green("M")
	case OperationDelete:
		opColor = red("D")
	}

	if verbose {
		_, _ = fmt.Fprintf(p.Writer, "\n%s %s\n", opColor, name)
		_, _ = fmt.Fprintf(p.Writer, "  %s %s\n", cyan("Staged:"), entry.StagedAt.Format("2006-01-02 15:04:05"))
		if entry.Operation == OperationCreate || entry.Operation == OperationUpdate {
			value := entry.Value
			if len(value) > 100 {
				value = value[:100] + "..."
			}
			_, _ = fmt.Fprintf(p.Writer, "  %s %s\n", cyan("Value:"), value)
		} else if entry.Operation == OperationDelete && showDeleteOptions && entry.DeleteOptions != nil {
			if entry.DeleteOptions.Force {
				_, _ = fmt.Fprintf(p.Writer, "  %s force (immediate, no recovery)\n", cyan("Delete:"))
			} else if entry.DeleteOptions.RecoveryWindow > 0 {
				_, _ = fmt.Fprintf(p.Writer, "  %s %d days recovery window\n", cyan("Delete:"), entry.DeleteOptions.RecoveryWindow)
			}
		}
	} else {
		_, _ = fmt.Fprintf(p.Writer, "  %s %s\n", opColor, name)
	}
}
