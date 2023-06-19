package printing

import (
	"github.com/aymanbagabas/go-udiff"
	"github.com/aymanbagabas/go-udiff/myers"
	"github.com/mpyw/suve/pkg/core/revisioning"
)

type PrettyPrinter interface {
	GenerateVersionDiffText(prev, current *revisioning.Revision) string
	GenerateVersionDescription(current *revisioning.Revision) string
}

func generateVersionDiffText(prev, current *revisioning.Revision) string {
	edits := myers.ComputeEdits(prev.Content.String(), current.Content.String())
	unified, _ := udiff.ToUnified(prev.Version.String(), current.Version.String(), prev.Content.String(), edits)
	return unified
}
