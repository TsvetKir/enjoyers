#!/usr/bin/env python3
"""Render a dependency-free SVG from load-client.sh's request CSV."""
import csv
import datetime as dt
import pathlib
import sys

source = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "../reports/chaos-results.csv")
target = pathlib.Path(sys.argv[2] if len(sys.argv) > 2 else "../reports/chaos-availability.svg")
rows = list(csv.DictReader(source.open(encoding="utf-8-sig", newline="")))
if not rows:
    raise SystemExit("CSV has no samples")
start = dt.datetime.fromisoformat(rows[0]["timestamp"])
samples = [( (dt.datetime.fromisoformat(r["timestamp"]) - start).total_seconds(),
             r["success"].lower() == "true", float(r["latency_ms"])) for r in rows]
width, height, left, right = 1000, 440, 70, 970
top1, bottom1, top2, bottom2 = 45, 205, 270, 390
duration = max(samples[-1][0], 1)
latmax = max(max(s[2] for s in samples), 1)
failed = sum(not ok for _, ok, _ in samples)
x = lambda t: left + (right-left) * t / duration
y = lambda v: bottom2 - (bottom2-top2) * v / latmax
svg = ['<svg xmlns="http://www.w3.org/2000/svg" width="1000" height="440" viewBox="0 0 1000 440">',
 '<rect width="100%" height="100%" fill="white"/>',
 '<style>text{font:14px sans-serif}.axis{stroke:#555}.ok{fill:#167d46}.bad{fill:#cc3344}.lat{fill:#2463a6}</style>',
 '<text x="70" y="25" font-size="18">Shop request result during pod chaos</text>',
 f'<text x="70" y="63">samples={len(samples)} failed={failed}; HTTP 201 success = green, failed request = red</text>',
 f'<line class="axis" x1="{left}" y1="{top1}" x2="{left}" y2="{bottom1}"/><line class="axis" x1="{left}" y1="{bottom1}" x2="{right}" y2="{bottom1}"/>',
 '<text x="12" y="105">ok</text><text x="12" y="188">fail</text>',
 '<text x="70" y="246" font-size="18">Client response latency</text>',
 f'<line class="axis" x1="{left}" y1="{top2}" x2="{left}" y2="{bottom2}"/><line class="axis" x1="{left}" y1="{bottom2}" x2="{right}" y2="{bottom2}"/>',
 f'<text x="7" y="{top2+6}">{latmax:.0f} ms</text><text x="30" y="{bottom2}">0</text><text x="450" y="425">elapsed seconds (0–{duration:.0f})</text>']
step = max(len(samples)//1000, 1)
for i, (t, ok, latency) in enumerate(samples):
    svg.append(f'<circle class="{"ok" if ok else "bad"}" cx="{x(t):.1f}" cy="{100 if ok else 180}" r="2.2"/>')
    if i % step == 0:
        svg.append(f'<circle class="lat" cx="{x(t):.1f}" cy="{y(latency):.1f}" r="1.5"/>')
svg.append('</svg>')
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text('\n'.join(svg), encoding='utf-8')
print(f"samples={len(samples)} failed={failed} svg={target}")
