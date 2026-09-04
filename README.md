# homerouter

[![ci](https://github.com/mklarsen/homerouter/actions/workflows/ci.yml/badge.svg)](https://github.com/mklarsen/homerouter/actions/workflows/ci.yml)
[![release](https://github.com/mklarsen/homerouter/actions/workflows/release.yml/badge.svg)](https://github.com/mklarsen/homerouter/actions/workflows/release.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Turn a small x86 box with multiple NICs into an Ubuntu 24.04 LTS router: static
IPv4/IPv6 WAN, bridged Wi‑Fi access point, DHCP + SLAAC for LAN clients and a
stateful nftables firewall — installed by one idempotent Bash script.

All site-specific values (addresses, prefixes, interfaces, Wi‑Fi credentials)
live in a git-ignored `.env`; the repository only ships templates.

## Features

- Netplan/systemd-networkd WAN with up to three static IPv4 addresses + static IPv6
- LAN bridge combining a cable NIC and the Wi‑Fi AP
- Optional second segment for self-hosted services, isolated from the LAN
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
| LAN bridge | `BRIDGE_IF` | `br0` — `10.10.10.1/24` |
| Server segment (optional) | `SRV_IF` / `SRV_BRIDGE_IF` | `enp5s0f0np0` / `br1` — `10.10.20.1/24` |
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
│   ├── changelog.sh                          # read/validate CHANGELOG.md
│   ├── lib.sh                                # helpers, .env loading, rendering
│   ├── selftest.sh                           # render + validate, no system changes
│   └── verify.sh                             # post-install health check
├── .env.example
├── CHANGELOG.md
├── install.sh                                # bootstrap: fetch latest release
├── setup.sh
└── LICENSE
```

## Install (latest release)

One-liner on a fresh Ubuntu 24.04 box — downloads the newest release, verifies
its SHA-256 checksum and unpacks it to `/opt/homerouter`:

```bash
curl -fsSL https://raw.githubusercontent.com/mklarsen/homerouter/main/install.sh | sudo bash
```

Then configure and deploy:

```bash
sudo nano /opt/homerouter/.env      # ISP addresses, LAN subnet, Wi-Fi
sudo /opt/homerouter/setup.sh --dry-run
sudo /opt/homerouter/setup.sh
```

Options are passed after `bash -s --`:

```bash
# pin a version, install elsewhere, and run setup.sh right away
curl -fsSL https://raw.githubusercontent.com/mklarsen/homerouter/main/install.sh \
  | sudo bash -s -- --version v0.1.7 --dir /opt/homerouter --run
```

Rerunning the one-liner upgrades an existing install in place; your `.env` is
never overwritten.

## Install (from source)

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

## Two networks: private LAN + hosted services

The default LAN is `10.10.10.0/24`. If you want to host things without exposing
your private devices, enable the optional server segment — a separate layer‑2
network on one of the spare NICs, not just another subnet:

```bash
SRV_ENABLE="1"
SRV_IF="enp5s0f0np0"       # must NOT also appear in UNUSED_IFS
SRV_BRIDGE_IF="br1"
SRV_IP4="10.10.20.1"
SRV_NET4="10.10.20.0/24"
SRV_DHCP4_START="10.10.20.100"
SRV_DHCP4_END="10.10.20.200"
SRV_IP6="2a13:0db8:100:2::1"   # second /64 out of the same routed prefix
SRV_TO_LAN="0"                 # 1 = servers may also open connections into the LAN
```

What you get after `sudo ./setup.sh`:

| Direction | Default |
| --- | --- |
| LAN → internet | allowed (NAT via `WAN_IP4_PRIMARY`) |
| Servers → internet | allowed (NAT via `WAN_IP4_SECONDARY`, so hosted services get their own public identity) |
| LAN → servers | allowed |
| Servers → LAN | **blocked** (`SRV_TO_LAN=1` opens it) |
| Servers → router | DNS/DHCP/ping only, no SSH |
| Internet → servers | blocked until you uncomment a port forward |

DHCP, SLAAC and DNS are served on both segments with per-segment options, and
the port-forwarding examples in the nftables template already point at
`10.10.20.x`. Plug a switch into `SRV_IF` and everything on it lands in the
server network.

With `SRV_ENABLE=0` every server-segment line is rendered as a comment, so the
files on disk always show exactly what is and is not active.

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

To validate the configuration without touching the system (also what CI runs):

```bash
./scripts/selftest.sh
```

## Development & releases

- Work happens on feature branches; `main` is the only long-lived branch and is
  protected — merging requires a pull request with green CI.
- Every pull request runs [ci.yml](.github/workflows/ci.yml): ShellCheck,
  changelog validation, template rendering, `nft --check`, `dnsmasq --test`,
  netplan YAML parsing and a guard against committed site-specific values.
- Releases are driven by [CHANGELOG.md](CHANGELOG.md). Add your entry under
  `Unreleased` as part of your pull request; when you want to publish, rename
  that heading to `## [x.y.z] - YYYY-MM-DD` and put a fresh empty `Unreleased`
  above it.
- On merge, [release.yml](.github/workflows/release.yml) reads the newest
  version heading. If it has not been released yet, it builds
  `homerouter-<version>.tar.gz` + `SHA256SUMS` and publishes a GitHub release
  using that changelog section as the notes. Merges without a new version
  heading simply do not release.
- `install.sh` always resolves `releases/latest`, so the one-liner installs the
  most recently published version.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow, coding rules and
the operational handover notes.

## Disclaimer

Provided as-is, without warranty of any kind. Running a router directly on the
internet is your own responsibility — review the firewall rules before using
this on a production line.

## License

[MIT](LICENSE) — free to use, modify and share, no liability.

## Support

If this saved you an evening, feel free to
[buy me a coffee](https://buymeacoffee.com/mklarsen).
