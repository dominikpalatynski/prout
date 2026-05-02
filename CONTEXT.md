# Toolshed

Toolshed is a self-hosted GitHub automation bot for personal-scale repositories. Its first concrete capability is creating and tearing down pull-request preview environments.

## Language

**Operator**:
The person who installs, configures, and maintains Toolshed on the host.
_Avoid_: Admin, maintainer, owner

**Repository**:
A GitHub repository registered in Toolshed and managed as a unit of automation and preview behavior.
_Avoid_: Project, app, codebase

**Pull Request Source Repository**:
The GitHub repository that contains the current **Pull Request Head Commit** targeted for preview work.
_Avoid_: Head repo, fork repo, source repo

**Pull Request**:
A GitHub pull request within one **Repository** that can trigger preview automation.
_Avoid_: PR, merge request

**Pull Request Head Commit**:
The current commit at the head of one **Pull Request** that runtime work targets.
_Avoid_: SHA, revision, ref

**Runtime Environment**:
A managed ephemeral runtime that Toolshed tracks from preparation through teardown.
_Avoid_: Environment, workspace, deployment, stack

**Runtime Environment Type**:
A global category that describes what kind of runtime Toolshed is managing.
_Avoid_: Operation Type, trigger type, environment class

**Runtime Environment Status**:
The operator-visible lifecycle state of one **Runtime Environment** attempt.
_Avoid_: Job status, queue status

**Preview Environment**:
A **Runtime Environment** of type `preview` created to expose one **Pull Request** for review.
_Avoid_: Deployment, stack, sandbox

**Trigger**:
A repository-specific enablement of one built-in **Trigger Type** that authorizes Toolshed to start work for a pull request.
_Avoid_: Generic matcher, hook, command, automation

**Event Family**:
A global built-in classification of supported GitHub webhook events that Toolshed uses to route, validate, and normalize incoming webhook payloads.
_Avoid_: Trigger Type, webhook delivery, generic event

**Repository Event Family**:
A repository-specific enablement of one built-in **Event Family** that determines whether Toolshed considers that webhook family for that repository.
_Avoid_: Event subscription, webhook switch, trigger group

**Trigger Type**:
A global built-in trigger preset that fixes one event-matching rule and the **Operation Type** created when it matches.
_Avoid_: Generic matcher, event family, trigger instance

**Operation Type**:
A built-in kind of runtime work that Toolshed performs after a **Trigger** matches.
_Avoid_: Trigger Type, dispatch type, command

**Operation Handler**:
The known execution entry point bound to one **Operation Type** that runs that operation's technical workflow.
_Avoid_: Callback, switch case, job body

**Operation Outcome**:
A machine-readable result of handling one queued operation.
_Avoid_: Status, log message, error text

**GitHub Delivery**:
One signed webhook request sent by GitHub to Toolshed.
_Avoid_: Event, notification, callback

**Webhook Event**:
An immutable inbox record created from one verified GitHub Delivery and stored for deduplication, audit, and downstream handling.
_Avoid_: Job, trigger, notification

**Pull Request Conversation Comment**:
A comment added in the main conversation thread of a pull request, not an inline review comment on the diff.
_Avoid_: PR comment, review comment, inline comment

**Operation Request**:
The durable queued request for downstream asynchronous work created after a **Trigger** matches a **Webhook Event**.
_Avoid_: Queue job, task, dispatch

**Operation Request Status**:
The technical handling state of one **Operation Request**.
_Avoid_: Operation Outcome, runtime status

**Operation Request Source**:
The origin of one **Operation Request**, such as a matched **Trigger** or a system-initiated action.
_Avoid_: Trigger Type, actor, operation type

**Server Configuration**:
Install-time system configuration loaded at startup and edited outside the panel.
_Avoid_: Repo config, runtime settings

**Repository Configuration**:
Per-repository behavior managed through Toolshed and applied to later preview actions.
_Avoid_: Server config, app config

## Relationships

