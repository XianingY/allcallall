# SaaS Launch Readiness Checklist

This is the go/no-go checklist for a **public SaaS release**. It complements,
and does not replace, the [Beta Smoke Checklist](../development/beta-smoke.md).
The Beta checklist validates the product loop for a small team; this document
adds deployment, security, support, billing, rollout, and rollback evidence
needed before opening the service to external users.

## How to use this checklist

- Do not mark an item complete from code review, unit tests, or "it should
  work". Each item names the evidence that must be attached to the launch
  record.
- A launch requires every **hard gate** to pass. **Conditional gates** are
  required only when the named capability is exposed in the release.
- Record the release commit, deployment URL, evidence links, and date in the
  launch ticket. A checklist without an associated commit and environment is
  not launch evidence.
- If a hard gate fails, the launch is blocked. Do not soften the gate by
  deleting the check; either fix the issue or explicitly reduce the launch
  scope and record the decision.

## 1. Release candidate and automated gates

Hard gate for every launch.

- [ ] Release candidate is a specific git commit on `main`, not a local
  working tree or mutable branch head.
  Evidence: commit SHA and `git status --porcelain` output.
- [ ] `make verify-full` passes at the release commit.
  Evidence: command output, including backend build/vet/tests, web and mobile
  type/lint/test suites, desktop check, and Python Agent/RAG runtime tests.
- [ ] `ci.yml` passes at the release commit.
  Evidence: GitHub Actions run URL and conclusion.
- [ ] `platform-ci.yml` passes at the release commit.
  Evidence: GitHub Actions run URL and conclusion.
- [ ] `security.yml` passes at the release commit.
  Evidence: GitHub Actions run URL and conclusion.
- [ ] The OpenAPI contract is committed and in sync.
  Evidence: `make web-contract-check` output.
- [ ] No forbidden files are in the release commit: `.env`, `.omo`,
  `.workbuddy`, or `output/`.
  Evidence: `git diff --name-only <previous-release>..<release-commit>`.

## 2. Deployment and network

Hard gate for every launch.

- [ ] The production deployment uses the same image digests and migration
  version as the release commit.
  Evidence: deployment manifest/image digests and migration job output.
- [ ] The database schema migration succeeds before API Pods receive traffic.
  `DB_AUTO_MIGRATE=0` is required in production.
  Evidence: migration job logs and runtime configuration.
- [ ] `/api/v1/health` and `/api/v1/ready` pass after deployment.
  Evidence: probe URLs or monitoring screenshots including timestamps.
- [ ] TLS is terminated at a public edge and `SECURITY_REQUIRE_TLS=true`.
  Plain-text `/api/v1` requests are rejected, while kubelet/Compose health
  probes remain reachable.
  Evidence: `curl -i http://.../api/v1/...` rejection and successful HTTPS
  health check.
- [ ] Public origins are explicit in `CORS_ALLOWED_ORIGINS`; no wildcard
  origin is used with credentials.
  Evidence: rendered deployment configuration.
- [ ] `X-Forwarded-Proto` is set correctly by the edge proxy.
  Evidence: a successful authenticated HTTPS API request.
- [ ] Web, API, WebSocket, and signaling proxy timeouts support long-lived
  realtime connections.
  Evidence: two concurrent browser sessions remain connected for at least 30
  minutes and recover after a client reconnect.
- [ ] TURN is reachable from the public internet and restricted-network
  conditions.
  Evidence: successful audio/video call on cellular or another restricted
  network.
- [ ] Public DNS, certificate renewal, and TLS certificate monitoring are
  configured.
  Evidence: DNS records, renewal job, and monitoring alert.

## 3. Product and realtime validation

Hard gate for every launch.

- [ ] The complete Beta Smoke Checklist passes on the production deployment,
  not only on a local or staging environment.
  Evidence: completed checklist and dated screenshots/recording.
- [ ] Two users can register, join the same organization, chat, and call.
  Evidence: user IDs, conversation/room IDs, and timestamps.
- [ ] Recording start/stop and recording playback/download work.
  Evidence: recording ID and a successful authenticated download.
- [ ] Real transcription runs with `TRANSCRIPTION_PROVIDER` set to
  `openai_compatible`; mock output is not presented to users.
  Evidence: provider configuration summary (secret redacted) and a ready
  transcript.
- [ ] Agent meeting briefs run with `AGENT_PROVIDER_STRICT=true` and include
  citations that resolve to the transcript.
  Evidence: run ID, transcript ID, and citation links.
- [ ] Read tools execute automatically and write tools enter approval.
  Evidence: one approved write tool and one rejected write tool.
- [ ] The release scope explicitly documents the supported room size and
  single-media-node Beta topology.
  Evidence: launch scope section in the release ticket.
- [ ] Mobile production build installs and launches on the supported Android
  and iOS minimum versions.
  Evidence: signed build IDs and device/OS matrix.
- [ ] Mobile call, microphone, speaker routing, push notification, file
  download, and app foreground/background behavior are validated on real
  Android and iOS devices.
  Evidence: device model/OS matrix and screen recordings or QA records.

