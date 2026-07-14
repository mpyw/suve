import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  cpSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { binaryName, packageForPlatform } from "./lib/resolve.mjs";

const pkgRoot = dirname(fileURLToPath(import.meta.url)); // npm/suve

test("install.mjs no-ops (exit 0, shim untouched) when no platform package is present", () => {
  const dir = mkdtempSync(join(tmpdir(), "suve-noopt-"));
  try {
    const suveDir = join(dir, "suve");
    cpSync(pkgRoot, suveDir, { recursive: true });

    const r = spawnSync(process.execPath, ["install.mjs"], {
      cwd: suveDir,
      encoding: "utf8",
    });
    assert.equal(r.status, 0, r.stderr);

    const shim = join(suveDir, "bin", "suve");
    const st = lstatSync(shim);
    assert.ok(st.isFile() && !st.isSymbolicLink(), "JS shim must be left in place");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("install.mjs relinks bin/suve straight to the native binary when installed", (t) => {
  if (process.platform === "win32") {
    return t.skip("relink is POSIX-only; Windows keeps the JS shim");
  }
  const pkg = packageForPlatform(process.platform, process.arch);
  if (!pkg) {
    return t.skip(`no platform package for ${process.platform}-${process.arch}`);
  }

  const dir = mkdtempSync(join(tmpdir(), "suve-opt-"));
  try {
    // node_modules/suve + node_modules/@mpyw/suve-<os>-<cpu>/bin/<native>
    const nm = join(dir, "node_modules");
    const suveDir = join(nm, "suve");
    cpSync(pkgRoot, suveDir, { recursive: true });

    const platBin = join(nm, pkg, "bin");
    mkdirSync(platBin, { recursive: true });
    const native = join(platBin, binaryName(process.platform));
    writeFileSync(native, "#!/bin/sh\necho FAKE_NATIVE_SUVE\n");
    chmodSync(native, 0o755);

    const r = spawnSync(process.execPath, ["install.mjs"], {
      cwd: suveDir,
      encoding: "utf8",
    });
    assert.equal(r.status, 0, r.stderr);

    // Executing the (now relinked) shim must run the native binary directly.
    const shim = join(suveDir, "bin", "suve");
    const out = execFileSync(shim, [], { encoding: "utf8" });
    assert.match(out, /FAKE_NATIVE_SUVE/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