- An **Operator** manages one Toolshed installation
- A Toolshed installation manages one or more **Repositories**
- A **Repository** can have many **Pull Requests**
- A **Pull Request** has one current **Pull Request Head Commit**
- A **Pull Request** references one current **Pull Request Source Repository**
- A **Repository** can define multiple **Repository Event Families**
- A **Repository Event Family** belongs to exactly one **Repository**
- A **Repository Event Family** belongs to exactly one **Event Family**
- A **Repository** can define multiple **Triggers**
- A **Trigger** belongs to exactly one **Trigger Type**
- A **Repository** can have at most one **Trigger** of a given **Trigger Type**
- A matched **Trigger** can create one **Operation Request**
- A **Trigger Type** maps to exactly one **Event Family**
- A **Trigger Type** maps to exactly one **Operation Type**
- An **Operation Type** maps to exactly one **Operation Handler**
- An **Operation Type** is global to the system, not per **Repository**
- An **Operation Type** targets exactly one **Runtime Environment Type**
- An **Operation Type** defines how Toolshed handles an already existing **Runtime Environment** for the same target
- An **Operation Request** freezes its target **Pull Request Head Commit** when it is created
- An **Operation Request** keeps an immutable snapshot of the operation intent needed for retries and historical inspection
- An **Operation Request** can create one **Runtime Environment** when asynchronous runtime preparation begins
- An **Operation Request** has one current **Operation Request Status**
- An **Operation Request** has one **Operation Request Source**
- A handled **Operation Request** produces one **Operation Outcome**
- A handled **Operation Request** can reference the **Runtime Environment** it created or reused
- A cleanup-style **Operation Type** can act on an existing **Runtime Environment** without creating a new one
- Marking a **Preview Environment** as `superseded` can create an automatic cleanup **Operation Request** in the same transaction
- A cleanup-style **Operation Request** freezes its target **Runtime Environment** when it is created
- A **Runtime Environment** belongs to exactly one **Repository**
- A **Runtime Environment** belongs to exactly one **Runtime Environment Type**
- A **Runtime Environment** has one current **Runtime Environment Status**
- A **Preview Environment** is one kind of **Runtime Environment** and belongs to exactly one **Pull Request**
- A **Preview Environment** targets exactly one **Pull Request Head Commit**
- A **Preview Environment** keeps its own immutable target **Pull Request Head Commit** even if the **Pull Request** later moves to a newer head commit
- A **Pull Request** can produce multiple **Preview Environments** over time
- **Server Configuration** applies to the whole installation, while **Repository Configuration** applies to one **Repository**

## Example dialogue

> **Dev:** "If I change a repository's trigger, do I need to restart the server?"
> **Domain expert:** "No — that's **Repository Configuration**, so it should affect later preview actions without changing the install-time **Server Configuration**."

> **Dev:** "What's the difference between turning off pull-request comments for one repository and turning off one comment trigger?"
> **Domain expert:** "The first is a **Repository Event Family** choice about one webhook family; the second is a **Trigger** choice about one built-in preset inside that family."

> **Dev:** "The tarball was downloaded but the preview is not running yet. Do we already have a record for it?"
> **Domain expert:** "Yes — the **Runtime Environment** already exists, and its **Runtime Environment Status** tells you whether it is `preparing`, `prepared`, or `failed`."

> **Dev:** "We have a `preview` label trigger and a `/preview` comment trigger. Are those two different things?"
> **Domain expert:** "They are different **Trigger Types** and can be enabled as different **Triggers**, but they map to the same **Operation Type**, so they must not create competing **Runtime Environments** for the same target."

> **Dev:** "If someone comments `/preview` while a preview is already preparing, do we start another one?"
> **Domain expert:** "No — the `preview-start` **Operation Type** is an ensure operation. It reuses the current attempt and reports that the preview is already preparing."

> **Dev:** "The worker finished successfully, but did it create a new preview or reuse an existing one?"
> **Domain expert:** "That is what the **Operation Outcome** tells you. Technical success alone is not enough."

