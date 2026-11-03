# homerouter-hawk - IP Tracking Service

**homerouter-hawk** is an automatic WAN IP address tracker for homerouter routers. It polls your public IPv4 and IPv6 addresses hourly and provides a professional web dashboard to visualize IP history and changes.

## Features

- ✅ **Automatic IP Polling** — Polls public IPv4/IPv6 every 60 minutes
- ✅ **Historical Tracking** — Stores all IP records with timestamps in SQLite
- ✅ **Professional Dashboard** — Modern web UI with charts and analytics (mpsconsole.com inspired)
- ✅ **PDF Export** — Export IP history reports as PDF documents
- ✅ **Basic Authentication** — Secured with username/password
- ✅ **Traefik Integration** — Accessible via `hawk.homerouter.io` or direct port `3002`

## Architecture

```
homerouter-hawk/
├── backend (Node.js + express)
│   └── Polls public IP every 60 min
│   └── Stores results in SQLite database
│   └── API endpoints: /api/ip-history, /api/ip-changes, /api/latest
│
└── ui (React + Vite)
    └── Express.js backend for static serving
    └── Dynamic charts with Recharts
    └── PDF export via html2pdf
    └── Basic Auth middleware
```

## Deployment on homerouter

### System Requirements
- Docker daemon running on 10.10.10.1
- GHCR access (GitHub Container Registry)
- Network: traefik-edge docker network

### Service Configuration

Both services run as Docker containers, orchestrated by `/opt/stacks/docker-compose.yml`:

#### Backend: `homerouter-hawk`
```yaml
image: ghcr.io/mklarsen/homerouter-hawk:1.0.0
network_mode: host  # ⚠️ CRITICAL: Required for IPv6 detection
environment:
  - DATABASE_PATH=/data/ip-tracker.db
ports: (internal only - shared via host network)
volumes:
  - ./homerouter-hawk-data:/data  # SQLite database
```

**What it does:**
- Starts at container boot
- Runs on **host network** so it can directly access host's IPv6 address
- Polls `api.ipify.org` for IPv4 and `api64.ipify.org` for IPv6 every 60 minutes
- Writes results to `/data/ip-tracker.db` (SQLite)

**Why `network_mode: host`?**
- Docker containers are isolated from the host network by default
- Without host mode, containers cannot detect the host's IPv6 address
- This causes fallback to IPv4 even if IPv6 is available
- Host network mode allows the backend to see real IPv6 (e.g., `2a13:e740:2f1:d5cb::2`)

#### Frontend: `homerouter-hawk-ui`
```yaml
image: ghcr.io/mklarsen/homerouter-hawk-ui:1.0.3
networks:
  - traefik-edge  # ⚠️ CRITICAL: Must be on traefik-edge for Traefik Docker DNS
environment:
  - PORT=3002
  - DATABASE_PATH=/data/ip-tracker.db
  - HAWK_USER=${HAWK_UI_USER:-admin}
  - HAWK_PASSWORD=${HAWK_UI_PASSWORD:-changeme}
ports:
  - "10.10.10.1:3002:3002"  # Direct LAN access
volumes:
  - ./homerouter-hawk-data:/data:ro  # Read-only database access
```

**What it does:**
- Serves React static build + Express backend
- Provides REST API for dashboard
- Listens on port 3002 (configurable via `PORT` env)
- Performs Basic Auth on all endpoints except `/api/health`
- Displays IP history with charts and PDF export

**Why `networks: [traefik-edge]`?**
- Traefik needs to reach the UI container via Docker internal DNS
- Using `network_mode: host` breaks Traefik Docker provider routing (502 Bad Gateway)
- Solution: Run UI on `traefik-edge` network so Traefik can resolve `homerouter-hawk-ui:3002`
- Still bound to host IP `10.10.10.1:3002` for direct LAN access

### Accessing the Dashboard

#### Direct LAN Access (Recommended)
```
http://10.10.10.1:3002
Username: admin
Password: (from .env HAWK_UI_PASSWORD)
```

#### Via Traefik (hawk.homerouter.io)
Requires:
- DNS pointing `hawk.homerouter.io` to 198.51.100.132 (documentation example, replace with real ISP external IP)
- Traefik file-based routing configured in `/opt/stacks/dynamic/all-routes.yml`
- hawk-ui container on `traefik-edge` network (for Traefik Docker DNS resolution)

```
https://hawk.homerouter.io
```

## Configuration

### Credentials (.env on router)

Located at `/opt/stacks/.env`:

```bash
HAWK_UI_USER=admin
HAWK_UI_PASSWORD=rsdfc¤R%#87687!
```

**⚠️ IMPORTANT:** These credentials are LOCAL to the router and never committed to git.

### Data Storage

**Database:** `/opt/stacks/homerouter-hawk-data/ip-tracker.db` (SQLite)

