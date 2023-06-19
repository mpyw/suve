package main

import (
	"context"
	"log"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/mpyw/suve/pkg/actions"
	"github.com/mpyw/suve/pkg/core/printing"
)

func main() {
	ctx := context.Background()
	ctx = actions.WithWriter(ctx, os.Stdout)
	ctx = actions.WithPrinter(ctx, func() printing.PrettyPrinter {
		if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
			return &printing.TTYPrinter{}
		} else {
			return &printing.PTYPrinter{}
		}
	}())
	if err := App.RunContext(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}
