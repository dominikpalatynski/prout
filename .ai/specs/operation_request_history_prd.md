# PRD — Operation Request History

## Problem Statement

As the Operator, I can currently see the latest handling snapshot of an **Operation Request**, but I cannot inspect a clear chronological history of how that operation was handled over time.

Today, the system overwrites the current technical step state as work progresses. That makes it hard to answer practical audit questions such as when an operation started, whether it retried, which milestone it most recently crossed, whether it failed before or after a specific step, and what the system was doing while the operation was still in flight. This gap applies not only to preview-related work, but to any future **Operation Type**, including ones that do not create or reference a **Runtime Environment**.

The Operator needs a durable, operator-visible history for every **Operation Request** and an Operator API that makes that history observable while work is still happening.

## Solution

As the Operator, I will gain an **Operation Request History** for every **Operation Request**. That history will be made of append-only **Operation Request History Entries** representing operator-visible milestones in the handling of the operation.

The history will be owned by the **Operation Request** itself rather than by a **Runtime Environment**, because not every **Operation Type** creates or targets runtime work. The history will remain readable and operator-facing: it will capture meaningful milestones such as operation start, retry, step entry, step completion, step failure, and final outcome, rather than raw line-by-line command output.

The Operator API will expose both historical retrieval and live observation of new **Operation Request History Entries** as they are created. The current snapshot fields on the **Operation Request** will remain in place as the fast answer to “where is it now?”, while the new history answers “how did it get here?”.

Raw technical command output such as `docker compose` line-by-line output will not be part of **Operation Request History** in this slice. If that lower-level visibility is needed later, it will be modeled separately rather than collapsing the operator-facing history into a noisy technical log.

## User Stories

1. As an Operator, I want every **Operation Request** to have an **Operation Request History**, so that I can inspect how that operation was handled from start to finish.
2. As an Operator, I want **Operation Request History** to exist even for operations that do not involve a **Runtime Environment**, so that audit behavior stays consistent across **Operation Types**.
3. As an Operator, I want **Operation Request History Entries** to be chronological, so that I can reconstruct the order of milestones without inferring timing indirectly.
4. As an Operator, I want to see when an **Operation Request** started, so that I know when the system began handling it.
5. As an Operator, I want to see when an **Operation Request** was retried, so that I can distinguish one technical handling attempt from another.
6. As an Operator, I want to see when one technical step entered progress, so that I know which part of the workflow is currently active.
7. As an Operator, I want to see when one technical step completed, so that I can tell which milestones were already crossed successfully.
8. As an Operator, I want to see when one technical step failed, so that I can locate where an operation stopped progressing.
9. As an Operator, I want to see when an **Operation Request** completed successfully, so that I can confirm the final handling result.
10. As an Operator, I want to see when an **Operation Request** failed permanently, so that I can distinguish final failure from transient retryable trouble.
11. As an Operator, I want **Operation Request History** to stay concise and readable, so that it remains useful as an operator-facing audit surface instead of becoming an unreadable technical dump.
12. As an Operator, I do not want raw command line output mixed into **Operation Request History Entries**, so that the history stays focused on milestones instead of noise.
13. As an Operator, I want the system to preserve the current snapshot of the active step separately from the history, so that I can answer both “what is happening now?” and “what already happened?”.
14. As an Operator, I want to fetch the full **Operation Request History** of one operation through the Operator API, so that I can inspect one request without querying the database directly.
15. As an Operator, I want to observe new **Operation Request History Entries** while an operation is still running, so that I can monitor progress in near real time.
16. As an Operator, I want live history updates to use the same language as stored history entries, so that polling and streaming views stay consistent.
17. As an Operator, I want history entries to be safe to expose through the Operator API, so that secrets and raw repository environment-variable values are not leaked.
18. As an Operator, I want one cleanup-style **Operation Request** to have its own history, so that cleanup work is auditable separately from the operation that triggered it.
19. As an Operator, I want system-initiated **Operation Requests** to produce history the same way trigger-created requests do, so that there are no blind spots in background automation.
20. As an Operator, I want history entries to remain durable after the request finishes, so that I can review completed or failed operations later.
21. As an Operator, I want the history of one request to remain stable even if related repository configuration changes later, so that historical audit does not drift.
22. As an Operator, I want the history to tell me that an operation retried without creating a new **Operation Request**, so that retry semantics remain understandable.
23. As an Operator, I want history entries to identify the technical step they refer to when relevant, so that I can correlate milestones with the workflow model already visible on the request snapshot.
24. As an Operator, I want failure entries to provide a clear operator-facing summary, so that I do not need to infer failure meaning only from a raw error blob.
25. As an Operator, I want the history feature to work for current preview flows and future **Operation Types**, so that audit coverage grows with the product instead of being preview-specific.
26. As a future maintainer, I want one simple write interface for appending **Operation Request History Entries**, so that worker flows can emit history without duplicating formatting logic.
27. As a future maintainer, I want one simple read interface for loading history entries, so that Operator API handlers do not grow their own custom query logic.
28. As a future maintainer, I want live delivery of new history entries separated from persistence concerns, so that storage and streaming can evolve independently.
29. As a future maintainer, I want history recording to live at the **Operation Request** orchestration layer, so that preview-specific code is not treated as the owner of all future audit semantics.
30. As a future maintainer, I want the first slice of this feature to stop short of full technical output recording, so that the operator-facing history model can become stable before lower-level observability is added.
31. As a future maintainer, I want the same history model to work whether an **Operation Request** creates a new **Runtime Environment**, reuses one, or never touches runtime work at all, so that the design is not coupled to one operational path.
32. As a future maintainer, I want history entries to be append-only, so that the recorded operator-visible story of one request is durable and easy to reason about.
33. As a future maintainer, I want one conceptual **Operation Request History** without a separate parent database row, so that persistence stays simple and the model does not gain unnecessary wrapper records.
34. As a future maintainer, I want the Operator API to expose a dedicated history surface instead of forcing clients to reconstruct history from step snapshots, so that external consumers stay simple and robust.
35. As a future maintainer, I want the design to leave room for a later lower-level technical-output entity, so that future deep observability can be added without redefining **Operation Request History**.
36. As an Operator, I want terminal-visible debug logs for history recording and request progression, so that I can see what the worker is doing even before I open the Operator API.
37. As a future maintainer, I want **Operation Request History Entries** to be emitted from all important **Operation Request** orchestration points, so that no major milestone is missing from the audit story.
38. As an Operator, I want the new history endpoints represented in the Bruno collection, so that I can inspect and exercise the audit API quickly during development and operations.

