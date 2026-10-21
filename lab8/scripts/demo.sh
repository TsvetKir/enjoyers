#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
demo_dir="$(mktemp -d)"
trap 'rm -rf "$demo_dir"' EXIT
for release in 2026.10.1 2026.10.2; do
  curl --fail-with-body --max-time 650 -sS \
    -H 'Content-Type: application/json' \
    -d "{\"release\":\"$release\",\"service\":\"checkout\"}" \
    http://localhost:15678/webhook/release-review \
    | tee "$demo_dir/response-$release.json"
  printf '\n'
done
python3 - "$demo_dir" <<'PY'
import json
import sys
from pathlib import Path
for release, status in [('2026.10.1','blocked'),('2026.10.2','ready')]:
    response=json.loads((Path(sys.argv[1]) / f'response-{release}.json').read_text())
    assert response['status']==status, response
    trace=json.loads(Path('outputs',response['run_id']+'.json').read_text())
    assert len(trace['discovery'])==2 and len(trace['tool_calls'])==4
    assert trace['model_calls'] and trace['analysis']
    assert all(t['status']=='success' for t in trace['tool_calls'])
    assert Path('outputs',response['run_id']+'.md').is_file()
    print(f'PASS {release}: {status}; 2 MCP servers, 4 tool calls, saved AI report')
PY
