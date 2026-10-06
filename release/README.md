# Release process

`release/manifest.yaml` is the single source of truth for a tagged release:
product version, the pinned SHA of the `allcallall-agent-runtime` checkout,
the contract hash, the DB schema version, client versions, and image
digests. CI reads the manifest (see `platform-ci.yml`), so a release is
reproducible from this file alone.

## Cutting a release

1. Land all changes on `main` and keep `ci.yml` + `platform-ci.yml` green.
2. Bump `product.version` in `release/manifest.yaml` and update the version
   references listed in `scripts/release-check.sh` (Helm `values.yaml`,
   `Chart.yaml` `appVersion`, `docker-compose.production.yml`,
   `backend/README.md`, `docs/architecture/python-runtime-integration.md`).
   Re-run `make release-check`.
3. If the runtime repo moved, update
   `repositories.allcallall-agent-runtime.sha` and `tag`, regenerate the
   contract hash (`make contracts` in the runtime repo, then
   `cd contracts/schemas && { for f in $(ls *.json | sort); do cat "$f";
   printf '\n'; done; } | sha256sum`), and re-run `make release-check`.
4. Stamp and tag:

   ```sh
   make release-stamp   # writes repositories.allcallall.sha = HEAD
   git add release/manifest.yaml && git commit -m "release: stamp manifest for <version>"
   git tag <version> && git push origin <version>
   ```

   Pushing the tag runs `release.yml` (main repo) and, after the runtime repo
   is tagged at the pinned SHA, `publish.yml` (runtime repo). Both workflows
   build, Trivy-scan, SBOM, and cosign-sign every image.
5. Fill in image digests after the publish completes:

   ```sh
   gh api "repos/XianingY/allcallall/packages/container/<image>/versions" \
     --jq '.[] | select(.metadata.container.tags[] == "<version>") | .name'
   ```

   Write each digest into `release/manifest.yaml`, commit, and push. Helm and
   compose keep using the tag; the manifest is the audit trail that pins the
   exact bytes.

## Nightly main-sync check

`.github/workflows/nightly-main-sync.yml` runs daily and verifies that this
repo's `main` still works against the runtime repo's `main`. It is
non-blocking: a failure is a signal to re-pin the manifest (step 3 above),
not a PR gate.

## Local prerequisites

- `make verify-full` requires the sibling `../allcallall-agent-runtime`
  checkout with its dev dependencies installed (`make install-dev` there).
  The Makefile pre-checks this and fails with that instruction instead of a
  wall of pytest collection errors.
- `make helm-check` needs `kubeconform` on PATH; without it it falls back to
  the CI image `ghcr.io/yannh/kubeconform:v0.8.0` via Docker.
