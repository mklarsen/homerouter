# homerouter

Turn a small x86 box with multiple NICs into an Ubuntu 24.04 LTS router: static
IPv4/IPv6 WAN, bridged Wi‑Fi access point, DHCP + SLAAC for LAN clients and a
stateful nftables firewall — installed by one idempotent Bash script.

All site-specific values (addresses, prefixes, interfaces, Wi‑Fi credentials)
live in a git-ignored `.env`; the repository only ships templates.

## Features

- Netplan/systemd-networkd WAN with up to three static IPv4 addresses + static IPv6
- LAN bridge combining a cable NIC and the Wi‑Fi AP
- hostapd AP with WPA2/WPA3 mixed mode (or WPA3‑only)
- dnsmasq: DNS cache, DHCPv4 and IPv6 Router Advertisements (SLAAC)
- nftables: `input`/`forward` default DROP, IPv4 NAT, commented port-forward examples
- Idempotent `setup.sh` with `--dry-run`, backups of every replaced file, and a health check

## Interface roles

| Role | Variable | Example |
| --- | --- | --- |
| WAN | `WAN_IF` | `enp3s0` |
| LAN (cable) | `LAN_IF` | `enp4s0` |
| Wi‑Fi AP | `WIFI_IF` | `wlp6s0` (added to the bridge by hostapd at runtime) |
| LAN bridge | `BRIDGE_IF` | `br0` |
| Unused NICs | `UNUSED_IFS` | left unaddressed |

## Repository layout

```text
.
├── config/
│   ├── default/hostapd                      # systemd defaults for hostapd
│   ├── dnsmasq/router.conf.template         # DHCPv4 + RA/SLAAC
│   ├── hostapd/hostapd.conf.template        # SSID/PSK from .env
│   ├── netplan/01-router.yaml.template      # WAN + bridge
│   ├── nftables/nftables.conf.template      # firewall + NAT
│   ├── sysctl/99-router.conf                # forwarding + hardening
│   └── systemd/resolved-no-stub.conf        # free up port 53 for dnsmasq
├── scripts/
│   ├── lib.sh                               # helpers, .env loading, rendering
│   └── verify.sh                            # post-install health check
├── .env.example
├── setup.sh
└── LICENSE
```

## Usage

```bash
git clone https://github.com/mklarsen/homerouter.git
cd homerouter
cp .env.example .env
chmod 600 .env
$EDITOR .env            # ISP addresses, LAN subnets, Wi-Fi SSID/passphrase
sudo ./setup.sh
```

`setup.sh` is idempotent — rerun it after editing `.env` or anything in `config/`.

```bash
sudo ./setup.sh --dry-run    # render + validate, change nothing
sudo ./setup.sh --no-apply   # install files, do not restart services
sudo ./setup.sh --verify     # only run the health checks
```

Every replaced file is backed up to `/var/backups/homerouter/<timestamp>/`.

> **Warning:** applying netplan over SSH can drop your session if the WAN/LAN
> cabling does not match your interface map. Have console access ready the
> first time.

## Configuration

`.env.example` documents every variable and ships with RFC 5737 / RFC 3849
documentation addresses as placeholders. Fill in the values from your ISP:

- `WAN_IP4_PRIMARY` / `WAN_IP4_SECONDARY` / `WAN_IP4_TERTIARY`, `WAN_GW4`
- `WAN_IP6`, `WAN_GW6`
- `LAN_NET6` — the routed prefix (typically a `/48`), `LAN_IP6` — the router
  address inside the `/64` announced to clients
- `LAN_IP4`, `LAN_NET4`, DHCP range
- `WIFI_SSID`, `WIFI_PASSPHRASE`, `WIFI_COUNTRY_CODE`, channel/band

Leave `WAN_IP4_SECONDARY` / `WAN_IP4_TERTIARY` empty if your ISP gave you a
single address; the corresponding netplan entries are dropped automatically.

## Secrets

`.env` is git-ignored and holds the Wi‑Fi passphrase and your public addressing.
The rendered `/etc/hostapd/hostapd.conf` is installed as `root:root` mode `0600`.

## Port forwarding

`config/nftables/nftables.conf.template` ships with commented DNAT examples for
the secondary/tertiary WAN addresses plus matching IPv6 forward examples.
Uncomment, adjust, then rerun `sudo ./setup.sh`.

## Verify

```bash
sudo ./scripts/verify.sh
```

Checks forwarding sysctls, addressing, bridge membership, nftables ruleset,
service state and outbound IPv4/IPv6 reachability.

## Disclaimer

Provided as-is, without warranty of any kind. Running a router directly on the
internet is your own responsibility — review the firewall rules before using
this on a production line.

## License

[MIT](LICENSE) — free to use, modify and share, no liability.

## Support

If this saved you an evening, feel free to
[buy me a coffee](https://buymeacoffee.com/mklarsen).
