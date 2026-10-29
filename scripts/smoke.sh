#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p bin var
go build -o bin/app ./cmd/app
python3 scripts/mvp_smoke.py
