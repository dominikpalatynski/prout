# Toolshed

Toolshed is a self-hosted GitHub automation bot for personal-scale repositories. Its first concrete capability is creating and tearing down pull-request preview environments.

## Language

**Operator**:
The person who installs, configures, and maintains Toolshed on the host.
_Avoid_: Admin, maintainer, owner

**Repository**:
A GitHub repository registered in Toolshed and managed as a unit of automation and preview behavior.
_Avoid_: Project, app, codebase

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
A repository-specific rule that authorizes Toolshed to start work for a pull request.
_Avoid_: Hook, command, automation

**Trigger Type**:
A global trigger category built into Toolshed that defines one family of event matching and the configuration shape expected by that rule.
_Avoid_: Trigger instance, repository rule, plugin

**Operation Type**:
A built-in kind of runtime work that Toolshed performs after a **Trigger** matches.
_Avoid_: Trigger Type, dispatch type, command

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
- A **Repository** can define multiple **Triggers**
- A matched **Trigger** can create one **Operation Request**
- A **Trigger** maps to exactly one **Operation Type**
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

> **Dev:** "The tarball was downloaded but the preview is not running yet. Do we already have a record for it?"
> **Domain expert:** "Yes — the **Runtime Environment** already exists, and its **Runtime Environment Status** tells you whether it is `preparing`, `prepared`, or `failed`."

> **Dev:** "We have a `preview` label trigger and a `/preview` comment trigger. Are those two different things?"
> **Domain expert:** "They are different **Triggers**, but they map to the same **Operation Type**, so they must not create competing **Runtime Environments** for the same target."

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
- The meaning of `prepared` was unclear. Resolved: `prepared` means runtime preparation succeeded and the materialized workspace still exists for the next lifecycle step; it is not only a historical success marker.
- "preview label" and "preview comment" were being treated as separate kinds of work. Resolved: different **Triggers** can map to the same **Operation Type**, and idempotency belongs to the **Operation Type**.
- It was unclear whether **Operation Type** belongs to repository configuration. Resolved: **Operation Type** is a global built-in system category, while repositories only choose which **Triggers** are enabled.
- It was unclear whether technical retries create new attempts. Resolved: retries of the same queued operation stay within the same **Runtime Environment**; a new **Runtime Environment** only starts when a new operation attempt begins.
- It was unclear what happens after `failed`. Resolved: a later domain-level retry of the same **Operation Type** starts a new **Runtime Environment** rather than reopening the failed one.
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
- It was unclear whether every queued operation must come from a trigger. Resolved: **Operation Requests** have an explicit source and may be created either from matched **Triggers** or from system-initiated actions such as automatic cleanup.
- It was unclear whether automatic cleanup of superseded preview attempts is best-effort or guaranteed with the state change. Resolved: when a preview attempt becomes `superseded`, the automatic cleanup **Operation Request** is created in the same transaction.
- It was unclear whether cleanup requests resolve their target dynamically in the worker. Resolved: cleanup-style **Operation Requests** store an immutable target **Runtime Environment** when they are created.
- It was unclear whether queued operations should depend only on live joined state. Resolved: each **Operation Request** stores its own immutable operation-intent snapshot so retries and audit do not depend on later changes to triggers, pull requests, or runtime attempts.
