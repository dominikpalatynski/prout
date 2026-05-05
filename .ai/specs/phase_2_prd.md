# Phase 2 PRD: Real GitHub Ingress and Trigger Dispatch

## Problem Statement

As the Operator, I need Phase 2 to replace the current mock webhook slice with a real GitHub ingress path that I can inspect end-to-end. Right now the system proves only that a GitHub-shaped HTTP request can enqueue a probe job and write `_ping`. That is enough for the walking skeleton, but it does not show how prout will behave once real signed GitHub Deliveries start arriving.

I specifically want to focus this phase on GitHub integration and event handling. I need to see how a verified GitHub Delivery becomes a persisted Webhook Event, how a Repository and its Triggers are looked up, how Trigger Types are evaluated, and how matched work is handed off asynchronously. Without that slice, later phases would add tarball retrieval, runtime deployment, panel work, and GitHub feedback on top of an ingress path that still hides the real system boundaries.

I also need a way to configure this behavior before the panel exists. That means Phase 2 needs an operator-oriented API for managing Repositories and Triggers, plus a read API that lets me inspect Webhook Events, Trigger evaluations, and Trigger Dispatch records directly.

## Solution

Phase 2 will introduce a real GitHub ingress built around an immutable `webhook_events` inbox for verified GitHub Deliveries. The webhook endpoint will verify the GitHub signature on the raw request body, classify the event type, and persist a Webhook Event exactly once per GitHub delivery identifier. Unsupported but verified deliveries will still be recorded in the inbox and marked as ignored, while invalid-signature deliveries will be rejected before any database write.

For supported event families, the webhook request will synchronously perform the lightweight parts of event handling: Repository lookup, Trigger loading, Trigger evaluation, evaluation persistence, Trigger Dispatch creation, and River job enqueue for downstream asynchronous processing. River will start at the Trigger Dispatch boundary, not at the event classification boundary. This keeps the request path responsible for the deterministic interpretation of a GitHub Delivery while leaving asynchronous execution to downstream work.

The first supported GitHub event scope in this phase is intentionally narrow:

- `pull_request.opened`
- `pull_request.labeled`
- `issue_comment.created` only when the payload refers to a pull request and the comment belongs to the main pull request conversation thread

Repositories will be registered explicitly through an operator API protected by a bearer token from Server Configuration. Trigger Types are global system capabilities, but Trigger instances remain repository-specific rules stored in the database. The read API will expose the inbox and its downstream artifacts so the Operator can observe real GitHub event handling without waiting for the panel phase.

## User Stories

