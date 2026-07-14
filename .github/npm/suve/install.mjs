// Post-install optimization (network-free): replace this package's JS launcher
// shim (bin/suve) with the native prebuilt binary from the platform-specific
// optionalDependency, so `node_modules/.bin/suve` resolves straight to the
// native binary and runs it directly — no lingering `node` parent process.
//
// This only rewrites files already present locally (installed via
// optionalDependencies); it never downloads anything. Every failure path is
// non-fatal: if anything goes wrong, or scripts are disabled, or we're on
// Windows, the committed JS shim keeps working via child_process (see bin/suve).
import { chmodSync, copyFileSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { fileURLToPath } from "node:url";

async function run() {
  // On Windows the npm-generated .cmd/.ps1 shims invoke `node` on the bin file,
  // so a native .exe cannot be swapped in transparently. Keep the JS shim.
  if (process.platform === "win32") return;

  let resolveBinary;
  try {
    ({ resolveBinary } = await import("./lib/resolve.mjs"));
  } catch {
    return;
  }

  let binary;
  try {
    binary = resolveBinary();
  } catch {
    // Platform package absent (e.g. --no-optional / unsupported host):
    // the JS shim surfaces a helpful error at runtime.
    return;
  }

  const shim = fileURLToPath(new URL("./bin/suve", import.meta.url));
  const tmp = `${shim}.tmp`;
  try {
    rmSync(tmp, { force: true });
    try {
      // Prefer a symlink; the kernel follows it straight to the native binary.
      symlinkSync(binary, tmp);
    } catch {
      // Filesystems without symlink support: copy the binary instead.
      copyFileSync(binary, tmp);
      chmodSync(tmp, 0o755);
    }
    // Atomically replace the JS shim with the native binary handle.
    renameSync(tmp, shim);
  } catch {
    rmSync(tmp, { force: true });
  }
}

run();
