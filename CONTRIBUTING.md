# Contributing to AllCallAll

We love your input! We want to make contributing to this project as easy and transparent as possible, whether it's:

- Reporting a bug
- Discussing the current state of the code
- Submitting a fix
- Proposing new features
- Becoming a maintainer

## We Develop with Github
We use github to host code, to track issues and feature requests, as well as accept pull requests.

## We Use [Github Flow](https://guides.github.com/introduction/flow/index.html)
All code changes happen through pull requests. Pull requests are the best way to propose changes to the codebase.

## Any contributions you make will be under the MIT Software License
In short, when you submit code changes, your submissions are understood to be under the same MIT License that covers the project. Feel free to contact the maintainers if that's a concern.

## Code Style & Quality

Every change must be clean under the language-native gates before it is merged. Run the narrowest relevant check for a small change, then broaden when shared behavior or generated surfaces are touched.

| Area | Format | Static check | Tests |
| --- | --- | --- | --- |
| `backend/` (Go) | `gofmt -w .` | `go vet ./...` | `go test ./...` |
| `web/` | ESLint autofix via `npm run lint -- --fix` | `npm run lint`, `npm run typecheck` | `npm test` |
| `mobile/` | — | `npx tsc --noEmit`, `npm run lint` | `npm test` |
| `desktop/` | — | `npm run check` | `npm run build` |

Repo-root convenience wrappers:

```bash
make fmt    # gofmt -w on backend/
make lint   # go vet (backend) + npm run lint (web)
```

Additional conventions:

- **Database migrations** are dual-tracked. Add the SQL pair under `backend/migrations/` *and* register the model in `models.AllModels()`. Then bump `currentSchemaVersion` in `backend/internal/runtime/migrations.go` and the matching assertion in `migrations_test.go`.
- **Event handlers**: each outbox event has exactly one handler, registered centrally in `internal/runtime`.
- **Tests** live beside the code as `internal/<pkg>/*_test.go`. Pure-function tests are conventionally split into `*_pure_test.go`.
- Commit only the files you intend to change; do not commit local session artifacts (`.omo/`, `.workbuddy/`, `output/`, `session.json`).

## Report bugs using Github's [issues](https://github.com/XianingY/AllCallAll/issues)
We use GitHub issues to track public bugs. Report a bug by opening a new issue; it's that easy!

## Write bug reports with detail, background, and sample code
**Great Bug Reports** tend to have:
- A quick summary and/or background
- Steps to reproduce
- What you expected would happen
- What actually happens
- Notes (possibly including why you think this might be happening, or stuff you tried that didn't work)
