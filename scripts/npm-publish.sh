#!/usr/bin/env bash
#
# Assemble and publish the suve npm packages from GitHub Release archives.
#
# Layout (see npm/):
#   suve                  main package (bin shim + optionalDependencies)
#   @mpyw/suve-<os>-<cpu> six platform packages, each shipping one prebuilt binary
#
# Reuses the archives already built in releases/ by the release workflow:
#   darwin / windows -> self-contained GUI build (binary: suve / suve.exe)
#   linux            -> CLI/TUI-only static build (binary: suve-cli -> renamed suve)
#
# Usage: scripts/npm-publish.sh <version-without-v> [--dry-run]
#   Requires: node, npm; releases/ populated; NODE_AUTH_TOKEN for a real publish.
set -euo pipefail

VERSION="${1:?usage: npm-publish.sh <version> [--dry-run]}"
DRY_RUN="${2:-}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NPM_DIR="$REPO_ROOT/npm"
RELEASES="$REPO_ROOT/releases"
LICENSE="$REPO_ROOT/LICENSE"

# platform-dir | archive filename | member inside archive | destination binary
PLATFORMS=(
  "darwin-arm64|suve_${VERSION}_darwin_arm64.tar.gz|suve|suve"
  "darwin-x64|suve_${VERSION}_darwin_amd64.tar.gz|suve|suve"
  "linux-arm64|suve-cli_${VERSION}_linux_arm64.tar.gz|suve-cli|suve"
  "linux-x64|suve-cli_${VERSION}_linux_amd64.tar.gz|suve-cli|suve"
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
for entry in "${PLATFORMS[@]}"; do
  IFS="|" read -r dir archive member dest <<<"$entry"
  src="$RELEASES/$archive"
  pkgdir="$NPM_DIR/$dir"
  bindir="$pkgdir/bin"
  [[ -f "$src" ]] || { echo "Error: missing archive $src"; exit 1; }
  mkdir -p "$bindir"

  tmp="$(mktemp -d)"
  case "$archive" in
    *.tar.gz) tar -xzf "$src" -C "$tmp" ;;
    *.zip)    unzip -q -o "$src" -d "$tmp" ;;
    *) echo "Error: unknown archive type $archive"; exit 1 ;;
  esac

  found="$(find "$tmp" -type f -name "$member" | head -1)"
  [[ -n "$found" ]] || { echo "Error: '$member' not found in $archive"; exit 1; }
  cp "$found" "$bindir/$dest"
  chmod +x "$bindir/$dest"
  cp "$LICENSE" "$pkgdir/LICENSE"
  rm -rf "$tmp"
  echo "   $dir/bin/$dest <- $archive ($member)"
done

cp "$LICENSE" "$NPM_DIR/suve/LICENSE"

# --- Publish: platform packages first, then the main meta-package ------------
PUBLISH_ARGS=(--provenance --access public)
if [[ "$DRY_RUN" == "--dry-run" ]]; then
  PUBLISH_ARGS+=(--dry-run)
  echo "==> DRY RUN (no packages will be published)"
fi

for entry in "${PLATFORMS[@]}"; do
  IFS="|" read -r dir _ _ _ <<<"$entry"
  echo "==> npm publish npm/$dir"
  (cd "$NPM_DIR/$dir" && npm publish "${PUBLISH_ARGS[@]}")
done

echo "==> npm publish npm/suve"
(cd "$NPM_DIR/suve" && npm publish "${PUBLISH_ARGS[@]}")

echo "==> Done."