1. As the Operator, I want real GitHub webhook signature verification, so that prout only accepts trusted GitHub Deliveries into its inbox.
2. As the Operator, I want Phase 2 to persist verified GitHub Deliveries as immutable Webhook Events, so that I can inspect exactly what prout accepted.
3. As the Operator, I want delivery deduplication by GitHub delivery identifier, so that the same GitHub Delivery is accepted once even if GitHub retries it.
4. As the Operator, I want duplicate deliveries to return success semantics to GitHub without duplicating downstream work, so that idempotency does not look like an error to the sender.
5. As the Operator, I want invalid-signature deliveries rejected before persistence, so that the inbox remains a record of trusted GitHub input rather than mixed trusted and untrusted traffic.
6. As the Operator, I want unsupported but verified event types recorded and marked as ignored, so that the inbox remains a truthful record of what GitHub actually sent.
7. As the Operator, I want the system to classify event type before touching Repository state, so that unsupported deliveries do not cause unnecessary Repository lookups.
8. As the Operator, I want a narrow initial event subscription surface, so that Phase 2 focuses on the most useful pull request interactions without flooding the inbox.
9. As the Operator, I want `pull_request.opened` handled, so that prout can observe the beginning of a pull request lifecycle.
10. As the Operator, I want `pull_request.labeled` handled, so that label-based Trigger evaluation becomes visible.
11. As the Operator, I want `issue_comment.created` on pull requests handled, so that comment-driven Trigger evaluation becomes visible.
12. As the Operator, I want pull request conversation comments treated separately from inline review comments, so that prout starts from a clean and unambiguous comment model.
13. As the Operator, I want supported events for an unregistered Repository to be persisted and marked ignored, so that I can see the event without accidentally creating unmanaged Repository records.
14. As the Operator, I want supported events for a disabled Repository to be persisted and marked ignored, so that I have a Repository-wide kill switch without deleting configuration.
15. As the Operator, I want Repositories to be registered explicitly through prout, so that a Repository is a managed unit of automation rather than a side effect of the first webhook.
16. As the Operator, I want Repository lookup keyed by GitHub repository ID, so that webhook handling is stable even if repository names change.
17. As the Operator, I want Repository registration to resolve GitHub repository metadata from `owner/name`, so that I do not need to manually enter GitHub numeric identifiers.
18. As the Operator, I want prout to persist the GitHub installation identifier for each Repository, so that later GitHub API calls know which installation auth context to use.
19. As the Operator, I want Repository registration to be an upsert, so that repeated registration calls safely refresh metadata instead of creating duplicates.
20. As the Operator, I want Trigger Types to be global system capabilities, so that all Repositories draw from one built-in catalog of supported Trigger behavior.
21. As the Operator, I want Trigger instances to remain repository-specific rules, so that each Repository controls when prout starts work.
22. As the Operator, I want Trigger configuration stored in the database before the panel exists, so that Phase 2 uses real Repository Configuration rather than temporary hardcoded rules.
23. As the Operator, I want each Trigger to belong to exactly one event family, so that matching logic stays simple and predictable.
24. As the Operator, I want a Repository-level `enabled` flag, so that I can suspend all automation for one Repository without deleting its configuration.
25. As the Operator, I want a Trigger-level `enabled` flag, so that I can suspend one rule without deleting it.
26. As the Operator, I want Trigger creation to be idempotent by semantic identity, so that repeated API calls do not create duplicate rules that would double-fire downstream work.
27. As the Operator, I want `pull_request.opened` to be representable as a real Trigger even though it has no parameters, so that all automation entry points use one consistent Trigger model.
28. As the Operator, I want `pull_request.labeled` Trigger configuration to target a specific label, so that label-driven automation is explicit.
29. As the Operator, I want comment command Trigger configuration to define how a pull request conversation comment is interpreted, so that comment matching can evolve without changing the storage model.
30. As the Operator, I want comment command matching to start with a strict exact-first-line strategy, so that comment triggers remain explicit and predictable in Phase 2.
31. As the Operator, I want comment command matching to be case-sensitive, so that command semantics are deterministic rather than fuzzy.
32. As the Operator, I want prout to build in-memory normalized event types after inbox persistence, so that downstream matching logic works on stable local types rather than raw GitHub payload shape.
33. As the Operator, I want Trigger evaluation to happen synchronously inside webhook processing, so that the request path fully explains how a supported delivery was interpreted.
34. As the Operator, I want every supported event to evaluate all configured matching Triggers in its Repository, so that repository-specific rules behave independently rather than first-match-wins.
35. As the Operator, I want Trigger evaluations persisted even when no Trigger matches, so that I can see why a supported event produced no downstream action.
36. As the Operator, I want Trigger evaluation history to store a snapshot of the Trigger at evaluation time, so that later Trigger edits do not rewrite the meaning of old Webhook Events.
37. As the Operator, I want matched Triggers to create durable Trigger Dispatch records, so that downstream asynchronous work is visible as part of the product model rather than hidden only inside River.
38. As the Operator, I want one downstream job per matched Trigger, so that each matched repository-specific rule produces its own asynchronous handoff.
39. As the Operator, I want Trigger Dispatch records to store a snapshot of dispatch intent, so that downstream work remains historically accurate even if code-level mappings change later.
40. As the Operator, I want River to start at the Trigger Dispatch boundary, so that asynchronous work is reserved for downstream execution rather than simple event interpretation.
41. As the Operator, I want Webhook Event persistence, Trigger evaluation persistence, Trigger Dispatch creation, and River enqueue to be one transaction, so that accepted event handling is never left half-written.
42. As the Operator, I want Trigger Dispatch worker jobs to read their source of truth from the database, so that retries always re-execute the exact persisted dispatch intent.
43. As the Operator, I want a minimal downstream generic dispatch in this phase, so that I can observe async handoff before preview-specific runtime work exists.
44. As the Operator, I want a read API for Webhook Events, so that I can inspect real event handling without manually querying the database.
45. As the Operator, I want to filter the Webhook Event list by Repository, status, event type, and delivery ID, so that I can find the exact flow I am debugging.
46. As the Operator, I want the detail view for one Webhook Event to include Trigger evaluations and Trigger Dispatches, so that I can inspect one delivery end-to-end in a single API call.
47. As the Operator, I want an operator API for Repository registration and Trigger management before the panel exists, so that I can exercise the real configuration model during Phase 2.
48. As the Operator, I want the operator API protected by a bearer token from Server Configuration, so that this temporary management surface is simple but not anonymous.
49. As the future panel, I want a machine-readable Trigger Type catalog from the backend, so that the UI can build configuration forms from backend metadata instead of hardcoding every Trigger Type.
50. As the future maintainer of prout, I want Phase 2 to clearly separate GitHub Delivery intake, Trigger evaluation, and Trigger Dispatch, so that later phases can add preview-specific behavior without unpicking the event pipeline.

