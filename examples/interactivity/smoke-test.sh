#!/bin/bash
# Smoke-test both transports with plain Docker (no cortex needed; works on macOS).
#
#   ./smoke-test.sh            # build images, then test http + ros
#   ./smoke-test.sh --no-build # reuse existing images
#
# The app (and, for ros, the bridge) share one network namespace, standing in
# for the Pi's host networking. probe.py plays cortex's role, calling exactly
# the URLs cortex would. Latency here is NOT decision-grade — use bench.sh on the Pi.
set -euo pipefail
cd "$(dirname "$0")"

if [ -t 1 ]; then
  COLOR=1; B=$'\033[1m'; D=$'\033[2m'; G=$'\033[32m'; R=$'\033[31m'; X=$'\033[0m'
else
  COLOR=0; B=""; D=""; G=""; R=""; X=""
fi

ID=counter-1-dev-1
FAILED=0
SUMMARIES=()

cleanup() { docker rm -f zc-http zc-ros zc-bridge >/dev/null 2>&1 || true; }
trap cleanup EXIT

header() { printf '\n%s━━ %s%s\n%s%s%s\n\n' "$B" "$1" "$X" "$D" "$2" "$X"; }

probe() { # label container url
  local out status=0
  docker cp probe.py "$2":/tmp/probe.py
  out=$(docker exec -e PROBE_COLOR=$COLOR "$2" python3 /tmp/probe.py "$3") || status=$?
  grep -v '^SUMMARY ' <<<"$out" || true
  SUMMARIES+=("$1|$(grep '^SUMMARY ' <<<"$out" | cut -d' ' -f2- || echo '0 6 - - - -')")
  [ $status -eq 0 ] || FAILED=1
}

if [ "${1:-}" != "--no-build" ]; then
  printf '%sBuilding images…%s ' "$D" "$X"
  docker build -q -f Dockerfile.base -t zara-commands-base:dev . >/dev/null
  docker build -q -f apps/interactive-counter/Dockerfile -t interactive-counter:dev . >/dev/null
  docker build -q -f apps/command-bridge/Dockerfile -t command-bridge:dev . >/dev/null
  printf '%sdone%s\n' "$G" "$X"
fi
cleanup

header "HTTP" "app's own server on its cortex-assigned port (127.0.0.1:9100)"
docker run -d --name zc-http -e ZARA_INSTANCE_ID=$ID \
  -e ZARA_COMMAND_TRANSPORT=http -e ZARA_COMMAND_PORT=9100 interactive-counter:dev >/dev/null
probe "HTTP" zc-http http://127.0.0.1:9100/commands

header "ROS bridge" "bridge (127.0.0.1:9000) → ROS service /zara/i_${ID//-/_}/command"
docker run -d --name zc-ros -e ZARA_INSTANCE_ID=$ID \
  -e ZARA_COMMAND_TRANSPORT=ros interactive-counter:dev >/dev/null
docker run -d --name zc-bridge --network container:zc-ros command-bridge:dev >/dev/null
probe "ROS bridge" zc-ros http://127.0.0.1:9000/commands/$ID

header "Summary" "round-trip latency in ms"
printf '  %s%-16s %-8s %8s %8s %8s %8s%s\n' "$D" "option" "checks" "p50" "p95" "p99" "max" "$X"
for row in "${SUMMARIES[@]}"; do
  IFS='|' read -r label rest <<<"$row"
  read -r passed total p50 p95 p99 mx <<<"$rest"
  c=$G; [ "$passed" = "$total" ] || c=$R
  printf '  %-16s %s%-8s%s %8s %8s %8s %8s\n' "$label" "$c" "$passed/$total" "$X" "$p50" "$p95" "$p99" "$mx"
done
echo
if [ $FAILED -eq 0 ]; then
  printf '%s✓ all checks passed%s\n' "$G" "$X"
else
  printf '%s✗ some checks failed%s\n' "$R" "$X"
fi
exit $FAILED
