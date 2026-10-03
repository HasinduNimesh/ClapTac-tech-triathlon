#!/usr/bin/env bash
# Unit tests for the Python order/dashboard assistants. Uses the local Python
# when its dependencies are installed, otherwise a throwaway python:3.12 container.
set -euo pipefail
cd "$(dirname "$0")/../services/agent-assistants"
if python3 -c "import langgraph, fastapi, httpx, jwt, prometheus_client" 2>/dev/null; then
  python3 -m unittest discover -s tests -t .
else
  docker run --rm -v "$PWD:/app" -w /app python:3.12-slim \
    sh -c "pip install -q --disable-pip-version-check -r requirements.txt && python -m unittest discover -s tests -t ."
fi
