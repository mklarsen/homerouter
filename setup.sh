#!/usr/bin/env bash
#
# homerouter -- idempotent deployment of an Ubuntu 24.04 LTS router / Wi-Fi AP.
#
#   sudo ./setup.sh              install + apply everything
#   sudo ./setup.sh --dry-run    render and validate only, change nothing
#   sudo ./setup.sh --no-apply   install files, do not (re)start services
#   sudo ./setup.sh --verify     run the health checks only
#
set -Eeuo pipefail

REPO_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
CONF_DIR="$REPO_DIR/config"
# shellcheck source=scripts/lib.sh
source "$REPO_DIR/scripts/lib.sh"

DRY_RUN=0
APPLY=1
VERIFY_ONLY=0

PACKAGES=(hostapd dnsmasq nftables iw rfkill bridge-utils ethtool gettext-base iproute2 dnsutils)

BACKUP_DIR="/var/backups/homerouter/$(date +%Y%m%d-%H%M%S)"
RENDER_DIR=""

cleanup() { [[ -n ${RENDER_DIR:-} ]] && rm -rf -- "$RENDER_DIR"; return 0; }
trap 'err "aborted at line $LINENO"' ERR
trap cleanup EXIT

# ------------------------------------------------------------------ arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)  DRY_RUN=1; APPLY=0 ;;
        --no-apply) APPLY=0 ;;
        --verify)   VERIFY_ONLY=1 ;;
        -h|--help)  sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *)          die "unknown argument: $1" ;;
    esac
    shift
done

require_root

if [[ $VERIFY_ONLY -eq 1 ]]; then
    exec "$REPO_DIR/scripts/verify.sh"
fi

# ------------------------------------------------------------ pre-flight
log "Pre-flight checks"

if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    if [[ ${ID:-} != "ubuntu" || ${VERSION_ID:-} != "24.04" ]]; then
        warn "tested on Ubuntu 24.04 LTS, found ${PRETTY_NAME:-unknown} -- continuing"
    else
        ok "Ubuntu 24.04 LTS"
    fi
fi

if systemctl is-active --quiet NetworkManager 2>/dev/null; then
    warn "NetworkManager is active; this setup uses systemd-networkd via netplan."
    warn "Consider: systemctl disable --now NetworkManager"
fi

# ------------------------------------------------------------ configuration
log "Loading configuration"

load_env "$REPO_DIR"
if [[ $ENV_LOADED -eq 1 ]]; then
    ok ".env loaded"
else
    warn ".env not found -- copy .env.example to .env and fill in your values"
fi

for ifname in "$WAN_IF" "$LAN_IF" "$WIFI_IF"; do
    if ip link show "$ifname" >/dev/null 2>&1; then
        ok "interface present: $ifname"
    else
        warn "interface missing: $ifname -- check the interface map in .env"
    fi
done

if [[ $SRV_ENABLE == "1" ]]; then
    [[ -n $SRV_IF ]] || die "SRV_ENABLE=1 but SRV_IF is empty"
    for taken in $UNUSED_IFS; do
        [[ $taken == "$SRV_IF" ]] && die "SRV_IF ($SRV_IF) must not also be listed in UNUSED_IFS"
    done
    if ip link show "$SRV_IF" >/dev/null 2>&1; then
        ok "interface present: $SRV_IF (server segment)"
    else
        warn "interface missing: $SRV_IF -- server segment will not come up"
    fi
    ok "server segment enabled: $SRV_IP4/$SRV_PREFIXLEN4 on $SRV_BRIDGE_IF"
fi

for required in WAN_IP4_PRIMARY WAN_GW4 LAN_IP4 LAN_NET4; do
    [[ -n ${!required} ]] || die "$required is not set -- see .env.example"
done
for recommended in WAN_IP6 WAN_GW6 LAN_IP6 LAN_NET6; do
    [[ -n ${!recommended} ]] || warn "$recommended is empty -- IPv6 will be incomplete"
done
ok "network settings loaded (WAN $WAN_IP4_PRIMARY, LAN $LAN_IP4/$LAN_PREFIXLEN4)"

if [[ -z $WIFI_PASSPHRASE ]]; then
    die "WIFI_PASSPHRASE is empty -- copy .env.example to .env and set it"
