#!/usr/bin/env bash
#
# Assemble and publish the suve npm packages from GitHub Release archives.
#
# Layout (see .github/npm/):
#   suve                  main package (bin shim + optionalDependencies)
#   @mpyw/suve-<os>-<cpu> six platform packages, each shipping one prebuilt binary
#
# Reuses the archives already built in releases/ by the release workflow:
#   darwin / windows -> self-contained GUI build (binary: suve / suve.exe)
#   linux            -> CLI/TUI-only static build; the release workflow already
#                       renames suve-cli -> suve inside the archive, so the member
#                       is "suve" even though the archive is named suve-cli_*.
#
# Usage: .github/scripts/npm-publish.sh <version-without-v> [--dry-run]
#   Requires: node, npm; releases/ populated. A real publish authenticates via
#   whatever the environment provides (OIDC trusted publishing in CI); this
#   script does not configure auth itself.
set -euo pipefail

VERSION="${1:?usage: npm-publish.sh <version> [--dry-run]}"
DRY_RUN="${2:-}"

# This script lives at .github/scripts/; the repo root is two levels up.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NPM_DIR="$REPO_ROOT/.github/npm"
RELEASES="$REPO_ROOT/releases"
LICENSE="$REPO_ROOT/LICENSE"

# platform-dir | archive filename | member inside archive | destination binary
PLATFORMS=(
  "darwin-arm64|suve_${VERSION}_darwin_arm64.tar.gz|suve|suve"
  "darwin-x64|suve_${VERSION}_darwin_amd64.tar.gz|suve|suve"
  "linux-arm64|suve-cli_${VERSION}_linux_arm64.tar.gz|suve|suve"
  "linux-x64|suve-cli_${VERSION}_linux_amd64.tar.gz|suve|suve"
  "win32-arm64|suve_${VERSION}_windows_arm64.zip|suve.exe|suve.exe"
  "win32-x64|suve_${VERSION}_windows_amd64.zip|suve.exe|suve.exe"
)

# --- Set the version on every package.json (main + platforms + deps) ----------
echo "==> Setting npm package versions to ${VERSION}"
VERSION="$VERSION" node - "$NPM_DIR" <<'NODE'
const fs = require("node:fs");
const path = require("node:path");
const version = process.env.VERSION;
const npmDir = process.argv[2];
const dirs = fs.readdirSync(npmDir, { withFileTypes: true })
  .filter((d) => d.isDirectory())
  .map((d) => path.join(npmDir, d.name, "package.json"))
  .filter((p) => fs.existsSync(p));
// Guard against a wrong NPM_DIR or a dropped package dir: expect exactly the
// main package plus the six platform packages.
if (dirs.length !== 7) {
  console.error(`Expected 7 npm package.json files, found ${dirs.length} under ${npmDir}`);
  process.exit(1);
}
for (const p of dirs) {
  const pkg = JSON.parse(fs.readFileSync(p, "utf8"));
  pkg.version = version;
  if (pkg.optionalDependencies) {
    for (const dep of Object.keys(pkg.optionalDependencies)) {
      if (dep.startsWith("@mpyw/suve-")) pkg.optionalDependencies[dep] = version;
    }
  }
  fs.writeFileSync(p, JSON.stringify(pkg, null, 2) + "\n");
  console.log(`   ${pkg.name}@${pkg.version}`);
}
NODE

# --- Extract each prebuilt binary into its platform package -------------------
echo "==> Extracting prebuilt binaries from ${RELEASES}"
TMPROOT="$(mktemp -d)"
trap 'rm -rf "$TMPROOT"' EXIT
for entry in "${PLATFORMS[@]}"; do
  IFS="|" read -r dir archive member dest <<<"$entry"
  src="$RELEASES/$archive"
  pkgdir="$NPM_DIR/$dir"
  bindir="$pkgdir/bin"
  [[ -f "$src" ]] || { echo "Error: missing archive $src"; exit 1; }
  mkdir -p "$bindir"

  tmp="$(mktemp -d "$TMPROOT/XXXXXX")"
  case "$archive" in
    *.tar.gz) tar -xzf "$src" -C "$tmp" ;;
    *.zip)    unzip -q -o "$src" -d "$tmp" ;;
    *) echo "Error: unknown archive type $archive"; exit 1 ;;
  esac

  # -print -quit stops at the first match (no pipe, so no SIGPIPE/pipefail risk).
  found="$(find "$tmp" -type f -name "$member" -print -quit)"
  [[ -n "$found" ]] || { echo "Error: '$member' not found in $archive"; exit 1; }
  cp "$found" "$bindir/$dest"
  chmod +x "$bindir/$dest"
  cp "$LICENSE" "$pkgdir/LICENSE"
  echo "   $dir/bin/$dest <- $archive ($member)"
done

cp "$LICENSE" "$NPM_DIR/suve/LICENSE"

# --- Publish: platform packages first, then the main meta-package ------------
if [[ "$DRY_RUN" == "--dry-run" ]]; then
  # No provenance on a dry-run: nothing is published, so attestation is
  # meaningless, and skipping it keeps the rehearsal free of OIDC/Sigstore.
  PUBLISH_ARGS=(--access public --dry-run)
  echo "==> DRY RUN (no packages will be published)"
else
  PUBLISH_ARGS=(--provenance --access public)
fi

# Publish one package directory idempotently, so re-running after a partial
# failure can complete the set. npm hard-errors on a duplicate version, which
# under `set -e` would otherwise abort before the remaining packages — and the
# main package — get published.
publish_one() {
  local pkgdir="$1" name version published out
  name="$(node -p "require('${pkgdir}/package.json').name")"
  version="$(node -p "require('${pkgdir}/package.json').version")"
  local label=".github/npm/$(basename "$pkgdir")"

  # Fast path: skip if this exact version is already on the registry. Only a
  # clean `npm view` (exit 0) reporting the same version is trusted as "already
  # published" — a nonzero exit (E404 = not published, or a transient error) is
  # NOT treated as published; we fall through and let the publish itself decide,
  # tolerating a duplicate-version error below. A dry-run never conflicts, so it
  # always rehearses (no skip).
  if [[ "$DRY_RUN" != "--dry-run" ]]; then
    if published="$(npm view "${name}@${version}" version 2>/dev/null)" \
       && [[ "$published" == "$version" ]]; then
      echo "==> ${name}@${version} already published, skipping"
      return 0
    fi
  fi

  echo "==> npm publish ${name}@${version} (${label})"
  if out="$(cd "$pkgdir" && npm publish "${PUBLISH_ARGS[@]}" 2>&1)"; then
    printf '%s\n' "$out"
    return 0
  fi
  printf '%s\n' "$out"
  # Idempotent skip: a duplicate-version error means someone already published
  # this exact version (e.g. an earlier partial run), so treat it as success
  # rather than wedging the rest of the set.
  if grep -qiE 'cannot publish over|previously published|EPUBLISHCONFLICT' <<<"$out"; then
    echo "==> ${name}@${version} already published (publish conflict), skipping"
    return 0
  fi
  return 1
}

for entry in "${PLATFORMS[@]}"; do
  IFS="|" read -r dir _ _ _ <<<"$entry"
  publish_one "$NPM_DIR/$dir"
done

publish_one "$NPM_DIR/suve"

echo "==> Done."
