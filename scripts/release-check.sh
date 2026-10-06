#!/usr/bin/env bash
# Release consistency gate. Verifies that every version reference in the
# repo agrees with release/manifest.yaml: Helm image tags, Chart appVersion,
# compose image tags, docs, DB schema version, client versions, the runtime
# repo pin, and the contract hash. Run via `make release-check`.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

manifest="release/manifest.yaml"
if [[ ! -f "$manifest" ]]; then
  echo "ERROR: $manifest is missing" >&2
  exit 1
fi

version="$(awk '/^product:/{f=1;next} f&&/^  version:/{print $2;exit}' "$manifest")"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "ERROR: could not parse product.version from $manifest" >&2
  exit 1
fi

failures=0
fail() {
  echo "RELEASE-CHECK FAIL: $1" >&2
  failures=$((failures + 1))
}

# 1. Version references. Each file has a targeted rule so unrelated versions
#    (e.g. the Helm chart's own `version: 0.1.0`) are not flagged.
check_tags() {
  local file="$1" pattern="$2" strip="$3"
  [[ -f "$file" ]] || { fail "expected version-reference file missing: $file"; return; }
  while read -r ref; do
    [[ "$ref" == "$version" ]] || fail "$file references $ref but manifest pins $version"
  done < <(grep -hoE "$pattern" "$file" | sed "$strip" | sort -u)
}

check_tags infra/helm/allcallall/values.yaml 'tag: v[0-9]+\.[0-9]+\.[0-9]+' 's/^tag: //'
check_tags infra/docker-compose.production.yml ':v[0-9]+\.[0-9]+\.[0-9]+' 's/^://'
check_tags backend/README.md 'v[0-9]+\.[0-9]+\.[0-9]+' ''
check_tags docs/architecture/python-runtime-integration.md 'v[0-9]+\.[0-9]+\.[0-9]+' ''

# Chart appVersion has no leading v.
app_version="$(grep -hoE '^appVersion: "[0-9]+\.[0-9]+\.[0-9]+"' infra/helm/allcallall/Chart.yaml | sed 's/^appVersion: "//;s/"$//')"
if [[ "$app_version" != "${version#v}" ]]; then
  fail "Chart appVersion is $app_version but manifest pins ${version#v}"
fi

# 2. DB schema version must equal the highest numbered migration.
db_version="$(awk '/^database:/{f=1;next} f&&/^  schema_version:/{gsub(/"/,"",$2);print $2;exit}' "$manifest")"
latest_migration="$(ls backend/migrations | grep -E '^[0-9]{6}_' | sed 's/_.*//;s/^0*//' | sort -n | tail -1)"
if [[ "$db_version" != "$latest_migration" ]]; then
  fail "manifest database.schema_version=$db_version but latest migration is $latest_migration"
fi

# 3. Client versions.
check_client() {
  local label="$1" actual="$2" expected="$3"
  [[ "$actual" == "$expected" ]] || fail "$label is $actual but manifest pins $expected"
}

manifest_client() {
  awk -v key="$1" -v indent="$2" '
    $0 == indent "clients:" { in_clients=1; next }
    in_clients && $0 ~ "^" indent "  " key ":" { gsub(/"/, "", $2); print $2; exit }
  ' "$manifest"
}

check_client web "$(node -p "require('./web/package.json').version")" "$(manifest_client web '')"
check_client mobile-app "$(node -p "require('./mobile/app.json').expo.version")" "$(manifest_client mobile_app '')"
check_client mobile-version-code "$(node -p "require('./mobile/app.json').expo.android.versionCode")" "$(manifest_client mobile_app_version_code '')"
check_client mobile-package "$(node -p "require('./mobile/package.json').version")" "$(manifest_client mobile_package '')"
check_client desktop "$(node -p "require('./desktop/package.json').version")" "$(manifest_client desktop '')"

# 4. Runtime repo pin and contract hash, when the runtime repo is checked out
#    (sibling or in-tree, matching the Makefile lookup order).
runtime_dir="$(for d in allcallall-agent-runtime ../allcallall-agent-runtime; do [[ -d "$d" ]] && { echo "$d"; break; }; done)"
if [[ -n "$runtime_dir" ]]; then
  pinned_sha="$(awk '/^  allcallall-agent-runtime:/{f=1;next} f&&/^    sha:/{print $2;exit}' "$manifest")"
  head_sha="$(git -C "$runtime_dir" rev-parse HEAD)"
  if [[ "$head_sha" != "$pinned_sha" ]]; then
    fail "runtime repo at $runtime_dir is $head_sha but manifest pins $pinned_sha"
  fi

  contract_hash="$(awk '/^contracts:/{f=1;next} f&&/^  sha256:/{print $2;exit}' "$manifest")"
  actual_hash="$(cd "$runtime_dir/contracts/schemas" && { for f in $(ls *.json | sort); do cat "$f"; printf '\n'; done; } | sha256sum | awk '{print $1}')"
  if [[ "$actual_hash" != "$contract_hash" ]]; then
    fail "contract hash is $actual_hash but manifest pins $contract_hash"
  fi

  runtime_tag="$(awk '/^  allcallall-agent-runtime:/{f=1;next} f&&/^    tag:/{print $2;exit}' "$manifest")"
  if git -C "$runtime_dir" tag --list "$runtime_tag" | grep -q .; then
    echo "OK: runtime tag $runtime_tag exists"
  else
    echo "NOTICE: runtime tag $runtime_tag does not exist yet; expected before tagging."
  fi
else
  echo "NOTICE: runtime repo not checked out; skipping pin and contract hash checks."
fi

if [[ "$failures" -gt 0 ]]; then
  echo "RELEASE-CHECK: $failures failure(s)." >&2
  exit 1
fi

echo "release-check passed: all references agree with $manifest (product $version)."
