#!/usr/bin/env bash
#
# homerouter-status -- live status of a homerouter box.
#
# Standalone by design: no dependency on the homerouter scripts, so it can move
# into its own repository and be added back as a submodule or add-on.
#
#   ./status.sh              one-shot, human readable
#   ./status.sh --watch      refresh every 2 seconds
#   ./status.sh --watch 5    refresh every 5 seconds
#   ./status.sh --json       machine readable, for mail/alerting integrations
#
# Read-only: it never changes the system, so it is safe to run as a normal user
# (a few details need root and are then reported as "n/a").
#
set -Eeuo pipefail

ENV_FILE="${HOMEROUTER_ENV:-/opt/homerouter/.env}"
MODE="human"
INTERVAL=2

while [[ $# -gt 0 ]]; do
    case "$1" in
        --json)  MODE="json"; shift ;;
        --watch) MODE="watch"; [[ ${2:-} =~ ^[0-9]+$ ]] && { INTERVAL="$2"; shift; }; shift ;;
        --env)   ENV_FILE="${2:?--env needs a path}"; shift 2 ;;
        -h|--help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -r $ENV_FILE ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
fi

: "${WAN_IF:=enp3s0}"
: "${LAN_IF:=enp4s0}"
: "${WIFI_IF:=wlp6s0}"
: "${WAN_MAC:=}"
: "${LAN_MAC:=}"
: "${WIFI_MAC:=}"
: "${BRIDGE_IF:=br0}"
: "${SRV_ENABLE:=0}"
: "${SRV_BRIDGE_IF:=br1}"
: "${LAN_IP4:=10.10.10.1}"
: "${DNS4_1:=1.1.1.1}"
: "${DNS6_1:=2606:4700:4700::1111}"

if [[ -t 1 && $MODE != "json" ]]; then
    C_G=$'\033[32m'; C_R=$'\033[31m'; C_Y=$'\033[33m'
    C_B=$'\033[34m'; C_D=$'\033[2m'; C_0=$'\033[0m'
else
    C_G=''; C_R=''; C_Y=''; C_B=''; C_D=''; C_0=''
fi

# ------------------------------------------------------------------ collectors

link_state() { ip -brief link show "$1" 2>/dev/null | awk '{print $2}'; }

resolve_ifname() {
    local configured="$1" mac="$2" candidate
    [[ -e /sys/class/net/$configured ]] && { printf '%s' "$configured"; return; }
    [[ -n $mac ]] || { printf '%s' "$configured"; return; }
    for candidate in /sys/class/net/*; do
        [[ -r $candidate/address ]] || continue
        [[ $(cat "$candidate/address") == "$mac" ]] || continue
        basename "$candidate"
        return
    done
    printf '%s' "$configured"
}

WAN_IF="$(resolve_ifname "$WAN_IF" "$WAN_MAC")"
LAN_IF="$(resolve_ifname "$LAN_IF" "$LAN_MAC")"
WIFI_IF="$(resolve_ifname "$WIFI_IF" "$WIFI_MAC")"

link_speed() {
    local sp
    sp="$(cat "/sys/class/net/$1/speed" 2>/dev/null || true)"
    if [[ -n $sp && $sp != "-1" ]]; then
        printf '%sMb/s' "$sp"
        return
    fi
    # Wi-Fi has no speed attribute; ask the driver for the current bitrate.
    if command -v iw >/dev/null 2>&1 && [[ -d /sys/class/net/$1/wireless ]]; then
        sp="$(iw dev "$1" link 2>/dev/null | sed -n 's/.*tx bitrate: \([0-9.]*\) MBit.*/\1/p' | head -n1)"
        [[ -n $sp ]] && { printf '%sMb/s' "$sp"; return; }
    fi
    printf 'n/a'
}

addr4() { ip -4 -brief addr show "$1" 2>/dev/null | awk '{$1="";$2="";print}' | xargs || true; }
addr6() { ip -6 -brief addr show "$1" 2>/dev/null | awk '{$1="";$2="";print}' | xargs || true; }

