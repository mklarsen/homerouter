#!/usr/bin/env bash
# Shared helpers for homerouter scripts.
# shellcheck shell=bash

if [[ -t 1 ]]; then
    C_RED=$'\033[31m'; C_GRN=$'\033[32m'; C_YEL=$'\033[33m'
    C_BLU=$'\033[34m'; C_DIM=$'\033[2m';  C_OFF=$'\033[0m'
else
    C_RED=''; C_GRN=''; C_YEL=''; C_BLU=''; C_DIM=''; C_OFF=''
fi

CHANGED_ITEMS=()
FAILED_CHECKS=0

log()  { printf '%s==>%s %s\n' "$C_BLU" "$C_OFF" "$*"; }
ok()   { printf '%s  ok%s   %s\n' "$C_GRN" "$C_OFF" "$*"; }
warn() { printf '%s  warn%s %s\n' "$C_YEL" "$C_OFF" "$*" >&2; }
info() { printf '%s       %s%s\n' "$C_DIM" "$*" "$C_OFF"; }
err()  { printf '%s  fail%s %s\n' "$C_RED" "$C_OFF" "$*" >&2; }
die()  { err "$*"; exit 1; }

check() {
    # check "<description>" <command...>
    local desc="$1"; shift
    if "$@" >/dev/null 2>&1; then
        ok "$desc"
    else
        err "$desc"
        FAILED_CHECKS=$((FAILED_CHECKS + 1))
    fi
}

