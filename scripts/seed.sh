#!/usr/bin/env bash
#
# Seed the local emulators with comprehensive demo data for manual verification
# (CLI or GUI). It is driven by the `mise run seed-*` tasks — see mise.toml — but
# `bash scripts/seed.sh <service>` works standalone too.
#
#   <service> = aws-param | aws-secret | gcloud-secret | azure-secret | azure-param
#
# It expects the target emulator's environment to be present (exactly what
# `mise run bash --aws/--gcloud/--azure` injects); a service whose env is absent
# is skipped with a hint rather than failing, so `seed-all` inside a partial dev
# shell just seeds whatever is running.
#
# What it lays down per service (breadth over depth, so every read/stage view has
# something to show):
#   - applied resources with several versions stacked up (for show/log/diff and
#     the #VERSION / ~SHIFT / :LABEL specs), some carrying tags;
#   - a working staging area holding one of every staged shape: a fresh add, an
#     edit, a delete, and a tag add / change / removal.
set -eo pipefail

root="${MISE_PROJECT_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$root"

# A throwaway CLI binary kept out of bin/suve so seeding never clobbers a
# GUI-enabled build. The seed-build mise task compiles it once up front; building
# here too keeps a direct `bash scripts/seed.sh ...` self-contained.
suve_bin="$root/bin/suve-seed"
if [ ! -x "$suve_bin" ]; then
  echo "==> Building $suve_bin"
  go build -o "$suve_bin" ./cmd/suve
fi

# Staging writes go through the keychain-encrypted file store; pin a deterministic
# data key (base64 of 32 zero bytes) so seeding never blocks on an OS keychain
# prompt, exactly like `mise test` / `mise coverage`. This MUST match the key the
# `mise run bash` dev shell injects (see mise-tasks/bash) — otherwise the shell
# would read the working store under a different key and the staged changes below
# would be invisible there. Honor an override if one is already exported.
export SUVE_STAGING_KEY="${SUVE_STAGING_KEY:-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=}"

# run echoes the command, then executes it tolerating failure: seeding is
# best-effort and re-runnable, so an "already exists" create (or a service the
# emulator does not implement) is a skip, not a hard stop.
run() {
  printf '   $ suve %s\n' "$*"
  if ! "$suve_bin" "$@" </dev/null >/dev/null 2>&1; then
    printf '     ! skipped (already present, or unsupported by this emulator)\n' >&2
  fi
}

# run_stdin feeds $1 to the command over stdin via --value-stdin. Needed whenever
# a value would otherwise be misread as a flag (e.g. a PEM block beginning with
# "-----BEGIN"), and also mirrors how secrets are meant to be piped in practice.
run_stdin() {
  local value="$1"
  shift
  printf '   $ printf … | suve %s --value-stdin\n' "$*"
  if ! printf '%s' "$value" | "$suve_bin" "$@" --value-stdin >/dev/null 2>&1; then
    printf '     ! skipped (already present, or unsupported by this emulator)\n' >&2
  fi
}

# require_env VAR "human name" "mise run bash flag" — returns non-zero (so the
# caller can `|| return 0`) when the emulator env is not injected.
require_env() {
  if [ -z "${!1:-}" ]; then
    # The literal "$NAME" is shown to the user on purpose (not a shell expansion).
    # shellcheck disable=SC2016
    printf '==> %s: $%s not set — skipping. Start it with `mise run bash %s`,\n' "$2" "$1" "$3"
    printf '    then re-run this inside that shell.\n'
    return 1
  fi
  return 0
}

