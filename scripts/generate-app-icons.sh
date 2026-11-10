#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"
node web/scripts/generate-pwa-icons.mjs
# The generated Windows-only object files are committed with the source, so
# normal builds do not need a resource compiler or download a new dependency.
# Regeneration uses a pinned, isolated Go tool and does not edit go.mod.
go run github.com/tc-hib/go-winres@v0.3.3 make \
  --arch amd64 --in assets/branding/winres.json --out cmd/ta/rsrc
cp cmd/ta/rsrc_windows_amd64.syso cmd/ta-updater/rsrc_windows_amd64.syso
