#!/bin/sh
# suve installer for macOS and Linux — https://github.com/mpyw/suve
#
# By default it installs a bare binary into a bin directory. On Linux you can
# instead install a native package (registered with apt/dnf for clean upgrade/
# removal, with GUI dependencies resolved) via --deb / --rpm.
#
# Usage (curl or wget — whichever your host has):
#   curl -fsSL  https://raw.githubusercontent.com/mpyw/suve/main/scripts/install.sh | sh
#   wget -qO-   https://raw.githubusercontent.com/mpyw/suve/main/scripts/install.sh | sh
#
# Pass flags through the pipe with `sh -s --`:
#   curl -fsSL .../install.sh | sh -s -- --deb --gui
#
# Environment variables:
#   VERSION           Version to install, e.g. "0.5.3" or "v0.5.3". Default: latest release.
#   SUVE_INSTALL_DIR  Bare-binary target dir. Default: /usr/local/bin -> ~/.local/bin.
#   SUVE_WEBKIT       GUI WebKit2GTK ABI: "4.1" or "4.0". Default: auto-detected.
#
# Flags:
#   Packaging (Linux only; macOS is always a bare, self-contained binary):
#     (default)   Bare binary into a bin directory.
#     --deb       Install the .deb via apt (Debian/Ubuntu).
#     --rpm       Install the .rpm via dnf/yum (Fedora/RHEL).
#   Build variant (Linux only):
#     (default)   Auto: GUI when WebKit2GTK is already present, else CLI/TUI-only.
#     --cli       Force the dependency-free CLI/TUI-only build.
#     --gui       Force the GUI build (GTK3 + WebKit2GTK).
#   -h, --help    Show this help.
#
# Security: HTTPS-only downloads from GitHub Releases, each verified by SHA-256
# against the release's checksums.txt before install. The whole script is
# wrapped in main() and invoked on the last line, so a truncated download can
# never execute a partial script.
#
# Functions are grouped by concern via a prefix: log_*, cli_*, http_*,
# version_*, checksum_*, platform_*, webkit_*, bin_* (bare binary), pkg_*
# (native package).

set -eu

REPO="mpyw/suve"
BASE_URL="https://github.com/${REPO}/releases/download"

# --- log ---------------------------------------------------------------------
log_info() { printf 'suve-install: %s\n' "$*" >&2; }
log_error() {
	printf 'suve-install: error: %s\n' "$*" >&2
	exit 1
}

# --- cli ---------------------------------------------------------------------
cli_need_cmd() {
	command -v "$1" >/dev/null 2>&1 || log_error "required command not found: $1"
}

cli_usage() {
	cat >&2 <<'EOF'
suve installer for macOS and Linux — https://github.com/mpyw/suve

Usage (curl or wget — whichever your host has):
  curl -fsSL  https://raw.githubusercontent.com/mpyw/suve/main/scripts/install.sh | sh
  wget -qO-   https://raw.githubusercontent.com/mpyw/suve/main/scripts/install.sh | sh

Pass flags through the pipe with `sh -s --`:
  curl -fsSL .../install.sh | sh -s -- --deb --gui

Environment variables:
  VERSION           Version to install, e.g. "0.5.3" or "v0.5.3". Default: latest release.
  SUVE_INSTALL_DIR  Bare-binary target dir. Default: /usr/local/bin -> ~/.local/bin.
  SUVE_WEBKIT       GUI WebKit2GTK ABI: "4.1" or "4.0". Default: auto-detected.

Flags:
  Packaging (Linux only; macOS is always a bare, self-contained binary):
    (default)   Bare binary into a bin directory.
    --deb       Install the .deb via apt (Debian/Ubuntu).
    --rpm       Install the .rpm via dnf/yum (Fedora/RHEL).
  Build variant (Linux only):
    (default)   Auto: GUI when WebKit2GTK is already present, else CLI/TUI-only.
    --cli       Force the dependency-free CLI/TUI-only build.
    --gui       Force the GUI build (GTK3 + WebKit2GTK).
  -h, --help    Show this help.
EOF
}

