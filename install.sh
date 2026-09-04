#!/usr/bin/env bash
#
# homerouter -- bootstrap installer.
#
# Downloads a release tarball, verifies its SHA-256 checksum, unpacks it to
# /opt/homerouter and (optionally) runs setup.sh.
#
#   curl -fsSL https://raw.githubusercontent.com/mklarsen/homerouter/main/install.sh | sudo bash
#
# Options (after `bash -s --`):
#   --version <tag>   install a specific release instead of the latest
#   --dir <path>      install directory (default: /opt/homerouter)
#   --run             run setup.sh afterwards (requires a filled-in .env)
#   --no-packages     do not install the apt packages the router needs
#   --no-verify       skip checksum verification (not recommended)
#
set -Eeuo pipefail

REPO="mklarsen/homerouter"
INSTALL_DIR="/opt/homerouter"
VERSION="latest"
RUN_SETUP=0
VERIFY=1
PACKAGES_WANTED=1

say()  { printf '==> %s\n' "$*"; }
warn() { printf 'warn: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) VERSION="${2:?--version needs a tag}"; shift 2 ;;
        --dir)     INSTALL_DIR="${2:?--dir needs a path}"; shift 2 ;;
        --run)     RUN_SETUP=1; shift ;;
        --no-packages) PACKAGES_WANTED=0; shift ;;
        --no-verify) VERIFY=0; shift ;;
        -h|--help) sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *)         die "unknown argument: $1" ;;
    esac
done

[[ $EUID -eq 0 ]] || die "must run as root (pipe into 'sudo bash')"

for cmd in curl tar sha256sum install; do
    command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
done

API="https://api.github.com/repos/$REPO/releases"
if [[ $VERSION == "latest" ]]; then
    say "Looking up the latest release of $REPO"
    VERSION="$(curl -fsSL "$API/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
    [[ -n $VERSION ]] || die "could not determine the latest release tag"
fi
say "Version: $VERSION"

BASE="https://github.com/$REPO/releases/download/$VERSION"
TARBALL="homerouter-$VERSION.tar.gz"

TMP_DIR="$(mktemp -d /tmp/homerouter-install.XXXXXX)"
trap 'rm -rf -- "$TMP_DIR"' EXIT
chmod 700 "$TMP_DIR"

say "Downloading $TARBALL"
curl -fsSL --proto '=https' --tlsv1.2 -o "$TMP_DIR/$TARBALL" "$BASE/$TARBALL" \
    || die "download failed -- does release $VERSION exist?"

if [[ $VERIFY -eq 1 ]]; then
    say "Verifying checksum"
    curl -fsSL --proto '=https' --tlsv1.2 -o "$TMP_DIR/SHA256SUMS" "$BASE/SHA256SUMS" \
        || die "could not download SHA256SUMS (use --no-verify to skip)"
    ( cd "$TMP_DIR" && sha256sum --check --ignore-missing --status SHA256SUMS ) \
        || die "checksum mismatch -- refusing to install"
    say "Checksum OK"
else
    warn "checksum verification disabled"
fi

say "Unpacking to $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"
SRC_DIR="$TMP_DIR/homerouter-$VERSION"
[[ -d $SRC_DIR ]] || die "unexpected tarball layout"

# .env is site-specific state and must survive upgrades.
cp -a "$SRC_DIR/." "$INSTALL_DIR/"
chown -R root:root "$INSTALL_DIR"
chmod 755 "$INSTALL_DIR"/setup.sh "$INSTALL_DIR"/scripts/*.sh

if [[ $PACKAGES_WANTED -eq 1 ]]; then
    # Done here, not in setup.sh: this script only ever runs with a working
    # internet connection, while setup.sh may run after the uplink is gone.
    say "Installing runtime packages"
    # shellcheck source=scripts/lib.sh
    source "$INSTALL_DIR/scripts/lib.sh"
    install_packages
else
    warn "skipping package installation (--no-packages)"
fi

if [[ ! -f "$INSTALL_DIR/.env" ]]; then
    install -m 600 "$INSTALL_DIR/.env.example" "$INSTALL_DIR/.env"
    say "Created $INSTALL_DIR/.env from the example -- fill it in before deploying"
    NEEDS_CONFIG=1
else
    chmod 600 "$INSTALL_DIR/.env"
    say "Kept the existing $INSTALL_DIR/.env"
    NEEDS_CONFIG=0
fi

if [[ $RUN_SETUP -eq 1 ]]; then
    if [[ $NEEDS_CONFIG -eq 1 ]]; then
        die "--run given but .env was just created; edit it first, then run: $INSTALL_DIR/setup.sh"
    fi
    say "Running setup.sh"
    exec "$INSTALL_DIR/setup.sh"
fi

cat <<EOF

homerouter $VERSION installed in $INSTALL_DIR

Next:
  sudo \$EDITOR $INSTALL_DIR/.env      # ISP addresses, LAN subnet, Wi-Fi
  sudo $INSTALL_DIR/setup.sh --dry-run
  sudo $INSTALL_DIR/setup.sh

The packages are installed, so setup.sh no longer needs internet access.
EOF