fi
if (( ${#WIFI_PASSPHRASE} < 8 || ${#WIFI_PASSPHRASE} > 63 )); then
    die "WIFI_PASSPHRASE must be 8-63 characters (got ${#WIFI_PASSPHRASE})"
fi
if [[ $WIFI_SSID == *$'\n'* || ${#WIFI_SSID} -gt 32 ]]; then
    die "WIFI_SSID must be a single line of at most 32 characters"
fi
ok "Wi-Fi settings validated (SSID: $WIFI_SSID, ${WIFI_HW_MODE}/ch${WIFI_CHANNEL})"

# ------------------------------------------------------------ packages
log "Installing packages"

missing=()
for pkg in "${PACKAGES[@]}"; do
    dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q "ok installed" || missing+=("$pkg")
done

if (( ${#missing[@]} == 0 )); then
    ok "all packages already installed"
elif [[ $DRY_RUN -eq 1 ]]; then
    warn "would install: ${missing[*]} (dry-run)"
else
    info "installing: ${missing[*]}"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq "${missing[@]}"
    ok "packages installed"
fi

# ------------------------------------------------------------ render hostapd
log "Rendering configuration"

RENDER_DIR="$(mktemp -d /tmp/homerouter.XXXXXX)"
chmod 700 "$RENDER_DIR"
HOSTAPD_RENDERED="$RENDER_DIR/hostapd.conf"
NETPLAN_RENDERED="$RENDER_DIR/01-router.yaml"
NFT_RENDERED="$RENDER_DIR/nftables.conf"
DNSMASQ_RENDERED="$RENDER_DIR/dnsmasq-router.conf"

render_template "$CONF_DIR/hostapd/hostapd.conf.template"      "$HOSTAPD_RENDERED"
render_template "$CONF_DIR/netplan/01-router.yaml.template"    "$NETPLAN_RENDERED"
render_template "$CONF_DIR/nftables/nftables.conf.template"    "$NFT_RENDERED"
render_template "$CONF_DIR/dnsmasq/router.conf.template"       "$DNSMASQ_RENDERED"

# Expand the optional list of unused NICs into the netplan ethernets section.
{
    while IFS= read -r line; do
        if [[ $line == "#__UNUSED_IFS__" ]]; then
            for ifname in $UNUSED_IFS; do
                printf '    %s:\n      dhcp4: false\n      dhcp6: false\n      accept-ra: false\n      optional: true\n' "$ifname"
            done
        else
            printf '%s\n' "$line"
        fi
    done < "$NETPLAN_RENDERED"
} > "$NETPLAN_RENDERED.new"
mv "$NETPLAN_RENDERED.new" "$NETPLAN_RENDERED"
chmod 0600 "$NETPLAN_RENDERED"

if [[ $WIFI_VHT != "1" || $WIFI_HW_MODE != "a" ]]; then
    # 802.11ac only exists on 5 GHz -- neutralise the VHT block.
    sed -i -E 's/^(ieee80211ac|vht_capab|vht_oper_chwidth|vht_oper_centr_freq_seg0_idx)=/#\1=/' \
        "$HOSTAPD_RENDERED"
    info "802.11ac disabled (hw_mode=$WIFI_HW_MODE, vht=$WIFI_VHT)"
fi

if [[ $WIFI_WPA3_ONLY == "1" ]]; then
    sed -i \
        -e 's/^wpa_key_mgmt=.*/wpa_key_mgmt=SAE/' \
        -e 's/^ieee80211w=.*/ieee80211w=2/' \
        -e 's/^wpa_passphrase=/#wpa_passphrase=/' \
        "$HOSTAPD_RENDERED"
    info "WPA3-SAE only mode"
fi
ok "hostapd.conf rendered"
ok "netplan / nftables / dnsmasq rendered"

# ------------------------------------------------------------ validation
log "Validating configuration"

if nft --check --file "$NFT_RENDERED"; then
    ok "nftables ruleset syntax"
else
    die "nftables ruleset failed validation"
fi

if command -v dnsmasq >/dev/null 2>&1; then
    if dnsmasq --test --conf-file="$DNSMASQ_RENDERED" 2>&1 | grep -q "syntax check OK"; then
        ok "dnsmasq syntax"
    else
        dnsmasq --test --conf-file="$DNSMASQ_RENDERED" || true
        die "dnsmasq config failed validation"
    fi
fi

if python3 -c 'import sys,yaml; yaml.safe_load(open(sys.argv[1]))' "$NETPLAN_RENDERED" 2>/dev/null; then
    ok "netplan YAML parses"
else
    warn "could not pre-parse netplan YAML (python3-yaml missing?)"
fi

# ------------------------------------------------------------ install files
log "Installing configuration files"

CHANGED_NET=0; CHANGED_SYSCTL=0; CHANGED_NFT=0
CHANGED_HOSTAPD=0; CHANGED_DNSMASQ=0; CHANGED_RESOLVED=0

if install_managed "$NETPLAN_RENDERED"                /etc/netplan/01-router.yaml  0600; then CHANGED_NET=1; fi
if install_managed "$CONF_DIR/sysctl/99-router.conf"  /etc/sysctl.d/99-router.conf 0644; then CHANGED_SYSCTL=1; fi
if install_managed "$NFT_RENDERED"                    /etc/nftables.conf           0755; then CHANGED_NFT=1; fi
if install_managed "$HOSTAPD_RENDERED"                /etc/hostapd/hostapd.conf    0600; then CHANGED_HOSTAPD=1; fi
if install_managed "$CONF_DIR/default/hostapd"        /etc/default/hostapd         0644; then CHANGED_HOSTAPD=1; fi
if install_managed "$DNSMASQ_RENDERED"                /etc/dnsmasq.d/router.conf   0644; then CHANGED_DNSMASQ=1; fi
if install_managed "$CONF_DIR/systemd/resolved-no-stub.conf" \
                   /etc/systemd/resolved.conf.d/homerouter.conf 0644; then CHANGED_RESOLVED=1; fi

# Netplan yaml files from other sources (cloud-init, installer) would conflict.
shopt -s nullglob
for stale in /etc/netplan/*.yaml /etc/netplan/*.yml; do
    [[ $stale == /etc/netplan/01-router.yaml ]] && continue
    warn "other netplan file present: $stale (may conflict -- review and remove)"
done
shopt -u nullglob

if [[ $DRY_RUN -eq 1 ]]; then
    log "Dry run complete -- nothing was changed"
    exit 0
fi

if [[ $APPLY -eq 0 ]]; then
    log "Files installed; --no-apply given, skipping service activation"
    exit 0
fi

# ------------------------------------------------------------ apply
log "Applying system state"

if [[ $CHANGED_SYSCTL -eq 1 ]] || [[ $(sysctl -n net.ipv4.ip_forward) != "1" ]]; then
    sysctl --system >/dev/null
    ok "sysctl reloaded"
fi

# Wi-Fi radio must not be soft-blocked before hostapd starts.
if command -v rfkill >/dev/null 2>&1 && rfkill list wifi | grep -q "Soft blocked: yes"; then
    rfkill unblock wifi
    ok "wifi rfkill unblocked"
fi

if [[ $CHANGED_RESOLVED -eq 1 ]]; then
    systemctl restart systemd-resolved
    ok "systemd-resolved restarted (stub listener off)"
fi
if [[ -e /run/systemd/resolve/resolv.conf && ! /etc/resolv.conf -ef /run/systemd/resolve/resolv.conf ]]; then
    ln -sf /run/systemd/resolve/resolv.conf /etc/resolv.conf
    ok "/etc/resolv.conf repointed at systemd-resolved"
fi

if [[ $CHANGED_NET -eq 1 ]]; then
    netplan generate
    warn "applying netplan -- an SSH session over the reconfigured NICs may drop"
    netplan apply
    ok "netplan applied"
    # Give networkd a moment to bring the bridge up before dnsmasq/hostapd bind.
    for _ in {1..15}; do
        ip -brief link show "$BRIDGE_IF" 2>/dev/null | grep -qE "UP|UNKNOWN" && break
        sleep 1
    done
fi

# nftables: persist across reboots, reload only when needed.
systemctl enable nftables >/dev/null 2>&1 || true
if [[ $CHANGED_NFT -eq 1 ]] || ! nft list table inet filter >/dev/null 2>&1; then
    nft --file /etc/nftables.conf
    ok "nftables ruleset loaded"
else
    ok "nftables ruleset already current"
fi

# hostapd ships masked on Ubuntu.
systemctl unmask hostapd >/dev/null 2>&1 || true
systemctl enable hostapd >/dev/null 2>&1 || true
systemctl enable dnsmasq >/dev/null 2>&1 || true

if [[ $CHANGED_DNSMASQ -eq 1 || $CHANGED_NET -eq 1 ]]; then
    systemctl restart dnsmasq
    ok "dnsmasq restarted"
else
    systemctl is-active --quiet dnsmasq || { systemctl start dnsmasq; ok "dnsmasq started"; }
fi

if [[ $CHANGED_HOSTAPD -eq 1 || $CHANGED_NET -eq 1 ]]; then
    systemctl restart hostapd
    ok "hostapd restarted"
else
    systemctl is-active --quiet hostapd || { systemctl start hostapd; ok "hostapd started"; }
fi

# ------------------------------------------------------------ result
if (( ${#CHANGED_ITEMS[@]} )); then
    log "Changed: ${#CHANGED_ITEMS[@]} file(s); backups in $BACKUP_DIR"
else
    log "No configuration changes were necessary"
fi

log "Running health checks"
if "$REPO_DIR/scripts/verify.sh"; then
    log "Router is up."
else
    warn "deployment finished with failing health checks -- see output above"
    exit 1
fi
