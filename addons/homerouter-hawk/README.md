# homerouter-hawk

IP address tracker for homerouter. Logs public IPv4 and IPv6 addresses every 15 minutes to detect delegation prefix changes.

## Usage

```bash
docker run -v hawk-data:/data ghcr.io/mklarsen/homerouter-hawk:latest
```

## Features

- Polls external IP services every 15 minutes
- Stores history in SQLite database
- Logs IPv4/IPv6 changes to file
- Detects WAN prefix delegation changes

## Database

- `ip-tracker.db`: SQLite database with `ip_history` table
- `ip-changes.log`: Log file with human-readable change notifications

## Query history

```bash
# Recent entries
sqlite3 ip-tracker.db "SELECT timestamp, ipv4, ipv6 FROM ip_history ORDER BY timestamp DESC LIMIT 10;"

# Changes only
grep "IP CHANGE" ip-changes.log
```
