#!/usr/bin/env bash
# Repository gate entrypoint. The Makefile owns the steps.
set -euo pipefail
cd "$(dirname "$0")/.."
exec make check