# ---------------------------------------------------------------------------
# AWS Parameter Store
# ---------------------------------------------------------------------------
seed_aws_param() {
  require_env AWS_ENDPOINT_URL "AWS Parameter Store" "--aws" || return 0
  echo "==> Seeding AWS Parameter Store…"

  # Applied baseline: multiple versions stacked, a spread of value types, tags.
  run aws param create /suve-demo/app/database-url "postgres://db.internal:5432/app?sslmode=require" --description "Primary database DSN"
  run aws param update --yes /suve-demo/app/database-url "postgres://db.internal:5432/app?sslmode=verify-full"
  run aws param update --yes /suve-demo/app/database-url "postgres://db-primary.internal:5432/app?sslmode=verify-full"
  run aws param tag /suve-demo/app/database-url env=production team=payments managed-by=suve

  run aws param create /suve-demo/app/database-password "correct-horse-battery-staple" --secure --description "DB password (rotated quarterly)"
  run aws param update --yes /suve-demo/app/database-password "Tr0ub4dour-and-3-more" --secure
  run aws param tag /suve-demo/app/database-password rotation=90d env=production

  run aws param create /suve-demo/app/feature-flags "beta,dark-mode" --type StringList --description "Enabled feature flags"
  run aws param update --yes /suve-demo/app/feature-flags "beta,dark-mode,new-checkout"

  run aws param create /suve-demo/app/config '{"timeout":30,"retries":3}' --description "App tuning knobs"
  run aws param update --yes /suve-demo/app/config '{"timeout":60,"retries":5,"backoff":"exponential"}'
  run aws param tag /suve-demo/app/config format=json

  run aws param create /suve-demo/shared/region "us-east-1"

  # Applied resources that back the staged scenarios below.
  run aws param create /suve-demo/staging/edit-me "live-value-v1"
  run aws param update --yes /suve-demo/staging/edit-me "live-value-v2"
  run aws param create /suve-demo/staging/delete-me "obsolete, staged for deletion"
  run aws param create /suve-demo/staging/tag-add "value"
  run aws param tag /suve-demo/staging/tag-add existing=kept
  run aws param create /suve-demo/staging/tag-change "value"
  run aws param tag /suve-demo/staging/tag-change env=staging
  run aws param create /suve-demo/staging/tag-remove "value"
  run aws param tag /suve-demo/staging/tag-remove temporary=yes keep=yes

  # Working-area staged changes, deliberately left un-applied for stage status/diff.
  run aws stage param add /suve-demo/staging/new-param "created via staging, not yet applied"
  run aws stage param edit /suve-demo/staging/edit-me "edited via staging (v3 pending)"
  run aws stage param delete /suve-demo/staging/delete-me
  run aws stage param tag /suve-demo/staging/tag-add owner=team-a      # stage a NEW tag
  run aws stage param tag /suve-demo/staging/tag-change env=production # stage a CHANGED tag value
  run aws stage param untag /suve-demo/staging/tag-remove temporary    # stage a tag REMOVAL
}

# ---------------------------------------------------------------------------
# AWS Secrets Manager
# ---------------------------------------------------------------------------
seed_aws_secret() {
  require_env AWS_ENDPOINT_URL "AWS Secrets Manager" "--aws" || return 0
  echo "==> Seeding AWS Secrets Manager…"

  run aws secret create suve-demo/db-credentials '{"username":"app","password":"p@ss-v1"}' --description "Service DB credentials"
  run aws secret update --yes suve-demo/db-credentials '{"username":"app","password":"p@ss-v2"}'
  run aws secret update --yes suve-demo/db-credentials '{"username":"app","password":"p@ss-v3"}'
  run aws secret tag suve-demo/db-credentials env=production team=payments

  run aws secret create suve-demo/api-key "ak_live_0001"
  run aws secret update --yes suve-demo/api-key "ak_live_0002"

  # Piped over stdin: the value leads with "-----", which would otherwise be
  # parsed as a flag (and it is how you would feed a real cert anyway).
  run_stdin "$(printf -- '-----BEGIN CERTIFICATE-----\nMIIB-demo-not-a-real-cert\nQU5ETUlORw==\n-----END CERTIFICATE-----')" aws secret create suve-demo/tls-cert --description "PEM certificate (multiline)"

  # Applied resources that back the staged scenarios below.
  run aws secret create suve-demo/staging/edit-me "live-secret-v1"
  run aws secret update --yes suve-demo/staging/edit-me "live-secret-v2"
  run aws secret create suve-demo/staging/delete-me "obsolete secret"
  run aws secret create suve-demo/staging/tag-add "value"
  run aws secret tag suve-demo/staging/tag-add existing=kept
  run aws secret create suve-demo/staging/tag-change "value"
  run aws secret tag suve-demo/staging/tag-change env=staging
  run aws secret create suve-demo/staging/tag-remove "value"
  run aws secret tag suve-demo/staging/tag-remove temporary=yes keep=yes

  # Working-area staged changes.
  run aws stage secret add suve-demo/staging/new-secret "created via staging, not yet applied"
  run aws stage secret edit suve-demo/staging/edit-me "edited via staging"
  run aws stage secret delete suve-demo/staging/delete-me
  run aws stage secret tag suve-demo/staging/tag-add owner=team-a
  run aws stage secret tag suve-demo/staging/tag-change env=production
  run aws stage secret untag suve-demo/staging/tag-remove temporary
}

