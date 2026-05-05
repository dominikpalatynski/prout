# Phase 4 PRD: Docker Compose Runtime Deployment, Repository Runtime Configuration, and Superseded Attempt Teardown

## Problem Statement

As the Operator, I can already see prout accept a GitHub Delivery, evaluate Triggers, create durable Operation Requests, create or reuse the correct Preview Environment attempt, and materialize an isolated workspace for one exact Pull Request Head Commit.

What I still cannot see is the first real runtime-execution slice of preview work. A `prepared` Preview Environment currently means only that a workspace exists. There is no repository-scoped runtime configuration for how a preview should be started, no deployment metadata frozen per Runtime Environment attempt, no Docker Compose preparation pipeline, no Compose sanitization boundary, no structured distinction between retryable infrastructure failures and permanent configuration failures, and no runtime teardown path for superseded attempts that already created Docker resources.

Without this slice, later work such as public routing, TLS, GitHub feedback, operator-driven teardown, and reconciliation would sit on top of an incomplete deployment model. Phase 4 needs to turn one prepared workspace into one running preview stack while preserving the existing Runtime Environment vocabulary, the existing Operation Outcome semantics, and the existing ensure-style behavior of `preview-start`.

## Solution

As the Operator, I will gain the first real runtime deployment phase inside `preview-start`.

When a matched Trigger requests preview work, the existing Operation Request flow will still create or reuse the correct Preview Environment attempt. For a new or retried attempt, `preview-start` will continue materializing source into a workspace and will then advance through two additional technical steps: deployment preparation and runtime deployment.

Deployment preparation will read repository-scoped runtime settings and repository-scoped environment variables, validate and sanitize the configured Docker Compose input, inject prout-managed metadata, runtime networks, and server-wide resource limits, and freeze that resolved deployment input for exactly one Runtime Environment attempt. That frozen deployment input will be persisted as deployment metadata for the attempt and rendered into backend-specific artifacts inside the workspace through a shared workspace/filesystem abstraction.

Runtime deployment will then consume that frozen deployment input and start the preview stack through the Docker Compose runtime path. In this phase, `prepared` will continue to be the coarse Runtime Environment Status that signals successful completion of the current phase. After Phase 4, that means runtime startup completed successfully for that attempt.

Automatic cleanup for superseded Preview Environments will also deepen in this phase. If a superseded attempt has already started runtime deployment, its cleanup flow will first tear down runtime resources and then remove the workspace, while preserving the existing cleanup-style Operation Type rather than inventing a second business-level cleanup concept.

This phase intentionally stops before public routing, TLS, GitHub PR feedback, and application-specific health waiting. The goal is a real Docker Compose deployment slice with a clean technical contract and durable state model, not yet a publicly reachable preview URL.

## User Stories

