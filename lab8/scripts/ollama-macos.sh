#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(uname -s)" != Darwin ]]; then
  echo 'Use the container-model Compose profile on Linux.' >&2
  exit 1
fi
runtime="$PWD/.runtime/ollama"
mkdir -p "$runtime"
if [[ ! -x "$runtime/ollama" ]]; then
  curl --fail --location --retry 2 \
    https://github.com/ollama/ollama/releases/download/v0.11.4/ollama-darwin.tgz \
    --output "$runtime/ollama.tgz"
  tar -xzf "$runtime/ollama.tgz" -C "$runtime"
fi
export OLLAMA_MODELS="$runtime/models"
case "${1:-serve}" in
  serve) OLLAMA_HOST=0.0.0.0:11434 exec "$runtime/ollama" serve ;;
  pull) exec "$runtime/ollama" pull qwen3:1.7b ;;
  *) echo 'Usage: bash scripts/ollama-macos.sh [serve|pull]' >&2; exit 1 ;;
esac
