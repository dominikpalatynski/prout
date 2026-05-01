# Phase 3B PRD: Tarball Retrieval, Workspace Materialization, and Reusable Operation State Machine

## Problem Statement

As the Operator, I can already see Toolshed accept a GitHub Delivery, evaluate Triggers, create durable Operation Requests, and create or reuse a Preview Environment attempt. What I still cannot see is the first real source-materialization slice of preview work.

Today, `preview-start` stops at a Preview Environment in `preparing`. There is no real repository retrieval, no isolated materialized workspace tied to one Runtime Environment attempt, no fork-aware source-repository model for exact head-commit retrieval, no Operator API route centered on Runtime Environments, and no reusable technical state-machine foundation for extending one high-level Operation Type into later preparation and deployment steps.

Without this slice, later work such as Docker Compose deployment, health checks, Kubernetes-style runtime backends, and cleanup would sit on top of an incomplete preparation model. Phase 3B needs to extend the existing `preview-start` flow into real tarball retrieval and workspace materialization while keeping container startup out of scope.

## Solution

As the Operator, I will gain a real source-materialization phase inside `preview-start`.

When a matched Trigger requests preview work, the existing Operation Request flow will still create or reuse the correct Preview Environment attempt. For a new target, the worker will resolve the exact Pull Request Source Repository and Pull Request Head Commit, download the GitHub tarball through the GitHub App installation, stream-extract it into a staging workspace, and promote the staging result into a durable workspace owned by exactly one Runtime Environment attempt.

After successful promotion into the final workspace locator, the Runtime Environment will move from `preparing` to `prepared`. In this phase, `prepared` means the attempt has a backend-agnostic ready deployment input; it does not mean that containers are running.

This phase also introduces the first reusable technical state-machine foundation for Operation Types. `preview-start` remains one business operation, while its finer-grained execution progress is tracked separately on the Operation Request as technical step state that future phases can extend with source, deployment, health-check, and other steps.

This phase also extends the Operator API with a runtime-centric read model. The Operator will be able to list Runtime Environments, filter them by Repository and pull request number, see their statuses, and inspect the linked Pull Request context without starting from a Webhook Event audit record.

## User Stories