1. As the Operator, I want `preview-start` to remain one business-level Operation Type, so that preview deployment still expresses one stable user intent even as the internal pipeline gains more technical steps.
2. As the Operator, I want one prepared workspace to become one running preview stack, so that prout moves from source preparation into real runtime execution.
3. As the Operator, I want repository-scoped runtime settings to define where the preview Compose file lives, so that preview deployment does not depend on hardcoded repository conventions.
4. As the Operator, I want repository-scoped runtime settings to define the exposed service name, so that prout knows which service is intended for routing.
5. As the Operator, I want repository-scoped runtime settings to define the canonical exposed service port, so that routing does not rely on guessing from Compose declarations.
6. As the Operator, I want repository-scoped environment variables to remain part of Repository Configuration rather than Runtime Environment attempt state, so that later attempts for the same Repository can reuse the same configuration surface.
7. As the Operator, I want repository-scoped runtime settings to exist by default for each Repository, even when incomplete, so that there is always a stable configuration record to edit.
8. As the Operator, I want incomplete repository-scoped runtime settings to fail the resulting preview attempt during deployment preparation rather than blocking trigger ingestion entirely, so that audit history still shows why preview delivery failed.
9. As the Operator, I want deployment preparation to freeze the runtime settings used by one Runtime Environment attempt, so that later repository edits do not rewrite the meaning of an already-started attempt.
10. As the Operator, I want deployment preparation to freeze the repository-scoped environment variables used by one Runtime Environment attempt, so that retries and teardown stay deterministic for that attempt.
11. As the Operator, I want one technical deployment record per Runtime Environment attempt, so that runtime metadata is attached to the same attempt lifecycle rather than spread across ad hoc state.
12. As the Operator, I want retries of the same Operation Request to update the same deployment record rather than creating deployment history inside a single attempt, so that one attempt stays one lifecycle unit.
13. As the Operator, I want deployment preparation to validate the configured Compose path relative to the workspace root, so that preview deployment cannot reach outside the prepared repository contents.
14. As the Operator, I want absolute Compose paths and escaping paths rejected, so that repository configuration cannot target arbitrary host files.
15. As the Operator, I want missing Compose files to fail deployment preparation deterministically, so that configuration mistakes do not masquerade as infrastructure outages.
16. As the Operator, I want preview deployment to reject host-published ports in Compose input, so that preview stacks cannot conflict with services already running on the host.
17. As the Operator, I want preview deployment to reject host networking and similar escape hatches, so that Pull Request workloads cannot bypass runtime isolation.
18. As the Operator, I want preview deployment to reject external Docker networks declared by repository Compose input, so that preview stacks cannot attach themselves to arbitrary shared infrastructure.
19. As the Operator, I want repository-defined internal networks to remain usable when they stay local to the Compose project, so that normal multi-service stack modeling is not unnecessarily broken.
20. As the Operator, I want each preview attempt to have one private runtime network, so that services from different attempts stay isolated from each other.
21. As the future maintainer of prout, I want the shared Traefik-facing network model to stay compatible with later phases, so that public routing can be added without redefining deployment semantics.
22. As the Operator, I want host-published ports to be invalid even before public routing is implemented, so that the runtime contract does not drift between phases.
23. As the Operator, I want the exposed service port for routing to come from repository-scoped runtime settings rather than Compose inference, so that configuration stays explicit and auditable.
24. As the Operator, I want deployment preparation to accept most safe Compose features while blocking explicitly unsafe or unsupported constructs, so that preview support is broad without giving up safety boundaries.
25. As the Operator, I want backend-specific validation to end with `docker compose config` on the rendered deployment input, so that final Compose semantics are checked by the same CLI path used for deployment.
26. As the Operator, I want server-wide CPU, memory, and PID limits injected into every preview stack, so that one runaway preview cannot starve the host.
27. As the Operator, I do not want per-Repository resource caps in this phase, so that Phase 4 stays aligned with the current Server Configuration model.
28. As the Operator, I want deployment artifacts such as the rendered Compose file to live inside the workspace of the attempt, so that teardown and diagnosis stay scoped to that attempt.
29. As the future maintainer of prout, I want all runtime file operations to go through one shared workspace/filesystem abstraction, so that backend-specific code does not scatter raw filesystem access across the codebase.
30. As the future maintainer of prout, I want the workspace abstraction to expose a narrow path-resolution escape hatch, so that backend tooling can still run external commands without bypassing workspace safety rules.
31. As the future maintainer of prout, I want the runtime backend interface to align with the technical pipeline, so that orchestration can treat deployment preparation, runtime deployment, and runtime teardown as distinct responsibilities.
32. As the future maintainer of prout, I want the runtime backend to receive explicit resolved input rather than performing its own database lookups, so that backend implementations remain testable and infrastructure-focused.
33. As the future maintainer of prout, I want the orchestration layer to remain responsible for persisting deployment metadata and step progress, so that the runtime backend does not take on database responsibilities.
34. As the Operator, I want `preview-start` to keep using the existing high-level Operation Outcomes such as `already_preparing` and `already_prepared`, so that operator-visible semantics stay stable as the pipeline deepens.
35. As the Operator, I want `already_preparing` to cover all in-flight technical steps of `preview-start`, so that repeated ensure-style requests still report one consistent current attempt.
36. As the Operator, I want `already_prepared` to mean that the same target attempt has already reached the ready state of the current phase, so that repeated ensure-style requests reuse a good running preview rather than replacing it.
37. As the Operator, I want a later `preview-start` request to reuse a prepared attempt only while the local artifacts proving that prepared state still exist, so that stale status alone is not trusted indefinitely.
38. As the Operator, I want missing deployment artifacts behind a supposedly prepared attempt to trigger creation of a new attempt, so that self-repair still happens at the ensure boundary.
39. As the Operator, I want deployment preparation failures caused by configuration or sanitization to be treated as permanent failures, so that clearly invalid inputs do not consume queue retries.
40. As the Operator, I want infrastructure-style deployment failures to remain retryable, so that transient Docker or subprocess outages can recover without manual intervention.
41. As the future maintainer of prout, I want retry classification to cross the runtime-backend boundary through normal wrapped errors, so that the basic `error` contract stays idiomatic and simple.
42. As the Operator, I want permanent preview deployment failures to finalize immediately on the current queue attempt, so that queue retries are reserved for genuinely retryable conditions.
43. As the Operator, I want retryable failures to resume from the current technical step of the same Runtime Environment attempt, so that transient issues do not create duplicate attempts.
44. As the Operator, I want deployment preparation retries to rebuild the frozen deployment input for the same attempt, so that partially written artifacts can be repaired in place.
45. As the Operator, I want runtime deployment retries to reuse the already frozen deployment input for the same attempt, so that retry does not silently pick up newer Repository Configuration.
46. As the Operator, I want a partially failed runtime deployment to attempt immediate best-effort teardown, so that active Docker resources do not linger under a failed attempt unless teardown itself also fails.
47. As the Operator, I want failed attempts to keep workspace and metadata available for diagnosis even when runtime teardown is attempted, so that failure analysis remains possible.
48. As the Operator, I want automatic cleanup of superseded attempts to tear down runtime resources before removing the workspace, so that a replaced preview does not leave behind active containers or networks.
49. As the Operator, I want workspace-only cleanup to remain sufficient when a superseded attempt never reached runtime deployment, so that cleanup work stays proportional to what the attempt actually created.
50. As the future maintainer of prout, I want `preview-cleanup-superseded` to remain the same cleanup-style Operation Type as deployment work deepens, so that cleanup semantics stay stable instead of multiplying business operations.
51. As the future maintainer of prout, I want the technical step pipeline to remain small and retry-oriented, so that `current_step` reflects real resume boundaries rather than verbose internal milestones.
52. As the future maintainer of prout, I want only the first technical step to begin in `pending`, so that later steps do not introduce redundant intermediate states.
53. As the future maintainer of prout, I want successful step completion to move directly into the next step’s `in_progress` state, so that retries can always resume from the current real boundary.
54. As the future maintainer of prout, I want the preview worker logic to be split into focused flow modules rather than one growing file, so that source materialization, deployment preparation, runtime deployment, and cleanup stay readable.
55. As the Operator, I want Phase 4 to stop before public preview URLs, TLS, and GitHub feedback, so that Docker Compose runtime execution is delivered as a clean vertical slice without coupling to routing and publication concerns.