# --- http (downloader: curl or wget) -----------------------------------------
# Minimal Ubuntu/Debian images ship neither; Fedora ships curl; desktops both.
HTTP_DL=""
HTTP_WGET_NOCONFIG="" # "--no-config" when GNU wget supports it (>= 1.20)
http_init() {
	if command -v curl >/dev/null 2>&1; then
		HTTP_DL=curl
		return
	fi
	if command -v wget >/dev/null 2>&1; then
		# The wget backend needs GNU options (--https-only, --spider,
		# --max-redirect); BusyBox wget has none of them, so probe before
		# committing to it rather than failing mid-download.
		_help="$(wget --help 2>&1 || true)"
		case "$_help" in
			*--https-only*)
				HTTP_DL=wget
				# --no-config (wget >= 1.20) ignores /etc/wgetrc + ~/.wgetrc, the
				# wget analogue of curl's -q, so a stray check_certificate=off or
				# proxy can't weaken TLS.
				case "$_help" in
					*--no-config*) HTTP_WGET_NOCONFIG="--no-config" ;;
				esac
				return
				;;
		esac
		log_error "found wget but it lacks --https-only (BusyBox?); install curl or GNU wget"
	fi
	log_error "need curl or wget to download files"
}

# wget with --no-config prepended when supported (keeps the flag properly quoted,
# unlike an unquoted variable expansion).
http_wget() {
	if [ -n "$HTTP_WGET_NOCONFIG" ]; then
		wget "$HTTP_WGET_NOCONFIG" "$@"
	else
		wget "$@"
	fi
}

# Download $1 to file $2 over HTTPS only. For curl, `-q` ignores ~/.curlrc so a
# stray --insecure/proxy can't weaken us, and --proto/--proto-redir pin every
# hop to HTTPS; for wget, --no-config + --https-only do the same.
http_get() {
	case "$HTTP_DL" in
		curl) curl -q --proto '=https' --proto-redir '=https' -fsSL "$1" -o "$2" ;;
		wget) http_wget -q --https-only -O "$2" "$1" ;;
	esac || log_error "download failed: $1"
}

# Print the URL that .../releases/latest redirects to (avoids the rate-limited API).
http_latest_redirect_url() {
	_u="https://github.com/${REPO}/releases/latest"
	case "$HTTP_DL" in
		curl)
			curl -q --proto '=https' --proto-redir '=https' -fsSLI -o /dev/null \
				-w '%{url_effective}' "$_u"
			;;
		wget)
			http_wget -S --spider --max-redirect=0 --https-only "$_u" 2>&1 |
				awk 'tolower($1) == "location:" { print $2; exit }' | tr -d '\r'
			;;
	esac
}

# --- version -----------------------------------------------------------------
version_resolve() {
	if [ -n "${VERSION:-}" ]; then
		printf '%s' "${VERSION#v}"
		return
	fi
	_final="$(http_latest_redirect_url)"
	_tag="${_final##*/tag/}"
	{ [ -n "$_tag" ] && [ "$_tag" != "$_final" ]; } ||
		log_error "could not determine the latest version"
	printf '%s' "${_tag#v}"
}

# --- checksum ----------------------------------------------------------------
checksum_sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		log_error "no SHA-256 tool found (need sha256sum or shasum)"
	fi
}

checksum_verify() {
	_dir="$1"
	_asset="$2"
	_tag="$3"
	_sums="${_dir}/checksums.txt"
	http_get "${BASE_URL}/${_tag}/checksums.txt" "$_sums"
	_expected="$(awk -v f="$_asset" '$2 == f {print $1; exit}' "$_sums")"
	[ -n "$_expected" ] || log_error "checksum for ${_asset} not found in checksums.txt"
	_actual="$(checksum_sha256 "${_dir}/${_asset}")"
	[ "$_expected" = "$_actual" ] ||
		log_error "checksum mismatch for ${_asset}: expected ${_expected}, got ${_actual}"
	log_info "checksum OK"
}

# --- platform ----------------------------------------------------------------
platform_os() {
	case "$(uname -s)" in
		Linux) echo linux ;;
		Darwin) echo darwin ;;
		*) log_error "unsupported OS: $(uname -s) (Linux/macOS only; on Windows use Scoop or npm)" ;;
	esac
}

# Canonical arch used by the binary and .deb assets.
platform_arch() {
	case "$(uname -m)" in
		x86_64 | amd64) echo amd64 ;;
		aarch64 | arm64) echo arm64 ;;
		*) log_error "unsupported architecture: $(uname -m)" ;;
	esac
}

# .rpm uses different arch tokens.
platform_rpm_arch() {
	case "$1" in
		amd64) echo x86_64 ;;
		arm64) echo aarch64 ;;
	esac
}

