#!/bin/sh
# AllCallAll pre-commit secret scanner.
#
# Scans staged changes for secrets BEFORE they leave the machine.
# Three layers (any hit blocks the commit):
#   1. Self-referential env defaults: a variable whose name signals a secret
#      is given a default that points back to itself.
#      -- GitGuardian's generic detector false-positives on these; we block them
#         locally so the pattern can never reach a PR again. Comment lines are
#         skipped so documentation/examples are not flagged.
#   2. High-signal provider-pattern regex (AWS/GitHub/OpenAI/Slack/private keys).
#      Always on (gitleaks sometimes needs extra context to fire on these).
#   3. gitleaks (full rule set) when installed.
#
# Exit 0 = clean (commit proceeds).  Exit 1 = leak found (commit blocked).
# Intentionally bypass with:  git commit --no-verify   (not recommended)

set -eu

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
CONFIG="$ROOT/.gitleaks.toml"

# Staged, added/changed/modified, tracked files only.
STAGED="$(git diff --cached --name-only --diff-filter=ACM 2>/dev/null || true)"
if [ -z "$STAGED" ]; then
  exit 0
fi

echo "Scanning staged files for secrets..."

# ---------------------------------------------------------------------------
# Layer 1: self-referential secret env defaults (the GitGuardian false-positive
# class). Blocks the self-referential env-default idiom: a variable whose
# name signals a secret is given a default that points back to itself.
# The default message must contain a secret keyword to trigger, so benign
# "${VAR}" and "${VAR:?required}" are NOT flagged. Comment lines are skipped.
# ---------------------------------------------------------------------------
selfref_tmp="$(mktemp)"
content_tmp="$(mktemp)"
while IFS= read -r f; do
  [ -z "$f" ] && continue
  git show ":$f" > "$content_tmp" 2>/dev/null || continue
  [ -s "$content_tmp" ] || continue
  # One awk process per file. The previous implementation piped the file into a
  # `while read` loop and spawned a `grep -qE` process for EVERY line, which
  # hung for minutes once large generated files (schema.d.ts, ~6k lines) were
  # staged. awk keeps identical semantics: skip blank lines and comment lines
  # (YAML/shell #, go //, sql --, ini ;, go doc *), then match the pattern.
  awk -v F="$f" '
    /^[[:space:]]*$/ { next }
    /^[[:space:]]*(#|\/\/|;|--|\*)/ { next }
    $0 ~ /[$][{][A-Za-z_]+:[-?][^}]*(PASSWORD|SECRET|KEY|TOKEN)[^}]*[}]/ {
      print F ":" FNR ": " $0
    }
  ' "$content_tmp" >> "$selfref_tmp"
done <<EOF
$STAGED
EOF
rm -f "$content_tmp"
if [ -s "$selfref_tmp" ]; then
  cat "$selfref_tmp"
  rm -f "$selfref_tmp"
  echo ""
  echo "Blocked: a secret-looking env default was found (self-referential default)."
  echo "  GitGuardian flags this as a Generic Password. Use plain \${VAR} or a"
  echo "  non-secret error message (e.g. \${VAR:?required})."
  echo "  Bypass only with: git commit --no-verify"
  exit 1
fi
rm -f "$selfref_tmp"

# ---------------------------------------------------------------------------
# Layer 2: high-signal provider patterns (always on).
# NOTE: patterns are written so they do NOT match their own definition inside
# this script (e.g. AKIA is followed by a regex class, not 16 real alphanumerics).
# ---------------------------------------------------------------------------
prov_src="$(mktemp)"
while IFS= read -r f; do
  [ -z "$f" ] && continue
  git show ":$f" >> "$prov_src" 2>/dev/null || true
done <<EOF
$STAGED
EOF
prov_hit="$(grep -nE 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{36,}|sk-[A-Za-z0-9]{32,}|xox[baprs]-[A-Za-z0-9-]{10,}|-----BEGIN (RSA|EC|OPENSSH|PRIVATE) PRIVATE KEY-----' "$prov_src" || true)"
rm -f "$prov_src"
if [ -n "$prov_hit" ]; then
  echo ""
  echo "Blocked: high-signal secret pattern detected in staged changes:"
  printf '%s\n' "$prov_hit"
  echo "  Rotate the credential, use an env-var reference, and re-stage."
  echo "  Bypass only with: git commit --no-verify"
  exit 1
fi

# ---------------------------------------------------------------------------
# Layer 3: gitleaks (preferred, full rule set)
# ---------------------------------------------------------------------------
if command -v gitleaks >/dev/null 2>&1; then
  if [ -f "$CONFIG" ]; then
    gitleaks protect --staged --redact --exit-code 1 --config "$CONFIG" --no-banner 2>&1 || {
      echo ""
      echo "gitleaks detected a secret in your staged changes."
      echo "  Rotate the leaked credential, replace it with an env-var reference,"
      echo "  and re-stage. False positive? allowlist the path in .gitleaks.toml."
      echo "  Force the commit only with: git commit --no-verify"
      exit 1
    }
  else
    gitleaks protect --staged --redact --exit-code 1 --no-banner 2>&1 || {
      echo "gitleaks detected a secret in your staged changes (see above)."
      exit 1
    }
  fi
  echo "No secrets detected (gitleaks + patterns)."
  exit 0
fi

echo "No secrets detected (patterns only; install gitleaks for full coverage)."
exit 0