## Implementation Decisions

- The feature introduces **Operation Request History** as an operator-facing concept and **Operation Request History Entry** as the persisted append-only record inside that history.
- Every **Operation Request** has one conceptual **Operation Request History**, but that history does not get its own parent persistence row.
- Persistence is modeled as append-only history entries owned by `operation_request_id`.
- The canonical owner of a history entry is the **Operation Request**, not the **Runtime Environment**.
- The history model must work for **Operation Types** that do not create or target a **Runtime Environment**.
- The current snapshot fields already stored on the **Operation Request** remain in place and are not replaced by the history feature.
- The history answers “what happened over time,” while the snapshot answers “where is the request now.”
- **Operation Request History Entries** are milestone-oriented rather than line-oriented.
- History entries include operator-visible handling milestones such as request start, retry, step entry, step completion, step failure, success, and permanent failure.
- Raw technical command output is explicitly excluded from **Operation Request History** in this slice.
- If lower-level output recording is added later, it should be modeled as a separate technical record rather than by widening the meaning of **Operation Request History Entry**.
- The persistence shape should be an append-only table of history entries with stable chronological ordering.
- Each history entry should carry the minimum fields needed for operator audit: owner request, time, entry kind, operator-facing message, and structured details when useful.
- Step-related entries should include the relevant technical step when that context exists.
- History details must be safe for operator exposure and must not include secret values or raw repository environment-variable values.
- History entry creation must be integrated into the important **Operation Request** orchestration points rather than being limited to one helper or one preview-specific path.
- At minimum, entries should be recorded when the worker starts handling a request, when a request is retried, when a step enters progress, when a step completes, when a step fails, when a request is finalized successfully, and when a request is finalized as failed.
- Preview-start and cleanup-style orchestration flows should add request-history milestones around their meaningful request-level transitions instead of leaving those transitions visible only through overwritten current-step snapshots.
- Request-finalization paths must append the final history entry before the request is considered complete from an operator-history perspective.
- The write side should be encapsulated behind one small history-recording interface used by worker flows and supporting orchestration code.
- The read side should be encapsulated behind one small history-query interface used by Operator API handlers.
- Live delivery of new history entries should be handled through a separate subscription or broadcast component rather than being fused into persistence code.
- The first Operator API slice should expose request-scoped history retrieval and live observation of new history entries for one **Operation Request**.
- The live API should speak in terms of **Operation Request History Entries**, not in terms of raw logs or generic “events”.
- New Operator API history endpoints must be added to the Bruno collection so they are easy to call during local development and manual operational diagnosis.
- The Bruno collection should cover the normal history retrieval endpoints at minimum, and any live endpoint that is awkward to exercise in Bruno should still be represented or documented consistently alongside the rest of the operator-facing API surface.
- System-initiated **Operation Requests** such as automatic cleanup must record history through the same mechanism as trigger-created operations.
- Preview-specific flows should emit history through shared request-history abstractions instead of defining a preview-only audit vocabulary.
- Retry behavior should append new history entries to the same request history rather than fork history into separate records.
- Final failure handling should append operator-visible failure history before the request is considered complete from an audit perspective.
- The design deliberately stops short of defining a cross-request **Runtime Environment** history model as a first-class API surface in this slice.
- The design deliberately stops short of introducing a separate `operation_request_histories` wrapper record because it does not add meaningful domain value over the owner request plus append-only entries.
- The worker and request-history path should also emit structured debug logs at the important milestones so terminal output shows request start, retries, step progression, history-entry creation, and finalization outcomes with request identifiers and step context.
- Debug logs are a companion visibility surface, not a replacement for **Operation Request History**. The history remains the durable operator audit record, while terminal debug logs remain transient developer and operator diagnostics.

