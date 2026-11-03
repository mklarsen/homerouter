# homerouter-hawk

IP address tracker for homerouter. Logs public IPv4 and IPv6 addresses every 15 minutes to detect delegation prefix changes and WAN IP modifications.

## Features

- Polls external IP services every 15 minutes
- Detects IPv4 and IPv6 changes instantly
- Stores history in SQLite database
- Logs change events with timestamps
- Minimal Alpine-based Docker image (< 50MB)
- Health check via database presence

## Deployment

### Option 1: Add to docker-compose.yml (recommended)

1. Copy the service definition from `docker-compose.snippet.yml` to your `/opt/stacks/docker-compose.yml`:

```yaml
  homerouter-hawk:
    container_name: homerouter-hawk
    image: homerouter-hawk:latest
    build:
      context: ./addons/homerouter-hawk
      dockerfile: Dockerfile
    restart: always
    volumes:
      - ./homerouter-hawk-data:/data
    environment:
      - TZ=UTC
    healthcheck:
      test: ["CMD", "test", "-f", "/data/ip-tracker.db"]
      interval: 60s
      timeout: 10s
      retries: 3
```

2. Deploy:
```bash
cd /opt/stacks
docker-compose up -d homerouter-hawk
```

### Option 2: Run standalone

```bash
docker run -d \
  --name homerouter-hawk \
  --restart always \
  -v hawk-data:/data \
  homerouter-hawk:latest
```

## Database

Data persists in the mounted volume at `/data/`:

- **ip-tracker.db**: SQLite database with `ip_history` table (schema auto-created on startup)
- **ip-changes.log**: Human-readable change log with timestamps

## Querying

### Recent IP history

```bash
sqlite3 /opt/stacks/homerouter-hawk-data/ip-tracker.db \
  "SELECT timestamp, ipv4, ipv6 FROM ip_history ORDER BY timestamp DESC LIMIT 20;"
```

### Change events only

```bash
tail -20 /opt/stacks/homerouter-hawk-data/ip-changes.log
```

### Watch for new changes (tail follow)

```bash
tail -f /opt/stacks/homerouter-hawk-data/ip-changes.log
```

## Database Schema

```sql
CREATE TABLE ip_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp TEXT NOT NULL,
  ipv4 TEXT,
  ipv6 TEXT,
  change INTEGER DEFAULT 0
);
```

Columns:
- `timestamp`: ISO 8601 datetime (UTC)
- `ipv4`: Public IPv4 address (or NULL if not available)
- `ipv6`: Public IPv6 address (or NULL if not available)
- `change`: 1 if either address changed since last poll, 0 otherwise

## Troubleshooting

### Container won't start

Check logs:
```bash
docker logs -f homerouter-hawk
```

### Database locked error

Wait a few seconds; the entrypoint.sh script should retry automatically.

### No data in database after 15+ minutes

Verify the container is running:
```bash
docker ps | grep homerouter-hawk
```

Check if external IP services are responding:
```bash
docker exec homerouter-hawk curl -s https://api.ipify.org
docker exec homerouter-hawk curl -s https://api64.ipify.org
```

## Development

Build locally:
```bash
docker build -t homerouter-hawk:latest .
docker run -v test-data:/data homerouter-hawk:latest
```

Run entrypoint.sh directly (for testing):
```bash
docker run -it -v test-data:/data homerouter-hawk:latest /bin/bash
/entrypoint.sh
```
