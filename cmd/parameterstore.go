package main

import "github.com/urfave/cli/v2"

var ParameterStoreCommand = &cli.Command{
	Name:    "parameter-store",
	Aliases: []string{"parameterstore", "ps", "ssm"},
	Usage:   "Interact with AWS Parameter Store",
}

func init() {
	App.Commands = append(App.Commands, ParameterStoreCommand)
}
