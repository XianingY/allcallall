# PR Description Template

Use this as a lightweight template for documentation, backend, mobile, or infrastructure changes.

```markdown
## Summary

- What changed?
- Why is it needed?
- Which user, developer, or interview/demo flow does it support?

## Scope

- Backend:
- Mobile/Web:
- Infra/Scripts:
- Docs:

## Verification

- [ ] Backend tests or targeted package tests:
- [ ] Mobile/Web checks:
- [ ] Docs/link checks:
- [ ] Manual smoke:

## Risk Notes

- Runtime/config impact:
- Data/model impact:
- Rollback notes:

## Follow-ups

- Follow-up work that is intentionally out of scope:
```

For doc-only changes, keep the verification section small and include `git diff --check` plus any link/reference checks that were run.
