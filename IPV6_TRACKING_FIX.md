# IPv6 Tracking Fix Summary

## Problem
After the initial deployment, hawk-backend was storing **IPv4 address in the IPv6 column** for all subsequent polls after the first one:

```sql
-- Record #1 (startup): Real IPv6 ✅
SELECT ipv4, ipv6 FROM ip_history WHERE id = 1;
80.248.139.130 | 2a13:e740:2f1:d5cb::2

-- Records #2-#12 (subsequent polls): IPv4 fallback ❌
SELECT ipv4, ipv6 FROM ip_history WHERE id = 2;
80.248.139.130 | 80.248.139.130  <-- ipv6 should not equal ipv4!
```

## Root Cause
The backend's `get_ips()` function in `entrypoint.sh` was accepting ANY response from `curl https://api64.ipify.org`, including:
- Timeout/failure → empty string → stored as "unknown" ✓ (but not happening)
- IPv4 fallback → stored directly without validation ✗ (THIS WAS HAPPENING)

When `api64.ipify.org` experienced issues or rate-limiting, it fell back to returning IPv4 (because the service detects IPv4-only connection and returns IPv4 instead). The script had no validation to detect this.

## Solution
Added IPv6 format validation: **Check if response contains `:` (colon) before accepting**

```bash
# Before (lines 50-57)
for attempt in $(seq 1 $MAX_RETRIES); do
    ipv6=$(curl -s -m 5 https://api64.ipify.org 2>/dev/null || echo "")
    if [[ -n "$ipv6" ]]; then
        break  # ← Accepted any non-empty response, even IPv4!
    fi
done

# After (lines 50-57)
for attempt in $(seq 1 $MAX_RETRIES); do
    ipv6=$(curl -s -m 5 https://api64.ipify.org 2>/dev/null || echo "")
    if [[ -n "$ipv6" && "$ipv6" == *":"* ]]; then
        break  # ← Only accept if contains colon (real IPv6 format)
    fi
done

# Also add final validation (lines 62-65)
if [[ -z "$ipv6" || "$ipv6" != *":"* ]]; then
    ipv6="unknown"  # ← Reject IPv4 fallback, use "unknown" instead
fi
```

## Result
✅ **Real IPv6 now stored consistently**

```sql
-- After fix (records 12-14): Real IPv6
SELECT id, ipv4, ipv6 FROM ip_history WHERE id >= 12 ORDER BY id DESC;
14 | 80.248.139.130 | 2a13:e740:2f1:d5cb::2  ✅
13 | 80.248.139.130 | 2a13:e740:2f1:d5cb::2  ✅
12 | 80.248.139.130 | 2a13:e740:2f1:d5cb::2  ✅
```

## Changes
- **File**: `addons/homerouter-hawk/entrypoint.sh`
- **Function**: `get_ips()` lines 41-68
- **Change**: Added colon (`:`) check to validate IPv6 before storing
- **Image**: `ghcr.io/mklarsen/homerouter-hawk:1.0.1-ipv6-fix`
- **Deployed**: November 3, 2026

## UI Impact
- Dashboard now shows real IPv6 in "Current IPv6" section
- History table displays IPv6 correctly (with `:` format validation)
- Old records (#1-#11) with fallback IPv4 remain but won't be added to
- New records use real IPv6 format consistently

## Deployment Steps
```bash
# Update docker-compose.yml on router
sed -i 's|homerouter-hawk:1.0.0|homerouter-hawk:1.0.1-ipv6-fix|' /opt/stacks/docker-compose.yml

# Restart backend
cd /opt/stacks
docker compose down homerouter-hawk
docker compose up -d homerouter-hawk

# Verify
docker ps | grep homerouter-hawk
curl -s http://localhost:3002/api/latest | jq .ipv6
```

## Testing
```bash
# Test from container (should always get real IPv6)
docker exec homerouter-hawk curl -s https://api64.ipify.org

# Check database (should only have IPv6 with colons)
sqlite3 /opt/stacks/homerouter-hawk-data/ip-tracker.db \
  "SELECT ipv4, ipv6 FROM ip_history ORDER BY id DESC LIMIT 5;"
```

## Future Improvements
1. Add metrics to track poll success/failure rate
2. Log fallback events separately for debugging
3. Consider implementing a "best effort" cache (if api64 fails, keep last known IPv6)
4. Add configuration for alternative IPv6 detection services
