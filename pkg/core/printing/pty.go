package printing

import (
	"fmt"
	"time"

	"github.com/mpyw/suve/pkg/core/revisioning"
)

var _ PrettyPrinter = (*PTYPrinter)(nil)

type PTYPrinter struct{}

func (*PTYPrinter) GenerateVersionDiffText(prev, current *revisioning.Revision) string {
	return generateVersionDiffText(prev, current)
}

func (*PTYPrinter) GenerateVersionDescription(current *revisioning.Revision) string {
	return fmt.Sprintf(
		"Version: %s\nDate: %s",
		current.Version.String(),
		current.Date.In(time.Local).Format(time.RFC3339),
	)
}
