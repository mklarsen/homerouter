# Traefik Routing Fix - November 3, 2026

## Problem
The hawk.homerouter.io hostname returned **502 Bad Gateway** even though:
- File-based routing config existed (`/opt/stacks/dynamic/all-routes.yml`)
- Direct port access worked (`http://10.10.10.1:3002`)
- Traefik container was running

## Root Cause
**Network mode mismatch**: hawk-ui container was configured with `network_mode: host`, which means:
- Container shares the host's network stack directly
- Container **cannot** communicate with other Docker containers via their container names
- When Traefik tried to reach `http://homerouter-hawk-ui:3002`, DNS resolution failed
- This caused Traefik to report 502 Bad Gateway

## Solution
**Split network configuration**:

### Before
```yaml
homerouter-hawk-ui:
  network_mode: host  # ❌ Breaks Traefik Docker routing
```

### After
```yaml
homerouter-hawk-ui:
  networks:
    - traefik-edge  # ✅ Allows Traefik to reach it via container DNS

homerouter-hawk:
  network_mode: host  # ✅ Needed for IPv6 detection (direct access to host's network interfaces)
```

## Why This Works

1. **hawk-ui on `traefik-edge` network**: 
   - Traefik (also on `traefik-edge`) can now reach `homerouter-hawk-ui:3002` via Docker internal DNS
   - Routes correctly to the container's port 3002
   - Still binds port 3002 to host IP `10.10.10.1` for direct access

2. **hawk backend on `host` network**:
   - Needed to detect real IPv6 address (host's `2a13:e740:2f1:d5cb::2`)
   - Connects to `api64.ipify.org` and `api.ipify.org` with proper source IPs
   - hawk-ui connects to hawk backend via `localhost` (both on host network)

## Verification

### From router (10.10.10.1)
```bash
curl -s -I http://hawk.homerouter.io/
# HTTP/1.1 200 OK ✅

curl -s -I https://hawk.homerouter.io/
# HTTP/2 200 OK ✅
```

### From local machine (Windows)
```bash
curl -s -I https://hawk.homerouter.io/
# HTTP/1.1 200 OK ✅
```

## Changes Made
- **File**: `/opt/stacks/docker-compose.yml`
  - Line: hawk-ui service definition
  - Changed: `network_mode: host` → `networks: [traefik-edge]`
  - Backup: `docker-compose.yml.bak` created on router

## Result
✅ hawk.homerouter.io now accessible via Traefik
✅ Direct port access still works (http://10.10.10.1:3002)
✅ HTTPS routing works (via Let's Encrypt certs)
✅ IPv6 tracking still works (hawk backend on host network)
✅ Date format is YYYY-MM-DD HH:MM:SS (from previous session)
