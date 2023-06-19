package printing

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mpyw/suve/pkg/core/revisioning"
)

var _ PrettyPrinter = (*TTYPrinter)(nil)

type TTYPrinter struct{}

func (*TTYPrinter) GenerateVersionDiffText(prev, current *revisioning.Revision) string {
	lines := strings.Split(generateVersionDiffText(prev, current), "\n")
	var prevFunc func(format string, a ...interface{}) string
	for i := range lines {
		switch {
		case i == 0, i == 1:
			lines[i] = color.New(color.Reset, color.Bold).Sprint(lines[i])
		case i == 2:
			lines[i] = color.CyanString("%s", lines[i])
		case strings.HasPrefix(lines[i], "-"):
			lines[i] = color.RedString("%s", lines[i])
			prevFunc = color.RedString
		case strings.HasPrefix(lines[i], "+"):
			lines[i] = color.GreenString("%s", lines[i])
			prevFunc = color.GreenString
		case strings.HasPrefix(lines[i], "\\") && prevFunc != nil:
			lines[i] = prevFunc("%s", lines[i])
		default:
			prevFunc = nil
		}
	}
	return strings.Join(lines, "\n")
}

func (*TTYPrinter) GenerateVersionDescription(current *revisioning.Revision) string {
	return color.YellowString("Version: %s\n", current.Version.String()) +
		fmt.Sprintf("Date: %s", current.Date.In(time.Local).Format(time.RFC3339))
}