## Implementation Decisions

- Phase 4 extends the existing `preview-start` flow; it does not introduce a separate business-level deployment Operation Type.
- `preview-start` grows into a three-step technical pipeline: `source_materialization`, `compose_preparation`, and `runtime_deployment`.
- Only the initial technical step begins in `pending`. Once execution starts, successful completion may move directly into the next step’s `in_progress` state.
- `preview-cleanup-superseded` deepens from a workspace-only cleanup flow into a two-stage cleanup flow that can perform runtime teardown before workspace cleanup when deployment has already started.
- Runtime Environment Status remains the same coarse lifecycle model: `preparing`, `prepared`, `failed`, and `superseded`.
- `prepared` remains backend-agnostic. In Phase 4 it means runtime startup completed successfully for the attempt.
- High-level Operation Outcome values remain stable. `already_preparing` and `already_prepared` are not replaced by deployment-specific outcome names.
- The hot path for ensure-style reuse continues to avoid live Docker inspection. Reuse of a prepared attempt depends on the presence of the local artifacts that define the prepared state for the current phase.
- Repository-scoped runtime settings are stored as a minimal deployment-focused settings record keyed 1:1 by Repository.
- Phase 4 runtime settings are intentionally minimal and include only the fields needed for deployment preparation: Compose file path, exposed service name, and exposed service port.
- Repository-scoped environment variables remain a separate repository-level configuration table rather than being absorbed into runtime settings or Runtime Environment attempts.
- Runtime settings are created by default alongside the Repository and may remain incomplete until the Operator fills in deployment-specific values.
- Incomplete or invalid runtime settings do not block trigger ingestion. They cause deployment preparation for the resulting attempt to fail permanently.
- Deployment-specific metadata is stored in one technical deployment record per Runtime Environment attempt rather than directly on the Runtime Environment row.
- The deployment record freezes both the runtime settings and the repository-scoped environment variables used by that attempt.
- Deployment preparation produces frozen per-attempt deployment input and deployment artifacts before runtime deployment begins.
- The runtime backend interface is split by responsibility rather than staying monolithic. The contract separates deployment preparation, deployment start from prepared input, and deployment teardown.
- The runtime backend receives fully resolved input from the orchestration layer and does not load live Repository Configuration or database state by itself.
- The runtime backend returns prepared deployment data to the orchestration layer, and the orchestration layer persists deployment metadata and step progress.
- Runtime/backend errors continue to cross the interface as ordinary wrapped errors.
- A shared runtime error-classification wrapper marks failures as retryable or permanent without changing the basic `error` contract.
- Permanent failures are finalized immediately on the current queue attempt. Retryable failures continue to use the queue retry policy.
- Deployment preparation retries rebuild the frozen deployment input for the same Runtime Environment attempt.
- Runtime deployment retries reuse the already frozen deployment input for the same Runtime Environment attempt.
- Runtime deployment that partially creates Docker resources and then fails performs best-effort runtime teardown immediately before the attempt is finalized as `failed`.
- Runtime deployment artifacts should be written through a scoped workspace/filesystem abstraction rather than ad hoc direct filesystem calls.
- The shared workspace/filesystem abstraction remains split between workspace lifecycle management and a scoped workspace handle for file operations inside one runtime-attempt workspace.
- The scoped workspace handle may expose a narrow path-resolution escape hatch for external tooling that requires real filesystem paths.
- Repository runtime settings store the Compose file path relative to the workspace root of the Runtime Environment attempt.
- Deployment preparation rejects absolute Compose paths, escaping paths, and missing Compose files.
- Preview deployment rejects host-published ports in repository Compose input.
- Preview deployment rejects `network_mode`, external Docker networks, and other explicitly unsafe or unsupported Compose constructs established by the sanitization rules.
- Repository-defined internal networks remain acceptable when they stay local to the Compose project.
- Repository-scoped runtime settings provide the canonical exposed service name and exposed service port for routing-related deployment preparation. These values are not inferred from repository Compose declarations.
- The long-term networking model remains one private runtime network per attempt plus one shared ingress-facing network for the exposed service, but public ingress behavior itself remains a later phase concern.
- In Phase 4, host-published ports remain invalid even though public routing is still out of scope.
- Server-wide CPU, memory, and PID limits are injected during deployment preparation.
- Phase 4 does not introduce per-Repository resource caps.
- Deployment preparation uses explicit deny rules for known unsafe or unsupported Compose constructs and finishes with rendered `docker compose config` as a backend-level sanity check.
- The current large preview-operation worker logic should be split into focused flow modules rather than extended indefinitely in one file.
- Deep modules to build or extend in this phase should include:
  - a repository runtime settings module
  - a repository environment-variable module
  - a runtime-environment deployment metadata module
  - a runtime deployment orchestration flow for `preview-start`
  - a cleanup orchestration flow for `preview-cleanup-superseded`
  - a Docker Compose backend that prepares, deploys, and tears down frozen deployment input
  - a shared workspace/filesystem module that supports both materialization and deployment artifacts
  - a runtime error-classification helper shared across orchestration and runtime backend code