## Implementation Decisions

- Phase 2 refines the broad implementation-phase goal into a narrower slice centered on real GitHub ingress, Trigger evaluation, and generic asynchronous Trigger Dispatch. Tarball retrieval, runtime deployment, and GitHub feedback remain later phases.
- The current Phase 1 probe path is replaced with a real webhook intake path rather than extended in place. `_ping` remains a historical walking-skeleton probe and is not the event model for Phase 2.
- A verified GitHub Delivery becomes one immutable `webhook_events` inbox record. This inbox is the durable acceptance boundary for GitHub input.
- `webhook_events` stores `payload_json` as `JSONB`, not the exact raw request bytes. Signature verification still runs against the raw HTTP body before persistence.
- `webhook_events` is populated only for verified deliveries. Invalid-signature requests are rejected before any database write.
- Duplicate GitHub Deliveries are deduplicated by `delivery_id`. A duplicate returns success semantics to GitHub but does not create a second inbox record or a second downstream handoff.
- Unsupported but verified event types are persisted in `webhook_events` and marked `ignored` with a machine-readable reason such as `unsupported_event_type`.
- Supported event processing checks event type first, and only then looks up the Repository. This avoids unnecessary Repository queries for unsupported deliveries.
- The initial GitHub event scope is intentionally small: `pull_request.opened`, `pull_request.labeled`, and `issue_comment.created` only for pull request conversation comments in the main thread.
- Review comments, review threads, review submissions, issue comments on plain issues, and installation lifecycle events are deferred.
- `Repository` is canonically identified inside prout by `github_repository_id`. `owner/name` is stored as metadata and can be refreshed.
- `github_installation_id` is stored on the Repository as the auth context needed for later installation-token GitHub API calls. It is not the identity of the Repository itself.
- Repository registration is explicit and happens through an operator API rather than implicitly from webhook traffic.
- Repository registration accepts `owner/name`, resolves GitHub metadata through GitHub App credentials, and stores `github_repository_id` plus `github_installation_id`.
- Repository registration behaves as an upsert keyed by `github_repository_id`, refreshing mutable metadata rather than creating duplicates.
- Supported events for unknown Repositories are persisted as ignored with a reason such as `repository_not_registered`.
- Repositories gain an `enabled` flag. Supported events for disabled Repositories are persisted as ignored with a reason such as `repository_disabled`.
- Trigger Types are global, built-in categories defined in code. Trigger instances remain repository-specific rules persisted in the database.
- Trigger instances live in a generic `repository_triggers` table with a global `type`, a persisted `config_json`, an `enabled` flag, and a backend-computed semantic identity key.
- Trigger creation behaves as an upsert keyed by Repository plus Trigger semantic identity, not as a blind insert.
- The backend computes a stable `identity_key` or equivalent semantic key for each Trigger so database uniqueness does not rely on raw `JSONB` equality.
- Each Trigger belongs to exactly one event family. Mixed event-family Triggers are explicitly excluded from Phase 2.
- `pull_request_opened` is treated as a first-class Trigger Type even though its configuration is empty.
- `pull_request_label` Trigger configuration carries a label value.
- `pull_request_comment_command` Trigger configuration carries a comment-matching kind plus command text.
- Comment-trigger matching is intentionally modeled behind its own interface so future comment strategies can be added without changing Trigger storage.
- The first supported comment strategy is a strict exact-first-line matcher, applied case-sensitively.
- Webhook handling builds in-memory normalized event types after inbox persistence. These are not persisted as separate entities in Phase 2.
- Trigger evaluation is synchronous in the HTTP request path. River is not used to interpret supported events or compute Trigger matches.
- All enabled Triggers for the Repository are evaluated for a supported event. The system does not stop at the first match.
- Trigger evaluation results are persisted in a `webhook_event_trigger_evaluations` table, including non-matching results for supported events.
- Evaluation records store a snapshot of the Trigger definition at evaluation time so old results remain historically accurate after later Trigger edits.
- A matched Trigger creates a `trigger_dispatches` record representing downstream asynchronous work to be performed.
- Trigger Dispatch is a durable product concept separate from River job internals. It exists so operator-facing diagnostics and later panel views do not depend on River tables.
- `trigger_dispatches` has its own `dispatch_type`, independent from `trigger.type`. The mapping from Trigger Type to dispatch type is code-defined in Phase 2 and not configurable per Repository.
- Trigger Dispatch records store a snapshot of dispatch intent, including `dispatch_type` and any `dispatch_payload_json`, so worker retries execute the originally scheduled work.
- The minimal Trigger Dispatch lifecycle for Phase 2 is `queued`, `processed`, and `failed`.
- `webhook_events` uses a minimal status model in Phase 2: `ignored`, `processed`, and `failed`.
- `ignored` is used only when the request path concludes that no downstream processing should happen, for example unsupported event type, unknown Repository, or disabled Repository.
- `processed` means a supported event was successfully interpreted and Trigger evaluation completed, even if zero Triggers matched.
- `failed` means the event pipeline attempted supported-event processing but encountered a technical failure.
- Trigger evaluation and Trigger Dispatch creation happen in the same database transaction as `webhook_events` persistence for supported events.
- River enqueue for downstream Trigger Dispatch jobs is part of the same transaction as event persistence, evaluation persistence, and dispatch creation.
- One matched Trigger yields one Trigger Dispatch and one downstream River job.
- The generic downstream worker is `TriggerDispatchJob`. Its arguments carry only `trigger_dispatch_id`.
- `TriggerDispatchJob` reloads its source record from `trigger_dispatches` and updates status there. It does not rely on duplicated payload in job args.
- The operator API is a temporary management surface for Repository Configuration before the panel exists. It is protected by a bearer token from Server Configuration.
- The operator API includes Repository registration and listing, Trigger creation/upsert and listing, and patch endpoints for toggling `enabled` state on both Repositories and Triggers.
- Trigger updates through `PATCH` are restricted to operational toggles such as `enabled`. Changing Trigger semantics means posting a new upsert rather than mutating the existing Trigger in place.
- Trigger deletion is out of scope for Phase 2. Disabling a Trigger replaces hard deletion for this phase.
- The operator API uses internal prout Repository identifiers in resource paths rather than `owner/name`.
- The backend exposes a `GET /api/trigger-types` style catalog endpoint so future clients can discover global Trigger Types and their configuration contract.
- The Trigger Type catalog returns a simple backend-defined schema description rather than full JSON Schema. At minimum it describes the Trigger Type, event family, and required configuration fields.
- The read API exposes Webhook Events with filtering by Repository, status, event type, delivery ID, and limit.
- The Webhook Event detail endpoint returns the inbox record plus associated Trigger evaluations and Trigger Dispatches in one response.
- Deep modules worth shaping deliberately in Phase 2 are:
- A GitHub ingress module that verifies signatures, parses envelopes, classifies supported event families, and exposes a narrow API to the request handler.
- A Trigger catalog and validation module that owns global Trigger Type definitions, config validation, identity-key derivation, and form metadata for the operator API.
- A Trigger evaluation module that maps normalized event input against repository-specific Trigger instances and yields durable evaluation records plus dispatch intents.
- A Repository registration module that resolves GitHub metadata and installation context from `owner/name` and persists the minimal anchor needed for later phases.
- A Trigger Dispatch module that owns dispatch record creation, dispatch snapshot shape, and the generic asynchronous worker boundary.

