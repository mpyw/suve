package main

import (
	"github.com/urfave/cli/v2"
)

var App = &cli.App{
	Name:    "suve",
	Usage:   "Git-like CLI for AWS Parameter Store and Secrets Manager",
	Version: "0.1.0",
	Commands: []*cli.Command{
		ssmCommand,
		smCommand,
	},
}
