---
name: add-e2e-emulator
description: Use when adding emulator-backed e2e coverage for a provider — wiring an env-gated emulator seam, a compose.test.yaml service, a mise e2e-<provider> task, and a closed-network CI job. Covers emulator-vs-SDK divergences and coverage upload.
---

# Adding emulator-backed e2e

suve runs provider e2e against local emulators: Google Cloud (PR #256), Azure
App Configuration (PR #258), and Azure Key Vault (PR #259).

## Emulator seam

- Add an **env-gated seam in the adapter** that swaps credentials/TLS into
  emulator mode when set — e.g. `SUVE_AZURE_APPCONFIG_CONNECTION_STRING`,
  `SUVE_AZURE_KEYVAULT_ENDPOINT`. Document in the code that the variable is for
  emulator use and must never be pointed at production.

## Compose + task + CI

- Add a `compose.test.yaml` service under the provider's compose profile.
- Add a `mise e2e-<provider>` task (AWS is `mise e2e-aws`; Google Cloud is
  `mise e2e-gcloud`; Azure is `mise e2e-azure-appconfig` / `mise e2e-azure-keyvault`).
- Add a CI job that runs on a **closed compose network with no host ports**
  (#468) — the suite runs in-container and tears everything down on exit.
- **e2e CI jobs must upload coverage.**

## Vet the emulator against the real SDK

Upstream emulators break real SDKs — vet with the official SDK, not curl:

- App Configuration (#258) documents three divergences that the fork
  `ghcr.io/mpyw/azure-app-configuration-emulator` fixes: HMAC auth-header
  parsing, RFC3339 timestamp offset handling, and a missing `Sync-Token`
  response header.
- Key Vault (#259): lowkey-vault has 1-second-granular timestamps, so **space
  successive writes by 1 second** to keep version ordering deterministic; it
  returns 405 on an empty-version PATCH, so **cover tag/untag in unit tests**
  rather than through the emulator.

Document each quirk next to the code that routes around it.

## Assertions

- Assertions must assert **real behavior**, not merely "no panic". Weak
  error-case subtests (#443) were hardened to assert actual outcomes in #517.

## Local runs

- Keychain-dependent e2e blocks on OS keychain prompts when run locally. Bypass
  with `SUVE_STAGING_KEY`, or leave those paths to CI.
