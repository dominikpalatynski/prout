# Event Family Registry: Working Agreements

This document captures the working agreements for implementing the automation registry in prout.
It does not replace the PRD or the ADR. Its purpose is to explain the model, the responsibilities, and how to read the DSL.

Related documents:
- [event_family_registry_prd.md](./event_family_registry_prd.md)
- [0001-event-family-automation-registry.md](../../docs/adr/0001-event-family-automation-registry.md)

## Agreed Direction

We are keeping one central registry, but we do not interpret that as "one giant switch" or "one giant file."

The registry should be the single source of truth for built-in automation and should describe three levels of the system:
- `Event Family`
- `Trigger Type`
- `Operation Type`

The target flow should be easy to read:

`raw GitHub webhook -> Event Family -> Trigger Type -> Operation Type -> Operation Handler`

## What Is What

### Event Family

`Event Family` is responsible for system input.

Its responsibilities are:
- recognize which GitHub webhook has just arrived
- verify whether that event type is supported
- normalize the payload into the shared `webhook.NormalizedEvent`

Question answered by `Event Family`:

"What kind of webhook is this, and how do we turn it into the shared format used by the rest of the system?"

### Trigger Type

`Trigger Type` is responsible for the business rule.

Its responsibilities are:
- name the business preset in a way that is understandable to both operators and developers
- check whether the normalized event satisfies the trigger condition
- point to exactly one `Operation Type`

Question answered by `Trigger Type`:

"For this normalized event, should we start this specific automation?"

### Operation Type

`Operation Type` is responsible for technical execution.

Its responsibilities are:
- name the kind of work performed by the system
- point to a runtime environment type
- define the initial workflow step
- build the snapshot used for retry and audit
- point to one explicit `Operation Handler`

Question answered by `Operation Type`:

"If the trigger matched, what exactly should the system do now?"

### Operation Handler

`Operation Handler` is the technical entry point for executing one operation.

Its responsibilities are:
- execute the workflow for a given `Operation Type`
- use the data frozen in the snapshot
- avoid making routing decisions that belong earlier in the registry

## Why the DSL Structure Looks Like This

Agreed DSL version:

```go
var RegistryDefinitions = automation.Definitions{
	Operations: []automation.OperationDefinition{
		{
			Key:                    "preview-start",
			Name:                   "Preview Start",
			RuntimeEnvironmentType: "preview",
			InitialStep:            "resolve_target",
			BuildSnapshot:          BuildPreviewStartSnapshot,
			Handle:                 HandlePreviewStart,
		},
	},
	EventFamilies: []automation.EventFamilyDefinition{
		{
			Key:         "pull-request-opened",
			Name:        "Pull Request Opened",
			Description: "Pull request opened webhooks",
			Recognizes: []automation.GithubEventPattern{
				{Event: "pull_request", Action: "opened"},
			},
			Normalize: NormalizePullRequestOpened,
			TriggerTypes: []automation.TriggerTypeDefinition{
				{
					Key:             "preview_on_pull_request_opened",
					Name:            "Preview on pull request opened",
					Description:     "Creates a preview when a pull request is opened",
					Match:           MatchAlways("pull_request_opened_matched"),
					StartsOperation: "preview-start",
				},
			},
		},
	},
}
```

### Why `Operations` Are Top-Level

`Operation Type` is declared once because many different triggers can lead to the same operation.

Example:
- pull request opened
- label `preview`
- comment `/preview`

All of these inputs may end in the same `Operation Type`: `preview-start`.

If `Operation Type` were declared separately inside each trigger, we would duplicate:
- the handler
- the runtime environment type
- the initial step
- the snapshot semantics

### Why `TriggerTypes` Are Nested Under `EventFamily`

`Trigger Type` only makes sense in the context of a specific event family.

Example:
- a trigger like `preview_on_comment_preview` makes sense for a comment event
- it does not make sense for `pull_request.opened`

Because of that, authoring should read like this:
- this event family recognizes this webhook
- it normalizes it in this way
- and it exposes these trigger types

### Why the Repository Does Not Store Matcher Config

The agreement is that a repository-level trigger is not its own rule.
It is only enablement of a built-in preset.

The repository should say:

"enable trigger type `preview_on_comment_preview`"

The repository should not be the source of truth for:
- `event_family`
- `identity_key`
- matcher config
- operation mapping

All of that belongs to the code-level registry.

## How to Read This DSL

The simplest way to read it:

- `Operations`: what the system can execute
- `EventFamilies`: which webhooks the system can accept
- `TriggerTypes`: which built-in business rules we support for a given event

Reading one definition should look like this:

1. GitHub sends a webhook.
2. `Event Family` recognizes its type.
3. `Event Family` normalizes the payload.
4. `Trigger Type` checks whether the conditions match.
5. `Trigger Type` points to an `Operation Type`.
6. The worker executes the `Operation Handler` for that operation.

## Example: Comment `/preview`

Flow for a `/preview` comment:

1. GitHub sends `issue_comment` with `action=created`.
2. The registry resolves the `Event Family` for a pull request comment.
3. `NormalizePullRequestComment` builds the shared `webhook.NormalizedEvent`.
4. The trigger `preview_on_comment_preview` checks whether `CommentFirstLine == "/preview"`.
5. If yes, the trigger points to `StartsOperation: "preview-start"`.
6. The system creates an `Operation Request`.
7. `BuildPreviewStartSnapshot` freezes the data for retry and audit.
8. `HandlePreviewStart` executes the actual preview logic.

## Working Agreements for DX

The registry should be designed for Developer Experience.

This means:
- definitions should read like data plus small named functions
- no clever fluent API
- no hidden registration through `init()`
- no YAML or JSON DSL at this stage
- no splitting the shared event model into a large hierarchy of payload-specific event types

A new person in the project should be able to answer three questions without searching through the whole repository:
- which webhook do we support
- which trigger type comes from it
- which operation handler executes it in the end

## Initial Implementation Rules

During implementation, we will follow these rules:

- `Event Family` owns routing and normalization
- `Trigger Type` owns matching and operation selection
- `Operation Type` owns execution metadata and handler binding
- `Repository Trigger` is only repository-specific enablement of one built-in trigger type
- one repository can have at most one trigger of a given trigger type
- the registry is the code-level source of truth
- the database is not the source of truth for built-in trigger semantics

## What This Approach Is Not Trying to Do

This approach is not trying to:
- introduce user-defined triggers
- build a plugin system for automation
- support arbitrary per-repository matcher configuration
- replace the PRD or the ADR

Its purpose is only to make built-in automation:
- declarative
- explicit
- easy to extend
- easy to understand for someone who does not know the system yet