1. As the Operator, I want `preview-start` to retrieve real repository contents for one exact Pull Request Head Commit, so that preview work targets a deterministic source snapshot.
2. As the Operator, I want repository retrieval to use the GitHub tarball API rather than `git clone`, so that the implementation stays lightweight and fork-compatible in MVP.
3. As the Operator, I want tarball retrieval to work for Pull Requests from forks, so that external contributions can still be previewed when explicitly authorized.
4. As the Operator, I want the base Repository and the Pull Request Source Repository to be modeled separately, so that preview work can distinguish the managed repository from the repository that actually contains the head commit.
5. As the Operator, I want the Pull Request anchor to retain the current Pull Request Source Repository, so that later Operation Requests do not need to rediscover it from scratch.
6. As the Operator, I want each Operation Request to freeze the target Pull Request Source Repository together with the target Pull Request Head Commit, so that delayed execution does not drift onto a later PR state.
7. As the Operator, I want each Preview Environment attempt to retain its own immutable Pull Request Source Repository details, so that historical attempts remain self-describing after the Pull Request changes.
8. As the Operator, I want source retrieval to trust signed `pull_request.*` webhook payload data when it is complete, so that the request path does not perform unnecessary live GitHub lookups.
9. As the Operator, I want live GitHub PR resolution only for event types that do not carry the full target, such as comment-triggered requests, so that GitHub API usage stays proportional.
10. As the Operator, I want the materialized workspace to belong to one Runtime Environment attempt rather than one River job, so that technical retries reuse the same attempt-local identity.
11. As the Operator, I want the workspace identity to be stored as a backend-agnostic locator instead of a raw host path, so that future runtime backends are not blocked by filesystem-specific persistence.
12. As the Operator, I want filesystem storage configuration to live in Server Configuration, so that workspace resolution is installation-specific rather than repository-specific.
13. As the Operator, I want filesystem storage to start under a dedicated storage backend block, so that later storage backends can be introduced without renaming the contract again.
14. As the Operator, I want the workspace locator to be derived from the Runtime Environment attempt identifier, so that a later retry after `failed` produces a distinct workspace identity even for the same commit.
15. As the Operator, I want the final workspace root to be the repository root rather than GitHub’s tarball wrapper directory, so that later phases can treat the workspace as a normal repository checkout root.
16. As the Operator, I want tarball extraction to stream directly into staging rather than storing the raw tarball on disk, so that cleanup is simpler and less disk is wasted.
17. As the Operator, I want the staging directory to live under the same storage root as the final workspace, so that final promotion can use an atomic rename-like move.
18. As the Operator, I want successful materialization to promote the staging result into the final workspace locator atomically, so that partial extracts do not masquerade as prepared workspaces.
19. As the Operator, I want `prepared` to be set immediately after successful staging promotion, so that Phase 3B has a clear and testable boundary.
20. As the Operator, I want `prepared` to mean “ready deployment input exists” rather than “runtime is already running”, so that the status remains backend-agnostic across Docker Compose and future runtimes.
21. As the Operator, I want Phase 3B to stop before container startup, so that repository materialization stays separate from runtime execution concerns.
22. As the Operator, I want Compose-file validation and other backend-specific deployment input validation to remain outside Phase 3B, so that source preparation does not absorb Phase 4 responsibilities.
23. As the Operator, I want local technical cleanup for staging and partial materialization to be part of this phase, so that failed or superseded attempts do not accumulate obvious garbage on disk.
24. As the Operator, I want the first executable automatic cleanup flow for superseded Preview Environments in this phase, so that real materialized workspaces do not remain forever after a newer commit replaces them.
25. As the Operator, I want manual `preview-delete` to remain a separate later concern, so that automatic superseded cleanup does not become entangled with user-initiated deletion semantics.
26. As the Operator, I want a newer target commit to supersede an older Preview Environment attempt in the same worker transaction that creates the new attempt, so that there is no ambiguity about which attempt is current.
27. As the Operator, I want a superseded attempt’s cleanup Operation Request to be created in the same transaction as the superseded state change, so that cleanup intent is never lost.
28. As the Operator, I do not want Toolshed to rely on active cancellation of already-running workers, so that the execution model stays simple and robust.
29. As the Operator, I want an in-flight superseded worker to re-check its attempt state at safe boundaries and refuse to promote itself to `prepared`, so that older work cannot overwrite newer intent.
30. As the Operator, I want technical retries of the same Operation Request to stay within the same Runtime Environment attempt, so that transient tarball or extraction failures do not create duplicate attempts.
31. As the Operator, I want a Runtime Environment attempt to become `failed` only after a permanent error or exhausted retries, so that transient errors do not prematurely close the attempt.
32. As the Operator, I want a later domain-level retry after `failed` to create a new Runtime Environment attempt and a new workspace locator, so that historical failures stay closed and inspectable.
33. As the Operator, I want Toolshed to detect when a supposedly `prepared` attempt no longer has a valid workspace, so that stale database state cannot be mistaken for a real deployment input.
34. As the Operator, I want `preview-start` to create a fresh attempt when it finds an invalid or missing workspace behind a previously prepared target, so that repair is automatic at the operation boundary.
35. As the Operator, I want `preview-start` to remain one business-level Operation Type, so that repeated preview requests express one stable user intent even as the internal pipeline grows.
36. As the future maintainer of Toolshed, I want fine-grained source and deployment progress to be tracked as technical step state rather than exploding the Runtime Environment Status enum, so that the domain model stays readable.
37. As the future maintainer of Toolshed, I want technical step state to live on the Operation Request, so that step progress belongs to one execution of one operation rather than to the long-lived Runtime Environment identity.
38. As the future maintainer of Toolshed, I want a reusable operation-scoped state-machine interface, so that future Operation Types can share the same execution pattern without duplicating transition logic.
39. As the future maintainer of Toolshed, I want `preview-start` to provide the first machine definition for that interface, so that later work extends a proven abstraction rather than inventing a second one.
40. As the future maintainer of Toolshed, I want the operator-visible Runtime Environment Status model to stay coarse while the step machine carries details like source retrieval and deployment progress, so that UI and audit language remain stable.
41. As the future maintainer of Toolshed, I want the state machine to be scoped by Operation Type, so that different operations can evolve different technical pipelines without polluting one shared enum.
42. As the future maintainer of Toolshed, I want state-machine transitions to be validated centrally, so that later phases do not introduce invalid or contradictory step updates.
43. As the future maintainer of Toolshed, I want deep modules around storage resolution, workspace materialization, and operation step progression, so that later runtime backends can reuse them without invasive rewrites.
44. As the future maintainer of Toolshed, I want the resulting Phase 3B slice to leave Docker Compose startup, health checks, and other future steps as extensions of the same model rather than as separate ad hoc pipelines.
45. As the Operator, I want a runtime-centric Operator API endpoint for Runtime Environments, so that I can inspect preview lifecycle state without starting from Webhook Event audit views.
46. As the Operator, I want each Runtime Environment in that API to include its linked Pull Request summary, so that I can immediately see which pull request a status belongs to.
47. As the Operator, I want that API to support filtering by `repository_id`, so that I can narrow the view to one managed Repository.
48. As the Operator, I want that API to support filtering by `pr_number`, so that I can narrow the view to one pull request using the number I already know from GitHub.
49. As the Operator, I want pull-request filtering to use `repository_id` plus `pr_number` rather than internal pull-request identifiers, so that the API uses natural Operator-facing lookup keys instead of internal database ids.
50. As the Operator, I want the runtime list to return every historical Runtime Environment attempt newest first, so that I can see `prepared`, `failed`, `superseded`, and repaired attempts rather than only one collapsed current row.