# --- webkit ------------------------------------------------------------------
webkit_ldconfig() {
	if command -v ldconfig >/dev/null 2>&1; then
		ldconfig -p 2>/dev/null
	elif [ -x /sbin/ldconfig ]; then
		/sbin/ldconfig -p 2>/dev/null
	fi
}

# True if the WebKit2GTK $1 (e.g. "4.1") runtime is already installed — i.e. the
# GUI build is viable right now without pulling a large dependency tree.
webkit_installed() {
	_v="$1"
	webkit_ldconfig | grep -q "libwebkit2gtk-${_v}" && return 0
	# ldconfig-less fallback. Scan only the host's own multiarch dir, never the
	# blanket /usr/lib/* — on a multiarch box that would match a foreign-arch
	# library the GUI build could not actually load.
	_triplet=""
	case "$(uname -m)" in
		x86_64 | amd64) _triplet=x86_64-linux-gnu ;;
		aarch64 | arm64) _triplet=aarch64-linux-gnu ;;
	esac
	for _f in /usr/lib/libwebkit2gtk-"${_v}".so* \
		/usr/lib64/libwebkit2gtk-"${_v}".so* \
		/usr/lib/"${_triplet:-_none_}"/libwebkit2gtk-"${_v}".so*; do
		[ -e "$_f" ] && return 0
	done
	return 1
}

# For a GUI install, choose the ABI: explicit SUVE_WEBKIT, else an already-
# installed runtime, else (for --deb/--rpm) whatever the package manager can
# install, preferring the newer 4.1. $1 is the format (binary|deb|rpm).
webkit_pick() {
	if [ -n "${SUVE_WEBKIT:-}" ]; then
		echo "$SUVE_WEBKIT"
		return
	fi
	webkit_installed 4.1 && {
		echo 4.1
		return
	}
	webkit_installed 4.0 && {
		echo 4.0
		return
	}
	case "$1" in
		deb)
			if command -v apt-cache >/dev/null 2>&1; then
				if apt-cache show libwebkit2gtk-4.1-0 >/dev/null 2>&1; then
					echo 4.1
					return
				fi
				if apt-cache show libwebkit2gtk-4.0-37 >/dev/null 2>&1; then
					echo 4.0
					return
				fi
			fi
			;;
		rpm)
			_pm=""
			command -v dnf >/dev/null 2>&1 && _pm=dnf
			[ -z "$_pm" ] && command -v yum >/dev/null 2>&1 && _pm=yum
			if [ -n "$_pm" ]; then
				if "$_pm" -q list webkit2gtk4.1 >/dev/null 2>&1; then
					echo 4.1
					return
				fi
				if "$_pm" -q list webkit2gtk4.0 >/dev/null 2>&1; then
					echo 4.0
					return
				fi
			fi
			;;
	esac
	echo 4.1
}

# --- bin (bare-binary install) -----------------------------------------------
bin_dir_creatable() {
	_p="$1"
	while [ ! -e "$_p" ]; do
		_p="$(dirname "$_p")"
	done
	[ -w "$_p" ]
}

# `install` sets the destination mode regardless of the source, so under sudo
# the result is root-owned and NOT writable by the invoking user (unlike `mv`,
# which would preserve the user's ownership on a same-filesystem rename).
bin_emplace() {
	if command -v install >/dev/null 2>&1; then
		install -m 0755 "$1" "$2"
	else
		cp "$1" "$2" && chmod 0755 "$2"
	fi
}

bin_emplace_sudo() {
	if command -v install >/dev/null 2>&1; then
		sudo install -m 0755 "$1" "$2"
	else
		sudo cp "$1" "$2" && sudo chmod 0755 "$2"
	fi
}

bin_place() {
	_bin="$1"
	_dir="$2"
	if bin_dir_creatable "$_dir"; then
		mkdir -p "$_dir" && bin_emplace "$_bin" "${_dir}/suve"
	elif [ "$(id -u)" = "0" ]; then
		mkdir -p "$_dir" && bin_emplace "$_bin" "${_dir}/suve"
	elif command -v sudo >/dev/null 2>&1; then
		log_info "installing to ${_dir} (requires sudo)"
		sudo mkdir -p "$_dir" && bin_emplace_sudo "$_bin" "${_dir}/suve"
	else
		return 1
	fi
}

