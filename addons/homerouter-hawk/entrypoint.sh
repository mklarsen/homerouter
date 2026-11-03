#!/bin/bash
set -Eeuo pipefail

DB_FILE="/data/ip-tracker.db"
LOG_FILE="/data/ip-changes.log"
POLL_INTERVAL=900  # 15 minutes in seconds
MAX_RETRIES=3
RETRY_DELAY=2

# Initialize database if not exists
init_database() {
    if [[ ! -f "$DB_FILE" ]]; then
        sqlite3 "$DB_FILE" <<EOF
CREATE TABLE ip_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL DEFAULT (datetime('now', 'utc')),
    ipv4 TEXT,
    ipv6 TEXT,
    change INTEGER DEFAULT 0
);
CREATE INDEX idx_timestamp ON ip_history(timestamp);
EOF
        log_message "Database initialized at $DB_FILE"
    fi
}

# Log message with timestamp
log_message() {
    local msg="$1"
    local ts
    ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    echo "[$ts] $msg" >> "$LOG_FILE"
}

# Get current IPs with retry logic
get_ips() {
    local ipv4="unknown"
    local ipv6="unknown"
    local attempt=1
    
    # Try to get IPv4
    for attempt in $(seq 1 $MAX_RETRIES); do
        ipv4=$(curl -s -m 5 https://api.ipify.org 2>/dev/null || echo "")
        if [[ -n "$ipv4" ]]; then
            break
        fi
        if [[ $attempt -lt $MAX_RETRIES ]]; then
            sleep $RETRY_DELAY
        fi
    done
    
    # Try to get IPv6
    attempt=1
    for attempt in $(seq 1 $MAX_RETRIES); do
        ipv6=$(curl -s -m 5 https://api64.ipify.org 2>/dev/null || echo "")
        if [[ -n "$ipv6" ]]; then
            break
        fi
        if [[ $attempt -lt $MAX_RETRIES ]]; then
            sleep $RETRY_DELAY
        fi
    done
    
    # Fallback to "unknown" if not retrieved
    [[ -z "$ipv4" ]] && ipv4="unknown"
    [[ -z "$ipv6" ]] && ipv6="unknown"
    
    echo "$ipv4" "$ipv6"
}

# Insert IP into database and return change flag
insert_and_detect_change() {
    local ipv4="$1"
    local ipv6="$2"
    local change=0
    
    # Check if this is different from last entry
    if [[ -n "$(sqlite3 "$DB_FILE" "SELECT id FROM ip_history LIMIT 1;")" ]]; then
        local last_ipv4
        local last_ipv6
        read -r last_ipv4 last_ipv6 < <(sqlite3 "$DB_FILE" "SELECT ipv4, ipv6 FROM ip_history ORDER BY id DESC LIMIT 1;")
        
        if [[ "$ipv4" != "$last_ipv4" ]] || [[ "$ipv6" != "$last_ipv6" ]]; then
            change=1
        fi
    fi
    
    # Insert into database (with timeout and retry)
    local retry=1
    while [[ $retry -le $MAX_RETRIES ]]; do
        if sqlite3 "$DB_FILE" "INSERT INTO ip_history (ipv4, ipv6, change) VALUES ('$ipv4', '$ipv6', $change);" 2>/dev/null; then
            echo "$change"
            return 0
        fi
        if [[ $retry -lt $MAX_RETRIES ]]; then
            sleep $RETRY_DELAY
        fi
        ((retry++))
    done
    
    log_message "WARNING: Failed to insert IP after $MAX_RETRIES attempts"
    echo "0"
}

# Main tracking loop
init_database

log_message "homerouter-hawk starting - polling every $((POLL_INTERVAL / 60)) minutes"
log_message "IPv4: $(curl -s -m 5 https://api.ipify.org 2>/dev/null || echo "unknown") | IPv6: $(curl -s -m 5 https://api64.ipify.org 2>/dev/null || echo "unknown")"

while true; do
    read -r ipv4 ipv6 < <(get_ips)
    
    # Insert and detect change
    change=$(insert_and_detect_change "$ipv4" "$ipv6")
    
    if [[ "$change" == "1" ]]; then
        log_message "🔄 IP CHANGE: IPv4=$ipv4, IPv6=$ipv6"
    else
        log_message "✓ Poll: IPv4=$ipv4, IPv6=$ipv6"
    fi
    
    # Sleep until next poll
    sleep $POLL_INTERVAL
done
