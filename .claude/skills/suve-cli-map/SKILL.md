---
name: suve-cli-map
description: >-
  Load when you need the per-provider capability, versioning, and alias matrix
  for suve's CLI without reading the full docs/{aws,azure,gcloud}.md option
  tables. A one-page orientation; link out for per-command detail.
---

# suve CLI capability map

A condensed matrix of what each provider/service supports. For per-command
options and examples, read the full docs:
[docs/aws.md](../../../docs/aws.md), [docs/gcloud.md](../../../docs/gcloud.md),
[docs/azure.md](../../../docs/azure.md).

## Capability matrix

| Provider | Service | Command | Versioning model | Staging | Service aliases | Activation env var |
|----------|---------|---------|------------------|---------|-----------------|--------------------|
| AWS | Parameter Store | `suve aws param` | Integer versions; `#VERSION` `~SHIFT` | Yes | `ssm`, `ps` | `AWS_PROFILE` \| `AWS_ACCESS_KEY_ID` \| `AWS_VAULT` |
| AWS | Secrets Manager | `suve aws secret` | Opaque version id + staging labels; `#VERSION` `:LABEL` `~SHIFT` (`:AWSCURRENT`/`:AWSPREVIOUS`) | Yes | `sm`, `secretsmanager` | same as above |
| Google Cloud | Secret Manager | `suve gcloud secret` | Integer versions (`latest`); no labels; `#VERSION` `~SHIFT` | Yes (secret-only) | `secrets`, `sm` | `GOOGLE_CLOUD_PROJECT` |
| Azure | Key Vault | `suve azure secret` | Opaque ids; no labels; `#VERSION` `~SHIFT`; modified time is second-granular | Yes | `kv`, `keyvault` | `AZURE_KEYVAULT_NAME` |
| Azure | App Configuration | `suve azure param` | Unversioned (single current value); last-write-wins | Yes | `appconfig`, `ac`, `appcfg` | `AZURE_APPCONFIG_NAME` |

## Command shape

- Explicit group form is always available: `suve <provider> <service> <command>`.
- The bare `param` / `secret` / `stage` form works when exactly one provider is
  active for that service in the environment (see the
  `provider-selection-and-registry` skill).
- Staging shares one workflow across providers:
  `add` / `edit` / `delete` / `status` / `diff` / `apply` / `reset` /
  `tag` / `untag` / `export` / `import`. The `stage` alias is `stg`.

## Group aliases

| Group | Aliases |
|-------|---------|
| `aws` | (none) |
| `gcloud` | `gcp`, `google` |
| `azure` | `az` |

## Notes

- Google Cloud is secret-only (no parameter store), so there is never a Google
  Cloud `param` alias.
- Immutable-version services (Secrets Manager, Google Cloud Secret Manager,
  Azure Key Vault) apply a staged `edit` as a new version.
- Azure App Configuration is unversioned, so its staging uses last-write-wins
  (no modified-after conflict check).
- AWS also honors the standard AWS SDK credential chain; the env vars above are
  the signals suve's provider detection uses to activate the flat alias.