**Schema:**
```sql
CREATE TABLE ip_history (
  id INTEGER PRIMARY KEY,
  timestamp TEXT,        -- ISO 8601
<<<<<<< HEAD
  ipv4 TEXT,             -- IPv4 address (e.g., "80.248.139.130")
=======
  ipv4 TEXT,             -- IPv4 address (e.g., "198.51.100.130" - RFC 5737 doc address)
>>>>>>> d18ab694d99c45528ffb0a3a99714ce1c6289d07
  ipv6 TEXT,             -- IPv6 address (contains ':' if valid)
  change INTEGER         -- 1 if IP changed, 0 if stable
);
```

**Backup:**
```bash
ssh root@10.10.10.1
cp /opt/stacks/homerouter-hawk-data/ip-tracker.db /opt/stacks/homerouter-hawk-data/ip-tracker.db.backup
```

## API Endpoints

All API endpoints require Basic Auth (except `/api/health`).

### GET /api/health
Returns service health status.
```json
{"status":"ok","timestamp":"2026-11-03T21:00:00Z"}
```

### GET /api/latest
Returns the most recent IP record.
```json
<<<<<<< HEAD
{"timestamp":"2026-11-03 21:00:00","ipv4":"80.248.139.130","ipv6":"80.248.139.130"}
=======
{"timestamp":"2026-11-03 21:00:00","ipv4":"198.51.100.130","ipv6":"2001:db8::1"}
>>>>>>> d18ab694d99c45528ffb0a3a99714ce1c6289d07
```

### GET /api/ip-history?limit=100
Returns full IP history (up to limit).
```json
[
<<<<<<< HEAD
  {"id":1,"timestamp":"2026-11-03 20:33:04","ipv4":"80.248.139.130","ipv6":"80.248.139.130","change":0},
  {"id":2,"timestamp":"2026-11-03 20:47:21","ipv4":"80.248.139.130","ipv6":"80.248.139.130","change":1}
=======
  {"id":1,"timestamp":"2026-11-03 20:33:04","ipv4":"198.51.100.130","ipv6":"2001:db8::1","change":0},
  {"id":2,"timestamp":"2026-11-03 20:47:21","ipv4":"198.51.100.131","ipv6":"2001:db8::1","change":1}
>>>>>>> d18ab694d99c45528ffb0a3a99714ce1c6289d07
]
```

### GET /api/ip-changes
Returns only records where IP changed.
```json
[
<<<<<<< HEAD
  {"id":2,"timestamp":"2026-11-03 20:47:21","ipv4":"80.248.139.130","ipv6":"80.248.139.130"}
=======
  {"id":2,"timestamp":"2026-11-03 20:47:21","ipv4":"198.51.100.131","ipv6":"2001:db8::1"}
>>>>>>> d18ab694d99c45528ffb0a3a99714ce1c6289d07
]
```

## Dashboard Features

### Metrics
- **Current IPv4** — Active public IPv4 address
- **Current IPv6** — Public IPv6 (or "No IPv6 available")
- **Total Changes** — Count of IP change events
- **Tracking Period** — Days of history collected

### Charts
- **IP Address Timeline** — Area chart of all IPv4 addresses over time
- **IP Change Events** — Line chart showing when IPs changed

### History Table
- Sortable table of all 100 most recent records
- Color-coded rows for IP changes (yellow = changed)
- IPv6 shows as "—" if not available or invalid

### Export
- **Export Report Button** — Generates PDF of dashboard + metrics + charts

## Troubleshooting

### Dashboard not loading
**Check service health:**
```bash
ssh root@10.10.10.1
docker ps | grep hawk
docker logs homerouter-hawk-ui
```

**Check connectivity:**
```bash
curl -u admin:PASSWORD http://10.10.10.1:3002/api/health
```

### No IPv6 address shown
This is normal if your ISP doesn't provide IPv6. The backend detects real IPv6 (contains `:`) vs. fallback IPv4.

### Authentication fails
Verify `.env` credentials on router:
```bash
ssh root@10.10.10.1
grep HAWK_ /opt/stacks/.env
```

### Traefik routing (hawk.homerouter.io) not working
Check file provider config:
```bash
ssh root@10.10.10.1
docker exec homerouter-traefik-1 cat /dynamic/all-routes.yml | grep -A 5 hawk-ui
```

If missing, restart Traefik:
```bash
docker compose restart traefik
```

## Development

### Local build
```bash
cd addons/homerouter-hawk

# Backend
cd backend
npm install
node index.js

# UI
cd ../ui
npm install
npm run build
npm run dev  # Runs on :5173
```

### Docker build
```bash
# From repo root
docker build -f addons/homerouter-hawk/backend/Dockerfile -t homerouter-hawk:dev .
docker build -f addons/homerouter-hawk/ui/Dockerfile -t homerouter-hawk-ui:dev .
```

### Push to GHCR
```bash
docker tag homerouter-hawk-ui:dev ghcr.io/mklarsen/homerouter-hawk-ui:latest
docker push ghcr.io/mklarsen/homerouter-hawk-ui:latest
```

### Restart on router
```bash
ssh root@10.10.10.1
cd /opt/stacks
docker compose pull
docker compose up -d
```

## License

Part of [homerouter](https://github.com/mklarsen/homerouter) project.
