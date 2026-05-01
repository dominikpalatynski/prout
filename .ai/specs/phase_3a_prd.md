# Phase 3A PRD: Runtime Environment Lifecycle Foundation

## Problem Statement

As the Operator, I can currently see a GitHub Delivery enter Toolshed, become a Webhook Event, match Triggers, and hand off generic asynchronous work. What I still cannot see is the first durable runtime-domain slice of preview work. There is no stable Pull Request anchor, no durable queued Operation Request, no per-attempt Preview Environment record, and no shared language for whether preview work created a new attempt, reused an existing one, or was replaced by a newer target.

Without that slice, later work such as tarball retrieval, workspace materialization, Docker runtime deployment, cleanup, and GitHub feedback would be built on top of an ambiguous model. Phase 3A needs to introduce the durable runtime-domain model first, while keeping execution scope narrow enough that the phase ends with a clear, testable boundary.

## Solution

As the Operator, I will gain a first-class runtime-domain model for preview work.

When a matched Trigger requests preview work, Toolshed will create a durable Operation Request with an immutable snapshot of the target Pull Request Head Commit and the intended Operation Type. A worker will handle that Operation Request and either create or reuse the correct Preview Environment attempt for that target. The resulting Operation Outcome will be persisted durably, and the Preview Environment will end Phase 3A in `preparing`.

Phase 3A intentionally stops there. It does not yet download code or produce a materialized workspace on disk. Instead, it establishes the Pull Request anchor, Operation Request model, Preview Environment attempt model, runtime status model, and audit/read-path behavior that later phases can extend safely.

## User Stories

1. As the Operator, I want Toolshed to create a stable Pull Request record for a registered Repository, so that runtime work has a durable domain anchor beyond raw webhook payloads.
2. As the Operator, I want the Pull Request record to retain the current Pull Request Head Commit, so that runtime work can target an exact commit deterministically.
3. As the Operator, I want matched Triggers to create durable Operation Requests, so that queued runtime work is visible in the product model rather than hidden only inside River.
4. As the Operator, I want each matched Trigger to create its own Operation Request, so that audit history preserves which Trigger asked for runtime work.
5. As the Operator, I want Operation Requests to carry a global Operation Type, so that different Triggers can request the same business operation without redefining its meaning.
6. As the Operator, I want `preview-start` to be the first executable Operation Type in this phase, so that the runtime-domain slice stays narrow and testable.
7. As the Operator, I want Operation Requests to freeze the target Pull Request Head Commit at creation time, so that delayed worker execution does not silently drift onto a newer commit.
8. As the Operator, I want Operation Requests to keep an immutable snapshot of operation intent, so that retries and audit inspection do not depend on later changes to Triggers, Pull Requests, or Runtime Environments.
9. As the Operator, I want Operation Requests to record whether they came from a matched Trigger or from a system action, so that manual and automatic behavior remain distinguishable.
10. As the Operator, I want the worker to own creation or reuse of Preview Environment attempts, so that the runtime lifecycle begins only when asynchronous runtime preparation actually starts.
11. As the Operator, I want a Preview Environment to represent one concrete attempt, so that multiple attempts over time for one Pull Request remain historically distinct.
12. As the Operator, I want the same Preview Environment attempt to survive technical worker retries, so that transient failures do not create duplicate attempts.
13. As the Operator, I want a later domain-level retry after a failed attempt to create a new Preview Environment, so that historical failures stay closed and inspectable.
14. As the Operator, I want Preview Environments to be typed independently from Operation Types, so that multiple operation kinds can act on the same runtime kind in the future.
15. As the Operator, I want `preview` to be the first Runtime Environment Type, so that future runtime kinds can be introduced without renaming the preview model later.
16. As the Operator, I want `preview-start` to behave as an ensure-style operation, so that repeated requests for the same target reuse existing preview work instead of creating competing attempts.
17. As the Operator, I want `preview-start` to report whether preview work was newly created or already in progress, so that technical success is distinguishable from domain reuse.
18. As the Operator, I want Preview Environments to carry their own immutable target Pull Request Head Commit, so that each attempt remains self-describing even after the Pull Request advances.
19. As the Operator, I want Preview Environment statuses of `preparing`, `prepared`, `failed`, and `superseded`, so that runtime attempts can be read in domain terms instead of generic worker statuses.
20. As the Operator, I want `prepared` to mean that runtime preparation succeeded and the workspace still exists for the next lifecycle step, so that the status has concrete operational meaning.
21. As the Operator, I want `superseded` to be distinct from `failed`, so that an older preview attempt replaced by a newer target is not misreported as broken.
22. As the Operator, I want multiple Operation Requests for the same target to be able to reference the same Preview Environment, so that audit history and attempt deduplication can coexist.
23. As the Operator, I want automatic cleanup of superseded preview attempts to be modeled as its own future Operation Type, so that system cleanup has its own audit trail instead of being a hidden side effect.
24. As the Operator, I want manual preview deletion to be modeled as a different future Operation Type from automatic superseded cleanup, so that user intent and system cleanup remain distinguishable.
25. As the Operator, I want cleanup-style Operation Requests to target a concrete Runtime Environment attempt, so that deletion never depends on ambiguous “current preview” resolution inside a worker.
26. As the Operator, I want the webhook request-path transaction to persist Webhook Event interpretation, Pull Request anchor updates, Operation Request creation, and River enqueue together, so that accepted work is never half-written.
27. As the Operator, I want the Webhook Event detail view to include Operation Requests and linked runtime-attempt context, so that I can inspect one delivery end to end without needing the panel.
28. As the Operator, I want Phase 3A to stop with Preview Environments in `preparing`, so that runtime-domain modeling is completed before real tarball and workspace behavior starts.
29. As the future maintainer of Toolshed, I want the runtime-domain vocabulary to be stable before tarball retrieval and Docker runtime logic are added, so that later phases deepen the pipeline instead of renaming it.
30. As the future maintainer of Toolshed, I want Operation Types such as `preview-restart`, `preview-delete`, and `preview-cleanup-superseded` to be modeled now but not necessarily executable yet, so that later slices can plug into a consistent runtime-domain model.

