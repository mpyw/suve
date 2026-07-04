package internal

import (
	"io"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/pager"
)

// WithPager runs fn with the root command's stdout routed through the pager
// (unless noPager is set) and the root command's stderr. It centralizes the
// writer wiring shared by the paging commands so call sites only construct
// their Runner and invoke it.
func WithPager(cmd *cli.Command, noPager bool, fn func(stdout, stderr io.Writer) error) error {
	return pager.WithPagerWriter(cmd.Root().Writer, noPager, func(w io.Writer) error {
		return fn(w, cmd.Root().ErrWriter)
	})
}