bin_install() {
	_bin="$1"
	# -f follows symlinks, so reject a symlinked member explicitly.
	{ [ -f "$_bin" ] && [ ! -L "$_bin" ]; } ||
		log_error "expected a regular file 'suve' in the archive"

	if [ -n "${SUVE_INSTALL_DIR:-}" ]; then
		_dir="$SUVE_INSTALL_DIR"
		bin_place "$_bin" "$_dir" ||
			log_error "cannot write to ${_dir}; set SUVE_INSTALL_DIR to a writable path or re-run with sudo"
	else
		_dir="/usr/local/bin"
		if ! bin_place "$_bin" "$_dir"; then
			[ -n "${HOME:-}" ] || log_error "/usr/local/bin not writable and \$HOME is unset; set SUVE_INSTALL_DIR"
			_dir="${HOME}/.local/bin"
			log_info "/usr/local/bin not writable and no sudo; installing to ${_dir}"
			mkdir -p "$_dir" && bin_emplace "$_bin" "${_dir}/suve"
		fi
	fi
	_install_path="${_dir}/suve"
	log_info "installed: ${_install_path}"
	case ":${PATH}:" in
		*":${_dir}:"*) ;;
		*) log_info "note: ${_dir} is not in your PATH; add it to run 'suve' directly" ;;
	esac
	"$_install_path" --version 2>/dev/null || true
}

bin_run() {
	cli_need_cmd tar
	if [ "$os" = "darwin" ]; then
		asset="suve_${ver}_darwin_${arch}.tar.gz"
	elif [ "$variant" = "cli" ]; then
		asset="suve-cli_${ver}_linux_${arch}.tar.gz"
	else
		wk="$(webkit_pick binary)"
		if [ "$wk" = "4.1" ]; then
			asset="suve_${ver}_linux_${arch}_webkit2_41.tar.gz"
		else
			asset="suve_${ver}_linux_${arch}.tar.gz"
		fi
		log_info "GUI build, WebKit2GTK ${wk} (override with SUVE_WEBKIT=4.0|4.1)"
	fi

	log_info "downloading ${asset} (${tag})"
	http_get "${BASE_URL}/${tag}/${asset}" "${tmpdir}/${asset}"
	checksum_verify "$tmpdir" "$asset" "$tag"
	# Extract only the binary (members are ./suve, ./README.md, ./LICENSE).
	tar -xzf "${tmpdir}/${asset}" -C "$tmpdir" ./suve
	bin_install "${tmpdir}/suve"
}

# --- pkg (native package install) --------------------------------------------
pkg_run_as_root() {
	if [ "$(id -u)" = "0" ]; then
		"$@"
	elif command -v sudo >/dev/null 2>&1; then
		sudo "$@"
	else
		log_error "root privileges required (run as root or install sudo)"
	fi
}

# Confirm the package manager actually left a working suve — apt's `-f install`
# recovery can resolve a broken state by *removing* the just-unpacked package
# and still exit 0, so "installed" must not be announced on exit code alone.
pkg_verify() {
	command -v suve >/dev/null 2>&1 ||
		log_error "package install did not leave a working 'suve' on PATH"
	log_info "installed: $(command -v suve)"
	suve --version 2>/dev/null || true
}

pkg_deb_run() {
	if [ "$variant" = "cli" ]; then
		asset="suve-cli_${ver}-1_${arch}.deb"
	else
		wk="$(webkit_pick deb)"
		case "$wk" in
			4.1) asset="suve_webkit2_41_${ver}-1_${arch}.deb" ;;
			4.0) asset="suve_${ver}-1_${arch}.deb" ;;
			*) log_error "invalid SUVE_WEBKIT: ${wk} (expected 4.1 or 4.0)" ;;
		esac
		log_info "GUI build, WebKit2GTK ${wk} (override with SUVE_WEBKIT=4.0|4.1)"
	fi

	log_info "downloading ${asset} (${tag})"
	http_get "${BASE_URL}/${tag}/${asset}" "${tmpdir}/${asset}"
	checksum_verify "$tmpdir" "$asset" "$tag"

	log_info "installing ${asset}"
	_f="${tmpdir}/${asset}"
	if command -v apt-get >/dev/null 2>&1; then
		# apt-get resolves the package's declared GTK3/WebKit dependencies.
		if ! pkg_run_as_root apt-get install -y "$_f"; then
			log_info "retrying via dpkg + apt-get -f install"
			pkg_run_as_root dpkg -i "$_f" || true
			pkg_run_as_root apt-get -f install -y
		fi
	elif command -v dpkg >/dev/null 2>&1; then
		log_info "apt-get not found; using dpkg -i (dependencies are NOT resolved)"
		pkg_run_as_root dpkg -i "$_f"
	else
		log_error "neither apt-get nor dpkg found; --deb is for Debian/Ubuntu systems"
	fi
	pkg_verify
}

