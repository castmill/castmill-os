#!/usr/bin/env bash
# Print the Castmill OS version (SemVer 2.0.0) for the checked-out commit.
#
#   tag vX.Y.Z on HEAD            -> X.Y.Z
#   tag vX.Y.Z-rc.N on HEAD       -> X.Y.Z-rc.N
#   N commits after vX.Y.Z        -> X.Y.(Z+1)-dev.N.gSHA
#   N commits after vX.Y.Z-rc.K   -> X.Y.Z-rc.K.dev.N.gSHA
#   no release tag yet            -> 0.1.0-dev.N.gSHA
#
# Development versions use pre-release identifiers, not "+build" metadata:
# os-release's VERSION_ID only allows [a-zA-Z0-9._-].
set -euo pipefail

FIRST_VERSION="0.1.0"
semver_tag='v[0-9]*.[0-9]*.[0-9]*'

sha="$(git rev-parse --short=7 HEAD)"
if desc="$(git describe --tags --long --match "$semver_tag" 2>/dev/null)"; then
  # desc = vX.Y.Z[-pre]-N-gSHA
  tag="${desc%-*-g*}"
  rest="${desc#"$tag"-}"
  n="${rest%%-*}"
  base="${tag#v}"
  if [[ ! "$base" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$ ]]; then
    echo "error: tag $tag is not SemVer (vMAJOR.MINOR.PATCH[-pre])" >&2
    exit 1
  fi
  if [[ "$n" == 0 ]]; then
    echo "$base"
  elif [[ -n "${BASH_REMATCH[4]}" ]]; then
    echo "$base.dev.$n.g$sha"
  else
    echo "${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))-dev.$n.g$sha"
  fi
else
  echo "$FIRST_VERSION-dev.$(git rev-list --count HEAD).g$sha"
fi
