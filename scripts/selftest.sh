#!/usr/bin/env bash
#
# homerouter -- offline self-test: render every template and validate the result.
# Runs in CI and locally; changes nothing on the system.
#
#   ./scripts/selftest.sh            # uses .env, or .env.example as a fallback
#
set -Eeuo pipefail

REPO_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib.sh
source "$REPO_DIR/scripts/lib.sh"

CONF_DIR="$REPO_DIR/config"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf -- "$WORK_DIR"' EXIT

if [[ ! -f "$REPO_DIR/.env" ]]; then
    info "no .env -- self-testing against .env.example"
    cp "$REPO_DIR/.env.example" "$WORK_DIR/.env"
    ENV_SOURCE="$WORK_DIR"
else
    ENV_SOURCE="$REPO_DIR"
fi

log "Loading configuration"
load_env "$ENV_SOURCE"
ok "config loaded (WAN ${WAN_IP4_PRIMARY:-unset}, LAN ${LAN_IP4}/${LAN_PREFIXLEN4})"

log "Rendering templates"
render_template "$CONF_DIR/hostapd/hostapd.conf.template"   "$WORK_DIR/hostapd.conf"
render_template "$CONF_DIR/netplan/01-router.yaml.template" "$WORK_DIR/01-router.yaml"
render_template "$CONF_DIR/nftables/nftables.conf.template" "$WORK_DIR/nftables.conf"
render_template "$CONF_DIR/dnsmasq/router.conf.template"    "$WORK_DIR/router.conf"
ok "4 templates rendered"

log "Checking for unresolved placeholders"
if grep -rn '\${[A-Z0-9_]\+}' "$WORK_DIR" >/dev/null 2>&1; then
    grep -rn '\${[A-Z0-9_]\+}' "$WORK_DIR" || true
    die "unsubstituted variables left in the rendered output"
fi
ok "no unresolved placeholders"

log "Validating rendered output"
if command -v nft >/dev/null 2>&1; then
    check "nftables ruleset syntax" nft --check --file "$WORK_DIR/nftables.conf"
else
    warn "nft not installed -- skipping firewall syntax check"
fi

if command -v dnsmasq >/dev/null 2>&1; then
    check "dnsmasq syntax" bash -c \
        "dnsmasq --test --conf-file='$WORK_DIR/router.conf' 2>&1 | grep -q 'syntax check OK'"
else
    warn "dnsmasq not installed -- skipping DHCP/DNS syntax check"
fi

if python3 -c 'import yaml' >/dev/null 2>&1; then
    check "netplan YAML parses" python3 -c \
        'import sys,yaml; yaml.safe_load(open(sys.argv[1]))' "$WORK_DIR/01-router.yaml"
else
    warn "python3-yaml not installed -- skipping netplan YAML check"
fi

check "hostapd has an ssid"       grep -q '^ssid=..*'          "$WORK_DIR/hostapd.conf"
check "hostapd has a passphrase"  grep -qE '^(wpa_passphrase|sae_password)=..*' "$WORK_DIR/hostapd.conf"

echo
if (( FAILED_CHECKS == 0 )); then
    ok "self-test passed"
    exit 0
fi
err "$FAILED_CHECKS check(s) failed"
exit 1