## Testing Decisions

- Good tests should assert external behavior and durable outcomes, not implementation details or internal helper sequencing.
- Request-path tests should focus on observable contracts such as signature rejection, deduplication behavior, supported-versus-unsupported event handling, ignored reasons, and the persisted shape of Webhook Events, evaluations, and dispatches.
- Trigger tests should focus on matching behavior from the perspective of input and configuration, not on internal matcher implementation details.
- Operator API tests should focus on resource contracts: Repository registration upsert behavior, Trigger creation idempotency, `enabled` toggles, validation errors, and Trigger Type catalog responses.
- Dispatch-worker tests should focus on status transitions and durable updates to Trigger Dispatch records, not on River internals.
- Modules that deserve focused testing in Phase 2 are:
- GitHub signature verification and event classification behavior.
- Repository registration and GitHub metadata resolution behavior.
- Trigger catalog validation and identity-key derivation.
- Trigger evaluation for `pull_request_opened`, `pull_request_label`, and pull request conversation comment commands.
- Transactional event persistence, evaluation persistence, dispatch creation, and enqueue handoff behavior.
- Webhook Event read API hydration, including embedded evaluations and dispatches.
- Trigger Dispatch worker behavior for queued-to-processed and queued-to-failed transitions.
- Pure matching logic, config validation, and identity-key derivation are good candidates for narrow unit tests because they are deep modules with stable input/output boundaries.
- Database-backed behavior, especially idempotency and transactional handoff, is a good candidate for integration tests against real PostgreSQL rather than mocked stores.
- Prior art already exists for DB-backed test setup in the repository’s test database helper, which shows the codebase already anticipates real Postgres integration-style tests.
- Existing Phase 1 smoke behavior provides a useful baseline for request-path verification, but Phase 2 should move critical event-handling behavior into automated tests rather than leaving it entirely to manual curl verification.
- River itself does not need bespoke re-testing; the tests should verify prout’s use of River through persisted application state and job-handled outcomes.

