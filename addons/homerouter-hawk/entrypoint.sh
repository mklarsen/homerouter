#!/bin/bash
set -Eeuo pipefail

DB_FILE="/data/ip-tracker.db"
LOG_FILE="/data/ip-changes.log"

# Initialize database if not exists
if [[ ! -f "$DB_FILE" ]]; then
    sqlite3 "$DB_FILE" <<EOF
CREATE TABLE ip_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    ipv4 TEXT,
    ipv6 TEXT,
    wan_interface TEXT
);
CREATE INDEX idx_timestamp ON ip_history(timestamp);
EOF
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Database initialized" >> "$LOG_FILE"
fi

# Function to get current IPs
get_ips() {
    local ipv4=""
    local ipv6=""
    
    # Get WAN IPv4 (primary public IP)
    ipv4=$(curl -s -m 5 https://api.ipify.org || echo "unknown")
    
    # Get WAN IPv6 (public IPv6)
    ipv6=$(curl -s -m 5 https://api6.ipify.org || echo "unknown")
    
    echo "$ipv4" "$ipv6"
}

# Function to log IP change
log_ip_change() {
    local ipv4="$1"
    local ipv6="$2"
    local prev_ipv4="$3"
    local prev_ipv6="$4"
    
    if [[ "$ipv4" != "$prev_ipv4" ]] || [[ "$ipv6" != "$prev_ipv6" ]]; then
        echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] IP CHANGE: IPv4 $prev_ipv4 -> $ipv4, IPv6 $prev_ipv6 -> $ipv6" >> "$LOG_FILE"
    fi
}

# Main tracking loop
echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] homerouter-hawk starting (tracks WAN IP every 15 min)" >> "$LOG_FILE"

prev_ipv4=""
prev_ipv6=""

while true; do
    read -r ipv4 ipv6 < <(get_ips)
    
    # Insert into database
    sqlite3 "$DB_FILE" "INSERT INTO ip_history (ipv4, ipv6, wan_interface) VALUES ('$ipv4', '$ipv6', 'enp3s0');"
    
    # Log if changed
    log_ip_change "$ipv4" "$ipv6" "$prev_ipv4" "$prev_ipv6"
    
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] IPv4: $ipv4 | IPv6: $ipv6" >> "$LOG_FILE"
    
    prev_ipv4="$ipv4"
    prev_ipv6="$ipv6"
    
    # Sleep 15 minutes
    sleep 900
done
