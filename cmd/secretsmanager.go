package main

import "github.com/urfave/cli/v2"

var SecretsManagerCommand = &cli.Command{
	Name:    "secrets-manager",
	Aliases: []string{"secretsmanager", "sm"},
	Usage:   "Interact with AWS Secrets Manager",
}

func init() {
	App.Commands = append(App.Commands, SecretsManagerCommand)
}
