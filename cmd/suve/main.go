package main

import (
	"fmt"
	"os"

	"github.com/mpyw/suve/internal/cli"
)

func main() {
	if err := cli.App.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
