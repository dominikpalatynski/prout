# Event Family Registry PRD: Declarative Trigger Types and Operation Handlers

## Problem Statement

As the maintainer of Toolshed, I can already see the system accept a GitHub Delivery, normalize supported webhook input, evaluate repository-specific Triggers, create durable Operation Requests, and execute preview-related runtime work.

What I cannot do cleanly is extend this automation model without reopening multiple parts of the codebase at once. Today, supported webhook event routing, Trigger matching, Trigger-to-Operation mapping, and operation execution entry points are spread across separate switches and separate modules. Repository-specific Trigger records also carry technical metadata derived from built-in behavior, which creates two sources of truth and makes the model feel more configurable than it really is. Adding a new capability such as a new label-driven action or a new comment-driven action risks touching parsing, matching, persistence, and execution in several different places.

The current operator API in `internal/server/api.go` exposes `GET /api/trigger-types`, `GET /api/repositories/{repositoryID}/triggers`, `POST /api/repositories/{repositoryID}/triggers`, and `PATCH /api/repositories/{repositoryID}/triggers/{triggerID}`. That lets an Operator manage repository-specific Triggers, and `PATCH /api/repositories/{repositoryID}` can disable an entire Repository, but there is no repository-level API surface for one supported Event Family. I cannot disable pull-request comments for one Repository while leaving the Repository itself and its other Event Families enabled.

I need Toolshed's built-in automation model to become declarative, explicit, and easy to extend in one known place, while preserving the existing domain language around Trigger, Trigger Type, Event Family, Operation Type, Operation Handler, Operation Request, and Runtime Environment.

## Solution

As the maintainer of Toolshed, I will gain one central Event Family registry that acts as the local source of truth for built-in automation behavior.

Each Event Family will define the supported GitHub webhook classification, validate and normalize incoming webhook payloads into the shared event model, and declare the Trigger Types that can match that family. Each Trigger Type will be business-named, will represent a closed built-in preset with no per-Repository matcher config, and will map to exactly one Operation Type. Each Operation Type will be declared once and will expose one explicit Operation Handler as the known execution entry point for worker-side execution.

Repository-specific Event Family records will become the repository-level gate for supported webhook families: one Repository can enable or disable at most one Repository Event Family of a given Event Family. Repository-specific Trigger records will remain simple preset enablement rows under that gate: one Repository can enable or disable at most one Trigger of a given Trigger Type. The worker and webhook pipeline will consult the central registry plus repository-level Event Family enablement rather than separate switches when deciding whether one Repository supports that Webhook Event, how to normalize it, which Trigger Types exist for that Event Family, which Operation Type follows from a match, and which Operation Handler should execute the resulting Operation Request.

This keeps the model declarative without overcomplicating it. The system will still use one shared normalized event shape, but the ownership of routing, matching, and execution declarations will move into one obvious place.

## User Stories

