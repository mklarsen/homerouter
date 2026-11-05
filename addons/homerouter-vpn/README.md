# homerouter-vpn

WireGuard access for the router, with Authentik (OIDC) sign-in planned for
peer enrolment. This is a proof of concept designed to be re-brandable for
other platforms.

Status: **phase 2**. Peer management works through a token-protected API
(see `openapi.yaml` and the `bruno/` collection). The `fake` backend runs
anywhere; the `wg` backend needs a Linux host.

## Roadmap

1. Skeleton, container, optional firewall section (done).
2. `wg` interface and peer lifecycle with a fake backend for local tests (this phase).
3. Authentik OIDC sign-in, group gating and automatic peer revocation.
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
| `VPN_DATA_DIR` | `/data` | Holds `peers.json` and `server.key` (mode 0600) |

Private keys of generated peers are returned once in the create response and
are never stored. Clients may instead send their own `publicKey`.

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
