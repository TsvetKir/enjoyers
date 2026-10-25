#!/usr/bin/env bash
set -euo pipefail

duration="${1:-300}"
interval="${2:-8}"
context="${KUBE_CONTEXT:-kind-lab5}"
namespace="${KUBE_NAMESPACE:-shop}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
log_path="${3:-"$script_dir/../reports/chaos-actions.log"}"
mkdir -p -- "$(dirname -- "$log_path")"
exec > >(tee "$log_path") 2>&1

end=$((SECONDS + duration))
while (( SECONDS < end )); do
  if (( RANDOM % 2 )); then component=api; else component=worker; fi
  rows="$(kubectl --context "$context" -n "$namespace" get pods \
    -l "app.kubernetes.io/component=$component" \
    -o 'custom-columns=NAME:.metadata.name,PHASE:.status.phase,READY:.status.containerStatuses[0].ready,IP:.status.podIP,DELETING:.metadata.deletionTimestamp' \
    --no-headers)"
  candidates="$(printf '%s\n' "$rows" | awk '$2 == "Running" && $3 == "true" && $5 == "<none>" { print $1, $4 }')"
  count="$(printf '%s\n' "$candidates" | awk 'NF { n++ } END { print n + 0 }')"
  # No replica-count threshold: delete any one ready pod selected above.
  if (( count > 0 )); then
    victim="$(printf '%s\n' "$candidates" | shuf -n 1)"
    read -r pod ip <<<"$victim"
    printf '%s deleting %s component=%s ip=%s\n' "$(date -Is)" "$pod" "$component" "$ip"
    kubectl --context "$context" -n "$namespace" delete pod "$pod" --wait=false
  else
    printf '%s skip component=%s ready-pods=%s\n' "$(date -Is)" "$component" "$count"
  fi
  sleep "$interval"
done