## Testing Decisions

- Good tests should assert external behavior: what history entries are recorded, in what order, through which API contract, and under which request-handling scenarios.
- Tests should avoid coupling to internal helper structure or incidental function call order.
- The history persistence module should be tested for append-only behavior, stable ordering, and correct retrieval for one **Operation Request**.
- The request-handling worker flows should be tested for history emission at the important milestones: start, retry, step transitions, final success, and final failure.
- Cleanup-style request flows should be tested to confirm that system-created requests also produce their own histories.
- Operator API tests should assert both historical retrieval and live delivery semantics for request history.
- Tests should explicitly verify that raw technical command output is not recorded as ordinary **Operation Request History Entries** in this slice.
- Tests should verify that history entries stay safe for operator exposure and do not leak secret configuration values.
- Tests should verify that the main **Operation Request** orchestration paths actually emit the expected milestone entries instead of silently relying only on current-step snapshot mutation.
- Tests should verify that structured debug logs are emitted at the expected request-level milestones with the identifiers and step context needed for terminal diagnosis.
- Tests should verify that the Bruno collection examples for the new history endpoints remain in sync with the Operator API contract.
- Prior art already exists in the codebase for this test style:
- PostgreSQL-backed tests for **Operation Request** worker behavior and persisted lifecycle state.
- API response tests that validate external JSON shape and request handling.
- Step-machine tests that validate technical workflow progression.
- Focused module tests around shared logging and runtime behavior where externally visible output matters more than helper internals.

## Out of Scope

- Raw `docker compose` line-by-line output capture.
- A lower-level technical-output entity for command streams.
- Full tracing or distributed observability tooling.
- A first-class unified history surface grouped primarily by **Runtime Environment** across multiple **Operation Requests**.
- UI implementation details for a panel or dashboard.
- GitHub feedback features such as posting history into pull request comments.
- Generalized log aggregation outside the Operator API.
- Redefining coarse **Runtime Environment Status** around the new history model.

## Further Notes

- This PRD is intentionally narrower than a full observability initiative. It defines the operator-facing history model first, because that is the most stable conceptual slice.
- The resulting language should stay aligned with the glossary: **Webhook Event** remains the inbound GitHub record, while **Operation Request History Entry** becomes the operator-visible record of one operation’s handling.
- The feature is designed to scale to future **Operation Types** without assuming that runtime deployment is always involved.
- The history model should remain understandable to an Operator without requiring knowledge of tracing systems, streaming log protocols, or backend-specific command behavior.
- A later PRD can introduce the lower-level technical-output record once the boundary between operator-visible history and raw runtime output is proven in practice.