## Implementation Decisions

- Phase 3 is split into two slices. Phase 3A establishes the runtime-domain lifecycle foundation; Phase 3B extends the same `preview-start` operation into real tarball retrieval and workspace materialization.
- The old durable queued handoff concept is renamed from Trigger Dispatch to Operation Request.
- The webhook request path remains responsible for Webhook Event persistence, Trigger evaluation, Pull Request anchor updates, Operation Request creation, and River enqueue in one transaction.
- A matched Trigger creates exactly one Operation Request. Multiple matched Triggers for the same target are not collapsed in the request path.
- Operation Type is a global built-in system concept and is distinct from Trigger Type.
- Runtime Environment Type is also a global built-in system concept and is distinct from Operation Type.
- The first Runtime Environment Type is `preview`.
- The first executable Operation Type in Phase 3A is `preview-start`.
- Future operation types such as `preview-restart`, `preview-delete`, and `preview-cleanup-superseded` are part of the model but are not required to be executable in Phase 3A.
- `preview-start` is an ensure-style operation. It creates a new Preview Environment only when no current attempt exists for the same target; otherwise it reuses the existing attempt and reports that outcome.
- Worker execution, not the webhook request path, begins the lifecycle of a Preview Environment.
- A Preview Environment is per attempt, not per Pull Request.
- Preview Environment attempts are keyed conceptually by Repository, Pull Request, Runtime Environment Type, and immutable target Pull Request Head Commit.
- Operation Requests freeze their target Pull Request Head Commit at creation time.
- Preview Environments copy and retain their own immutable target Pull Request Head Commit.
- Operation Requests also store an immutable operation-intent snapshot so retries and historical inspection do not depend on live joins alone.
- Operation Requests can be sourced either by matched Triggers or by system-initiated actions.
- Operation Request lifecycle is intentionally technical: `queued`, `handled`, `failed`.
- Preview Environment lifecycle is intentionally domain-oriented: `preparing`, `prepared`, `failed`, `superseded`.
- Phase 3A ends with Preview Environments in `preparing`; it does not simulate `prepared`.
- Operation Outcome is persisted on the handled Operation Request, with an optional reference to the Runtime Environment it created or reused.
- The first Operation Outcomes should distinguish at least “new attempt created”, “already preparing”, “already prepared”, and “operation failed”.
- Pull Requests become a first-class table in this phase, anchored by Toolshed’s internal identifier while also retaining GitHub’s pull-request identifier.
- Pull Request identity should use a stable per-repository anchor based on Repository plus pull-request number, with a separate immutable GitHub identifier retained for external reference.
- The first Pull Request slice must at minimum track the current Pull Request Head Commit; richer Pull Request state is not required to unlock Phase 3A acceptance.
- Preview Environment replacement by a newer target is modeled as `superseded`, not `failed`.
- Automatic cleanup for superseded preview attempts is modeled as its own future operation flow, and its Operation Request should be created in the same transaction that marks an older attempt `superseded`.
- Cleanup-style Operation Requests target a concrete Runtime Environment attempt and freeze that target when created.
- The Webhook Event detail read model should be extended to hydrate Operation Requests and linked runtime-attempt context, rather than introducing a separate runtime index endpoint in Phase 3A.
- Deep modules worth extracting include an Operation Type mapper, an Operation Request snapshot builder, a Pull Request anchor service, a Preview Environment ensure/resolution module, a runtime state transition helper, and a read-model hydrator for event detail.

