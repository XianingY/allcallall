# Worktree Artifacts

This note tracks local artifacts that may appear during backend/Web verification and should not be committed as source.

## Current Local Artifact

- `backend/internal/collaboration/recordings/`
  - Source: local recording-storage test/development output.
  - Current observed content: `org-1/room-1/session-1/session.json`.
  - Decision: keep it untracked unless a test fixture explicitly needs it. Do not stage it during feature, refactor, or documentation commits.

## Rule

Generated recordings, local benchmark outputs, temporary login JSON files, and provider credentials stay outside Git. If a generated artifact becomes useful as a deterministic fixture, move only the minimal sanitized file into a dedicated `testdata/` directory and document the fixture owner.