pkg_rpm_run() {
	_rarch="$(platform_rpm_arch "$arch")"
	if [ "$variant" = "cli" ]; then
		asset="suve-cli-${ver}-1.${_rarch}.rpm"
	else
		wk="$(webkit_pick rpm)"
		case "$wk" in
			4.1) asset="suve_webkit2_41-${ver}-1.${_rarch}.rpm" ;;
			4.0) asset="suve-${ver}-1.${_rarch}.rpm" ;;
			*) log_error "invalid SUVE_WEBKIT: ${wk} (expected 4.1 or 4.0)" ;;
		esac
		log_info "GUI build, WebKit2GTK ${wk} (override with SUVE_WEBKIT=4.0|4.1)"
	fi

	log_info "downloading ${asset} (${tag})"
	http_get "${BASE_URL}/${tag}/${asset}" "${tmpdir}/${asset}"
	checksum_verify "$tmpdir" "$asset" "$tag"

	log_info "installing ${asset}"
	_f="${tmpdir}/${asset}"
	if command -v dnf >/dev/null 2>&1; then
		pkg_run_as_root dnf install -y "$_f"
	elif command -v yum >/dev/null 2>&1; then
		pkg_run_as_root yum install -y "$_f"
	elif command -v rpm >/dev/null 2>&1; then
		log_info "dnf/yum not found; using rpm -i (dependencies are NOT resolved)"
		pkg_run_as_root rpm -i "$_f"
	else
		log_error "no dnf/yum/rpm found; --rpm is for RPM-based systems"
	fi
	pkg_verify
}

# --- main --------------------------------------------------------------------
main() {
	format="binary"
	variant="auto"
	for arg in "$@"; do
		case "$arg" in
			--deb | --rpm)
				[ "$format" = "binary" ] || log_error "--deb and --rpm are mutually exclusive"
				format="${arg#--}"
				;;
			--cli)
				[ "$variant" = "auto" ] || log_error "--cli and --gui are mutually exclusive"
				variant="cli"
				;;
			--gui | --full)
				[ "$variant" = "auto" ] || log_error "--cli and --gui are mutually exclusive"
				variant="gui"
				;;
			-h | --help)
				cli_usage
				exit 0
				;;
			*) log_error "unknown option: ${arg} (see --help)" ;;
		esac
	done

	# Validate SUVE_WEBKIT once, centrally, so every path (binary/deb/rpm) rejects
	# a bad value instead of silently defaulting to 4.0.
	case "${SUVE_WEBKIT:-}" in
		"" | 4.1 | 4.0) ;;
		*) log_error "invalid SUVE_WEBKIT: ${SUVE_WEBKIT} (expected 4.1 or 4.0)" ;;
	esac

	http_init
	cli_need_cmd uname

	os="$(platform_os)"
	arch="$(platform_arch)"

	if [ "$format" != "binary" ] && [ "$os" != "linux" ]; then
		log_error "--deb/--rpm are Linux-only; omit them for a bare, self-contained macOS binary"
	fi

	ver="$(version_resolve)"
	tag="v${ver}"

	# Resolve the build variant. macOS has only the self-contained GUI binary.
	if [ "$os" = "darwin" ]; then
		[ "$variant" != "auto" ] && log_info "macOS ships only the self-contained build; ignoring --${variant}"
		variant="darwin"
	elif [ "$variant" = "auto" ]; then
		if webkit_installed 4.1 || webkit_installed 4.0; then
			variant="gui"
			log_info "detected WebKit2GTK; selecting the GUI build (pass --cli to override)"
		else
			variant="cli"
			log_info "no WebKit2GTK detected; selecting the CLI/TUI-only build (pass --gui to override)"
		fi
	fi

	tmpdir="$(mktemp -d 2>/dev/null || mktemp -d -t suve.XXXXXX)"
	trap 'rm -rf "$tmpdir"' EXIT
	trap 'rm -rf "$tmpdir"; exit 130' INT TERM HUP

	case "$format" in
		binary) bin_run ;;
		deb) pkg_deb_run ;;
		rpm) pkg_rpm_run ;;
	esac
}

main "$@"