## Testing Decisions

- Good tests should assert externally visible behavior and durable state transitions, not helper call order or incidental internal structure.
- Database-backed tests should continue to verify worker behavior through persisted Operation Requests, Runtime Environments, and deployment metadata rather than mock-driven implementation detail assertions.
- Step-machine tests should verify the new multi-step `preview-start` pipeline and the deeper cleanup pipeline with valid and invalid transitions.
- Runtime settings tests should verify default record creation, incomplete-configuration behavior, and stable repository-scoped persistence semantics.
- Deployment preparation tests should verify Compose-path validation, sanitizer rejections, environment-variable freezing, rendered deployment artifact creation, and stable deployment metadata output.
- Docker Compose backend tests should verify prepare/deploy/teardown behavior at the module boundary, including permanent-vs-retryable error classification and best-effort teardown after partial deployment failure.
- Workspace/filesystem tests should verify safe relative-path handling, scoped artifact writes, path-resolution escape-hatch behavior, and cleanup of deployment artifacts inside one runtime-attempt workspace.
- Worker flow tests should verify:
  - direct transition from materialization into deployment preparation
  - direct transition from deployment preparation into runtime deployment
  - reuse of an existing prepared attempt only when supporting artifacts still exist
  - creation of a new attempt when prepared artifacts are missing
  - immediate finalization of permanent failures
  - retry behavior for retryable deployment failures
  - reuse of frozen deployment input on runtime deployment retry
  - superseded cleanup behavior for attempts that already created runtime resources
