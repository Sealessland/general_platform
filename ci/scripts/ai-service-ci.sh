#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR/ai-service"

python -m compileall app tests
python -m unittest discover -s tests -v
python app/check_prompts.py