bridge_members() {
    local br="$1" m=()
    local path
    for path in /sys/class/net/"$br"/brif/*; do
        [[ -e $path ]] || continue
        m+=("$(basename "$path")")
    done
    printf '%s' "${m[*]:-none}"
}

svc_state() {
    # is-active prints the state itself and exits non-zero when not running.
    local s
    s="$(systemctl is-active "$1" 2>/dev/null || true)"
    printf '%s' "${s:-unknown}"
}

nft_counter() {
    # Sums packet counters of rules whose comment matches $1.
    nft -a list ruleset 2>/dev/null \
        | grep -F "comment \"$1\"" \
        | grep -oE 'packets [0-9]+' | awk '{s+=$2} END {print s+0}'
}

lease_count() {
    local f=/var/lib/misc/dnsmasq.leases
    [[ -r $f ]] && wc -l < "$f" | tr -d ' ' || echo "n/a"
}

wifi_clients() {
    command -v hostapd_cli >/dev/null 2>&1 || { echo "n/a"; return; }
    hostapd_cli -i "$WIFI_IF" list_sta 2>/dev/null | grep -c ':' || echo 0
}

wifi_ssid() {
    command -v hostapd_cli >/dev/null 2>&1 || { echo "n/a"; return; }
    hostapd_cli -i "$WIFI_IF" status 2>/dev/null | sed -n 's/^ssid\[0\]=//p' | head -n1
}

ping_ok() { ping -c1 -W2 "$@" >/dev/null 2>&1 && echo up || echo down; }

uptime_short() { uptime -p 2>/dev/null | sed 's/^up //' || echo "n/a"; }

load_avg() { awk '{print $1", "$2", "$3}' /proc/loadavg; }

cpu_temp() {
    local hw name t
    # AMD reports through k10temp/hwmon; thermal_zone is often absent.
    for hw in /sys/class/hwmon/hwmon*; do
        [[ -r $hw/name ]] || continue
        name="$(cat "$hw/name" 2>/dev/null || true)"
        case "$name" in
            k10temp|coretemp|zenpower|cpu_thermal|acpitz)
                for t in "$hw"/temp1_input "$hw"/temp2_input; do
                    [[ -r $t ]] || continue
                    awk '{printf "%.0f\n", $1/1000; exit}' "$t"
                    return
                done
                ;;
        esac
    done
    for t in /sys/class/thermal/thermal_zone*/temp; do
        [[ -r $t ]] || continue
        awk '{printf "%.0f\n", $1/1000; exit}' "$t"
        return
    done
    echo "n/a"
}

conntrack_count() { cat /proc/sys/net/netfilter/nf_conntrack_count 2>/dev/null || echo "n/a"; }

# ------------------------------------------------------------------ rendering

badge() {
    case "$1" in
        UP|up|active|yes) printf '%s%s%s' "$C_G" "$1" "$C_0" ;;
        DOWN|down|inactive|failed|no) printf '%s%s%s' "$C_R" "$1" "$C_0" ;;
        *) printf '%s%s%s' "$C_Y" "$1" "$C_0" ;;
    esac
}

row() { printf '  %-16s %s\n' "$1" "$2"; }

