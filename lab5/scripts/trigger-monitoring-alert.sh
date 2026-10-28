#!/usr/bin/env bash
# Deliberately fail only API metric scraping; restore even on interruption.
set -euo pipefail
context="${KUBE_CONTEXT:?Set KUBE_CONTEXT explicitly to the verified lab 5 context}"
namespace="${KUBE_NAMESPACE:-shop}"
release="${HELM_RELEASE:-shop}"
[[ "$context" == kind-lab5 ]] || { echo 'Refusing a context other than kind-lab5' >&2; exit 1; }
kubectl config get-contexts "$context" -o name | grep -Fx "$context" >/dev/null
k=(kubectl --context "$context" -n "$namespace")
monitor="$release-api"
original="$("${k[@]}" get servicemonitor "$monitor" -o 'jsonpath={.spec.endpoints[0].path}')"
[[ "$original" == /metrics ]] || { echo 'Expected /metrics path; refusing to overwrite custom configuration' >&2; exit 1; }
restore() {
  local current
  current="$("${k[@]}" get servicemonitor "$monitor" -o 'jsonpath={.spec.endpoints[0].path}')" || return 1
  [[ "$current" == /metrics ]] && return 0
  # Do not overwrite a path changed by another operator during the experiment.
  if ! "${k[@]}" patch servicemonitor "$monitor" --type=json -p='[{"op":"test","path":"/spec/endpoints/0/path","value":"/deliberately-missing-metrics"},{"op":"replace","path":"/spec/endpoints/0/path","value":"/metrics"}]'; then
    echo 'Automatic restore failed or scrape path changed concurrently; inspect the ServiceMonitor before restoring /metrics manually.' >&2
    return 1
  fi
}
sleep_pid=''
cleanup() {
  status=$?
  trap - EXIT
  if [[ -n "$sleep_pid" ]]; then
    kill "$sleep_pid" 2>/dev/null || true
    wait "$sleep_pid" 2>/dev/null || true
  fi
  restore || status=1
  exit "$status"
}
trap 'exit 130' INT
trap 'exit 143' TERM
trap cleanup EXIT
"${k[@]}" patch servicemonitor "$monitor" --type=json -p='[{"op":"test","path":"/spec/endpoints/0/path","value":"/metrics"},{"op":"replace","path":"/spec/endpoints/0/path","value":"/deliberately-missing-metrics"}]'
echo 'Scraping intentionally broken for 180 seconds. Capture ShopAPIMetricsUnavailable=firing in Prometheus now.'
sleep 180 &
sleep_pid=$!
wait "$sleep_pid"
sleep_pid=''
echo 'Restoring /metrics. Verify targets return to UP and the alert resolves.'
