package main

import (
	"os"

	"github.com/mpyw/suve/internal/cli"
	"github.com/mpyw/suve/internal/output"
)

func main() {
	if err := cli.App.Run(os.Args); err != nil {
		output.Error(os.Stderr, "%v", err)
		os.Exit(1)
	}
}