render_human() {
    printf '%s== homerouter ==%s  %s  up %s  load %s  temp %s°C  conntrack %s\n\n' \
        "$C_B" "$C_0" "$(hostname)" "$(uptime_short)" "$(load_avg)" "$(cpu_temp)" "$(conntrack_count)"

    printf '%sInterfaces%s\n' "$C_B" "$C_0"
    local ifname
    for ifname in "$WAN_IF" "$LAN_IF" "$WIFI_IF" "$BRIDGE_IF"; do
        [[ -e /sys/class/net/$ifname ]] || { row "$ifname" "$(badge missing)"; continue; }
        row "$ifname" "$(badge "$(link_state "$ifname")")  $(link_speed "$ifname")  ${C_D}$(addr4 "$ifname") $(addr6 "$ifname")${C_0}"
    done
    if [[ $SRV_ENABLE == "1" ]]; then
        row "$SRV_BRIDGE_IF" "$(badge "$(link_state "$SRV_BRIDGE_IF")")  ${C_D}$(addr4 "$SRV_BRIDGE_IF")${C_0}"
    fi
    row "bridge" "${C_D}$BRIDGE_IF: $(bridge_members "$BRIDGE_IF")${C_0}"
    echo

    printf '%sServices%s\n' "$C_B" "$C_0"
    local unit
    for unit in systemd-networkd nftables dnsmasq hostapd; do
        row "$unit" "$(badge "$(svc_state "$unit")")"
    done
    echo

    printf '%sClients%s\n' "$C_B" "$C_0"
    row "DHCP leases" "$(lease_count)"
    row "Wi-Fi" "$(wifi_clients) client(s) on '$(wifi_ssid)'"
    echo

    printf '%sTraffic (packets)%s\n' "$C_B" "$C_0"
    row "LAN -> WAN" "$(nft_counter 'LAN -> WAN')"
    row "NAT" "$(nft_counter 'NAT via primary WAN IP')"
    row "input drops" "$(nft_counter 'input policy drop')"
    row "forward drops" "$(nft_counter 'forward policy drop')"
    echo

    printf '%sReachability%s\n' "$C_B" "$C_0"
    row "IPv4 ($DNS4_1)" "$(badge "$(ping_ok "$DNS4_1")")"
    row "IPv6 ($DNS6_1)" "$(badge "$(ping_ok -6 "$DNS6_1")")"
    row "resolver" "$(badge "$(ping_ok "$LAN_IP4")")"
}

json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

render_json() {
    printf '{'
    printf '"host":"%s",' "$(json_escape "$(hostname)")"
    printf '"timestamp":"%s",' "$(date -Is)"
    printf '"uptime":"%s",' "$(json_escape "$(uptime_short)")"
    printf '"load":"%s",' "$(json_escape "$(load_avg)")"
    printf '"cpu_temp_c":"%s",' "$(cpu_temp)"
    printf '"conntrack":"%s",' "$(conntrack_count)"
    printf '"interfaces":{'
    local first=1 ifname
    for ifname in "$WAN_IF" "$LAN_IF" "$WIFI_IF" "$BRIDGE_IF"; do
        [[ $first -eq 1 ]] || printf ','
        first=0
        printf '"%s":{"state":"%s","speed":"%s","ipv4":"%s","ipv6":"%s"}' \
            "$ifname" "$(link_state "$ifname")" "$(link_speed "$ifname")" \
            "$(json_escape "$(addr4 "$ifname")")" "$(json_escape "$(addr6 "$ifname")")"
    done
    printf '},'
    printf '"services":{'
    first=1
    local unit
    for unit in systemd-networkd nftables dnsmasq hostapd; do
        [[ $first -eq 1 ]] || printf ','
        first=0
        printf '"%s":"%s"' "$unit" "$(svc_state "$unit")"
    done
    printf '},'
    printf '"clients":{"dhcp_leases":"%s","wifi":"%s","ssid":"%s"},' \
        "$(lease_count)" "$(wifi_clients)" "$(json_escape "$(wifi_ssid)")"
    printf '"counters":{"lan_to_wan":%s,"nat":%s,"input_drops":%s,"forward_drops":%s},' \
        "$(nft_counter 'LAN -> WAN')" "$(nft_counter 'NAT via primary WAN IP')" \
        "$(nft_counter 'input policy drop')" "$(nft_counter 'forward policy drop')"
    printf '"reachability":{"ipv4":"%s","ipv6":"%s","resolver":"%s"}' \
        "$(ping_ok "$DNS4_1")" "$(ping_ok -6 "$DNS6_1")" "$(ping_ok "$LAN_IP4")"
    printf '}\n'
}

case "$MODE" in
    json)  render_json ;;
    watch)
        trap 'printf "\033[?25h"; exit 0' INT TERM
        printf '\033[?25l'
        while true; do
            printf '\033[H\033[J'
            render_human
            printf '\n%s refreshing every %ss -- Ctrl-C to quit%s\n' "$C_D" "$INTERVAL" "$C_0"
            sleep "$INTERVAL"
        done
        ;;
    *) render_human ;;
esac
