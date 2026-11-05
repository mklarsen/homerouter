# homerouter-vpn

WireGuard access for the router with Authentik (OIDC) sign-in for peer
enrolment. This is a proof of concept designed to be re-brandable for other
platforms.

Status: **phase 3**. Users sign in with Authentik, create their own peers and
lose them automatically when their account is disabled or leaves the group.
A token-protected admin API is also available (see `openapi.yaml` and the
`bruno/` collection). The `fake` backend runs anywhere; the `wg` backend needs
a Linux host.

## Roadmap

1. Skeleton, container, optional firewall section (done).
2. `wg` interface and peer lifecycle with a fake backend for local tests (done).
3. Authentik OIDC sign-in, group gating and automatic peer revocation (this phase).
4. Enrolment UI with `.conf` and QR download.
5. Image publishing and Authentik setup guide.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `VPN_LISTEN` | `:8081` | HTTP listen address |
| `VPN_BRAND_NAME` | `VPN` | Product name shown to users (max 64 chars) |
| `VPN_BRAND_COLOR` | `#2563eb` | Accent colour, `#rrggbb` |
| `VPN_BRAND_LOGO_URL` | empty | `https://` URL or absolute path |
| `VPN_API_TOKEN` | empty | Bearer token (32+ chars). Unset disables the peer API |
| `VPN_BACKEND` | `fake` | `fake` (in memory) or `wg` (drives `wg`/`ip`, needs host network and `NET_ADMIN`) |
| `VPN_IF` | `wg0` | Interface name |
| `VPN_NET4` | `10.10.30.0/24` | Peer subnet (IPv4, /29 or larger); the first host is the server |
| `VPN_PORT` | `51820` | WireGuard UDP port |
| `VPN_ENDPOINT` | placeholder | Public `host:port` clients connect to; required with `wg` |
| `VPN_DNS` | empty | DNS server written into client configs |
| `VPN_ALLOWED_IPS` | `VPN_NET4` | Comma-separated routes clients send through the tunnel |
| `VPN_DATA_DIR` | `/data` | Holds `peers.json`, `owners.json` and `server.key` (all mode 0600) |
| `VPN_OIDC_ISSUER` | empty | Authentik issuer URL; setting it enables SSO |
| `VPN_OIDC_CLIENT_ID` | | Required with SSO |
| `VPN_OIDC_CLIENT_SECRET` | empty | Needed for a confidential client |
| `VPN_OIDC_REDIRECT_URL` | | `https://<host>/auth/callback` (http only for localhost) |
| `VPN_OIDC_GROUP` | `vpn-users` | Group a user must be in |
| `VPN_MAX_PEERS_PER_USER` | `5` | Peers one user may hold |
| `VPN_PEER_TTL` | `0` | Self-service peer lifetime, e.g. `720h`; `0` = no expiry |
| `VPN_RECONCILE_INTERVAL` | `5m` | How often accounts and expiry are re-checked (min `30s`) |

Private keys of generated peers are returned once in the create response and
are never stored. Clients may instead send their own `publicKey`.

## Single sign-on with Authentik

1. Create an OAuth2/OpenID provider (authorization code, PKCE). Redirect URI:
   `https://<host>/auth/callback`. Signing key: an RSA certificate.
2. Scopes: `openid`, `profile`, `email`, `offline_access`. The `profile` mapping
   must put the user's `groups` into the ID token.
3. Create an application for the provider and a group `vpn-users`; bind the
   group to the application.
4. Set the `VPN_OIDC_*` variables. The issuer is the provider's
   `https://<authentik>/application/o/<slug>/` URL.

Flow: `GET /auth/login` redirects to Authentik; the callback verifies the ID
token (signature, audience, expiry, nonce) and the group, then sets an
HttpOnly session cookie. `GET /api/v1/me`, and `GET`/`POST /api/v1/me/peers`
and `DELETE /api/v1/me/peers/{id}` act on the signed-in user's own peers only.

Revocation: every `VPN_RECONCILE_INTERVAL` the service refreshes each owner's
refresh token. A rejected refresh (account disabled) or an ID token without the
group removes that user's peers and sessions. A provider outage keeps peers in
place. Without `offline_access` there is no refresh token, so only
`VPN_PEER_TTL` limits a peer's life.

`owners.json` holds those refresh tokens and must be protected like
`server.key`.

## Local build and test

```bash
cd addons/homerouter-vpn
go test ./...
docker build -t homerouter-vpn:dev .
docker run --rm -p 8081:8081 -e VPN_BRAND_NAME="Example" \
  -e VPN_API_TOKEN="$(openssl rand -hex 24)" homerouter-vpn:dev
curl http://127.0.0.1:8081/api/v1/brand
```

## Firewall

The router firewall has an optional WireGuard section, off by default. Set
`VPN_ENABLE="1"` in `.env` and rerun `setup.sh` to open the UDP port on the WAN
and accept tunnel traffic. Peers cannot reach the LAN unless `VPN_TO_LAN="1"`.
The `wg` backend creates the interface when the container starts.

## Trademark

"WireGuard" is a registered trademark of Jason A. Donenfeld. This project is
not affiliated with or endorsed by the WireGuard project. WireGuard tooling is
licensed under GPLv2; keep attribution when redistributing it.