## Implementation Decisions

- Phase 3B extends the existing `preview-start` flow; it does not introduce a separate business operation just for repository materialization.
- Phase 3B covers repository retrieval and workspace materialization only. It explicitly does not start containers or perform runtime deployment.
- Canonical source retrieval uses the GitHub tarball REST API authenticated with the GitHub App installation token.
- Tarball retrieval must support Pull Requests from forks, so the target model must distinguish the registered Repository from the Pull Request Source Repository.
- Pull Request Source Repository is persisted as current PR anchor state, frozen into the Operation Request intent snapshot, and copied into each Preview Environment attempt so that historical attempts remain self-describing.
- Signed `pull_request.*` webhook payloads are accepted as the primary source of target data when complete. Live GitHub resolution is reserved for events such as issue comments that do not include the full target.
- Workspace persistence is modeled as fields on Runtime Environments rather than as a separate Workspace entity because the workspace is a technical artifact owned 1:1 by one Runtime Environment attempt.
- The persistent workspace identity is a backend-agnostic workspace locator rather than a raw host path.
- The initial storage contract lives in Server Configuration under a storage backend block. The first backend is filesystem-based and resolves workspace locators under one configured workspace root.
- The workspace locator is derived from the Runtime Environment attempt identity, not from the Operation Request identifier, River job identifier, or commit SHA alone.
- GitHub’s tarball wrapper directory is removed during extraction so the final workspace points at the repository root.
- Raw tarballs are not stored on disk. Materialization streams directly from the HTTP response into a staging directory.
- Staging directories live under the same storage backend/root as the final workspace so successful promotion can use an atomic move within one backend boundary.
- `prepared` is reached immediately after successful promotion from staging into the final workspace locator.
- In this phase, `prepared` means the attempt has a ready deployment input. It does not imply that any runtime is running.
- Phase 3B does not validate Docker Compose paths or other backend-specific deployment inputs before setting `prepared`; those concerns remain in the next deployment-oriented slice.
- Cleanup in Phase 3B includes both local technical cleanup of staging/partial materialization state and the first executable automatic cleanup flow for superseded attempts.
- Manual `preview-delete`, TTL cleanup, and broader teardown orchestration remain later concerns.
- When a newer Pull Request Head Commit creates a new Preview Environment attempt, the older attempt becomes `superseded` in the same transaction and a cleanup-style Operation Request for that older attempt is created there as well.
- Toolshed does not implement active cancellation for in-flight superseded workers. Instead, workers must re-check attempt state at safe boundaries and refuse to promote a superseded attempt to `prepared`.
- Technical retries of one Operation Request stay within the same Runtime Environment attempt. A Runtime Environment becomes `failed` only after a permanent error or exhausted retries.
- A later domain-level retry after `failed` creates a new Runtime Environment attempt rather than reopening the failed one.
- A `prepared` attempt is only trustworthy while its workspace remains valid. If the workspace is missing or invalid, later `preview-start` handling should treat that prepared state as inconsistent and create a new attempt for the same target.
- Runtime Environment Status remains a coarse operator-visible lifecycle model: `preparing`, `prepared`, `failed`, and `superseded`.
- Fine-grained technical progress is tracked separately as operation step state on the Operation Request rather than by expanding Runtime Environment Status for every internal phase.
- The reusable state-machine abstraction belongs to the Operation Type layer. `preview-start` remains one business operation whose internal execution can grow from source retrieval to deployment and health steps over time.
- Operation step persistence should include at minimum the current step identity, its state, and optional structured step details on the Operation Request.
- Operator-facing HTTP surfaces in this phase continue to use the existing Operator API terminology and bearer-protected `/api` route family throughout.
- Phase 3B adds a runtime-centric Operator API read model alongside the existing webhook-centric audit views rather than replacing those webhook views.
- The runtime read surface should expose a `GET /api/runtime-environments` endpoint that lists Runtime Environments with linked Pull Request summary data needed for operator inspection.
- The runtime read model should include at minimum the Runtime Environment identity and status together with the linked Pull Request identity needed for operator inspection, including the pull request number.
- Runtime list filtering should accept `repository_id` and `pr_number` query parameters. `repository_id` means the internal Toolshed Repository identifier, consistent with the existing Operator API.
- Pull-request filtering should use the natural Operator-facing key pair `repository_id + pr_number` rather than requiring the internal Toolshed `pull_request_id`.
- Runtime list results should return every matching Runtime Environment attempt rather than collapsing to only the latest attempt, and the default ordering should be newest first.
- The first deep modules in this phase should include:
  - a Pull Request target resolver that can hydrate exact source-repository and head-commit data
  - a GitHub tarball retrieval module isolated from the worker orchestration
  - a workspace manager that resolves locators, creates staging areas, strips tarball wrappers, promotes staging to final workspaces, and performs local cleanup
  - a runtime-attempt supersede/repair module that encapsulates same-transaction state changes and cleanup-request creation
  - a reusable Operation Type state-machine registry and transition helper
  - a `preview-start` execution module that composes ensure, materialize, and future deployment steps behind the same operation contract
  - a runtime-environment read module that joins Runtime Environments to Pull Requests for Operator API listing and filtering

