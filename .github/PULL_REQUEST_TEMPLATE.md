## Summary

<!-- 1-3 lines: what this PR changes and why -->

## Type of change

<!-- Check all that apply -->

- [ ] New feature
- [ ] Bug fix
- [ ] Refactor (no behavior change)
- [ ] Design system / styling
- [ ] Documentation
- [ ] Tests

## Design system check (required for any UI change)

- [ ] `cd frontend && bash scripts/audit-brutal.sh` shows `✅ audit clean`
- [ ] `cd frontend && npx tsc --noEmit` produces no new errors
- [ ] Any new component lives in `components/ui/` with type exports
- [ ] No new `rounded-{md,lg,sm,xl,2xl,3xl}` utilities
- [ ] No new default Tailwind color scales (e.g., `bg-green-500`)
- [ ] No new `backdrop-blur`, `shadow-{md,lg,2xl}` "soft" effects
- [ ] Buttons: using existing `Button` variants; new variants justified in the description

## E2E tests (if any page under `app/` was changed)

- [ ] `cd frontend && npx playwright test e2e/brutal-consistency.spec.ts` passes
- [ ] Affected pages have a screenshot in `.audit-shots/`

## Related

<!-- Link related issues / PRs. Use GitHub's auto-close syntax: "Closes #123" -->

Closes #

## Screenshots (if applicable)

<!-- Drag screenshots into the conversation. UI changes must include before/after. -->
