---
paths:
  - "**/*.{go,py,ts,tsx,js,jsx,rb,rs,java,kt,lua}"
---

# Encapsulation & factory rules

When a data structure carries invariants (validated fields, preconditions):

- Construct it only through a factory function (`NewX` / `from_*`) that checks
  every precondition and fails early — never as a raw literal at call sites.
- Make invariants unbypassable: keep fields private, and hide the type itself
  where the language allows (e.g. a Go unexported type returned by an exported
  factory), so the factory is the only construction path — including
  zero-value/default construction.
- Code receiving a factory-built value must not re-validate what construction
  already guarantees.
- Attach behavior as methods on the structure instead of free functions that
  take it as an argument, so related logic stays cohesive.
- Keep factories and planning logic pure: inject side effects (filesystem,
  clock, network) as function/interface parameters so tests stay table-driven.
- If a linter flags the intentional shape (e.g. revive `unexported-return` in
  Go), suppress it locally with a reasoned inline directive instead of
  disabling the rule globally.
