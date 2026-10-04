# Contributing to AllCallAll

Thank you for helping improve AllCallAll. Contributions may include bug fixes,
documentation, tests, design discussion, and focused feature work.

## Before You Start

- Search existing issues and pull requests before opening a new one.
- Use the repository issue templates for public bugs, features, and
  documentation problems.
- Report vulnerabilities privately through [SECURITY.md](SECURITY.md).
- Keep changes scoped. Discuss broad behavior or contract changes before
  investing in a large implementation.

## Development Setup

Clone over SSH and install JavaScript dependencies from the repository root:

```bash
git clone git@github.com:XianingY/allcallall.git
cd allcallall
npm ci
cd backend && go mod download
```

The Python Agent/RAG runtime is a separate repository. Clone it as the sibling
directory `../allcallall-agent-runtime` when working on cross-repository flows.

Do not add lockfiles under `web/` or `mobile/`; the root `package-lock.json` is
the only npm lockfile.

## Making Changes

1. Create a focused branch from the current default branch.
2. Add or update tests before changing behavior.
3. Preserve public HTTP routes, JSON/OpenAPI contracts, environment variables,
   commands, deep links, migrations, and published Python imports unless an
   approved change explicitly includes a migration path.
4. Update the canonical documentation whenever behavior or configuration
   changes.
5. Use clear, descriptive commit messages and push branches over SSH.

Never commit `.env`, `.omo`, `.workbuddy`, `output/`, credentials, tokens, or
private customer data.

## Verification

Run the narrowest relevant checks while developing.

```bash
make fmt
make lint
make test
make verify
```

Before requesting merge, run the complete gate:

```bash
make verify-full
```

Important component commands:

| Area | Commands |
| --- | --- |
| Backend | `cd backend && go build ./... && go test ./... && go vet ./...` |
| Web | `cd web && npm run typecheck && npm run lint && npx vitest run` |
| Mobile | `cd mobile && npm run typecheck && npm test && npm run lint` |
| Desktop | `cd desktop && npm run check && npm run build` |
| Documentation | `npm run test:docs && npm run docs:check` |

The Web project uses a solution `tsconfig`; use `npm run typecheck`, not a bare
`tsc --noEmit`. Mobile has separate Node and Jest runners; add each new test to
the appropriate configured scope so it is not silently skipped.

## Pull Requests

- Explain the user-visible outcome and the reason for the change.
- Identify tests run and any checks intentionally not run.
- Describe API, schema, environment, migration, security, and privacy impact.
- Include screenshots or recordings for visible UI changes.
- Keep generated files and their source changes in the same pull request.

All contributions are licensed under the repository's [MIT License](LICENSE).
Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md).