> **Dev:** "A newer commit arrived while the older preview was still preparing. Did the old one fail?"
> **Domain expert:** "No — that older **Runtime Environment** was `superseded`. It was replaced by a newer target, not broken by a preparation failure."

> **Dev:** "Do we remove workspace files automatically when preparation fails?"
> **Domain expert:** "Not by default. Workspace retention depends on **Runtime Environment Status**, and removal is handled by a separate cleanup-style **Operation Type**."

> **Dev:** "What happens when a preview attempt is superseded by a newer commit?"
> **Domain expert:** "The old **Runtime Environment** becomes `superseded`, and a cleanup-style **Operation Type** can remove its workspace automatically."

> **Dev:** "If I comment `/preview-delete`, which preview does it delete?"
> **Domain expert:** "It targets the latest **Preview Environment** for the current **Pull Request Head Commit**. Historical superseded attempts are cleaned up automatically."

## Flagged ambiguities

- "Phase 1 foundation" was being used to include GitHub App auth, OAuth, ACME, and panel/public-domain concerns. Resolved: Phase 1 only covers the walking skeleton needed for local bootstrap, database connectivity, migrations, queue startup, health checks, and a mock ingress-to-job path.
- "repository_id" was being used to mean both the internal **Repository** identifier and GitHub's repository identifier. Resolved: `repository_id` means the Toolshed **Repository** identifier; GitHub's identifier is `github_repository_id`.
- "pull_request_id" was being used to mean both the internal **Pull Request** anchor and GitHub's pull-request identifier. Resolved: `pull_request_id` means the Toolshed **Pull Request** identifier; GitHub's identifier is `github_pull_request_id`.
- "workspace" was being used to mean both the operator-visible lifecycle record and the extracted directory on disk. Resolved: **Runtime Environment** is the lifecycle record; workspace is only an internal technical artifact and not part of the domain glossary.
- "preview environment" was being used to mean both the long-lived PR slot and one concrete lifecycle attempt. Resolved: a **Preview Environment** is per attempt; one **Pull Request** can produce multiple **Preview Environments** over time.
- "queued preview" was being used to mean both queued asynchronous intent and a started runtime attempt. Resolved: **Operation Request** covers queued work before the worker starts; **Runtime Environment** begins when the worker starts runtime preparation.
- A **Runtime Environment Status** for Phase 3 was unclear. Resolved: the initial status set is `preparing`, `prepared`, `failed`, and `superseded`.
- The meaning of `prepared` was unclear. Resolved: `prepared` is a backend-agnostic **Runtime Environment Status** meaning one attempt has successfully reached the intended ready state for the current implementation phase. In Phase 3B that means the tarball was successfully materialized and promoted from staging into the final workspace, the materialized workspace still exists, and the runtime is not yet started. In Phase 4 that means runtime deployment startup completed successfully for that attempt.
- "preview label" and "preview comment" were being treated as separate kinds of work. Resolved: different **Triggers** can map to the same **Operation Type**, and idempotency belongs to the **Operation Type**.
- "Trigger Type" was being used to mean a generic family of event matching. Resolved: **Trigger Type** is a built-in business preset that fixes both matching semantics and the resulting **Operation Type**.
- It was unclear whether **Operation Type** is configured on each **Trigger**. Resolved: **Operation Type** is fixed by **Trigger Type** and is not configured separately on a repository-specific **Trigger** record.
- It was unclear whether a repository-specific **Trigger** stores its own matching semantics. Resolved: a **Trigger** only enables one built-in **Trigger Type** for one **Repository**; matching semantics and preset config belong to the **Trigger Type** definition in code.
- It was unclear whether a repository-specific **Trigger** carries per-repository matcher config. Resolved: a **Trigger** does not carry its own matcher config; if different matching behavior is needed, Toolshed introduces a different built-in **Trigger Type**.
- It was unclear whether one repository can enable the same **Trigger Type** more than once. Resolved: a **Repository** can have at most one **Trigger** of a given **Trigger Type**.
- It was unclear whether **Trigger Type** names should follow technical event families or business intent. Resolved: **Trigger Type** names follow business intent and outcome, such as `preview_on_label_preview`, rather than generic event families such as `pull_request_label`.
- It was unclear whether supported GitHub webhook classifications should be implicit string switches or an explicit model. Resolved: **Event Family** is an explicit built-in model, and each **Trigger Type** maps to exactly one **Event Family**.
- It was unclear whether **Event Family** only labels technical routing or also owns payload parsing. Resolved: **Event Family** owns both supported-event routing and normalization of webhook payloads into the shared trigger-input model.
- "supported events for a repository" was being used to mean both repository-level webhook-family enablement and repository-specific preset enablement. Resolved: repository-level webhook-family enablement is a **Repository Event Family**, while repository-level preset enablement remains a **Trigger**.
- It was unclear whether disabling a **Repository Event Family** should also rewrite child **Triggers**. Resolved: disabling a **Repository Event Family** leaves child **Triggers** unchanged but makes them inert until that **Repository Event Family** is enabled again.
- It was unclear whether **Repository Event Families** should be opt-in or created enabled by default. Resolved: when a **Repository** is registered, all supported **Repository Event Families** are created enabled by default so current webhook behavior stays live until the Operator narrows it.
- It was unclear whether a **Trigger** may be configured while its parent **Repository Event Family** is disabled. Resolved: a **Trigger** may still be created or enabled while its parent **Repository Event Family** is disabled, but it remains inert until that family is enabled again.
- It was unclear whether **Operation Type** should remain implicit in scattered switch statements. Resolved: **Operation Type** is an explicit built-in model with its own code-level definition, so execution behavior is extended through the registry rather than through scattered switches.
- It was unclear whether built-in automation rules should stay split across multiple packages or multiple peer registries. Resolved: built-in automation definitions live in one central **Event Family** registry that acts as the single source of truth, with **Trigger Type** and **Operation Type** mappings hanging from those event definitions rather than from separate top-level registries.
- It was unclear whether **Operation Type** belongs to repository configuration. Resolved: **Operation Type** is a global built-in system category, while repositories only choose which **Triggers** are enabled.
- It was unclear how operation execution should attach to one **Operation Type**. Resolved: each **Operation Type** has one explicit **Operation Handler** as its known execution entry point instead of relying on scattered switch-based dispatch.
- It was unclear whether **Operation Type** definitions should be repeated under each **Trigger Type**. Resolved: built-in automation stays in one central module, but each **Operation Type** definition is declared once and **Trigger Type** definitions only reference it.
- It was unclear whether technical retries create new attempts. Resolved: retries of the same queued operation stay within the same **Runtime Environment**; a new **Runtime Environment** only starts when a new operation attempt begins.
- It was unclear what happens after `failed`. Resolved: a later domain-level retry of the same **Operation Type** starts a new **Runtime Environment** rather than reopening the failed one.
- It was unclear when one **Runtime Environment** attempt becomes `failed` during technical retries. Resolved: transient retries of the same **Operation Request** stay within the same attempt; the attempt becomes `failed` only after a permanent error or after the allowed retries for that request are exhausted.
- It was unclear whether replacement rules are global or operation-specific. Resolved: concurrency and replacement policy belong to the **Operation Type**. For preview-triggered work, repeated `/preview` or `preview` label actions do not replace an in-flight attempt; a separate restart-style **Operation Type** handles explicit replacement.
- The `preview-start` **Operation Type** semantics were unclear. Resolved: `preview-start` is an ensure-style operation: create a new attempt only when no current preview attempt exists for that target; otherwise report the existing state instead of replacing it.
- It was unclear how to distinguish technical request handling success from the domain result of the operation. Resolved: handled operation requests produce a machine-readable **Operation Outcome** such as creating a new runtime attempt or reporting an already-existing one.
- It was unclear where **Operation Outcome** belongs. Resolved: it is persisted on the handled **Operation Request**, with an optional reference to the affected **Runtime Environment**.
- It was unclear whether a **Runtime Environment** should be typed by operation or by runtime kind. Resolved: **Runtime Environment Type** and **Operation Type** are distinct. Many operation types such as `preview-start` and `preview-restart` can target the same runtime type `preview`.
- It was unclear what defines the target of `preview-start`. Resolved: for preview runtime work, the target includes the **Repository**, **Pull Request**, runtime type `preview`, and the current **Pull Request Head Commit**.
- It was unclear how to classify an older preview attempt after a newer head commit becomes the target. Resolved: when a newer target replaces an older preview attempt, the older **Runtime Environment** becomes `superseded` rather than `failed`.
- Workspace retention was unclear. Resolved: `prepared` keeps its workspace for the next lifecycle step, `failed` keeps its workspace for diagnosis until cleanup, and `superseded` can be cleaned up.
- It was unclear whether workspace removal is an implicit side effect or a modeled operation. Resolved: workspace removal is handled by a separate cleanup-style **Operation Type** rather than being hidden inside preparation status changes.
- It was unclear what cleanup targets. Resolved: cleanup operations target a specific **Runtime Environment** attempt, not a whole **Pull Request**.
- "suppressed workspace" was being used to describe an outdated preview attempt. Resolved: the correct **Runtime Environment Status** is `superseded`.
- It was unclear how cleanup starts. Resolved: workspace cleanup can start automatically for a `superseded` **Runtime Environment** and can also be requested manually through a comment-triggered cleanup operation such as `/preview-delete`.
- It was unclear how `/preview-delete` resolves its target. Resolved: the manual delete command targets the latest preview attempt for the current **Pull Request Head Commit**; older superseded attempts are not selected by that command.
- "Trigger Dispatch" was the old name for the queued downstream work item. Resolved: the canonical term is **Operation Request** because the queue boundary now represents a typed operation request, not just a generic trigger handoff.
- It was unclear whether multiple matching triggers for the same target collapse into one queued record. Resolved: each matched **Trigger** creates its own **Operation Request**; reuse or deduplication happens later when the worker resolves the target **Runtime Environment**.
- The **Operation Request** lifecycle was unclear. Resolved: the initial status set is `queued`, `handled`, and `failed`.
- It was unclear when the target head commit is chosen for `preview-start`. Resolved: the **Operation Request** freezes the target **Pull Request Head Commit** at creation time, rather than reading the live PR head later in the worker.
- It was unclear whether a preview attempt reads its target commit indirectly from the live pull request. Resolved: each **Preview Environment** stores its own immutable target **Pull Request Head Commit** copied from the creating **Operation Request**.
- It was unclear whether repeated requests for the same target create parallel attempts. Resolved: multiple **Operation Requests** may reference the same **Preview Environment** when they resolve to the same target and current runtime attempt.
- It was unclear whether automatic and manual cleanup share one operation kind. Resolved: manual `preview-delete` and automatic cleanup of superseded preview attempts are different **Operation Types** with separate audit meaning.
- It was unclear what Phase 3B "cleanup" includes. Resolved: Phase 3B includes local technical cleanup of temporary materialization state and the first executable automatic cleanup flow for `superseded` preview attempts; manual `preview-delete` remains later work.
- It was unclear whether every queued operation must come from a trigger. Resolved: **Operation Requests** have an explicit source and may be created either from matched **Triggers** or from system-initiated actions such as automatic cleanup.
- It was unclear whether automatic cleanup of superseded preview attempts is best-effort or guaranteed with the state change. Resolved: when a preview attempt becomes `superseded`, the automatic cleanup **Operation Request** is created in the same transaction.
- It was unclear when an older preview attempt becomes `superseded` relative to a newer target. Resolved: when `preview-start` creates a new attempt for a newer **Pull Request Head Commit**, the older attempt becomes `superseded` in the same worker transaction, and its automatic cleanup request is created there as well.
- It was unclear how an in-flight superseded attempt behaves without active cancellation. Resolved: Toolshed does not actively cancel the worker; the worker must re-check attempt state at safe boundaries and a `superseded` attempt must never be promoted to `prepared`.
- It was unclear whether `prepared` can be trusted without checking storage state. Resolved: `prepared` requires a valid materialized workspace for that attempt; if the workspace is missing or invalid, the old attempt is no longer a trustworthy prepared input and a new attempt must be created for the same target.
- It was unclear whether every future lifecycle detail should expand the operator-visible status model. Resolved: **Runtime Environment Status** stays a coarse operator-visible lifecycle state, while finer-grained preparation and deployment progress should be tracked separately as technical pipeline step state.
- It was unclear whether future source/deploy/health stages should become separate business operations. Resolved: `preview-start` remains one high-level **Operation Type** expressing business intent, and its finer-grained state machine belongs inside that operation's technical execution model rather than multiplying operation types for each step.
- It was unclear whether cleanup requests resolve their target dynamically in the worker. Resolved: cleanup-style **Operation Requests** store an immutable target **Runtime Environment** when they are created.
- It was unclear whether queued operations should depend only on live joined state. Resolved: each **Operation Request** stores its own immutable operation-intent snapshot so retries and audit do not depend on later changes to triggers, pull requests, or runtime attempts.
- "real environment setup" was being used to mean both repository materialization and container runtime startup. Resolved: in Phase 3B, `preview-start` only retrieves repository source and materializes workspace for one **Runtime Environment** attempt; container startup belongs to later runtime deployment work.
- "workspace entity" was being proposed as if workspace were already a domain concept. Resolved: workspace remains a technical artifact on disk; any separate persistence for workspace location is an implementation choice attached to a **Runtime Environment**, not a new domain term by default.
- "workspace path" was being tied to one job/request execution. Resolved: the materialized workspace belongs to one **Runtime Environment** attempt and is identified from that attempt, so technical retries of the same **Operation Request** reuse the same workspace locator rather than creating per-job paths.
- It was unclear whether the materialized workspace should point at the tarball wrapper directory or the repository root itself. Resolved: the final workspace for one **Runtime Environment** attempt is the repository root after removing GitHub's tarball wrapper directory.
- "repository" was being used to mean both the registered base repository and the repository that actually contains the pull request head commit. Resolved: **Repository** remains the registered base repository in Toolshed, while **Pull Request Source Repository** names the repository that provides the previewed head commit.
- It was unclear whether Phase 4 deployment should introduce a separate `environment` entity alongside **Runtime Environment**. Resolved: deployment remains part of the same **Runtime Environment** attempt; no separate domain entity is introduced for that stage.
- It was unclear whether repository materialization and runtime deployment should become separate business operations. Resolved: `preview-start` remains one **Operation Type**, and both materialization and deployment are technical steps inside that operation's execution model.
- It was unclear whether adding deployment work should expand the coarse **Runtime Environment Status** model. Resolved: **Runtime Environment Status** remains `preparing`, `prepared`, `failed`, and `superseded`, while deployment progress is tracked through technical step state.
- It was unclear whether deployment teardown should target a whole **Pull Request** or one concrete preview attempt. Resolved: runtime deployment identity is anchored to one **Runtime Environment** attempt, so deploy and teardown work target a specific **Runtime Environment** rather than the pull request as a whole.
- It was unclear whether repository-scoped runtime settings should become a new domain entity. Resolved: runtime settings remain part of **Repository Configuration**; any `runtime_settings` table is a technical persistence choice for repository-scoped deployment settings, not a separate domain concept.
- "runtime env variables" was being used to describe repository-scoped deployment configuration. Resolved: repository-scoped env vars remain part of **Repository Configuration** and belong in repository-level configuration storage, not as runtime-attempt data on one **Runtime Environment**.
- It was unclear whether **Repository Configuration** needs a dedicated 1:1 persistence record separate from **Repository**. Resolved: **Repository** remains the persistence anchor, and repository-scoped configuration can be split across focused tables keyed by `repository_id` rather than introducing a separate `repository_configurations` row by default.
- It was unclear how far `preview-start` step granularity should go once deployment is added. Resolved: `preview-start` keeps a small retry-oriented technical pipeline of `source_materialization`, `compose_preparation`, and `runtime_deployment` rather than expanding into many micro-steps.
- It was unclear whether adding deployment requires a new cleanup-style business operation for runtime teardown. Resolved: `preview-cleanup-superseded` remains the same cleanup **Operation Type** for a superseded attempt and can grow from workspace-only cleanup into runtime teardown followed by workspace cleanup when deployment has already started.
- It was unclear whether deployment and cleanup steps should read live repository configuration on every retry. Resolved: deployment input is frozen per **Runtime Environment** attempt during technical preparation so later deploy, retry, and teardown work do not depend on newer **Repository Configuration** changes.
- It was unclear whether deployment-specific persistence should live directly on **Runtime Environment** or in a separate storage record. Resolved: deployment metadata may live in a separate technical table keyed to one **Runtime Environment** attempt without introducing a new domain entity beyond the attempt itself.
- It was unclear whether one **Runtime Environment** attempt may accumulate many deployment records in Phase 4. Resolved: Phase 4 uses at most one deployment-metadata record per **Runtime Environment** attempt, and retries update that same record rather than creating deployment history within the same attempt.
- It was unclear what repository-scoped configuration must be frozen into one deployment attempt. Resolved: the deployment snapshot for one **Runtime Environment** attempt freezes both repository-scoped runtime settings and the repository-scoped env vars used to prepare that deployment input.
- It was unclear what Phase 4 deployment success should mean before repository-specific health behavior exists. Resolved: Phase 4 treats successful runtime startup plus minimal backend-level verification as enough to finish `runtime_deployment`; repository-specific health waiting remains later work.
- It was unclear how preview deployments should avoid host port conflicts and public port exposure on the machine. Resolved: preview runtime deployment does not publish container ports onto the host; external traffic reaches previews only through the shared Traefik path, and host-published ports in preview compose input are treated as invalid deployment configuration.
- It was unclear whether preview routing should infer its target port from compose input or from repository-scoped configuration. Resolved: repository-scoped runtime settings provide the canonical exposed service name and exposed service port used for routing, rather than inferring them from user-supplied compose declarations.
- It was unclear how preview runtime networking should be structured to balance isolation and shared ingress. Resolved: each preview attempt uses one private per-attempt runtime network for the stack, while the exposed service additionally joins the shared Traefik-facing network used for ingress.
- It was unclear whether preview compose validation should accept only a narrow supported subset or block only explicitly unsafe constructs. Resolved: Phase 4 uses explicit validation rules for known unsafe or unsupported constructs and then relies on rendered `docker compose config` as a backend-level sanity check, rather than treating most compose features as invalid by default.
- It was unclear whether the runtime backend interface should stay monolithic once the preview pipeline gains technical steps. Resolved: the runtime backend contract should align with the technical pipeline so orchestration can separate deployment preparation, prepared deployment start, and deployment teardown rather than hiding all of that behind one monolithic deploy call.
- It was unclear whether deployment preparation should persist its own database state from inside the runtime backend. Resolved: the runtime backend returns prepared deployment data to the orchestration layer, and the orchestration layer remains responsible for persisting deployment metadata and step progress in the database.
- It was unclear whether the runtime backend should resolve repository configuration on its own. Resolved: the orchestration layer provides explicit resolved deployment input to the runtime backend, rather than letting the backend read live configuration or database state by itself.
- It was unclear whether backend-specific deployment preparation should perform ad hoc direct filesystem calls. Resolved: runtime preparation and deployment-related file operations should flow through one shared workspace/filesystem abstraction rather than scattered direct filesystem access inside backend-specific packages.
- It was unclear whether one shared filesystem abstraction should be flat or scoped to a single runtime attempt. Resolved: workspace lifecycle concerns stay on a higher-level manager, while file operations inside one runtime-attempt workspace use a scoped workspace handle rather than a global grab-bag filesystem interface.
- It was unclear whether a scoped workspace handle may expose real filesystem paths needed by backend tooling. Resolved: the workspace abstraction may provide a narrow path-resolution escape hatch for files already proven to live inside one runtime-attempt workspace, so backend tooling can invoke external commands without bypassing workspace safety rules.
- It was unclear how repository-scoped compose file configuration should resolve against deployment input on disk. Resolved: repository runtime settings store the compose file path relative to the runtime-attempt workspace root, and deployment preparation rejects absolute paths, escaping paths, and missing compose files rather than resolving them outside the workspace.
- It was unclear whether repository runtime settings should appear only after preview deployment is fully configured. Resolved: repository-scoped runtime settings are created as the default 1:1 settings record for a repository and may remain incomplete until the operator fills in deployment-specific values needed by later preview work.
- It was unclear whether incomplete repository runtime settings should block trigger ingestion before a runtime attempt exists. Resolved: trigger evaluation and operation-request creation still proceed, while deployment preparation fails the resulting runtime attempt if required repository runtime settings are missing or invalid for that attempt.
- It was unclear what should happen if runtime deployment partially creates Docker resources and then fails. Resolved: a failed deployment attempt performs best-effort runtime teardown immediately before the attempt is finalized as `failed`, while keeping workspace and attempt metadata available for diagnosis if teardown itself is imperfect.
- It was unclear whether preview runtime failures should all follow the same retry policy. Resolved: deployment preparation and deployment execution distinguish permanent configuration-style failures from retryable infrastructure-style failures, so deterministic misconfiguration does not consume worker retries like transient runtime outages.
- It was unclear how retry classification should cross the runtime-backend boundary. Resolved: runtime preparation and deployment still return ordinary wrapped errors, while a small shared error-classification wrapper marks failures as permanent or retryable without changing the basic `error` contract between layers.
- It was unclear whether permanent runtime preparation or deployment failures should still consume queue retries until the maximum attempt count. Resolved: permanent failures are finalized immediately on the current attempt, while only retryable failures are left to the queue retry policy.
- It was unclear whether repository runtime settings for Phase 4 should already absorb future health, lifecycle, or resource-policy concerns. Resolved: Phase 4 keeps repository runtime settings minimal and deployment-focused, limited to the fields needed to locate the compose file and define the canonical exposed service and port for routing.
- It was unclear whether later technical steps in one operation should pass through extra `pending` states between successful step completions. Resolved: only the initial technical step starts in `pending`; once execution begins, successful step completion may move directly into the next step's `in_progress` state rather than creating extra intermediate pending states.
- It was unclear how retries should resume after later technical steps have already started. Resolved: retries restart the current technical step for the same **Runtime Environment** attempt, so deployment preparation can rebuild the frozen deployment input for that attempt, while runtime deployment retries reuse the already frozen deployment input rather than re-reading live repository configuration.
- It was unclear when a `prepared` runtime attempt remains trustworthy enough to be reused by a later ensure-style request. Resolved: a later request may reuse a `prepared` attempt only while the attempt still has the local artifacts that define that prepared state for the current phase, rather than trusting the coarse status field alone after supporting artifacts disappear.
- It was unclear whether later implementation phases should rename high-level operation outcomes as the technical pipeline deepens. Resolved: high-level **Operation Outcome** names such as `already_preparing` and `already_prepared` stay stable across later technical phases, while finer-grained deployment detail remains visible through technical step state and step details rather than new outcome names.
