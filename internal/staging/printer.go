package staging

import (
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/colors"
	"github.com/mpyw/suve/internal/maputil"
)

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
		_, _ = fmt.Fprintf(p.Writer, "  %s %s\n", opColor, name)
		return
	}

	_, _ = fmt.Fprintf(p.Writer, "\n%s %s\n", opColor, name)
	_, _ = fmt.Fprintf(p.Writer, "  %s %s\n", colors.FieldLabel("Staged:"), entry.StagedAt.Format("2006-01-02 15:04:05"))

	switch entry.Operation {
	case OperationCreate, OperationUpdate:
		if entry.Value != nil {
			value := lo.FromPtr(entry.Value)
			if len(value) > 100 {
				value = value[:100] + "..."
			}
			_, _ = fmt.Fprintf(p.Writer, "  %s %s\n", colors.FieldLabel("Value:"), value)
		}
	case OperationDelete:
		if showDeleteOptions && entry.DeleteOptions != nil {
			switch {
			case entry.DeleteOptions.Force:
				_, _ = fmt.Fprintf(p.Writer, "  %s force (immediate, no recovery)\n", colors.FieldLabel("Delete:"))
			case entry.DeleteOptions.RecoveryWindow > 0:
				_, _ = fmt.Fprintf(p.Writer, "  %s %d days recovery window\n", colors.FieldLabel("Delete:"), entry.DeleteOptions.RecoveryWindow)
			}
		}
	}

	// Show tags
	if len(entry.Tags) > 0 {
		var tagPairs []string
		for _, k := range maputil.SortedKeys(entry.Tags) {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", k, entry.Tags[k]))
		}
		_, _ = fmt.Fprintf(p.Writer, "  %s %s\n", colors.FieldLabel("Tags:"), strings.Join(tagPairs, ", "))
	}

	// Show untag keys
	if entry.UntagKeys.Len() > 0 {
		_, _ = fmt.Fprintf(p.Writer, "  %s %s\n", colors.FieldLabel("Untag:"), strings.Join(entry.UntagKeys.Values(), ", "))
	}
}
