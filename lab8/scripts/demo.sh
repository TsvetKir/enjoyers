#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p evidence
for release in 2026.10.1 2026.10.2; do
  curl --fail-with-body --max-time 650 -sS \
    -H 'Content-Type: application/json' \
    -d "{\"release\":\"$release\",\"service\":\"checkout\"}" \
    http://localhost:15678/webhook/release-review \
    | tee "evidence/response-$release.json"
  printf '\n'
done
python3 - <<'PY'
import json
from pathlib import Path
for release, status in [('2026.10.1','blocked'),('2026.10.2','ready')]:
    response=json.loads(Path(f'evidence/response-{release}.json').read_text())
    assert response['status']==status, response
    trace=json.loads(Path('outputs',response['run_id']+'.json').read_text())
    assert len(trace['discovery'])==2 and len(trace['tool_calls'])==4
    assert trace['model_calls'] and trace['analysis']
    assert all(t['status']=='success' for t in trace['tool_calls'])
    assert Path('outputs',response['run_id']+'.md').is_file()
    print(f'PASS {release}: {status}; 2 MCP servers, 4 tool calls, saved AI report')
PY
