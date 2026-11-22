#!/usr/bin/env bash
# Check commit messages: Conventional Commits subject + DCO sign-off.
#
#   scripts/check-commits.sh BASE HEAD    check commits in BASE..HEAD
#
# See CONTRIBUTING.md#commit-messages.
set -euo pipefail

range="${1:?usage: check-commits.sh BASE HEAD}..${2:?usage: check-commits.sh BASE HEAD}"
types='feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert'
subject_re="^($types)(\([a-z0-9._/-]+\))?!?: [^ ].{0,98}$"

fail=0
for c in $(git rev-list --no-merges "$range"); do
  subject="$(git log -1 --format=%s "$c")"
  body="$(git log -1 --format=%B "$c")"
  author="$(git log -1 --format='%an <%ae>' "$c")"
  short="$(git rev-parse --short "$c")"
  if ! [[ "$subject" =~ $subject_re ]]; then
    echo "::error::$short: subject is not a Conventional Commit: \"$subject\"" >&2
    echo "  expected: <type>(<scope>)!: <summary>, type one of: ${types//|/, }" >&2
    fail=1
  fi
  if ! grep -qF "Signed-off-by: $author" <<< "$body"; then
    echo "::error::$short: missing DCO sign-off \"Signed-off-by: $author\" (use git commit -s)" >&2
    fail=1
  fi
done
if [[ $fail == 0 ]]; then
  echo "commits OK ($(git rev-list --no-merges --count "$range"))"
fi
exit $fail