## Testing Decisions

- Good tests should assert durable external behavior and valid state transitions, not helper call order or incidental implementation structure.
- The reusable state-machine abstraction should have focused unit tests that verify initial state, valid transitions, invalid transitions, and operation-type scoping behavior.
- Workspace-manager tests should verify observable filesystem outcomes such as staging creation, tarball-wrapper stripping, final promotion, and cleanup, rather than the exact internal extraction sequence.
- Pull Request target-resolution tests should verify fork-aware target selection and event-type-dependent GitHub lookup behavior.
- Operator API tests should verify runtime-environment list behavior from the HTTP contract perspective, including repository filtering, pull-request-number filtering, response ordering, and inclusion of linked Pull Request summary data.
- Worker tests should verify `preview-start` create-versus-reuse behavior, prepared promotion, supersede behavior, missing-workspace repair behavior, and retry semantics from the perspective of persisted Operation Requests and Runtime Environments.
- Cleanup-flow tests should verify that superseded attempts create cleanup Operation Requests in the same transaction and that cleanup targets the correct Runtime Environment attempt.
- Request-path tests should continue to verify transactional persistence of Webhook Events, Pull Request anchor updates, Operation Requests, and River enqueue, now extended with source-repository and initial step-state data.
- Database-backed behavior such as same-target reuse, new-attempt creation after `failed`, supersede transitions, and prepared-workspace repair should be exercised against real PostgreSQL because the correctness depends on durable state and transaction boundaries.
- Filesystem behavior should use temp-directory-based tests that assert final visible layout and cleanup results rather than implementation details of tar extraction.
- Read-model tests should verify that Webhook Event detail hydration can surface operation-level step state and linked runtime-attempt context consistently once those fields are exposed.
- Runtime read-model tests should verify that Runtime Environments are joined to the correct Pull Requests and that historical attempts remain visible in newest-first order.
- Existing prior art in the repository already supports this style:
  - focused unit tests for operation mapping and snapshot construction
  - integration-style worker tests for operation-request handling
  - request-path tests for webhook-trigger-to-operation-request behavior
  - read-model tests for webhook-event detail hydration
  - Operator API response-shaping tests for JSON omission and contract behavior
  - PostgreSQL-backed tests through the existing test database helper

