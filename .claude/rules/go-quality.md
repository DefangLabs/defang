---
paths:
  - "src/**/*.go"
---

# Go Quality Rules

- Make the smallest coherent change that fixes the root problem.
- Put behavior where the owning package already has the concept.
- Avoid exported API for one-off behavior.
- Prefer narrow options over duplicating path discovery or state reconstruction in callers.
- Return real errors with context; ignore missing optional files only when missing is the contract.
- Keep CLI commands thin and provider-generic code generic.
- Comments explain why, not what.
