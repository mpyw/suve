package main

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/mpyw/suve/pkg/actions"
	"github.com/mpyw/suve/pkg/api"
	"github.com/mpyw/suve/pkg/aws/apicore/parameterstore"
	"github.com/mpyw/suve/pkg/aws/apicore/secretsmanager"
	parameterstoreversion "github.com/mpyw/suve/pkg/aws/versioning/parameterstore"
	secretsmanagerversion "github.com/mpyw/suve/pkg/aws/versioning/secretsmanager"
	"github.com/urfave/cli/v2"
)

func withAwsConfig(fn func(c *cli.Context, cfg aws.Config) error) func(*cli.Context) error {
	return func(c *cli.Context) error {
		cfg, err := config.LoadDefaultConfig(c.Context)
		if err != nil {
			return err
		}
		return fn(c, cfg)
	}
}

func withParameterStore(fn func(*cli.Context) error) func(*cli.Context) error {
	return withAwsConfig(func(c *cli.Context, cfg aws.Config) error {
		c.Context = actions.WithAPI(c.Context, api.New(parameterstore.New(cfg, c.Bool("with-decryption"))))
		c.Context = actions.WithVersionParser(c.Context, parameterstoreversion.VersionParser)
		return fn(c)
	})
}

func withSecretsManager(fn func(*cli.Context) error) func(*cli.Context) error {
	return withAwsConfig(func(c *cli.Context, cfg aws.Config) error {
		c.Context = actions.WithAPI(c.Context, api.New(secretsmanager.New(cfg, c.Bool("include-deprecated"))))
		c.Context = actions.WithVersionParser(c.Context, secretsmanagerversion.VersionParser)
		return fn(c)
	})
}

func withSecretsManagerAlwaysIncludeDeprecated(fn func(*cli.Context) error) func(*cli.Context) error {
	return withAwsConfig(func(c *cli.Context, cfg aws.Config) error {
		c.Context = actions.WithAPI(c.Context, api.New(secretsmanager.New(cfg, true)))
		c.Context = actions.WithVersionParser(c.Context, secretsmanagerversion.VersionParser)
		return fn(c)
	})
}