1. As the maintainer of Toolshed, I want one central Event Family registry, so that I can extend built-in automation in one known place.
2. As the maintainer of Toolshed, I want Event Families to own webhook routing and normalization, so that supported GitHub input is not spread across multiple switches.
3. As the maintainer of Toolshed, I want Trigger Types to be business-named presets, so that the intent of each built-in rule is obvious from its name.
4. As the maintainer of Toolshed, I want Trigger Types to be closed presets without per-Repository matcher config, so that the built-in automation model stays explicit and auditable.
5. As the maintainer of Toolshed, I want Trigger Types to hang from Event Family definitions, so that supported triggers are grouped by the webhook family that can produce them.
6. As the maintainer of Toolshed, I want each Trigger Type to point to exactly one Operation Type, so that matching semantics and execution intent stay tied together.
7. As the maintainer of Toolshed, I want each Operation Type to be declared once, so that execution semantics are not duplicated under multiple Trigger Types.
8. As the maintainer of Toolshed, I want each Operation Type to have one explicit Operation Handler, so that the worker has one known execution entry point without switch-based dispatch.
9. As the maintainer of Toolshed, I want Operation Handlers to be obvious named units, so that humans can quickly find where one operation is executed.
10. As the maintainer of Toolshed, I want the worker to resolve Operation Handlers from the registry, so that adding a new Operation Type does not require editing a central switch.
11. As the Operator, I want a Repository-specific Trigger to mean only “this Trigger Type is enabled for this Repository,” so that Repository Configuration stays simple.
12. As the Operator, I want a Repository to have at most one Trigger of a given Trigger Type, so that enablement stays unambiguous.
13. As the maintainer of Toolshed, I want redundant trigger metadata removed from Repository-specific Trigger records, so that the database does not duplicate built-in definitions from code.
14. As the maintainer of Toolshed, I want the API for Repository Triggers to expose business-named Trigger Types, so that operators see the same concepts that the code uses.
15. As the maintainer of Toolshed, I want Repository Trigger creation to stop implying arbitrary matcher configuration, so that users are not misled into thinking built-in presets are ad hoc configurable.
16. As the maintainer of Toolshed, I want existing preview-related behavior to survive the refactor, so that the architecture becomes cleaner without changing preview semantics accidentally.
17. As the maintainer of Toolshed, I want multiple Trigger Types such as label-based and comment-based preview entry points to reuse the same Operation Type, so that idempotency and concurrency stay owned by the execution model.
18. As the maintainer of Toolshed, I want Operation Type semantics such as runtime target, step machine, and replacement policy to stay declared once, so that I do not have to synchronize multiple copies of the same logic.
19. As the maintainer of Toolshed, I want the shared normalized event shape to remain simple, so that the registry refactor does not introduce unnecessary payload-type complexity.
20. As the maintainer of Toolshed, I want unsupported Event Families, Trigger Types, and Operation Types to fail clearly, so that declarative extension remains safe.
21. As the maintainer of Toolshed, I want the registry definitions to read like data plus small named functions, so that the code stays easy to understand and modify.
22. As the maintainer of Toolshed, I want new built-in capabilities such as delete, restart, or deploy flows to be added by extending the registry, so that the system scales without architectural drift.
23. As the maintainer of Toolshed, I want migration of existing Trigger rows to be deterministic, so that legacy preview trigger rows can move to the new business-named Trigger Types safely.
24. As the Operator, I want later Repository Trigger changes to affect future Webhook Events without restart, so that Repository Configuration remains live.
25. As the maintainer of Toolshed, I want the architecture to make the entry point for automation execution obvious to both humans and AI assistants, so that future work stays readable and controlled.
26. As the Operator, I want a Repository Event Family to mean only “this Event Family is enabled for this Repository,” so that I can gate whole webhook families separately from individual Trigger Types.
27. As the Operator, I want the API to list supported Event Families, so that I can manage repository-level Event Family enablement through stable built-in names.
28. As the Operator, I want repository-level Event Families to be enabled or disabled through dedicated endpoints, so that I do not have to mutate Trigger rows just to stop one webhook family for one Repository.
29. As the maintainer of Toolshed, I want the webhook pipeline to consult repository-level Event Family enablement before per-Trigger evaluation, so that family-scoped repository policy is enforced in one obvious place.
30. As the maintainer of Toolshed, I want the current Trigger endpoints documented as a migration baseline, so that the refactor explains exactly what API surface is being replaced or reshaped.
31. As the maintainer of Toolshed, I want Event Family management endpoints and Trigger management endpoints to stay separate, so that the API mirrors the domain model instead of collapsing two decisions into one resource.
32. As the Operator, I want disabling one Repository Event Family to stop matching all Trigger Types in that family without erasing their own enabled state, so that I can pause one webhook family and later resume it without rebuilding trigger configuration.
33. As the maintainer of Toolshed, I want every newly registered Repository to receive all supported Repository Event Families as enabled by default, so that the refactor preserves current live webhook behavior unless an Operator narrows it.
34. As the maintainer of Toolshed, I want existing Repositories to be backfilled with enabled Repository Event Families during migration, so that rollout does not silently disable current automation.
35. As the Operator, I want to configure a Trigger even when its Repository Event Family is currently disabled, so that I can prepare repository-specific presets before reopening that webhook family.

## Implementation Decisions

- Build one central automation registry module whose top-level key is Event Family.
- Keep the automation registry declarative: definitions should read like explicit data with small named helper functions rather than one monolithic control-flow function.
- Each Event Family definition should include:
  - the stable Event Family key
  - the supported GitHub webhook classification it recognizes
  - validation and normalization behavior into the shared normalized event model
  - the Trigger Types available for that Event Family
- Each Trigger Type definition should include:
  - the stable business-named Trigger Type key
  - human-readable metadata for operator-facing APIs
  - its match rule against the shared normalized event model
  - the Operation Type it creates when matched
- Each Operation Type definition should be declared once and should include:
  - the stable Operation Type key
  - the Runtime Environment Type it targets
  - its technical execution metadata such as step-machine resolution and related execution semantics
  - its Operation Handler as the known entry point for execution