## 4. Security and privacy

Hard gate for every launch.

- [ ] The quarterly pentest workflow passes at the release commit.
  Evidence: `quarterly-pentest.yml` run URL and conclusion.
- [ ] DAST has a real staging target configured as `STAGING_BASE_URL`, and
  the ZAP baseline scan runs rather than being skipped.
  Evidence: workflow log showing a non-empty target and ZAP completion.
- [ ] SAST/SCA findings that are not ignored have been triaged with an owner
  and due date.
  Evidence: finding list and triage notes.
- [ ] Every entry in `.trivyignore` or equivalent ignore files has a current
  justification and expiry/review date.
  Evidence: ignore-file diff and security-team review.
- [ ] Secrets are supplied by the deployment secret manager, not image layers,
  environment files committed to git, or public configuration.
  Evidence: secret inventory and deployment manifests.
- [ ] Internal support APIs are reachable only from an administrative network
  and require `X-Support-Token`.
  Evidence: rejected request from the public edge and successful request from
  the allowed network.
- [ ] Metrics endpoints are not publicly exposed; internal-network and bearer
  token controls work as configured.
  Evidence: external request rejection and internal Prometheus scrape.
- [ ] Message retention, encryption, recall, erasure, moderation, and search
  minimization policies are configured intentionally.
  Evidence: privacy policy configuration summary and one exercised policy.
- [ ] Account deletion removes the documented personal data and leaves only
  the non-reversible audit summary.
  Evidence: deletion test user ID before/after database and API checks.
- [ ] Legal pages, support email, and current terms/privacy versions are
  reachable before and after login.
  Evidence: public URLs and version numbers.
- [ ] Any E2EE feature remains disabled unless a release-specific security
  review proves media-frame encryption for the production WebRTC runtime.
  Evidence: feature flag state and security review.

## 5. Billing and vendor integration

Conditional hard gate when billing, push, or commercial entitlements are
exposed.

- [ ] RevenueCat public and backend keys belong to the production project.
  Evidence: provider project ID and non-secret key fingerprint.
- [ ] Web and mobile subscription purchase flows work in production or the
  production app-store environment.
  Evidence: transaction IDs and entitlement state.
- [ ] Webhook signing and replay protection are validated.
  Evidence: signed webhook test and rejected invalid-signature test.
- [ ] Entitlement downgrade/cancellation behavior is exercised.
  Evidence: before/after entitlement records.
- [ ] Firebase Web Push and mobile FCM use production sender/project IDs.
  Evidence: successful Web Push and mobile notification.
- [ ] Provider rate limits, quota alerts, and billing contacts are documented.
  Evidence: vendor console settings and alert routing.

## 6. Operations, support, and recovery

Hard gate for every launch.

- [ ] Backup jobs run successfully and produce non-empty archives.
  Evidence: backup job log, archive size, and content listing.
- [ ] A full restore drill has been completed on an isolated environment from
  the current backup format.
  Evidence: restore environment, restore time, and post-restore smoke test.
- [ ] Recording storage backup/versioning is enabled for S3 or the equivalent
  local-volume backup procedure is documented and tested.
  Evidence: provider settings or tested archive procedure.
- [ ] Prometheus, Grafana, and log collection are deployed and reachable only
  from the internal network.
  Evidence: dashboard URL and access-control check.
- [ ] Alertmanager receivers are configured and have received a test alert.
  Evidence: test alert timestamp in the configured channel.
- [ ] On-call rotation, escalation contact, and support hours are defined.
  Evidence: on-call schedule and escalation document.
- [ ] The support runbook is reachable to the on-call engineer.
  Evidence: runbook URL and acknowledgement from the responder.
- [ ] The abuse-report and account-deletion support path works end to end.
  Evidence: support email receipt and internal support API record.
- [ ] A rollback plan exists with the prior image digests, database plan, and
  required migration decision.
  Evidence: tested rollback or a migration-reversal review.

## 7. Rollout and launch record

Hard gate for every launch.

- [ ] The launch record records commit SHA, image digests, deployment URL,
  migration version, environment, and date.
  Evidence: launch ticket.
- [ ] A canary or limited-user rollout is used before full exposure.
  Evidence: rollout stages and observed user counts.
- [ ] Monitoring shows no critical alert during the soak period.
  Evidence: alert history during the documented soak window.
- [ ] A rollback has been rehearsed or the release contains no schema change
  requiring special rollback handling.
  Evidence: rollback drill output or migration review.
- [ ] Support and status communication channels are published.
  Evidence: support URL and status/incident channel.
- [ ] All hard-gate failures are either resolved or covered by a documented
  launch-scope reduction approved by the product owner.
  Evidence: launch decision record.

## Relationship to Beta validation

The Beta checklist proves the product loop for a 3-6 person workspace. Public
SaaS additionally requires deployment, security, vendor, support, capacity,
and rollback evidence. Passing one does not substitute for the other.

Use the deployment, privacy, observability, and Kubernetes references from
[the documentation index](../../README.md) as the implementation authority for
each section above.
