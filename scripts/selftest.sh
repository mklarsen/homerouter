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

run_pass() {
    local mode="$1" out="$WORK_DIR/srv$1"
    mkdir -p "$out"

    SRV_ENABLE_OVERRIDE="$mode" load_env "$ENV_SOURCE"

    log "Rendering templates (SRV_ENABLE=$mode)"
    render_template "$CONF_DIR/hostapd/hostapd.conf.template"   "$out/hostapd.conf"
    render_template "$CONF_DIR/netplan/01-router.yaml.template" "$out/01-router.yaml"
    render_template "$CONF_DIR/nftables/nftables.conf.template" "$out/nftables.conf"
    render_template "$CONF_DIR/dnsmasq/router.conf.template"    "$out/router.conf"
    ok "4 templates rendered"

    # shellcheck disable=SC2016  # the ${...} pattern is matched literally
    if grep -rn '\${[A-Z0-9_]\+}' "$out" >/dev/null 2>&1; then
        # shellcheck disable=SC2016
        grep -rn '\${[A-Z0-9_]\+}' "$out" || true
        die "unsubstituted variables left in the rendered output"
    fi
    ok "no unresolved placeholders"

    if command -v nft >/dev/null 2>&1; then
        check "nftables ruleset syntax" nft --check --file "$out/nftables.conf"
    else
        warn "nft not installed -- skipping firewall syntax check"
    fi

    if command -v dnsmasq >/dev/null 2>&1; then
        check "dnsmasq syntax" bash -c \
            "dnsmasq --test --conf-file='$out/router.conf' 2>&1 | grep -q 'syntax check OK'"
    else
        warn "dnsmasq not installed -- skipping DHCP/DNS syntax check"
    fi

    if python3 -c 'import yaml' >/dev/null 2>&1; then
        check "netplan YAML parses" python3 -c \
            'import sys,yaml; yaml.safe_load(open(sys.argv[1]))' "$out/01-router.yaml"
    else
        warn "python3-yaml not installed -- skipping netplan YAML check"
    fi

    check "hostapd has an ssid"      grep -q '^ssid=..*' "$out/hostapd.conf"
    check "hostapd has a passphrase" grep -qE '^(wpa_passphrase|sae_password)=..*' "$out/hostapd.conf"

    if [[ $mode == "1" ]]; then
        check "server bridge configured"  grep -q "^    $SRV_BRIDGE_IF:" "$out/01-router.yaml"
        check "server DHCP range present" grep -q "^dhcp-range=set:srv," "$out/router.conf"
        check "server isolated from LAN" bash -c \
            "! grep -qE '^ +iifname \\\$SRV oifname \\\$LAN accept' '$out/nftables.conf'"
    fi
}

# The optional server segment changes every rendered file, so validate both
# states regardless of what .env says.
for mode in 0 1; do
    run_pass "$mode"
done

echo
if (( FAILED_CHECKS == 0 )); then
    ok "self-test passed"
    exit 0
fi
err "$FAILED_CHECKS check(s) failed"
exit 1
