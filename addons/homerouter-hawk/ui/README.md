# homerouter-hawk UI - Web Interface

React frontend + Express backend for viewing IP history with PDF export.

## Features

- **Dashboard**: Current IPv4/IPv6 + historical graph
- **IP History Table**: Last 50 entries with timestamps
- **PDF Export**: Download history as PDF for ISP documentation
- **Basic Auth**: Username/password from `.env`
- **Design**: Dark theme inspired by mpsconsole.com
- **Real-time Updates**: Refreshes every minute

## Environment Variables

Required in `/opt/stacks/.env`:
```bash
HAWK_UI_USER=admin           # Default username for auth
HAWK_UI_PASSWORD=changeme    # Default password for auth
```

## docker-compose.yml

Add to services section:
```yaml
  homerouter-hawk-ui:
    build:
      context: .
      dockerfile: ./addons/homerouter-hawk/ui/Dockerfile
    container_name: homerouter-hawk-ui
    restart: always
    environment:
      DATABASE_PATH: /data/ip-tracker.db
      HAWK_USER: ${HAWK_UI_USER:-admin}
      HAWK_PASSWORD: ${HAWK_UI_PASSWORD:-changeme}
    volumes:
      - ./homerouter-hawk-data:/data:ro
    networks:
      - traefik-edge
    labels:
      traefik.enable: "true"
      traefik.docker.network: traefik-edge
      traefik.http.routers.hawk-ui.rule: Host(`hawk.homerouter.io`)
      traefik.http.routers.hawk-ui.entrypoints: websecure
      traefik.http.routers.hawk-ui.tls: "true"
      traefik.http.routers.hawk-ui.tls.certresolver: letsencrypt
      traefik.http.services.hawk-ui.loadbalancer.server.port: "3000"
```

## Access

- URL: `https://hawk.homerouter.io`
- Username: `${HAWK_UI_USER}`
- Password: `${HAWK_UI_PASSWORD}`

## API Endpoints

All require Basic Auth except `/api/health`:

- `GET /api/health` - Health check (no auth)
- `GET /api/latest` - Current IPv4/IPv6
- `GET /api/ip-history?limit=1000` - Historical data
- `GET /api/ip-changes` - Change events only
