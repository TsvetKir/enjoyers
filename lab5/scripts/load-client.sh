#!/usr/bin/env bash
set -euo pipefail

seconds="${1:-300}"
context="${KUBE_CONTEXT:-kind-lab5}"
namespace="${KUBE_NAMESPACE:-shop}"
client_pod="${CLIENT_POD:-postgres-1}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
output="${2:-"$script_dir/../reports/chaos-results.csv"}"
mkdir -p -- "$(dirname -- "$output")"

if ! kubectl --context "$context" -n "$namespace" get "pod/$client_pod" >/dev/null; then
  echo "client pod $namespace/$client_pod not found" >&2
  exit 1
fi

{
  printf 'timestamp,sequence,success,status,latency_ms,error,retries\n'
  kubectl --context "$context" -n "$namespace" exec -i "$client_pod" -- bash -s -- "$seconds" <<'REMOTE'
set +e
duration="${1:-300}"
end=$((SECONDS + duration))
sequence=0
host='shop-api.shop.svc.cluster.local'

while (( SECONDS < end )); do
  sequence=$((sequence + 1))
  started="$(date +%s%N)"
  retries=0
  while :; do
    response="$(timeout 5 bash -c '
      sequence="$1"
      host="$2"
      body="{\"item\":\"chaos-${sequence}\"}"
      # Exit 10 only when opening the TCP connection fails, before POST is sent.
      exec 3<>"/dev/tcp/${host}/8080" || exit 10
      printf "POST /order HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %s\r\nConnection: close\r\n\r\n%s" "$host" "${#body}" "$body" >&3
      IFS= read -r status <&3 || exit 11
      printf "%s" "$status"
    ' load-client "$sequence" "$host" 2>/dev/null)"
    result=$?
    if (( result != 10 || retries >= 3 )); then
      break
    fi
    retries=$((retries + 1))
    sleep "0.$((retries * 1))"
  done
  finished="$(date +%s%N)"
  latency_ms=$(((finished - started) / 1000000))
  timestamp="$(date -u +'%Y-%m-%dT%H:%M:%S.%3NZ')"
  status=0
  error=''
  if [[ "$response" =~ ^HTTP/[0-9.]+[[:space:]]([0-9]{3}) ]]; then
    status="${BASH_REMATCH[1]}"
  fi
  if (( result == 124 )); then
    error='timeout'
  elif (( result != 0 )); then
    error='connection_failed'
  elif [[ "$status" != 201 ]]; then
    error='unexpected_http_status'
  fi
  if (( result == 0 )) && [[ "$status" == 201 ]]; then
    success=true
  else
    success=false
  fi
  printf '%s,%s,%s,%s,%s,%s,%s\n' "$timestamp" "$sequence" "$success" "$status" "$latency_ms" "$error" "$retries"
  sleep 0.1
done
REMOTE
} >"$output"

awk -F, 'NR > 1 { total++; retries += $7; if ($3 != "true") failed++ } END { printf "requests=%d failed=%d connect_retries=%d csv=%s\n", total, failed, retries, ARGV[1]; if (failed > 0 || total == 0) exit 1 }' "$output"
