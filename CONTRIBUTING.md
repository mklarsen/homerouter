# Contributing & handover

This document covers both sides: how to work on the code, and what you need to
know if you take over running a box built from it.

## Overview

`homerouter` turns an Ubuntu 24.04 LTS machine into a router, firewall and Wi-Fi
access point. Everything site-specific lives in a git-ignored `.env`; the
repository only ships templates that `setup.sh` renders and installs.

| Rendered from | Installed to | Service |
| --- | --- | --- |
| `config/netplan/01-router.yaml.template` | `/etc/netplan/01-router.yaml` | systemd-networkd |
| `config/nftables/nftables.conf.template` | `/etc/nftables.conf` | nftables |
| `config/dnsmasq/router.conf.template` | `/etc/dnsmasq.d/router.conf` | dnsmasq |
| `config/hostapd/hostapd.conf.template` | `/etc/hostapd/hostapd.conf` | hostapd |
| `config/sysctl/99-router.conf` | `/etc/sysctl.d/99-router.conf` | kernel forwarding |
| `config/systemd/resolved-no-stub.conf` | `/etc/systemd/resolved.conf.d/homerouter.conf` | systemd-resolved |

`scripts/lib.sh` holds the shared helpers: `load_env` (defaults + derived
values), `render_template` (envsubst with an explicit variable list) and
`install_managed` (content-compare, backup, install).

## Development setup

You do not need a router to work on this. Everything but the final `setup.sh`
run can be validated on any Linux box or in WSL:

```bash
sudo apt-get install -y shellcheck nftables dnsmasq-base gettext-base python3-yaml
git clone https://github.com/mklarsen/homerouter.git
cd homerouter
./scripts/selftest.sh          # renders every template and validates the result
shellcheck -x setup.sh install.sh scripts/*.sh
```

`selftest.sh` falls back to `.env.example` when no `.env` exists, and always
renders both states of the optional server segment.

## Workflow

1. Branch off `main` (`feat/…`, `fix/…`, `docs/…`, `chore/…`). `main` is
   protected: no direct pushes, no force-pushes.
2. Make the change. If you touch a template, run `./scripts/selftest.sh`.
3. Add a bullet under `## [Unreleased]` in `CHANGELOG.md`.
4. Open a pull request. CI must be green before it can be merged.
5. Squash-merge.

### Adding a configuration knob

1. Add the variable to `.env.example` with a documented placeholder value.
2. Add a default in `load_env` in `scripts/lib.sh`, and add the name to
   `TEMPLATE_VARS` so `render_template` substitutes it.
3. Use `${VAR}` in the relevant template.
4. Add a check to `scripts/verify.sh` if the setting is observable at runtime.
5. Run `./scripts/selftest.sh` and update `CHANGELOG.md`.

Optional features follow the comment-gating pattern: a variable such as
`SRV_CMT` is either empty or `"# "`, and every line belonging to the feature is
prefixed with it. Disabled features are then visible as comments in the
installed file instead of disappearing.

## Rules of the road

- **Never commit `.env`, real public IPs, prefixes or passphrases.** CI fails on
  the addresses of the reference deployment; keep new code free of any.
- Bash scripts use `set -Eeuo pipefail`, are ShellCheck-clean and idempotent —
  rerunning `setup.sh` on an unchanged repo must change nothing.
- Prefer editing a template over adding a new file. Configuration belongs in
  `config/`, logic in `scripts/`.
- Firewall changes: keep `input` and `forward` default DROP, and add examples as
  comments rather than enabling them by default.
- Every file `setup.sh` replaces is backed up under
  `/var/backups/homerouter/<timestamp>/`; do not break that.

## Releasing

Releases come from `CHANGELOG.md`:

1. In your pull request, rename `## [Unreleased]` to `## [x.y.z] - YYYY-MM-DD`
   and add a fresh empty `## [Unreleased]` above it.
2. Merge. `release.yml` re-runs the checks, builds
   `homerouter-vx.y.z.tar.gz` + `SHA256SUMS` and publishes the release with that
   section as the notes.
3. Merges without a new version heading do not publish anything.

Version numbering: patch for fixes, minor for new features or changed defaults,
major once the configuration format is stable and breaks.

## Operational handover

What an operator taking over the running box needs:

- **Access:** console or KVM. Applying netplan can drop an SSH session if the
  cabling does not match the interface map, so never deploy blind.
- **Source of truth:** `/opt/homerouter` (installed by `install.sh`) with the
  live values in `/opt/homerouter/.env`, mode `0600`. Nothing is configured by
  hand — edit `.env` or a template and rerun `setup.sh`.
- **Deploy:** `sudo /opt/homerouter/setup.sh --dry-run`, then without the flag.
- **Health check:** `sudo /opt/homerouter/scripts/verify.sh`.
- **Upgrade:** rerun the install one-liner; it keeps the existing `.env`.
- **Rollback:** copy the files back from `/var/backups/homerouter/<timestamp>/`
  and run `netplan apply`, `nft --file /etc/nftables.conf`, then restart the
  affected services.
- **Logs:** `journalctl -u hostapd -u dnsmasq -u systemd-networkd -n 100`,
  plus `/var/log/dnsmasq.log` and `nft list ruleset` for dropped traffic.
- **Secrets to hand over:** the Wi-Fi passphrase in `.env`, ISP credentials and
  console access. Nothing else is stored on the box by this project.

### Common failures

| Symptom | Cause |
| --- | --- |
| `hostapd` fails to start | Wi-Fi interface already enslaved to the bridge by netplan, or the radio is rfkill-blocked |
| Clients get no IPv4 | dnsmasq bound before the bridge existed — restart dnsmasq |
| No IPv6 on clients | ICMPv6/NDP blocked, or the routed prefix is not actually routed to your WAN address |
| Port 53 in use | systemd-resolved stub listener; `config/systemd/resolved-no-stub.conf` disables it |
| No internet after deploy | Check `nft list table ip nat` for the SNAT rule and that the WAN address matches `.env` |

## Getting help

Open an issue with the output of `sudo ./scripts/verify.sh` and the relevant
`journalctl` lines. Redact your public addresses and passphrase first.

If you work through an AI coding agent, [AGENTS.md](AGENTS.md) is the equivalent
briefing for it.