## Testing Decisions

- Good tests should assert external behavior and durable outcomes, not helper call order or internal sequencing.
- Request-path tests should verify that supported webhook handling persists Webhook Events, Pull Request anchor updates, Trigger evaluations, Operation Requests, and River enqueue as one durable unit.
- Worker tests should verify create-versus-reuse behavior for `preview-start`, including `already_preparing` and other Operation Outcomes, rather than River internals.
- Runtime-attempt tests should verify Preview Environment state transitions and immutable target-commit behavior from the perspective of persisted records.
- Operation Request tests should verify immutable snapshot behavior, source tracking, and outcome persistence.
- Read-model tests should verify that one Webhook Event detail view can hydrate its Operation Requests and linked runtime-attempt context correctly.
- Modules that deserve focused tests in this phase are the Operation Type mapper, Operation Request snapshot builder, Pull Request upsert behavior, Preview Environment ensure logic, runtime state transition helper, and webhook-detail hydration.
- Pure mapping and normalization logic is a strong candidate for narrow unit tests because its boundaries are stable and deterministic.
- Database-backed behavior such as idempotency, reuse, immutable snapshots, and transaction boundaries is a strong candidate for integration tests against real PostgreSQL.
- Prior art already exists for database-backed testing through the repository’s test database helper that starts PostgreSQL for tests.
- Prior art also exists for focused unit-style tests around GitHub webhook parsing, Trigger catalog behavior, and server route contracts.
- Existing webhook and trigger tests are useful precedents for keeping parsing/matching logic isolated, while the database helper provides the right precedent for new transactional tests in this phase.

## Out of Scope

- Tarball download, archive extraction, and workspace materialization on disk.
- Any transition from `preparing` to `prepared`.
- Executable support for operation types other than `preview-start`.
- Runtime deployment, Docker Compose parsing, environment variable injection, health waiting, teardown runtime behavior, public routing, TLS, and GitHub feedback.
- Panel work, OAuth, multi-user access control, and separate runtime list screens.
- Full cleanup execution flow, even though cleanup-related operation types are already part of the domain model.
- Rich Pull Request state beyond what is needed to anchor current preview-target selection.
- Any implementation that depends on River internals instead of the durable Operation Request model.

## Further Notes

- Phase 3A is intentionally a domain-modeling slice, not a code-retrieval slice.
- Phase 3B should extend the same `preview-start` flow rather than inventing a second operation just for tarball retrieval.
- The model is deliberately richer than the minimum executable scope so that later phases can add restart, delete, cleanup, and newer-head replacement behavior without renaming core concepts again.
