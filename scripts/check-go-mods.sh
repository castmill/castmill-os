#!/usr/bin/env bash
# Keep the castmill-agent Yocto recipe's Go module pins in sync with agent/go.mod.
#
#   scripts/check-go-mods.sh            fail if versions differ (CI)
#   scripts/check-go-mods.sh --update   rewrite the .inc (downloads module zips
#                                       from proxy.golang.org to compute sha256)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INC="$ROOT/layers/meta-castmill/recipes-castmill/castmill-agent/castmill-agent-go-mods.inc"

# Modules the build needs: everything in go.mod's require blocks.
mods="$(cd "$ROOT/agent" && go list -m -f '{{if not .Main}}{{.Path}} {{.Version}}{{end}}' all |
  while read -r path version; do
    if (cd "$ROOT/agent" && grep -q "^[[:space:]]*$path $version" go.mod); then echo "$path $version"; fi
  done)"

sha256() { if command -v sha256sum >/dev/null; then sha256sum; else shasum -a 256; fi | cut -d' ' -f1; }

if [[ "${1:-}" == "--update" ]]; then
  {
    echo "# Go module dependencies of castmill-agent, pinned by checksum (from agent/go.sum)."
    echo "# Regenerate with scripts/check-go-mods.sh --update."
    echo "SRC_URI += \" \\"
    while read -r path version; do
      # Module proxy paths escape upper-case letters as !lower.
      esc="$(printf '%s' "$path" | awk '{ o = ""; for (i = 1; i <= length($0); i++) { c = substr($0, i, 1); o = o (c ~ /[A-Z]/ ? "!" tolower(c) : c) } print o }')"
      sum="$(curl -fsSL "https://proxy.golang.org/$esc/@v/$version.zip" | sha256)"
      echo "    gomod://$path;version=$version;sha256sum=$sum \\"
    done <<< "$mods"
    echo '"'
  } > "$INC"
  echo "updated $INC"
  exit 0
fi

fail=0
while read -r path version; do
  if ! grep -q "gomod://$path;version=$version;" "$INC"; then
    echo "missing or outdated in $(basename "$INC"): $path $version" >&2
    fail=1
  fi
done <<< "$mods"
pinned="$(grep -c 'gomod://' "$INC")"
wanted="$(grep -c . <<< "$mods")"
if [[ "$pinned" != "$wanted" ]]; then
  echo "$(basename "$INC") pins $pinned modules, go.mod requires $wanted" >&2
  fail=1
fi
if [[ $fail != 0 ]]; then
  echo "run: scripts/check-go-mods.sh --update" >&2
  exit 1
fi
echo "Go module pins OK ($wanted modules)"
