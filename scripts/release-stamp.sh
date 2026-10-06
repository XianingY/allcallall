#!/usr/bin/env bash
# Writes the current HEAD SHA into release/manifest.yaml under
# repositories.allcallall.sha. Run immediately before tagging so the tagged
# commit's manifest is self-referential. See release/README.md.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

manifest="release/manifest.yaml"
[[ -f "$manifest" ]] || { echo "ERROR: $manifest is missing" >&2; exit 1; }

# Untracked files are ignored: they are not part of the tagged commit, so
# they cannot make the stamp wrong.
dirty="$(git status --porcelain --untracked-files=no | grep -v '^.. release/manifest.yaml' || true)"
if [[ -n "$dirty" ]]; then
  echo "ERROR: commit or stash other changes before stamping:" >&2
  echo "$dirty" >&2
  exit 1
fi

sha="$(git rev-parse HEAD)"
tmp="$(mktemp)"
awk -v sha="$sha" '
  !done && /^    sha:/ { sub(/sha: .*/, "sha: " sha); done=1 }
  { print }
' "$manifest" > "$tmp"
mv "$tmp" "$manifest"

echo "Stamped repositories.allcallall.sha = $sha in $manifest"