# ---------------------------------------------------------------------------
# Google Cloud Secret Manager (secret-only; names are [A-Za-z0-9_-], labels lowercase)
# ---------------------------------------------------------------------------
seed_gcloud_secret() {
  require_env SUVE_GCLOUD_SECRETMANAGER_ENDPOINT "Google Cloud Secret Manager" "--gcloud" || return 0
  echo "==> Seeding Google Cloud Secret Manager…"

  run gcloud secret create suve-demo-db-password "db-pass-v1"
  run gcloud secret update --yes suve-demo-db-password "db-pass-v2"
  run gcloud secret update --yes suve-demo-db-password "db-pass-v3"
  run gcloud secret tag suve-demo-db-password env=production team=payments

  run gcloud secret create suve-demo-api-token "token-v1"
  run gcloud secret update --yes suve-demo-api-token "token-v2"

  run gcloud secret create suve-demo-config '{"region":"us","tier":"gold"}'

  # Applied resources that back the staged scenarios below.
  run gcloud secret create suve-demo-stage-edit "live-v1"
  run gcloud secret update --yes suve-demo-stage-edit "live-v2"
  run gcloud secret create suve-demo-stage-delete "obsolete"
  run gcloud secret create suve-demo-stage-tag-add "value"
  run gcloud secret tag suve-demo-stage-tag-add existing=kept
  run gcloud secret create suve-demo-stage-tag-change "value"
  run gcloud secret tag suve-demo-stage-tag-change env=staging
  run gcloud secret create suve-demo-stage-tag-remove "value"
  run gcloud secret tag suve-demo-stage-tag-remove temporary=yes keep=yes

  # Working-area staged changes.
  run gcloud stage add suve-demo-stage-new "created via staging, not yet applied"
  run gcloud stage edit suve-demo-stage-edit "edited via staging"
  run gcloud stage delete suve-demo-stage-delete
  run gcloud stage tag suve-demo-stage-tag-add owner=team-a
  run gcloud stage tag suve-demo-stage-tag-change env=production
  run gcloud stage untag suve-demo-stage-tag-remove temporary
}

