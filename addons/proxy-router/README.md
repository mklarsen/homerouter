# Homerouter GOST image

This addon provides a reproducible source build for the authenticated Homerouter proxy. The build uses the official GOST source and the existing `remote-box` Buildx builder on Homerouter.

## Pinned source

- Upstream: <https://github.com/go-gost/gost>
- Release: `v3.3.0`
- Commit: `cb76f63754768c7b5d68895a0d51635b0141b80f`
- Local image tag: `homerouter/gost:3.3.0-local.1`
- Target: `linux/amd64` (Homerouter reports `x86_64`)

The upstream checkout is cloned into `gost-source/` on first build and is excluded from Git. Its own Dockerfile builds the GOST binary from the pinned source. Docker still downloads the upstream builder and Alpine base images; the final GOST image is built by Homerouter rather than pulled as `gogost/gost`.

## Build and stage

Requirements: Docker Buildx builder `remote-box` configured for `ssh://root@10.10.10.1`, Git, and SSH/SCP access to Homerouter.

```powershell
.\addons\proxy-router\Build-GostImage.ps1
.\addons\proxy-router\Transfer-GostImage.ps1
```

The build script verifies the source commit and confirms that `remote-box` targets Homerouter. BuildKit on Homerouter compiles the source for `linux/amd64`; only the resulting Docker archive is exported to `addons/proxy-router/dist/`. The transfer script copies that archive and the Compose override to `/opt/stacks`, runs `docker load`, and verifies the image tag. It does **not** restart or replace the running proxy service.

## Apply future image updates

The `homerouter/gost:3.3.0-local.1` image is currently active on Homerouter. For a future rebuilt image, stage it with `Transfer-GostImage.ps1`, then recreate only the proxy service on Homerouter:

```sh
cd /opt/stacks
docker compose -f docker-compose.yml -f homerouter-gost-image.yml up -d --no-deps vpn-proxy
```

The override uses `pull_policy: never`, so Compose will not fall back to a public GOST image. This recreates only `vpn-proxy` and briefly interrupts proxy traffic. The base stack, Traefik, labels, and runtime command remain unchanged.

The existing proxy authentication command in the shared stack is not modified here. Replace any weak credentials with a strong unique secret before exposing the proxy publicly.
