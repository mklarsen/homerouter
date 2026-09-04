# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the project follows [Semantic Versioning](https://semver.org/).

**How releases work:** add your entry under `Unreleased` in the same pull
request as the change. When you want to publish, rename `Unreleased` to
`## [x.y.z] - YYYY-MM-DD` (and add a fresh, empty `Unreleased` above it).
Merging that pull request builds and publishes the release, using the section
as the release notes. Merges without a new version heading do not release.

## [Unreleased]

## [0.4.1] - 2026-09-04

### Fixed

- `dnsutils` was reported as missing on every run: it is a transitional package
  on recent Ubuntu releases, so dpkg never considers it installed. The presence
  of `dig` is checked instead, and `bind9-dnsutils` is installed when available.

## [0.4.0] - 2026-09-04

### Added
- `DOCKER_COMPAT` for boxes that also run Docker: forward rules for `docker0`
  and `br-*` bridges, so container traffic is not caught by the drop policy.
  `setup.sh` detects a running Docker daemon, warns when the option is off, and
  restarts Docker after loading the ruleset so it can reinstall its own chains.

### Fixed

- Wrong copyright holder in `LICENSE`.

### Changed

- Documented that the spare `enp5s0f*np*` interfaces are SFP/SFP+ cage ports,
  which stay `NO-CARRIER` until a transceiver is fitted.

## [0.3.1] - 2026-09-04

### Fixed

- A broken third-party apt repository no longer aborts the installation:
  `apt-get update` failures are reported as a warning and the install continues
  with the package lists already on disk.

## [0.3.0] - 2026-09-04

### Added

- `CONTRIBUTING.md` covering the development workflow, release process and an
  operational handover section for whoever runs the box.

### Changed

- `install.sh` installs the runtime packages, since it always runs while the box
  still has its original internet connection. `setup.sh` keeps the same logic as
  a fallback, so a deploy after the uplink has been reconfigured no longer
  depends on `apt`. Opt out with `--no-packages`.

## [0.2.0] - 2026-09-04

### Added

- `CHANGELOG.md`, which now drives releases: the newest version heading decides
  the tag, and its section becomes the release notes.
- `scripts/changelog.sh` for reading and validating the changelog, wired into CI.

### Changed

- Releases are published when a merge introduces a new version heading instead
  of on every merge to `main`.

## [0.1.5] - 2026-09-04

### Added

- Optional server/DMZ segment: a separate layer-2 network (default
  `10.10.20.0/24` on `br1`) for self-hosted services, enabled with `SRV_ENABLE=1`.
- Per-segment DHCPv4, SLAAC/RA and DNS options in dnsmasq, tagged so settings
  never leak between segments.
- Firewall policy for the new segment: outbound and LAN-to-server traffic is
  allowed, server-to-LAN is blocked unless `SRV_TO_LAN=1`, and the router only
  answers DNS/DHCP/ping there.
- Dedicated SNAT for the server segment via the secondary WAN address, so hosted
  services get their own public identity.
- `verify.sh` checks for the server bridge, its uplink and the isolation rule.

### Changed

- Default LAN moved from `192.168.1.0/24` to `10.10.10.0/24`.
- `selftest.sh` renders and validates both segment states on every run.
- Port-forwarding examples now target the server subnet.

## [0.1.4] - 2026-09-04

### Added

- Branch ruleset for `main` tracked as code: pull request required, CI must pass,
  force-push and deletion blocked.

## [0.1.3] - 2026-09-04

### Added

- `ci` workflow: ShellCheck, template rendering, `nft --check`, `dnsmasq --test`,
  netplan YAML parsing and a guard against committed site-specific values.
- `release` workflow: builds a versioned tarball with `SHA256SUMS` and publishes
  a GitHub release.
- `install.sh` bootstrap installer with checksum verification, for the
  `curl | sudo bash` one-liner.
- `scripts/selftest.sh` for running the same validation locally.

### Fixed

- ShellCheck findings in `setup.sh`, `scripts/lib.sh` and `scripts/selftest.sh`.

## [0.1.2] - 2026-09-04

### Added

- Initial release: Ubuntu 24.04 LTS router and Wi-Fi access point.
- Netplan WAN with up to three static IPv4 addresses plus static IPv6, and a LAN
  bridge for cable and Wi-Fi clients.
- hostapd access point with WPA2/WPA3 mixed mode or WPA3-only.
- dnsmasq DNS cache, DHCPv4 and IPv6 router advertisements.
- nftables stateful firewall with default-drop input/forward, IPv4 NAT and
  commented port-forwarding examples.
- Idempotent `setup.sh` with `--dry-run`, `--no-apply` and `--verify`, backups of
  every replaced file, and `scripts/verify.sh` health checks.
- All site-specific values moved to a git-ignored `.env`; the repository ships
  only templates.