# ---------------------------------------------------------------------------
# Azure Key Vault (secret; names are [0-9A-Za-z-], opaque-versioned)
# ---------------------------------------------------------------------------
seed_azure_secret() {
  require_env SUVE_AZURE_KEYVAULT_ENDPOINT "Azure Key Vault" "--azure-keyvault" || return 0
  echo "==> Seeding Azure Key Vault…"

  # Key Vault tags live on a secret *version* (read-modify-write against the
  # current one), and each update mints a fresh version with no tags. Tag v1 and
  # the latest v3 but not the middle v2, so the per-version nature is visible in
  # `azure secret log`: v1 → tagged, v2 → bare, v3 (current) → tagged.
  run azure secret create suve-demo-db-password "db-pass-v1"
  run azure secret tag suve-demo-db-password imported=legacy
  run azure secret update --yes suve-demo-db-password "db-pass-v2"
  run azure secret update --yes suve-demo-db-password "db-pass-v3"
  run azure secret tag suve-demo-db-password env=production team=payments

  run azure secret create suve-demo-api-key "ak-live-0001"
  run azure secret update --yes suve-demo-api-key "ak-live-0002"

  # Applied resources that back the staged scenarios below.
  run azure secret create suve-demo-stage-edit "live-v1"
  run azure secret update --yes suve-demo-stage-edit "live-v2"
  run azure secret create suve-demo-stage-delete "obsolete"
  run azure secret create suve-demo-stage-tag-add "value"
  run azure secret tag suve-demo-stage-tag-add existing=kept
  run azure secret create suve-demo-stage-tag-change "value"
  run azure secret tag suve-demo-stage-tag-change env=staging
  run azure secret create suve-demo-stage-tag-remove "value"
  run azure secret tag suve-demo-stage-tag-remove temporary=yes keep=yes

  # Working-area staged changes.
  run azure stage secret add suve-demo-stage-new "created via staging, not yet applied"
  run azure stage secret edit suve-demo-stage-edit "edited via staging"
  run azure stage secret delete suve-demo-stage-delete
  run azure stage secret tag suve-demo-stage-tag-add owner=team-a
  run azure stage secret tag suve-demo-stage-tag-change env=production
  run azure stage secret untag suve-demo-stage-tag-remove temporary
}

# ---------------------------------------------------------------------------
# Azure App Configuration (param; UNVERSIONED, keys may contain slashes,
# namespace = the label axis)
# ---------------------------------------------------------------------------
seed_azure_param() {
  require_env SUVE_AZURE_APPCONFIG_CONNECTION_STRING "Azure App Configuration" "--azure-appconfig" || return 0
  echo "==> Seeding Azure App Configuration…"

  # No version history here (last-write-wins); updates overwrite in place.
  run azure param create suve-demo/app/database-url "postgres://db.internal:5432/app"
  run azure param update --yes suve-demo/app/database-url "postgres://db-primary.internal:5432/app"
  run azure param tag suve-demo/app/database-url env=production team=payments

  run azure param create suve-demo/app/feature-flags "beta,dark-mode"

  # The label axis (Azure calls it a "label"; suve exposes it as --namespace).
  # Same key under the default (NULL) label plus dev / prd, with a distinct value
  # each. Tags are per-(key, label): tag the default and prd rows, leave dev bare.
  run azure param create suve-demo/app/log-level "info"                     # (NULL) label
  run azure param --namespace dev create suve-demo/app/log-level "debug"
  run azure param --namespace prd create suve-demo/app/log-level "warn"
  run azure param tag suve-demo/app/log-level owner=platform
  run azure param --namespace prd tag suve-demo/app/log-level owner=platform change-ticket=CHG-1024

  # Applied settings that back the staged scenarios below (default namespace).
  run azure param create suve-demo/staging/edit-me "live-value"
  run azure param create suve-demo/staging/delete-me "obsolete setting"
  run azure param create suve-demo/staging/tag-add "value"
  run azure param tag suve-demo/staging/tag-add existing=kept
  run azure param create suve-demo/staging/tag-change "value"
  run azure param tag suve-demo/staging/tag-change env=staging
  run azure param create suve-demo/staging/tag-remove "value"
  run azure param tag suve-demo/staging/tag-remove temporary=yes keep=yes

  # Working-area staged changes.
  run azure stage param add suve-demo/staging/new-setting "created via staging, not yet applied"
  run azure stage param edit suve-demo/staging/edit-me "edited via staging"
  run azure stage param delete suve-demo/staging/delete-me
  run azure stage param tag suve-demo/staging/tag-add owner=team-a
  run azure stage param tag suve-demo/staging/tag-change env=production
  run azure stage param untag suve-demo/staging/tag-remove temporary
}

case "${1:-}" in
  aws-param) seed_aws_param ;;
  aws-secret) seed_aws_secret ;;
  gcloud-secret) seed_gcloud_secret ;;
  azure-secret) seed_azure_secret ;;
  azure-param) seed_azure_param ;;
  *)
    echo "usage: bash scripts/seed.sh {aws-param|aws-secret|gcloud-secret|azure-secret|azure-param}" >&2
    exit 2
    ;;
esac
