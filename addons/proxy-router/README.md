# Homerouter GOST image

This addon publishes the Homerouter proxy image to GitHub Container Registry as [`ghcr.io/mklarsen/homerouter-gost`](https://github.com/mklarsen/homerouter/pkgs/container/homerouter-gost). No GOST source checkout or image archive is needed in the local repository.

## Pinned source

- Upstream: <https://github.com/go-gost/gost>
- Release: `v3.3.0`
- Commit: `cb76f63754768c7b5d68895a0d51635b0141b80f`
- Published image: `ghcr.io/mklarsen/homerouter-gost:3.3.0`
- Target: `linux/amd64` (Homerouter reports `x86_64`)

The GitHub Actions workflow builds from the upstream GOST repository at the pinned commit and publishes version, commit, and `latest` tags to GHCR. Publishing is restricted to workflow runs on `main`. The image uses upstream's Dockerfile and includes OCI source, license, and vendor labels. Docker pulls the upstream Go and Alpine base images during the build.

## Build and publish

The workflow runs when its relevant files change on `main`, or can be started manually from the Actions tab with **Publish Homerouter GOST image**. It needs no custom registry secret: GitHub Actions uses the repository-scoped `GITHUB_TOKEN` with `packages: write`.

After the first successful publish, set the `homerouter-gost` package visibility to **Public** in GitHub Packages (`https://github.com/users/mklarsen/packages/container/package/homerouter-gost`). GitHub creates new container packages as private by default; public visibility is required for Homerouter to pull anonymously.

To switch Homerouter from the currently running local image after the package is public, copy `docker-compose.ghcr.yml` to `/opt/stacks/homerouter-gost-image.yml`, then run:

```sh
cd /opt/stacks
docker compose -f docker-compose.yml -f homerouter-gost-image.yml pull vpn-proxy
docker compose -f docker-compose.yml -f homerouter-gost-image.yml up -d --no-deps vpn-proxy
```

This only recreates `vpn-proxy`; it briefly interrupts proxy traffic.

## Upstream credit and license

This is a Homerouter-maintained derivative image built from [GOST](https://github.com/go-gost/gost) v3.3.0; it is not an official GOST image. GOST is copyright its upstream authors and distributed under the MIT License, preserved in [`LICENSE.GOST`](LICENSE.GOST). Homerouter's own code and configuration remain under the repository-root license.

The existing proxy authentication command in the shared stack is not modified here. Replace any weak credentials with a strong unique secret before exposing the proxy publicly.
