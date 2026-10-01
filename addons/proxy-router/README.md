# Homerouter GOST image

This addon contains the minimal Go source needed to build a Homerouter-branded GOST derivative and publish it to GitHub Container Registry as `ghcr.io/mklarsen/homerouter-proxy`.

## Pinned source

- Upstream: <https://github.com/go-gost/gost>
- Release: `v3.3.0`
- Commit: `cb76f63754768c7b5d68895a0d51635b0141b80f`
- Published image: `ghcr.io/mklarsen/homerouter-proxy:3.3.0`
- Target: `linux/amd64` (Homerouter reports `x86_64`)

The GOST Go source, upstream Dockerfile, module files, and license are vendored in `upstream/`. The snapshot is pinned to the commit above and omits upstream `.git`, `.github` CI/nightly workflows, tests, and release tooling. This repository has no automatic or scheduled proxy-image build; publishing is manual only.

## Manual build and publish

The build runs on Homerouter through the existing Docker Buildx `remote-box` builder. The local Docker CLI sends the vendored source directory to that Linux builder, which builds `upstream/Dockerfile` and pushes directly to GHCR. No image tarball or temporary source clone is needed.

Grant the GitHub CLI package-write scope once, then run the manual publisher:

```powershell
gh auth refresh -h github.com -s write:packages
.\addons\proxy-router\Publish-GostImage.ps1
```

The script verifies the upstream commit and `remote-box` endpoint, then pushes `3.3.0`, `3.3.0-cb76f63`, and `latest` tags to GHCR. GitHub creates new packages as private by default. After the first push, set `homerouter-proxy` to **Public** in [GitHub Packages](https://github.com/users/mklarsen/packages/container/package/homerouter-proxy); otherwise Homerouter cannot pull it anonymously.

After verifying an anonymous pull from `10.10.10.1`, copy `docker-compose.ghcr.yml` to `/opt/stacks/homerouter-gost-image.yml` and switch only `vpn-proxy`:

```sh
cd /opt/stacks
docker pull ghcr.io/mklarsen/homerouter-proxy:3.3.0
docker compose -f docker-compose.yml -f homerouter-gost-image.yml pull vpn-proxy
docker compose -f docker-compose.yml -f homerouter-gost-image.yml up -d --no-deps vpn-proxy
```

This only recreates `vpn-proxy`; it briefly interrupts proxy traffic.

## Upstream credit and license

This is a Homerouter-maintained derivative image built from [GOST](https://github.com/go-gost/gost) v3.3.0; it is not an official GOST image. GOST is copyright its upstream authors and distributed under the MIT License, preserved in [`LICENSE.GOST`](LICENSE.GOST). The upstream source, release, commit, and exclusions are recorded in [`UPSTREAM.md`](UPSTREAM.md); the upstream English README is retained under `upstream/`. Homerouter's own code and configuration remain under the repository-root license.

The existing proxy authentication command in the shared stack is not modified here. Replace any weak credentials with a strong unique secret before exposing the proxy publicly.
