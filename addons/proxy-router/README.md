# Homerouter Proxy

This addon is an independently maintained HTTP/CONNECT forward proxy written in Go using only the standard library. It serves an administrator interface at `/vpnadm` for managing proxy credentials.

## Behavior

- HTTP Basic proxy authentication; unauthenticated requests receive `407 Proxy Authentication Required`.
- HTTP forwarding and HTTPS CONNECT tunnels are supported on destination ports 80 and 443.
- Private, loopback, link-local, multicast, reserved, and documentation destinations are rejected.
- `/vpnadm` provides admin login, mandatory bootstrap-password rotation, and proxy-user create/disable/delete operations.
- `/api/v1` is a versioned JSON REST API with list/create/read/update/delete user operations. Browser sessions and optional Bearer-token service authentication are supported.
- `/api/v1/openapi.yaml` publishes the API contract; `bruno/` contains runnable request examples.
- Credentials are salted PBKDF2-HMAC-SHA256 hashes in an atomic JSON file under `/data`.
- The admin overview reports the configured host data path and active/disabled user counts.
- The container runs as an unprivileged UID.

Set `PROXY_ADMIN_PASSWORD` to a unique secret before the first start. The default admin username is `admin`; its first successful login must change the bootstrap password. Changing the environment variable after the user database exists does not reset the password.

The proxy is intended to sit behind the existing Traefik TLS router. Keep `/vpnadm` on HTTPS and do not expose the container's HTTP listener directly to the internet.

## REST API

The OpenAPI document is available at `https://vpn.homerouter.io/api/v1/openapi.yaml`. Browser requests may use the `homerouter_admin` session cookie. Trusted backend services may use `Authorization: Bearer <PROXY_API_TOKEN>`; configure a random token of at least 32 characters in `/opt/stacks/.env`. Never embed that service token in browser code.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/status` | API, storage path, and active/disabled counts |
| `GET` | `/api/v1/users` | List user metadata |
| `POST` | `/api/v1/users` | Create a user with `{"username":"alice","password":"<secret>"}` |
| `GET` | `/api/v1/users/{username}` | Read one user's metadata |
| `PATCH` | `/api/v1/users/{username}` | Update `password`, `disabled`, or both |
| `DELETE` | `/api/v1/users/{username}` | Delete a user |

Passwords and hashes are never returned. Current service authentication is a single static Bearer token; Authentik/OIDC is not implemented yet. It can be added later as a token-validation provider while keeping these resource routes stable.

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
.\addons\proxy-router\Publish-ProxyImage.ps1 -Version 1.1.0
```

The script verifies that Buildx builder `remote-box` targets Homerouter at `10.10.10.1`, then pushes versioned and `latest` tags to GHCR. GitHub may create the package as private. Set `homerouter-proxy` to **Public** in [GitHub Packages](https://github.com/users/mklarsen/packages/container/package/homerouter-proxy) before relying on anonymous pulls from Homerouter.

The Compose overlay bind-mounts `/opt/stacks/proxy-data` to `/data`; create that directory as UID/GID 10001 with mode `0700` before starting the container. `users.json` remains owned by UID 10001 with mode `0600`. Set `PROXY_ADMIN_PASSWORD` in `/opt/stacks/.env` with mode `0600`. Set `PROXY_API_TOKEN` there to enable Bearer authentication for trusted backend services; configured tokens must be at least 32 characters, and an empty value disables token auth. The admin overview shows the host and container paths. When merging the overlay with the base stack, existing network and Traefik labels are preserved and the legacy GOST command is cleared. Back up the active volume and Compose file under `/BACKUPS/copilot` before switching mounts; recreating `vpn-proxy` briefly interrupts proxy traffic.

For machine-to-machine CRUD, send `Authorization: Bearer <PROXY_API_TOKEN>` over HTTPS to `/api/v1/users`. Do not put the token in browser JavaScript. The same endpoints also accept the administrator's secure session cookie for the browser UI. Authentik/OIDC is not enabled yet; it can be added later as another authentication provider without changing the versioned resource routes.

## Historical GOST attribution

The previous image used GOST v3.3.0. This implementation does not contain or derive from GOST source. [`UPSTREAM.md`](UPSTREAM.md) records that history, and [`LICENSE.GOST`](LICENSE.GOST) is retained for historical attribution.