## Out of Scope

- Container startup, Docker Compose execution, runtime health checks, and any running preview stack behavior.
- Compose-file sanitization, compose-path validation, and other backend-specific deployment-input validation.
- Public routing, preview URLs, TLS, GitHub PR comments, GitHub commit statuses, and build-log publication.
- Manual `preview-delete`, TTL-based cleanup, generic orphan reconciliation, and broader Operator teardown flows beyond superseded-attempt cleanup.
- Secret management, fork approval gating for secrets, and encrypted configuration storage.
- Git submodule support, Git LFS support, or mandatory `.git` history preservation.
- A git-clone fallback path for repositories that require git metadata; that remains a possible later extension rather than part of MVP Phase 3B.
- Panel and OAuth work, repository runtime settings UI, or any multi-user access-control surface.
- A Runtime Environment detail route beyond the list/filter read surface needed for operator status inspection.
- A separate persistent workspace-history table beyond the Operation Request step state and Runtime Environment workspace locator already needed for this phase.
- Splitting `preview-start` into multiple business Operation Types just to represent internal pipeline stages.

## Further Notes

- This phase intentionally keeps one stable business intention: ensure a Preview Environment for a target Pull Request head. The implementation grows underneath that intention rather than redefining it.
- The resulting `prepared` status should be read as “ready deployment input exists” across future backends, not as a Docker Compose-specific milestone.
- The filesystem storage backend is only the first implementation of the workspace locator contract. Future Kubernetes-oriented or other storage backends should reuse the same persistent locator semantics.
- The technical state machine should evolve with future phases, but the operator-visible Runtime Environment vocabulary should remain stable.
- The runtime Operator API is additive to the existing Webhook Event audit API. Webhook views remain the source for delivery-level audit, while runtime views become the source for status-oriented inspection.
- If a future repository genuinely requires git metadata or submodules, a separate optional retrieval mode can be introduced later without invalidating the Phase 3B contract.
- This document intentionally folds in the planning decisions from the full session so that future implementation work can proceed without reopening the same terminology and boundary questions.
