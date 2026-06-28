#!/bin/bash
# pin-to-7.sh — adapt the hotel QR ordering to the Wi-Fi you're CURRENTLY on.
# It detects this network's subnet, pins the laptop to a fixed IP on it, and
# rebinds the app to that IP. Run once after joining a new Wi-Fi.
#
# IMPORTANT: the QR codes embed an IP address, so after running this you must
# REPRINT the codes (the script tells you the new address + how).

set -u
cd "$(dirname "$0")" || exit 1

# 1. Active Wi-Fi connection + device
CONN=$(nmcli -t -f NAME,TYPE connection show --active | awk -F: '$2=="802-11-wireless"{print $1; exit}')
DEV=$(nmcli -t -f DEVICE,TYPE device | awk -F: '$2=="wifi"{print $1; exit}')
[ -z "${CONN:-}" ] && { echo "❌ Not connected to Wi-Fi. Join the network first, then re-run."; exit 1; }

# 2. This network's gateway + subnet (works on ANY 192.168.x / 10.x / etc.)
GW=$(ip route | awk '/^default/ && /wl/ {print $3; exit}')
[ -z "${GW:-}" ] && { echo "❌ No gateway found for $DEV."; exit 1; }
SUBNET=$(echo "$GW" | cut -d. -f1-3)
TARGET="${SUBNET}.7"
echo "📶 Wi-Fi: $CONN    Gateway: $GW    (subnet ${SUBNET}.x)"

# 3. If .7 is taken by another device, fall back to .77
CUR=$(ip -4 -o addr show dev "$DEV" | awk '{print $4}' | cut -d/ -f1)
if [ "$CUR" != "$TARGET" ] && ping -c1 -W1 "$TARGET" >/dev/null 2>&1; then
  TARGET="${SUBNET}.77"
  echo "⚠️  ${SUBNET}.7 is already in use — using $TARGET instead."
fi

# 4. Pin the laptop to that IP (keep internet via this network's gateway + DNS)
echo "📌 Pinning laptop to $TARGET ..."
nmcli connection modify "$CONN" \
  ipv4.addresses "$TARGET/24" ipv4.gateway "$GW" ipv4.dns "$GW 8.8.8.8" ipv4.method manual
nmcli connection up "$CONN" >/dev/null
echo "   laptop IP: $(ip -4 -o addr show dev "$DEV" | awk '{print $4}')"

# 5. Rebind the app so new codes/images use this IP
echo "🚀 Rebinding the app to $TARGET (this can take a few seconds)..."
HOST_IP="$TARGET" docker compose up -d >/dev/null 2>&1
ok=0
for _ in $(seq 1 25); do curl -s -m 3 -o /dev/null "http://$TARGET:3000" && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] && echo "   ✅ app live at http://$TARGET:3000" || echo "   ⚠️ app slow to start — wait a few seconds and check http://$TARGET:3000"

cat <<MSG

────────────────────────────────────────────────────────
✅ Ready on this Wi-Fi. Codes now point to: http://$TARGET:3000
⚠️ REPRINT the codes — they changed to $TARGET:
     open  http://$TARGET:3000/admin  → Rooms & QR Codes → download each → print
     (or ask Claude to rebuild the print sheet)

Guests: connect to "$CONN", then scan.

↩️ Undo the IP pin later:  nmcli connection modify "$CONN" ipv4.method auto && nmcli connection up "$CONN"
────────────────────────────────────────────────────────
MSG
