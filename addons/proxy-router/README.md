# Homerouter Proxy

This addon is an independently maintained HTTP/CONNECT forward proxy written in Go using only the standard library. It serves an administrator interface at `/vpnadm` for managing proxy credentials.

## Behavior

- HTTP Basic proxy authentication; unauthenticated requests receive `407 Proxy Authentication Required`.
- HTTP forwarding and HTTPS CONNECT tunnels are supported on destination ports 80 and 443.
- Private, loopback, link-local, multicast, reserved, and documentation destinations are rejected.
- `/vpnadm` provides admin login, mandatory bootstrap-password rotation, and proxy-user create/disable/delete operations.
- Credentials are salted PBKDF2-HMAC-SHA256 hashes in an atomic JSON file under `/data`.
- The container runs as an unprivileged UID.

Set `PROXY_ADMIN_PASSWORD` to a unique secret before the first start. The default admin username is `admin`; its first successful login must change the bootstrap password. Changing the environment variable after the user database exists does not reset the password.

The proxy is intended to sit behind the existing Traefik TLS router. Keep `/vpnadm` on HTTPS and do not expose the container's HTTP listener directly to the internet.

## Manual build and publish

Run tests locally with Go 1.26 or newer:

```powershell
cd addons/proxy-router
go test ./...
go build ./...
```

The multi-stage Dockerfile also runs the tests before building the `linux/amd64` image. Image publication is manual only; no scheduled or automatic proxy-image workflow is used.

Use an existing GitHub CLI login with package-write access and run the publisher. It does not modify authentication scopes:

```powershell
.\addons\proxy-router\Publish-ProxyImage.ps1 -Version 1.0.0
```

The script verifies that Buildx builder `remote-box` targets Homerouter at `10.10.10.1`, then pushes versioned and `latest` tags to GHCR. GitHub may create the package as private. Set `homerouter-proxy` to **Public** in [GitHub Packages](https://github.com/users/mklarsen/packages/container/package/homerouter-proxy) before relying on anonymous pulls from Homerouter.

Before changing the live Compose stack, make a backup under `/BACKUPS/copilot`. Set `PROXY_ADMIN_PASSWORD` in `/opt/stacks/.env` with mode `0600`; the overlay requires it and creates the persistent `homerouter-proxy-users` volume. When merged with the base stack, it preserves the existing network and Traefik labels while replacing the GOST command. Recreating `vpn-proxy` briefly interrupts proxy traffic.

## Historical GOST attribution

The previous image used GOST v3.3.0. This implementation does not contain or derive from GOST source. [`UPSTREAM.md`](UPSTREAM.md) records that history, and [`LICENSE.GOST`](LICENSE.GOST) is retained for historical attribution.