require_root() {
    [[ ${EUID} -eq 0 ]] || die "must run as root (use sudo)"
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

# Everything the router needs at runtime. Installed by install.sh while the box
# still has its original internet connection, because setup.sh may well run
# after the uplink has been reconfigured.
PACKAGES=(hostapd dnsmasq nftables iw rfkill bridge-utils ethtool gettext-base iproute2 dnsutils)

# missing_packages -- prints the packages that are not installed yet.
missing_packages() {
    local pkg
    for pkg in "${PACKAGES[@]}"; do
        if ! dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q "ok installed"; then
            printf '%s\n' "$pkg"
        fi
    done
}

# install_packages -- idempotent; honours DRY_RUN.
install_packages() {
    local missing=()
    mapfile -t missing < <(missing_packages)

    if (( ${#missing[@]} == 0 )); then
        ok "all packages already installed"
        return 0
    fi

    if [[ ${DRY_RUN:-0} -eq 1 ]]; then
        warn "would install: ${missing[*]} (dry-run)"
        return 0
    fi

    info "installing: ${missing[*]}"
    export DEBIAN_FRONTEND=noninteractive
    # A single broken third-party repository must not abort the deployment;
    # the cached lists are usually good enough for these packages.
    if ! apt-get update -qq; then
        warn "apt-get update reported errors -- a third-party repository is likely broken"
        warn "continuing with the package lists already on disk"
    fi
    if ! apt-get install -y -qq "${missing[@]}"; then
        err "could not install: ${missing[*]}"
        die "fix apt (see the output above) and rerun"
    fi
    ok "packages installed"
}

# Every variable substituted into config/*.template.
TEMPLATE_VARS=(
    WAN_IF LAN_IF WIFI_IF BRIDGE_IF
    WAN_IP4_PRIMARY WAN_IP4_SECONDARY WAN_IP4_TERTIARY WAN_IP4_PREFIXLEN WAN_GW4
    WAN_IP6 WAN_IP6_PREFIXLEN WAN_GW6
    LAN_IP4 LAN_NET4 LAN_PREFIXLEN4 LAN_NETMASK4
    LAN_DHCP4_START LAN_DHCP4_END LAN_DHCP4_LEASE
    LAN_IP6 LAN_IP6_PREFIXLEN LAN_NET6 LAN_IP6_PREFIX_EXAMPLE LAN_DOMAIN
    SRV_IF SRV_BRIDGE_IF SRV_CMT SRV_LAN_CMT
    SRV_IP4 SRV_NET4 SRV_PREFIXLEN4 SRV_NETMASK4 SRV_DHCP4_START SRV_DHCP4_END
    SRV_IP6 SRV_IP6_PREFIXLEN SRV_DOMAIN
    DOCKER_CMT
    DNS4_1 DNS4_2 DNS6_1
    NFT_IP4_SECONDARY NFT_IP4_TERTIARY
    WIFI_SSID WIFI_PASSPHRASE WIFI_COUNTRY_CODE WIFI_HW_MODE WIFI_CHANNEL WIFI_VHT
)

# load_env <repo_dir>
# Sources .env (if present) and fills in defaults for everything unset.
load_env() {
    local repo="$1"

    if [[ -f "$repo/.env" ]]; then
        set -a
        # shellcheck disable=SC1091
        source "$repo/.env"
        set +a
        ENV_LOADED=1
    else
        ENV_LOADED=0
    fi

    : "${WAN_IF:=enp3s0}"
    : "${LAN_IF:=enp4s0}"
    : "${WIFI_IF:=wlp6s0}"
    : "${BRIDGE_IF:=br0}"
    : "${UNUSED_IFS:=}"

    : "${WAN_IP4_PRIMARY:=}"
    : "${WAN_IP4_SECONDARY:=}"
    : "${WAN_IP4_TERTIARY:=}"
    : "${WAN_IP4_PREFIXLEN:=24}"
    : "${WAN_GW4:=}"

    : "${WAN_IP6:=}"
    : "${WAN_IP6_PREFIXLEN:=64}"
    : "${WAN_GW6:=}"

    : "${LAN_IP4:=10.10.10.1}"
    : "${LAN_NET4:=10.10.10.0/24}"
    : "${LAN_NETMASK4:=255.255.255.0}"
    : "${LAN_DHCP4_START:=10.10.10.100}"
    : "${LAN_DHCP4_END:=10.10.10.200}"
    : "${LAN_DHCP4_LEASE:=12h}"

    : "${LAN_NET6:=}"
    : "${LAN_IP6:=}"
    : "${LAN_IP6_PREFIXLEN:=64}"
    : "${LAN_DOMAIN:=lan}"

    : "${SRV_ENABLE:=0}"
    # Lets the self-test render both variants without touching .env.
    if [[ -n ${SRV_ENABLE_OVERRIDE:-} ]]; then
        SRV_ENABLE="$SRV_ENABLE_OVERRIDE"
    fi
    : "${SRV_IF:=}"
    : "${SRV_BRIDGE_IF:=br1}"
    : "${SRV_IP4:=10.10.20.1}"
    : "${SRV_NET4:=10.10.20.0/24}"
    : "${SRV_NETMASK4:=255.255.255.0}"
    : "${SRV_DHCP4_START:=10.10.20.100}"
    : "${SRV_DHCP4_END:=10.10.20.200}"
    : "${SRV_IP6:=}"
    : "${SRV_IP6_PREFIXLEN:=64}"
    : "${SRV_DOMAIN:=srv}"
    : "${SRV_TO_LAN:=0}"

    : "${DOCKER_COMPAT:=0}"
    if [[ -n ${DOCKER_COMPAT_OVERRIDE:-} ]]; then
        DOCKER_COMPAT="$DOCKER_COMPAT_OVERRIDE"
    fi

    : "${DNS4_1:=1.1.1.1}"
    : "${DNS4_2:=8.8.8.8}"
    : "${DNS6_1:=2606:4700:4700::1111}"

    : "${WIFI_SSID:=homerouter}"
    : "${WIFI_PASSPHRASE:=}"
    : "${WIFI_COUNTRY_CODE:=DK}"
    : "${WIFI_HW_MODE:=a}"
    : "${WIFI_CHANNEL:=36}"
    : "${WIFI_VHT:=1}"
    : "${WIFI_WPA3_ONLY:=0}"

    # Derived values; consumed by render_template via TEMPLATE_VARS.
    LAN_PREFIXLEN4="${LAN_NET4##*/}"
    if [[ $LAN_PREFIXLEN4 == "$LAN_NET4" ]]; then
        LAN_PREFIXLEN4=24
    fi
    # nftables `define` cannot be empty; fall back to the primary address.
    # shellcheck disable=SC2034  # exported through TEMPLATE_VARS
    NFT_IP4_SECONDARY="${WAN_IP4_SECONDARY:-$WAN_IP4_PRIMARY}"
    # shellcheck disable=SC2034
    NFT_IP4_TERTIARY="${WAN_IP4_TERTIARY:-$WAN_IP4_PRIMARY}"
    # Only used inside a commented example in nftables.conf.
    # shellcheck disable=SC2034
    LAN_IP6_PREFIX_EXAMPLE="${LAN_IP6%::*}"

    SRV_PREFIXLEN4="${SRV_NET4##*/}"
    if [[ $SRV_PREFIXLEN4 == "$SRV_NET4" ]]; then
        SRV_PREFIXLEN4=24
    fi
    # Lines belonging to the optional second segment are commented out when it
    # is disabled, which keeps every config file a single rendered template.
    # shellcheck disable=SC2034  # exported through TEMPLATE_VARS
    if [[ $SRV_ENABLE == "1" ]]; then
        SRV_CMT=""
    else
        SRV_CMT="# "
    fi
    # shellcheck disable=SC2034
    if [[ $SRV_ENABLE == "1" && $SRV_TO_LAN == "1" ]]; then
        SRV_LAN_CMT=""
    else
        SRV_LAN_CMT="# "
    fi
    # shellcheck disable=SC2034
    if [[ $DOCKER_COMPAT == "1" ]]; then
        DOCKER_CMT=""
    else
        DOCKER_CMT="# "
    fi

    export SRV_ENABLE SRV_TO_LAN DOCKER_COMPAT

    export UNUSED_IFS WIFI_WPA3_ONLY ENV_LOADED
    export "${TEMPLATE_VARS[@]}"
}

# render_template <src.template> <dst>
# envsubst with an explicit variable list so that shell-like tokens in the
# target syntax (e.g. nftables `$WAN`) survive untouched.
render_template() {
    local src="$1" dst="$2" list=""
    [[ -f $src ]] || die "template missing: $src"
    local v
    for v in "${TEMPLATE_VARS[@]}"; do list+="\${$v} "; done
    ( umask 077; envsubst "$list" < "$src" > "$dst" )
    # Drop address entries whose variable was empty (e.g. "  - /24").
    sed -i -E '/^[[:space:]]*-[[:space:]]*"?\/[0-9]+"?[[:space:]]*$/d' "$dst"
}

# install_managed <src> <dst> <mode>
# Copies only when the content differs. Backs up the previous version.
# Returns 0 when the destination changed, 1 when it was already up to date.
install_managed() {
    local src="$1" dst="$2" mode="${3:-0644}"

    [[ -f $src ]] || die "source file missing: $src"

    if [[ -f $dst ]] && cmp -s "$src" "$dst"; then
        # Content matches; still enforce ownership and permissions.
        chown root:root "$dst"
        chmod "$mode" "$dst"
        info "unchanged  $dst"
        return 1
    fi

    if [[ ${DRY_RUN:-0} -eq 1 ]]; then
        warn "would install $dst (dry-run)"
        CHANGED_ITEMS+=("$dst")
        return 0
    fi

    if [[ -e $dst ]]; then
        mkdir -p "$BACKUP_DIR$(dirname "$dst")"
        cp -a "$dst" "$BACKUP_DIR$dst"
        info "backup     $BACKUP_DIR$dst"
    fi

    install -D -o root -g root -m "$mode" "$src" "$dst"
    ok "installed  $dst"
    CHANGED_ITEMS+=("$dst")
    return 0
}
