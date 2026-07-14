import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

// Maps a Node.js `process.platform`-`process.arch` pair to the scoped
// platform package that ships the matching prebuilt suve binary. These are the
// only platforms suve publishes to npm; other targets should build from source.
export const PLATFORM_PACKAGES = {
  "darwin-arm64": "@mpyw/suve-darwin-arm64",
  "darwin-x64": "@mpyw/suve-darwin-x64",
  "linux-arm64": "@mpyw/suve-linux-arm64",
  "linux-x64": "@mpyw/suve-linux-x64",
  "win32-arm64": "@mpyw/suve-win32-arm64",
  "win32-x64": "@mpyw/suve-win32-x64",
};

// The prebuilt binary basename inside each platform package.
export function binaryName(platform) {
  return platform === "win32" ? "suve.exe" : "suve";
}

// The scoped platform package name for a platform/arch, or undefined if suve
// does not publish a prebuilt binary for it.
export function packageForPlatform(platform, arch) {
  return PLATFORM_PACKAGES[`${platform}-${arch}`];
}

// Resolves the absolute path to the prebuilt binary for the running platform.
// Throws a user-friendly error when the platform is unsupported or when the
// optional platform dependency was not installed (e.g. --no-optional, or an
// npm version that skipped it on an incompatible host).
export function resolveBinary(platform = process.platform, arch = process.arch) {
  const pkg = packageForPlatform(platform, arch);
  if (!pkg) {
    const supported = Object.keys(PLATFORM_PACKAGES).join(", ");
    throw new Error(
      `suve: no prebuilt binary for ${platform}-${arch}.\n` +
        `Supported platforms: ${supported}.\n` +
        `Install from another source or build from source: https://github.com/mpyw/suve`
    );
  }

  try {
    return require.resolve(`${pkg}/bin/${binaryName(platform)}`);
  } catch {
    throw new Error(
      `suve: the platform package "${pkg}" is not installed.\n` +
        `This usually means npm skipped optionalDependencies (for example with ` +
        `--no-optional, or --ignore-scripts on an incompatible host).\n` +
        `Reinstall to fetch it: npm install suve`
    );
  }
}
