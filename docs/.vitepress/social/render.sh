#!/usr/bin/env bash
# Render the social cover (cover.html) to docs/public/og.png with headless
# Chrome. Needs Chrome or Chromium and the docs dependencies (npm ci).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
out="$here/../../public/og.png"
chrome="${CHROME:-}"
if [[ -z "$chrome" ]]; then
  for c in "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
           google-chrome chromium chromium-browser; do
    if command -v "$c" >/dev/null 2>&1 || [[ -x "$c" ]]; then chrome="$c"; break; fi
  done
fi
[[ -n "$chrome" ]] || { echo "Chrome not found; set CHROME" >&2; exit 1; }

profile="$(mktemp -d)"
trap 'rm -rf "$profile"' EXIT
"$chrome" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
  --allow-file-access-from-files --user-data-dir="$profile" \
  --virtual-time-budget=3000 --window-size=1200,630 \
  --screenshot="$out" "file://$here/cover.html" 2>/dev/null
echo "wrote $out"
