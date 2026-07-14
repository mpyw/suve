import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  PLATFORM_PACKAGES,
  binaryName,
  packageForPlatform,
  resolveBinary,
} from "./resolve.mjs";

test("packageForPlatform maps every supported target", () => {
  const cases = [
    ["darwin", "arm64", "@mpyw/suve-darwin-arm64"],
    ["darwin", "x64", "@mpyw/suve-darwin-x64"],
    ["linux", "arm64", "@mpyw/suve-linux-arm64"],
    ["linux", "x64", "@mpyw/suve-linux-x64"],
    ["win32", "arm64", "@mpyw/suve-win32-arm64"],
    ["win32", "x64", "@mpyw/suve-win32-x64"],
  ];
  for (const [platform, arch, expected] of cases) {
    assert.equal(packageForPlatform(platform, arch), expected);
  }
});

test("packageForPlatform returns undefined for unsupported targets", () => {
  assert.equal(packageForPlatform("linux", "ia32"), undefined);
  assert.equal(packageForPlatform("freebsd", "x64"), undefined);
  assert.equal(packageForPlatform("darwin", "ppc64"), undefined);
});

test("binaryName is suve.exe only on Windows", () => {
  assert.equal(binaryName("win32"), "suve.exe");
  assert.equal(binaryName("darwin"), "suve");
  assert.equal(binaryName("linux"), "suve");
});

test("resolveBinary throws a helpful error for unsupported platforms", () => {
  assert.throws(
    () => resolveBinary("freebsd", "x64"),
    /no prebuilt binary for freebsd-x64/
  );
});

test("resolveBinary throws when the platform package is not installed", () => {
  // The platform packages are not installed in the repo checkout, so a
  // supported target still fails at resolution — with the install hint.
  assert.throws(
    () => resolveBinary("linux", "x64"),
    /platform package "@mpyw\/suve-linux-x64" is not installed/
  );
});

test("PLATFORM_PACKAGES and optionalDependencies stay in sync", () => {
  const pkgPath = fileURLToPath(new URL("../package.json", import.meta.url));
  const pkg = JSON.parse(readFileSync(pkgPath, "utf8"));
  const optional = Object.keys(pkg.optionalDependencies).sort();
  const mapped = Object.values(PLATFORM_PACKAGES).sort();
  assert.deepEqual(optional, mapped);
});
