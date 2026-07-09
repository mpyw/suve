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

// PrintEntry prints a single staged entry identified by key.
// If verbose is true, shows detailed information including timestamp and value.
// If showDeleteOptions is true, shows delete options (Force/RecoveryWindow) for delete operations.
func (p *EntryPrinter) PrintEntry(key EntryKey, entry Entry, verbose, showDeleteOptions bool) {
	pal := colors.For(p.Writer)

	var opColor string

	switch entry.Operation {
	case OperationCreate:
		opColor = pal.OpAdd("A")
	case OperationUpdate:
		opColor = pal.OpModify("M")
	case OperationDelete:
		opColor = pal.OpDelete("D")
	}

	// Azure App Configuration entries carry a namespace (the label axis); show it
	// inline so a name staged under several namespaces is unambiguous. Empty is
	// the null/default namespace (and every other provider), shown bare.
	nsSuffix := ""
	if key.Namespace != "" {
		nsSuffix = " " + pal.FieldLabel("["+key.Namespace+"]")
	}

	if !verbose {
		output.Printf(p.Writer, "  %s %s%s\n", opColor, key.Name, nsSuffix)

		return
	}

	output.Printf(p.Writer, "\n%s %s%s\n", opColor, key.Name, nsSuffix)
	output.Printf(p.Writer, "  %s %s\n", pal.FieldLabel("Staged:"), entry.StagedAt.Format("2006-01-02 15:04:05"))

	switch entry.Operation {
	case OperationCreate, OperationUpdate:
		if entry.Value != nil {
			value := lo.FromPtr(entry.Value)
			if len(value) > maxValueDisplayLength {
				value = value[:maxValueDisplayLength] + "..."
			}

			output.Printf(p.Writer, "  %s %s\n", pal.FieldLabel("Value:"), value)
		}
	case OperationDelete:
		if showDeleteOptions && entry.DeleteOptions != nil {
			switch {
			case entry.DeleteOptions.Force:
				output.Printf(p.Writer, "  %s force (immediate, no recovery)\n", pal.FieldLabel("Delete:"))
			case entry.DeleteOptions.RecoveryWindow > 0:
				output.Printf(p.Writer, "  %s %d days recovery window\n", pal.FieldLabel("Delete:"), entry.DeleteOptions.RecoveryWindow)
			}
		}
	}
}
