# Security Policy

## Supported Versions

AllCallAll is pre-1.0. Security fixes are made on the current default branch;
older snapshots and downstream forks are not maintained by this project.

## Reporting a Vulnerability

Do not open a public issue for a suspected vulnerability. Send a private report
to `security@allcallall.com` with:

- the affected component and commit SHA;
- reproduction steps or a minimal proof of concept;
- the expected impact and required permissions or feature flags;
- any suggested mitigation or disclosure constraints.

Maintainers will coordinate triage, remediation, and disclosure with the
reporter. Timing depends on severity, reproducibility, and release risk; this
policy does not promise a fixed response or remediation window.

## Scope

In scope:

- authentication, authorization, tenant isolation, approvals, and audit paths;
- API, Web, mobile, desktop, worker, and Agent/RAG integration boundaries;
- secrets handling, transport security, recording storage, and deployment
  assets maintained in this repository;
- CI, build, packaging, and release configuration.

Generally out of scope:

- reports without a reproducible security impact;
- vulnerabilities already publicly documented by an upstream dependency;
- volumetric denial-of-service reports without a product-specific flaw;
- attacks requiring a previously compromised device, browser extension, or
  self-hosted deployment modified outside this repository.

## Safe Harbor

Good-faith research is welcome when it avoids privacy violations, data loss,
service disruption, persistence, and access beyond what is needed to
demonstrate the issue. Allow maintainers a reasonable opportunity to remediate
before public disclosure.

For non-sensitive usage questions and ordinary bugs, use [SUPPORT.md](SUPPORT.md).
