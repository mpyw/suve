package cli_test

import (
	"fmt"
	"testing"

	"github.com/urfave/cli/v3"

	appcli "github.com/mpyw/suve/internal/cli"
)

func TestFlagAddresses(t *testing.T) {
	// Check global flags
	fmt.Printf("=== Global flags ===\n")
	fmt.Printf("cli.HelpFlag: %p\n", cli.HelpFlag)
	fmt.Printf("cli.VersionFlag: %p\n", cli.VersionFlag)

	app1 := appcli.MakeApp()
	app2 := appcli.MakeApp()

	fmt.Printf("\n=== After MakeApp ===\n")
	fmt.Printf("cli.HelpFlag: %p\n", cli.HelpFlag)
	fmt.Printf("cli.VersionFlag: %p\n", cli.VersionFlag)

	fmt.Printf("\n=== App level ===\n")
	fmt.Printf("app1: %p\n", app1)
	fmt.Printf("app2: %p\n", app2)

	// Check SM cat subcommand flags deeply
	sm1 := app1.Commands[1]
	sm2 := app2.Commands[1]
	cat1 := sm1.Commands[1]
	cat2 := sm2.Commands[1]

	fmt.Printf("\n=== Cat subcommand flags ===\n")
	for i, f := range cat1.Flags {
		bf, ok := f.(*cli.BoolFlag)
		if ok {
			fmt.Printf("cat1.Flags[%d] (%s): ptr=%p\n", i, bf.Name, bf)
		}
	}
	for i, f := range cat2.Flags {
		bf, ok := f.(*cli.BoolFlag)
		if ok {
			fmt.Printf("cat2.Flags[%d] (%s): ptr=%p\n", i, bf.Name, bf)
		}
	}
}
