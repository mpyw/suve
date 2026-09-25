#!/bin/sh
#
# Run `declscope shrink`, then the declscope analyzer, under every build
# configuration this host can type-check.
#
# shrink goes first. It reports what nothing outside an internal/ package uses
# (`declscope shrink -fix ./...` unexports it), and an unexported declaration
# becomes private to its file, which only the analyzer then checks. Every shrink run finishes before the first analyzer
# run. A use that only one configuration compiles keeps a name exported, so
# shrink runs under each configuration too.
#
# declscope only reads the files the current build configuration selects, so a
# bare `declscope ./...` skips every file behind a build constraint. The tree
# has three kinds:
#
#   - `production || dev` (internal/gui, cmd/suve/gui.go) and its negation
#     (cmd/suve/gui_stub.go)
#   - `e2e` (e2e/, internal/tui/e2e.go)
#   - a GOOS suffix or constraint on top of `production || dev`
#     (internal/gui/run_{darwin,linux,windows}.go, cgo_darwin.go)
#
# declscope's own -tags flag does nothing, so the tags go through GOFLAGS.
#
# | Run                               | Files it adds                        |
# | --------------------------------- | ------------------------------------ |
# | No tags                           | gui_stub.go                          |
# | Host GOOS, production + e2e       | GUI, e2e and the host's OS files     |
# | GOOS=windows, production + e2e    | run_windows.go (pure Go, cross-checks from any host) |
# | gui/, host GOOS, production + e2e | gui/main.go                          |
#
# gui/ is a separate Go module (the Wails entry point), so `./...` from the
# root does not reach it. Its only file is behind `production || dev`, so a run
# with no tags matches no packages. It uses the root .declscope.yaml, the
# nearest one declscope finds.
#
# shrink judges only the internal/ directories nested below the root one, such
# as internal/staging/store/file/internal. The gui/ module can import the root
# internal/, and a run from the root does not load it, so shrink prints "not
# judged" for those packages and exits 0. gui/ has no internal/ of its own, so
# shrink there judges nothing today; it runs so that one added later is covered.
#
# The Wails backends for Linux and macOS use cgo, so run_linux.go needs a Linux
# host with the GTK/WebKit headers and run_darwin.go / cgo_darwin.go need a
# macOS host. CI runs this script on both.
set -eu

cd "$(git rev-parse --show-toplevel)"

# internal/gui embeds frontend/dist, which exists only after a frontend build.
# Type-checking needs a file there, so create a placeholder when it is missing
# and remove it again on exit.
dist=internal/gui/frontend/dist
if [ ! -e "$dist/index.html" ]; then
  created_dir=
  [ -d "$dist" ] || created_dir=1
  mkdir -p "$dist"
  : > "$dist/index.html"
  trap 'rm -f "$dist/index.html"; [ -z "$created_dir" ] || rmdir "$dist"' EXIT
fi

host=$(go env GOHOSTOS)
host_tags=production,e2e
# ubuntu-latest ships webkit2gtk-4.1, which Wails selects with webkit2_41.
[ "$host" = linux ] && host_tags=production,webkit2_41,e2e

# run COMMAND GOOS TAGS [DIR]
# COMMAND is "shrink" for `declscope shrink`, or empty for the analyzer.
run() {
  echo "declscope${1:+ $1}: ${4:-.} GOOS=$2 tags=${3:-(none)}"
  (cd "${4:-.}" && GOOS=$2 GOFLAGS="${3:+-tags=$3}" declscope ${1:+"$1"} ./...)
}

# every COMMAND: run COMMAND under each configuration.
every() {
  run "$1" "$host" ""
  run "$1" "$host" "$host_tags"
  [ "$host" = windows ] || run "$1" windows production,e2e
  run "$1" "$host" "$host_tags" gui
}

every shrink
every ""
