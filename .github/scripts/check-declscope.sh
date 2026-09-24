#!/bin/sh
#
# Run declscope under every build configuration this host can type-check.
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

# run GOOS TAGS [DIR]
run() {
  echo "declscope: ${3:-.} GOOS=$1 tags=${2:-(none)}"
  (cd "${3:-.}" && GOOS=$1 GOFLAGS="${2:+-tags=$2}" declscope ./...)
}

run "$host" ""
run "$host" "$host_tags"
[ "$host" = windows ] || run windows production,e2e
run "$host" "$host_tags" gui
