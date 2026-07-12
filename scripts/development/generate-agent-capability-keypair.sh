#!/usr/bin/env bash

set -euo pipefail

umask 077
private_pem="$(mktemp)"
trap 'rm -f "$private_pem"' EXIT

openssl genpkey -algorithm ED25519 -out "$private_pem"

# The application accepts the raw 32-byte RFC 8032 seed and public key.
private_seed="$(openssl pkey -in "$private_pem" -outform DER | tail -c 32 | base64 | tr -d '\n')"
public_key="$(openssl pkey -in "$private_pem" -pubout -outform DER | tail -c 32 | base64 | tr -d '\n')"

printf 'MCP_CAPABILITY_ED25519_PRIVATE_KEY=%s\n' "$private_seed"
printf 'SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY=%s\n' "$public_key"