## Out of Scope

- Tarball download and workspace materialization.
- Preview Environment runtime deployment and teardown.
- Public routing, TLS, and preview URL publication.
- GitHub PR comment publishing, status checks, and other outward GitHub feedback.
- Panel views, GitHub OAuth sign-in, and multi-user access control.
- Trigger authorization via live GitHub collaborator permission checks.
- Automatic Repository creation from webhook traffic.
- Installation lifecycle event handling as a first-class Phase 2 event family.
- Inline review comments, review submissions, and review-thread handling.
- Exact raw request body persistence.
- Hard deletion of Trigger configuration.
- Repository deletion flows.
- Multi-step downstream automation beyond a generic Trigger Dispatch handoff.
- Rich dispatch progress states such as `running`.
- Full retry-attempt history as part of Webhook Event or Trigger Dispatch read models.
- A public external API intended for third-party consumers.

## Further Notes

- This PRD deliberately sharpens the generic “Real GitHub Ingress” phase description into a product slice optimized for seeing real event handling as early as possible.
- The biggest refinement versus the earlier phase outline is that Trigger evaluation happens synchronously during webhook handling, while River begins only at the Trigger Dispatch boundary.
- Another deliberate refinement is that Repository registration happens through an operator API instead of installation webhook lifecycle events. That keeps Phase 2 focused on ingress, Trigger evaluation, and observability rather than on GitHub App installation management.
- The read API is a first-class part of the acceptance story for this phase. The Operator should be able to watch a GitHub Delivery turn into a Webhook Event, see Trigger evaluations, and confirm asynchronous handoff without reading internal River tables directly.
- The global Trigger Type catalog is intended to become the future panel’s source of truth for building Trigger configuration forms.
