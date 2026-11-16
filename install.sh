#!/bin/sh
# Install fleet from GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/huyng/fleet/main/install.sh | sh
#
# The script picks the tarball for this machine, checks it against the release
# checksums, and puts the binary on PATH. It is plain POSIX sh so it works on a
# minimal Linux or a stock macOS.
#
# Environment:
#   FLEET_VERSION    version to install, such as 0.1.0 (default: the latest release)
#   FLEET_BIN_DIR    where to put the binary (default: /usr/local/bin, or ~/.local/bin
#                    when that is not writable)
#   FLEET_BASE_URL   where to download from (default: the GitHub release download URL;
#                    useful for testing against a local directory)
#   FLEET_NO_VERIFY  set to 1 to skip the checksum check

set -eu

REPO="huyng/fleet"
DEFAULT_BASE="https://github.com/${REPO}/releases/download"

# Print a message to stderr and stop.
die() {
    printf 'install: %s\n' "$*" >&2
    exit 1
}

# Print a note, unless quiet.
say() {
    printf '%s\n' "$*"
}

# Work out the operating system name used in the release file names.
detect_os() {
    os=$(uname -s)
    case "$os" in
        Linux) echo "linux" ;;
        Darwin) echo "darwin" ;;
        *) die "unsupported system $os: fleet ships for Linux and macOS only" ;;
    esac
}

# Work out the architecture name used in the release file names.
detect_arch() {
    arch=$(uname -m)
    case "$arch" in
        x86_64 | amd64) echo "amd64" ;;
        arm64 | aarch64) echo "arm64" ;;
        *) die "unsupported architecture $arch: fleet ships for amd64 and arm64 only" ;;
    esac
}

# Find a downloader. curl is on macOS and most Linux; wget is the fallback.
downloader() {
    if command -v curl >/dev/null 2>&1; then
        echo curl
    elif command -v wget >/dev/null 2>&1; then
        echo wget
    else
        die "neither curl nor wget is installed"
    fi
}

# Fetch a URL to a file.
#
# Args:
#   url: where to fetch from.
#   out: file to write.
fetch() {
    case "$(downloader)" in
        curl) curl -fsSL --retry 3 -o "$2" "$1" ;;
        wget) wget -q -O "$2" "$1" ;;
    esac
}

# Read a URL to standard output.
fetch_stdout() {
    case "$(downloader)" in
        curl) curl -fsSL --retry 3 "$1" ;;
        wget) wget -q -O - "$1" ;;
    esac
}

# Ask the GitHub API which version is current.
#
# Returns:
#   The version without its leading v, such as 0.1.0.
latest_version() {
    api="https://api.github.com/repos/${REPO}/releases/latest"

    # The JSON is a single line of tag_name near the start, so sed is enough
    # and saves depending on jq.
    tag=$(fetch_stdout "$api" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
    if [ -z "$tag" ]; then
        die "could not read the latest version from $api; set FLEET_VERSION to install a specific one"
    fi
    echo "${tag#v}"
}

# Verify a file against the release checksums.
#
# Args:
#   file: the downloaded tarball.
#   name: its base name, as listed in the checksum file.
#   sums: the downloaded checksum file.
verify() {
    if [ "${FLEET_NO_VERIFY:-0}" = "1" ]; then
        say "skipping checksum check"
        return 0
    fi

    expected=$(sed -n "s/^\([0-9a-f]\{64\}\)[[:space:]]*[*]\{0,1\}${2}\$/\1/p" "$3" | head -n 1)
    if [ -z "$expected" ]; then
        die "no checksum for $2 in checksums.txt; set FLEET_NO_VERIFY=1 to install anyway"
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$1" | cut -d' ' -f1)
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$1" | cut -d' ' -f1)
    else
        die "no sha256sum or shasum to check the download with; set FLEET_NO_VERIFY=1 to skip"
    fi

    if [ "$expected" != "$actual" ]; then
        die "checksum mismatch for $2: expected $expected, got $actual"
    fi
    say "checksum ok"
}

# Pick where the binary goes, falling back to a user directory when the system
# one needs root.
target_dir() {
    if [ -n "${FLEET_BIN_DIR:-}" ]; then
        echo "$FLEET_BIN_DIR"
        return
    fi

    if [ -w /usr/local/bin ] 2>/dev/null; then
        echo /usr/local/bin
    else
        echo "${HOME}/.local/bin"
    fi
}

main() {
    os=$(detect_os)
    arch=$(detect_arch)

    version="${FLEET_VERSION:-}"
    if [ -z "$version" ]; then
        say "looking up the latest release"
        version=$(latest_version)
    fi
    version="${version#v}"

    base="${FLEET_BASE_URL:-${DEFAULT_BASE}/v${version}}"
    name="fleet_${version}_${os}_${arch}.tar.gz"

    tmp=$(mktemp -d)
    # shellcheck disable=SC2064
    trap "rm -rf '$tmp'" EXIT INT TERM

    say "downloading $name"
    fetch "${base}/${name}" "${tmp}/${name}" ||
        die "could not download ${base}/${name}; check that version ${version} has a release for ${os}/${arch}"

    say "downloading checksums"
    fetch "${base}/checksums.txt" "${tmp}/checksums.txt" ||
        die "could not download checksums from ${base}"

    verify "${tmp}/${name}" "$name" "${tmp}/checksums.txt"

    say "unpacking"
    tar -xzf "${tmp}/${name}" -C "$tmp"

    binary="${tmp}/fleet_${version}_${os}_${arch}/fleet"
    if [ ! -f "$binary" ]; then
        die "the tarball did not contain the fleet binary"
    fi

    dir=$(target_dir)
    mkdir -p "$dir"
    # Install by copy rather than move: the temporary directory and the target
    # can be on different filesystems.
    cp "$binary" "${dir}/fleet"
    chmod 755 "${dir}/fleet"

    say "installed ${dir}/fleet"
    "${dir}/fleet" version

    case ":${PATH}:" in
        *":${dir}:"*) ;;
        *) say "note: ${dir} is not on PATH, add it to use fleet from anywhere" ;;
    esac
}

main "$@"
