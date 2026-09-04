#!/usr/bin/env bash
#
# homerouter -- post-deployment health check. Read-only, safe to run any time.
#
set -Eeuo pipefail

REPO_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib.sh
source "$REPO_DIR/scripts/lib.sh"

load_env "$REPO_DIR"

log "Kernel forwarding"
check "net.ipv4.ip_forward = 1"          test "$(sysctl -n net.ipv4.ip_forward)" = "1"
check "net.ipv6.conf.all.forwarding = 1" test "$(sysctl -n net.ipv6.conf.all.forwarding)" = "1"

log "Addressing"
for addr in "$WAN_IP4_PRIMARY" "$WAN_IP4_SECONDARY" "$WAN_IP4_TERTIARY"; do
    [[ -n $addr ]] || continue
    check "$WAN_IF has $addr" grep -q "$addr" <<<"$(ip -4 addr show "$WAN_IF" 2>/dev/null)"
done
if [[ -n $WAN_IP6 ]]; then
    check "$WAN_IF has $WAN_IP6" grep -q "$WAN_IP6" <<<"$(ip -6 addr show "$WAN_IF" 2>/dev/null)"
fi
check "$BRIDGE_IF has $LAN_IP4/$LAN_PREFIXLEN4" \
    grep -q "$LAN_IP4/$LAN_PREFIXLEN4" <<<"$(ip -4 addr show "$BRIDGE_IF" 2>/dev/null)"
if [[ -n $LAN_IP6 ]]; then
    check "$BRIDGE_IF has $LAN_IP6/$LAN_IP6_PREFIXLEN" \
        grep -q "$LAN_IP6/$LAN_IP6_PREFIXLEN" <<<"$(ip -6 addr show "$BRIDGE_IF" 2>/dev/null)"
fi

log "Routing"
check "IPv4 default via $WAN_GW4" grep -q "default via $WAN_GW4" <<<"$(ip -4 route show)"
if [[ -n $WAN_GW6 ]]; then
    check "IPv6 default via $WAN_GW6" grep -q "$WAN_GW6" <<<"$(ip -6 route show default)"
fi

log "Bridge membership"
check "$LAN_IF enslaved to $BRIDGE_IF"  grep -q "master $BRIDGE_IF" <<<"$(ip link show "$LAN_IF" 2>/dev/null)"
check "$WIFI_IF enslaved to $BRIDGE_IF" grep -q "master $BRIDGE_IF" <<<"$(ip link show "$WIFI_IF" 2>/dev/null)"

log "Firewall"
check "table inet filter present"     nft list table inet filter
check "table ip nat present"          nft list table ip nat
check "forward policy is drop"        grep -q "hook forward priority filter; policy drop" <<<"$(nft list table inet filter)"
check "input policy is drop"          grep -q "hook input priority filter; policy drop"   <<<"$(nft list table inet filter)"
check "SNAT via $WAN_IP4_PRIMARY"     grep -qE "snat to $WAN_IP4_PRIMARY|masquerade" <<<"$(nft list table ip nat)"

log "Services"
for unit in systemd-networkd nftables dnsmasq hostapd; do
    check "$unit active" systemctl is-active --quiet "$unit"
    check "$unit enabled" systemctl is-enabled --quiet "$unit"
done

log "DHCP / RA"
check "dnsmasq listening on $LAN_IP4:53" grep -q "$LAN_IP4:53" <<<"$(ss -lntup 2>/dev/null)"
check "dnsmasq DHCP listening on :67"    grep -q ":67 " <<<"$(ss -lnup 2>/dev/null)"

log "Wi-Fi"
check "wifi radio not blocked" bash -c '! rfkill list wifi | grep -q "Soft blocked: yes"'
if command -v hostapd_cli >/dev/null 2>&1; then
    if hostapd_cli -i "$WIFI_IF" status >/dev/null 2>&1; then
        ssid=$(hostapd_cli -i "$WIFI_IF" status 2>/dev/null | sed -n 's/^ssid\[0\]=//p')
        state=$(hostapd_cli -i "$WIFI_IF" status 2>/dev/null | sed -n 's/^state=//p')
        ok "AP up (ssid=${ssid:-?} state=${state:-?})"
    else
        err "hostapd control interface not answering on $WIFI_IF"
        FAILED_CHECKS=$((FAILED_CHECKS + 1))
    fi
fi

log "Connectivity"
check "IPv4 egress ($DNS4_1)"        ping -c1 -W3 -I "$WAN_IF" "$DNS4_1"
if [[ -n $DNS6_1 ]]; then
    check "IPv6 egress ($DNS6_1)"    ping -6 -c1 -W3 -I "$WAN_IF" "$DNS6_1"
fi
check "DNS resolution via dnsmasq"   dig +short +time=3 +tries=1 "@$LAN_IP4" example.com

echo
if (( FAILED_CHECKS == 0 )); then
    ok "all checks passed"
    exit 0
fi
err "$FAILED_CHECKS check(s) failed"
info "logs: journalctl -u hostapd -u dnsmasq -u systemd-networkd -n 50 --no-pager"
exit 1
