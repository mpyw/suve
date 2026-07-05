#!/usr/bin/env bash
#
# Enforce the Google Cloud naming convention: the bare acronym "GCP" must never
# appear in the source. Use one of these instead:
#   - "Google Cloud"  prose / display names
#   - "gcloud"        CLI command group, package names, identifiers
#   - "GoogleCloud"   Go provider identifier (provider.ProviderGoogleCloud)
#
# The only allowed occurrence is the third-party emulator image name
# "gcp-secret-manager-emulator", which we do not control.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# Word-boundary, case-insensitive match of "gcp" across tracked text files
# (-I skips binary blobs). This script and the dependency manifests are excluded;
# the allowlisted third-party image name is dropped afterwards.
matches=$(
  git grep -nIwi 'gcp' -- \
    ':(exclude).github/scripts/check-naming.sh' \
    ':(exclude)go.sum' \
    ':(exclude)go.mod' \
    ':(exclude)**/package-lock.json' \
    ':(exclude)**/*.lock' \
    | { grep -vi 'gcp-secret-manager-emulator' || true; }
)

if [[ -n "${matches}" ]]; then
  echo "Forbidden 'GCP' acronym found. Use 'Google Cloud', 'gcloud', or 'GoogleCloud':" >&2
  echo "${matches}" >&2
  exit 1
fi

echo "Naming convention OK: no stray 'GCP' acronym."
