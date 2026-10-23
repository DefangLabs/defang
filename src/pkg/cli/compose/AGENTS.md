# Compose Package Guidance

This file applies to `src/pkg/cli/compose/`.

## Ownership

- This package owns Docker Compose loading, validation, normalization, fixup, project-directory behavior, and compose environment handling.
- Callers may pass narrow intent, such as a stack name or config paths, but should not duplicate compose project discovery or working-directory rules.
- Keep compose-go behavior localized here so callers do not need to know compose-go ordering, dotenv, or project-option details.

## Implementation Rules

- Prefer package-owned options over exported generic hooks for one-off behavior.
- Ignore missing optional files only when that is the documented contract.
- Return wrapped errors for malformed files, directories where files are expected, parse failures, and filesystem errors other than expected not-found cases.
- Do not add logging for normal internal compose loading unless it is user-actionable.
- Keep fixup/normalization deterministic and covered by tests.

## Tests

- Add behavior tests in adjacent `_test.go` files or package `testdata/` only when fixtures make the behavior clearer.
- Prefer small inline compose files for focused cases.
- Name fixtures after the behavior being tested.
- Cover path-resolution and precedence behavior at this package boundary.