- Cleanup-flow tests should verify that runtime teardown happens before workspace cleanup when appropriate and that workspace-only cleanup still works for pre-deployment attempts.
- Repository configuration tests should verify that repository-scoped environment variables and runtime settings remain separate but are frozen together into deployment metadata for one attempt.
- Prior art already exists in the repository for this style of testing:
  - focused unit tests for operation mapping and snapshot behavior
  - PostgreSQL-backed tests for operation-request worker lifecycle behavior
  - step-machine unit tests for transition validation
  - temp-directory-based workspace tests that assert visible filesystem outcomes
  - Operator API response-shaping tests centered on external JSON behavior rather than internal struct layout

## Out of Scope

- Public routing through Traefik, wildcard-domain publication, preview URLs, and TLS.
- GitHub feedback such as Pull Request comments, commit statuses, or log-viewer links.
- Repository-specific health endpoint semantics or application-level health waiting beyond minimal backend-level runtime verification.
- Manual `preview-delete`, TTL cleanup, generic orphan reconciliation, and broader operator teardown workflows beyond superseded-attempt cleanup.
- `preview-restart`, `preview-delete`, and other later business Operation Types beyond the existing executable set.
- Per-Repository CPU, memory, or PID overrides.
- Secrets management, encrypted repository-scoped environment variables, or fork approval gating for secrets.
- Panel UI, OAuth, or multi-user access-control work for editing runtime settings and repository environment variables.
- Live Docker-state verification on the hot path for ordinary ensure-style `preview-start` requests.
- Persistent deployment-history records beyond one deployment metadata record per Runtime Environment attempt.
- Kubernetes runtime behavior or any other non-Docker backend implementation.

## Further Notes

- Phase 4 is intentionally the Docker Compose runtime-execution slice, not the public-preview slice. Public ingress and GitHub-facing publication remain separate later work.
- The Operator-visible language stays stable: Preview Environment remains the domain concept, Runtime Environment Status stays coarse, and Operation Outcome remains high-level even as technical step state becomes richer.
- The runtime backend contract in this phase is designed to be deep rather than sprawling: small orchestration-facing methods, explicit frozen input, and backend-owned artifact preparation through a shared workspace abstraction.
- Repository-scoped runtime settings are intentionally narrow so they can be trusted as the minimal source of truth for deployment preparation instead of becoming a catch-all configuration surface too early.
- This PRD intentionally captures the planning decisions from the full session so that implementation can proceed without reopening the same terminology, retry, routing, and cleanup questions.
