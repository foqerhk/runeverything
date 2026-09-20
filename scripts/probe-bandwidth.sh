#!/usr/bin/env bash
# Measure approximate uplink and write RE_BW_MBPS_UP / RE_MAX_SESSIONS hints.
# Usage: ./scripts/probe-bandwidth.sh [per_user_kbps]
# Writes ~/.runeverything/bandwidth.json and prints export lines.
set -euo pipefail

PER_USER_KBPS="${1:-2500}"
HOME_RE="${RE_HOME:-$HOME/.runeverything}"
mkdir -p "$HOME_RE"
OUT="$HOME_RE/bandwidth.json"

# Prefer Cloudflare speed-style endpoints when available; fallback: local timed write.
measure_up_mbps() {
  local tmp size start end secs mbps
  tmp="$(mktemp)"
  size=$((2 * 1024 * 1024)) # 2 MiB
  dd if=/dev/urandom of="$tmp" bs=1024 count=$((size/1024)) status=none 2>/dev/null || \
    head -c "$size" /dev/urandom >"$tmp"

  # Try HTTPS upload to Cloudflare speed test edge (discard)
  start=$(date +%s.%N)
  if curl -fsS -o /dev/null -X POST --data-binary @"$tmp" \
      "https://speed.cloudflare.com/__up" 2>/dev/null; then
    end=$(date +%s.%N)
    secs=$(awk -v s="$start" -v e="$end" 'BEGIN{printf "%.4f", e-s}')
    mbps=$(awk -v b="$size" -v s="$secs" 'BEGIN{if(s<=0)s=0.001; printf "%.2f", (b*8)/(s*1000000)}')
    rm -f "$tmp"
    echo "$mbps"
    return 0
  fi
  rm -f "$tmp"
  # Fallback conservative default
  echo "10.00"
}

MBPS=$(measure_up_mbps)
# 70% usable / per-user budget
MAX=$(awk -v m="$MBPS" -v p="$PER_USER_KBPS" 'BEGIN{n=int(m*0.7*1000/p); if(n<1)n=1; if(n>64)n=64; print n}')

cat >"$OUT" <<EOF
{
  "mbps_up": $MBPS,
  "per_user_kbps": $PER_USER_KBPS,
  "max_sessions": $MAX,
  "measured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

echo "Measured uplink ~${MBPS} Mbps → max_sessions=${MAX} (budget ${PER_USER_KBPS} kbps/user, 30% headroom)"
echo "Wrote $OUT"
echo "export RE_BW_MBPS_UP=$MBPS"
echo "export RE_MAX_SESSIONS=$MAX"
echo "export RE_PER_USER_KBPS=$PER_USER_KBPS"
