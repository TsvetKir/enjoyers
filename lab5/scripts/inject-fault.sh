#!/usr/bin/env bash
set -euo pipefail

context="${KUBE_CONTEXT:-kind-lab5}"
release="${HELM_RELEASE:-shop}"
namespace="${KUBE_NAMESPACE:-shop}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
lab_dir="$(cd -- "$script_dir/.." && pwd)"
overlay="$lab_dir/reports/active-fault-values.yaml"
mkdir -p -- "$(dirname -- "$overlay")"

case "$((RANDOM % 4))" in
  0) cat >"$overlay" <<'YAML'
networkPolicy:
  ports:
    dns: 54
YAML
  ;;
  1) cat >"$overlay" <<'YAML'
networkPolicy:
  ports:
    postgres: 5433
YAML
  ;;
  2) cat >"$overlay" <<'YAML'
networkPolicy:
  dns:
    podValue: wrong-dns-label
YAML
  ;;
  3) cat >"$overlay" <<'YAML'
networkPolicy:
  ports:
    api: 8088
YAML
  ;;
esac

helm --kube-context "$context" upgrade "$release" "$lab_dir/charts/shop" \
  -n "$namespace" --reset-values -f "$lab_dir/charts/shop/values.yaml" -f "$overlay" \
  --wait --timeout 10m
echo 'A hidden network fault was applied. Do not inspect active-fault-values.yaml until diagnosis is complete.'