- The worker should resolve the Operation Handler from the Operation Type definition instead of dispatching through a switch on operation type.
- Repository-specific Event Family persistence should be introduced as a Repository plus Event Family enablement model with one row per Repository and Event Family.
- Repository-specific Event Family persistence should use stable Event Family keys from the central registry as the source of truth rather than repository-owned copies of GitHub webhook classification logic.
- Disabling one Repository Event Family should not rewrite or disable child Repository Trigger rows; it should only prevent their evaluation while that Event Family is disabled for the Repository.
- Repository registration should seed one enabled Repository Event Family row for every currently supported Event Family.
- Migration/backfill should seed enabled Repository Event Family rows for every existing Repository so the post-migration default matches today's behavior.
- Trigger write endpoints should continue to allow creating or enabling a Repository Trigger even when its parent Repository Event Family is disabled; the family gate should only affect webhook-time evaluation, not configuration-time writes.
- Repository-specific Trigger persistence should be simplified to a Repository plus Trigger Type enablement model with one row per Repository and Trigger Type.
- Repository-specific Trigger persistence should no longer treat Event Family, identity key, or preset matcher config as database-owned source of truth.
- The uniqueness rule for Repository-specific Trigger persistence should become one Trigger per Repository and Trigger Type.
- The current Trigger write path in `api.go` should be treated as migration context: today `POST /api/repositories/{repositoryID}/triggers` validates `type` and `config` through `s.triggerCatalog.ValidateAndNormalize` and persists `type`, `event_family`, `identity_key`, `config_json`, and `enabled`.
- Operator-facing APIs should expose both global supported Event Families and repository-level Event Family enablement, not only Trigger Types.
- Repository Trigger APIs should accept and return business-named Trigger Types instead of generic matcher families that imply arbitrary config.
- Repository Event Family APIs should accept and return stable Event Family keys rather than raw GitHub event headers, action strings, or Trigger row identifiers.
- The list of available Trigger Types returned to operators should be derived from the central registry rather than from hand-maintained switch logic.
- The list of available Event Families returned to operators should be derived from the same central registry rather than from hand-maintained switch logic.
- Existing preview entry points should be migrated from generic technical names toward business-named Trigger Types such as:
  - preview on pull request opened
  - preview on label preview
  - preview on comment preview
- Multiple Trigger Types may continue to point to the same Operation Type, especially for preview-start flows.
- The shared normalized event model should be retained to keep the registry simple, rather than splitting the model into separate per-family payload object hierarchies.
- Webhook handling should flow through a small sequence:
  - resolve Event Family from the incoming GitHub event
  - consult repository-level Event Family enablement for that Repository and Event Family
  - normalize the incoming payload through the Event Family definition
  - load enabled Repository-specific Triggers
  - match the Trigger Type definitions for that Event Family
  - create Operation Requests for the matching Trigger Types
- If the Repository Event Family is disabled, the webhook pipeline should stop before Trigger evaluation for that Repository and should leave existing Repository Trigger rows unchanged.
- Operation Request snapshots should continue freezing the matched Trigger Type, the resolved Operation Type, and the normalized input needed for retry and audit.
- Schema changes should include repository-level Event Family enablement storage, simplification of Repository Trigger storage, and migration of existing rows into the new Trigger Type vocabulary.
- Backward compatibility should be handled through explicit migration or one-time translation rules rather than long-term support for both old and new trigger shapes.
- The registry refactor should not change the already accepted semantics of preview-start, preview cleanup, Runtime Environment reuse, or Operation Outcome behavior.
- Deep modules worth extracting or reshaping in this work include:
  - the central automation registry
  - the Event Family normalization layer
  - the Repository Event Family persistence and API layer
  - the Repository Trigger persistence and API layer
  - the Operation Type definition and handler-dispatch layer
  - the migration/backfill logic for existing Trigger rows

## API Surface

### Current API surface in `api.go`

- `GET /api/trigger-types` returns the current Trigger catalog definitions.
- `GET /api/repositories/{repositoryID}/triggers` lists persisted Repository Trigger rows.
- `POST /api/repositories/{repositoryID}/triggers` upserts one Repository Trigger from `{type, config, enabled?}` after catalog validation.
- `PATCH /api/repositories/{repositoryID}/triggers/{triggerID}` toggles `enabled` on one persisted Repository Trigger row by row ID.
- `PATCH /api/repositories/{repositoryID}` toggles whole-Repository `enabled` state and is the only current repository-wide webhook gate.
- The current Trigger responses expose `type`, `event_family`, `identity_key`, `config`, and `enabled`, which reflects today's duplicated source-of-truth problem rather than the desired declarative target shape.
- `POST /api/repositories` currently registers one Repository and is the natural entry point for seeding default-enabled Repository Event Family rows.

### Proposed API additions and reshaping

