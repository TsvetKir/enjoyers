#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# Default: native Ollama on host, listening on 0.0.0.0:11434.
# Linux/container alternative:
# OLLAMA_URL=http://ollama:11434 docker compose --profile container-model up -d ollama
# docker compose exec ollama ollama pull qwen3:1.7b
# export OLLAMA_URL=http://ollama:11434

mkdir -p outputs
export LOCAL_UID="$(id -u)" LOCAL_GID="$(id -g)"
docker compose up -d --build
for i in {1..180}; do
  if curl -fsS http://localhost:15678/healthz >/dev/null 2>&1; then break; fi
  sleep 2
done
curl -fsS http://localhost:15678/healthz >/dev/null
docker compose exec -T n8n n8n import:workflow --input=/workflows/release-readiness.export.json
docker compose exec -T n8n n8n update:workflow --id=lab8ReleaseReview --active=true
docker compose restart n8n
for i in {1..180}; do
  if curl -fsS http://localhost:15678/healthz >/dev/null 2>&1; then break; fi
  sleep 2
done
curl -fsS http://localhost:15678/healthz >/dev/null
printf 'n8n: http://localhost:15678\nGrafana: http://localhost:13008/d/lab8-release\n'
