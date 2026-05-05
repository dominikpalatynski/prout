---
status: accepted
---

# Event-Family Automation Registry

prout will model built-in automation through one central Event Family registry. Each Event Family owns webhook routing, validation, and normalization into the shared event model, declares the business-named Trigger Types available for that event, and each Trigger Type points to exactly one Operation Type. Repository-specific Triggers only enable one Trigger Type per Repository and do not carry matcher config, while each Operation Type is declared once with one explicit Operation Handler as the known execution entry point. This keeps extension declarative, removes duplicated trigger metadata from the Repository layer, and replaces scattered switch-based dispatch with one obvious source of truth.

## Considered Options

- Keep generic configurable trigger types with per-trigger matcher config stored in the database.
- Create separate top-level registries for Event Families, Trigger Types, and Operation Types.
- Keep switch-based dispatch in webhook parsing, trigger evaluation, and operation execution.

## Consequences

- Repository trigger persistence can be simplified to repository-specific enablement of Trigger Types.
- New behavior is introduced by adding a new business-named Trigger Type and, when needed, a new Operation Type with its Operation Handler.
- Webhook parsing, trigger matching, and operation dispatch become easier to audit because their declarations live in one central place.
