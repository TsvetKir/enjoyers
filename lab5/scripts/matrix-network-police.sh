#!/usr/bin/env bash
set -euo pipefail

context="${KUBE_CONTEXT:-kind-lab5}"
namespace="${KUBE_NAMESPACE:-shop}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
output="${1:-"$script_dir/../reports/matrix-results.csv"}"
allow_mismatch=false
for arg in "$@"; do
  [[ "$arg" == '--allow-mismatch' ]] && allow_mismatch=true
done
if [[ "$output" == '--allow-mismatch' ]]; then
  output="$script_dir/../reports/matrix-results.csv"
fi
mkdir -p -- "$(dirname -- "$output")"

client_pod='postgres-1'
probe_image="$(kubectl --context "$context" -n "$namespace" get pod "$client_pod" -o jsonpath='{.spec.containers[?(@.name=="postgres")].image}')"
probe_node="$(kubectl --context "$context" -n "$namespace" get pod "$client_pod" -o jsonpath='{.spec.nodeName}')"
if [[ -z "$probe_image" || -z "$probe_node" ]]; then
  echo 'could not determine the existing PostgreSQL image and node' >&2
  exit 1
fi

printf 'source,destination,expected,actual,pass\n' >"$output"
printf '%-10s %-58s %-9s %-9s %s\n' SOURCE DESTINATION EXPECTED ACTUAL PASS
failures=0

for source in client api worker probe; do
  pod="lab5-matrix-$source"
  kubectl --context "$context" -n "$namespace" delete pod "$pod" --ignore-not-found --wait=true >/dev/null
  cat <<YAML | kubectl --context "$context" -n "$namespace" apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: $pod
  labels:
    app.kubernetes.io/name: shop
    app.kubernetes.io/part-of: shop
    app.kubernetes.io/component: $source
spec:
  nodeName: $probe_node
  restartPolicy: Never
  containers:
    - name: probe
      image: $probe_image
      imagePullPolicy: IfNotPresent
      command: ["/usr/bin/bash", "-c", "sleep 3600"]
YAML
  trap 'kubectl --context "$context" -n "$namespace" delete pod "$pod" --ignore-not-found --wait=true >/dev/null 2>&1 || true' EXIT
  kubectl --context "$context" -n "$namespace" wait --for=condition=Ready "pod/$pod" --timeout=90s >/dev/null

  for target in api worker postgres dns; do
    case "$target" in
      api) host="shop-api.$namespace.svc.cluster.local"; port=8080; destination="$host:$port" ;;
      worker) host="shop-worker.$namespace.svc.cluster.local"; port=8081; destination="$host:$port" ;;
      postgres) host="postgres-rw.$namespace.svc.cluster.local"; port=5432; destination="$host:$port" ;;
      dns) host="shop-api.$namespace.svc.cluster.local"; port=0; destination="DNS $host" ;;
    esac

    if [[ "$target" == dns ]]; then
      if kubectl --context "$context" -n "$namespace" exec "$pod" -- getent hosts "$host" >/dev/null 2>&1; then
        allowed=true
      else
        allowed=false
      fi
    elif kubectl --context "$context" -n "$namespace" exec "$pod" -- timeout 3 bash -c 'exec 3<>"/dev/tcp/$1/$2"' _ "$host" "$port" >/dev/null 2>&1; then
      allowed=true
    else
      allowed=false
    fi

    expected=false
    if [[ "$target" == dns ]] || [[ "$source:$target" == 'client:api' || "$source:$target" == 'api:postgres' || "$source:$target" == 'worker:postgres' ]]; then
      expected=true
    fi
    actual=blocked
    [[ "$allowed" == true ]] && actual=allowed
    pass=false
    [[ "$allowed" == "$expected" ]] && pass=true
    [[ "$pass" == true ]] || failures=$((failures + 1))
    expected_text=blocked
    [[ "$expected" == true ]] && expected_text=allowed
    printf '%s,%s,%s,%s,%s\n' "$source" "$destination" "$expected_text" "$actual" "$pass" >>"$output"
    printf '%-10s %-58s %-9s %-9s %s\n' "$source" "$destination" "$expected_text" "$actual" "$pass"
  done

  kubectl --context "$context" -n "$namespace" delete pod "$pod" --wait=true >/dev/null
  trap - EXIT
done

printf 'matrix_csv=%s mismatches=%s\n' "$output" "$failures"
if (( failures > 0 )) && [[ "$allow_mismatch" != true ]]; then
  exit 1
fi