- `GET /api/event-families` should list supported Event Families from the central registry, including stable keys, human-readable metadata, and available Trigger Types.
- `GET /api/repositories/{repositoryID}/event-families` should list repository-level Event Family enablement rows.
- `PUT /api/repositories/{repositoryID}/event-families/{eventFamily}` should create or replace one Repository Event Family enablement row using the stable Event Family key.
- `PATCH /api/repositories/{repositoryID}/event-families/{eventFamily}` should toggle `enabled` for one Repository Event Family without replacing future fields.
- A successful `POST /api/repositories` response should reflect that all supported Repository Event Families now exist enabled by default, either directly in the payload or through immediately consistent follow-up reads on the Event Family endpoint.
- Toggling one Repository Event Family to `disabled` should not mutate `enabled` on related Trigger rows; it should only change whether those Trigger rows are considered during webhook handling.
- Repository Trigger endpoints should continue to manage individual Trigger Types, but their request and response shape should move toward closed preset enablement and away from repository-owned `config`, `identity_key`, and `event_family` source-of-truth fields.
- Repository Trigger endpoints should not reject configuration writes only because the parent Repository Event Family is disabled; they may optionally surface that the Trigger is currently inert through derived metadata, but the write itself should succeed.
- Event Family endpoints and Trigger endpoints should remain separate resources because one models repository support for a webhook family and the other models repository enablement of one preset inside that family.

## Testing Decisions

- Good tests should verify externally visible behavior, durable state transitions, and stable registry outcomes rather than helper call order or incidental internal struct layout.
- The central automation registry should have focused tests that assert:
  - supported Event Families are discoverable
  - Trigger Types are attached to the correct Event Family
  - Trigger Types resolve to the correct Operation Type
  - Operation Types resolve to the correct Operation Handler
- Event Family tests should verify webhook routing, payload validation, and normalization into the shared event model.
- Trigger matching tests should verify that business-named Trigger Types match and mismatch correctly for their Event Family.
- Repository Event Family persistence tests should verify uniqueness by Repository and Event Family and should confirm that repository-level Event Family rows are separate from Repository Trigger rows.
- Registration and migration tests should verify that all supported Repository Event Families are present and enabled by default for new and existing Repositories.
- Integration tests should verify that disabling one Repository Event Family prevents Trigger evaluation and Operation Request creation for that family while preserving child Trigger enabled flags.
- API tests should verify that Trigger creation and enablement still succeed while the parent Repository Event Family is disabled and that later webhook handling still ignores those triggers until the family is re-enabled.
- Repository Trigger persistence tests should verify uniqueness by Repository and Trigger Type and should confirm that redundant matcher metadata is no longer the source of truth.
- Worker dispatch tests should verify that Operation Requests resolve handlers through the registry rather than through hardcoded switch behavior.
- Migration tests should verify deterministic conversion of existing preview-related Trigger rows into the new Trigger Type vocabulary.
- API tests should verify operator-facing Event Family listing, Repository Event Family enablement behavior, Trigger Type listing, and Repository Trigger enablement behavior.
- Good integration tests should continue to assert complete webhook-to-Operation Request behavior through persisted database state rather than through mocks of internal control flow.
- Prior art for the desired testing style already exists in the codebase:
  - focused trigger-catalog tests
  - PostgreSQL-backed webhook and Operation Request flow tests
  - step-machine and operation-mapping tests
  - API response-shaping tests centered on external JSON behavior
- The default testing scope for this refactor should cover:
  - registry definitions
  - Repository Event Family persistence and API behavior
  - webhook normalization and matching
  - Repository Trigger persistence and migration
  - Operation Handler dispatch

## Out of Scope

- Introducing user-defined Trigger Types or free-form Trigger matcher configuration.
- Changing the business semantics of preview-start, preview-cleanup-superseded, or existing Runtime Environment lifecycle behavior beyond the dispatch architecture.
- Adding a panel UX for managing Trigger Types beyond whatever the current operator-facing API already supports.
- Publishing this planning artifact to a GitHub issue tracker or changing triage workflows.
- Designing a generic plugin system for third-party automation rules.
- Splitting the shared normalized event model into a complex hierarchy of family-specific event payload types.
- Redefining repository-scoped runtime configuration, deployment metadata, or later runtime phases unrelated to the automation registry shape.

## Further Notes

- This PRD is intentionally local-only and is meant to drive implementation planning inside the repository rather than external issue-tracker workflow.
- The recommended architecture optimizes for declarative extension and code visibility: one place to declare built-in automation, one known entry point per Operation Type, and clear separation between Repository enablement and built-in behavior.
- The design deliberately keeps Trigger Type names business-oriented, because those names will appear in APIs, database rows, audit trails, and future implementation discussions.
- The refactor should reduce conceptual load for both humans and AI tools by making the control flow read as registry lookup rather than chained switch statements.
