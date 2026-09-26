# CLAUDE.md — compatibility pointer

This file exists only because some tooling looks for `CLAUDE.md` by name.

**`AGENTS.md` in this directory is the single source of truth.** Read it first.

It previously duplicated most of `AGENTS.md` and contradicted it in several
places — most importantly it listed build commands that skipped the web
typecheck, omitted the mobile `test:jest` suite, and described `make lint`
without the pagination gate. The content that was genuinely unique to this
file (architecture boundaries and security defaults) has been moved into
`AGENTS.md`; the conflicting duplicates were dropped rather than merged, so
the two files can no longer drift apart.

If you find a discrepancy between this file and `AGENTS.md`, `AGENTS.md` wins
and this pointer should be left alone.
