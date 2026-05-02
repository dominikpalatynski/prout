# PRD — Pull Request Conversation Comment Publishing

## Problem Statement

Toolshed has internal operator-facing audit capabilities such as **Operation Request History**, but it does not yet have one centralized way to publish standardized feedback back into the GitHub conversation thread of a **Pull Request**.

Without a dedicated module, each future caller would need to decide on its own how to choose a reusable markdown template, how to render that template safely, how to gather the GitHub target fields needed for publication, and how to call the GitHub App API. That would spread formatting rules, validation behavior, and transport details across the codebase. It would also make it harder to keep **Pull Request Conversation Comments** consistent when different **Operation Types** later need to report outcomes back to GitHub.

The immediate need is a simple, centralized interface in `githubapp` that can publish one new **Pull Request Conversation Comment** using a reusable markdown template and caller-provided typed context, while failing safely when the template input is incomplete.

## Solution

Introduce a dedicated comment-publishing capability inside `githubapp` for add-only publication of **Pull Request Conversation Comments**. The first version will expose one simple interface with a single method, `PublishComment`, that returns only an error.

The caller will remain responsible for gathering domain data and constructing the typed comment context. `githubapp` will not load data from storage or infer domain state on its own. Instead, the caller will pass a dedicated **Pull Request** comment target, a `TemplateKey` chosen from registered constants, and a typed data object prepared for rendering.

Templates will live in `githubapp` in a separate comment-template source file and will be registered through a simple `TemplateKey -> template` map. Each template will be a static markdown string with placeholders rather than a general-purpose template engine. Before any outbound GitHub request is made, the publisher will validate that the selected template exists and that all placeholders required by that template can be resolved from the provided data. If validation fails, `PublishComment` will return an error and publish nothing. Error logging will remain the caller's responsibility so the log entry can include full **Repository**, **Pull Request**, or **Operation Request** context.

This feature targets GitHub conversation feedback only. It does not redefine or extend **Operation Request History**. A future caller may use **Operation Request** or **Runtime Environment** data as input when building the typed comment context, but the published artifact remains a **Pull Request Conversation Comment** owned by the GitHub **Pull Request** thread.

## User Stories

1. As an Operator, I want Toolshed to publish a **Pull Request Conversation Comment** through one centralized interface, so that GitHub feedback stays consistent across features.
2. As an Operator, I want the comment-publishing capability to live in `githubapp`, so that GitHub transport behavior is not duplicated in worker or API code.
3. As an Operator, I want the publisher to target the main conversation thread of a **Pull Request**, so that the feedback appears in the expected GitHub location instead of becoming an inline diff review comment.
4. As an Operator, I want the first version to add a new comment each time, so that the behavior is simple and predictable.
5. As an Operator, I do not want the first version to silently edit or replace an older comment, so that I can reason about comment publication without hidden matching rules.
6. As an Operator, I want callers to choose a reusable template through a stable `TemplateKey`, so that comment selection is explicit and easy to audit.
7. As an Operator, I want all supported comment templates to be registered in one place, so that the system has a clear source of truth for reusable GitHub feedback.
8. As an Operator, I want comment templates to be written as markdown, so that published comments are readable and GitHub-native.
9. As an Operator, I want comment templates to use placeholders, so that the same template can render different **Pull Request** outcomes without manual string concatenation at each call site.
10. As an Operator, I want template rendering to fail before any network request if required data is missing, so that Toolshed never publishes a broken partial comment.
11. As an Operator, I want an unknown `TemplateKey` to fail immediately, so that misconfigured callers do not post unintended content.
12. As an Operator, I want the publisher to return an error rather than log internally, so that the caller can log one contextualized failure record with the right request and repository identifiers.
13. As an Operator, I want comment publication to use a dedicated **Pull Request** target object, so that callers do not pass storage models or unrelated fields into the GitHub adapter.
14. As an Operator, I want the target object to carry only the GitHub data needed to publish the comment, so that the interface stays small and stable.
15. As an Operator, I want the typed comment context to be prepared by the caller, so that domain orchestration stays outside `githubapp`.
16. As an Operator, I do not want `githubapp` to load **Repository**, **Pull Request**, **Operation Request**, or **Runtime Environment** data from storage, so that the package remains a GitHub integration boundary rather than a domain orchestrator.
17. As an Operator, I want template rendering to be based on typed context rather than loose unstructured maps, so that the contract between caller and template remains explicit.
18. As an Operator, I want template definitions separated from the publishing transport code, so that message wording can evolve without tangling with HTTP behavior.
19. As a future maintainer, I want a small `CommentPublisher` interface, so that other modules can depend on comment publication without coupling to the full GitHub client implementation.
20. As a future maintainer, I want the existing GitHub client to implement that interface, so that the system keeps one shared GitHub App transport implementation.
21. As a future maintainer, I want `PublishComment` to return only an error in the first version, so that the contract stays minimal until a concrete need for returned metadata appears.
22. As a future maintainer, I want validation, rendering, and outbound publication to happen in one well-bounded module, so that new callers do not recreate those steps differently.
23. As a future maintainer, I want the publishing feature to remain independent from persistence, so that no schema work is required just to add a comment.
24. As a future maintainer, I do not want the first version to persist GitHub comment identifiers, so that the initial slice stays focused on add-only publication.
25. As a future maintainer, I do not want the first version to search for or update prior comments, so that upsert semantics can be designed separately later if they become necessary.
26. As a future maintainer, I want missing-placeholder validation to guarantee that a failed render performs no outbound GitHub call, so that tests and operational behavior remain deterministic.
27. As a future maintainer, I want the template registry to be simple enough to extend with additional `TemplateKey` values, so that future comment types can be added without changing the publication contract.
28. As a future maintainer, I want the comment publisher to stay compatible with future comment contexts built from **Operation Request** or **Runtime Environment** outcomes, so that the interface survives beyond one initial caller.
29. As a future maintainer, I want the markdown rendering rules to stay intentionally simple, so that the project does not introduce a heavyweight general-purpose templating system before it is needed.
30. As a future maintainer, I want the distinction between internal **Operation Request History** and external **Pull Request Conversation Comments** to remain explicit, so that audit history and GitHub feedback do not collapse into one overloaded concept.

## Implementation Decisions

- Build one small `CommentPublisher` interface in `githubapp` with a single `PublishComment` method that returns only an error.
- Implement that interface on the existing GitHub App client rather than introducing a second outbound GitHub transport client.
- Introduce a dedicated publish input contract containing a dedicated **Pull Request** comment target DTO with only the GitHub publication fields needed by the adapter, a `TemplateKey` chosen from registered constant values, and a typed template-data object prepared by the caller.
- Use a dedicated **Pull Request** comment target DTO instead of passing storage models, `sqlc` records, or broad repository structs into the publisher.
- Keep the target anchored to the GitHub **Pull Request** conversation thread rather than to an **Operation Request**. **Operation Request** data may inform the rendered content, but the publication target remains the **Pull Request**.
- Store reusable comment templates in `githubapp` in a separate comment-template source file so message definitions are visibly distinct from HTTP transport logic.
- Register templates through a simple in-memory `TemplateKey -> template` map so lookup stays explicit and easy to extend.
- Model each template as a static markdown string with placeholders rather than using a general-purpose template engine.
- Use one simple placeholder convention for comment templates and keep the renderer intentionally narrow in scope.
- Require the caller to provide a typed data object for rendering instead of a loose `map[string]any`.
- Use a small data contract that can expose placeholder values from typed context while keeping the renderer unaware of higher-level domain storage concerns.
- Validate the selected `TemplateKey` before attempting to render or publish.
- Validate that every placeholder required by the selected template has a non-missing value before attempting any outbound GitHub request.
- If template lookup or placeholder validation fails, return an error immediately and publish nothing.
- Leave error logging to the caller so logs can include full context such as **Repository**, **Pull Request**, **Operation Request**, `TemplateKey`, and operational identifiers.
- Keep `githubapp` free of store-loading responsibilities for this feature. Callers must gather and shape all domain data before calling `PublishComment`.
- Publish comments through the GitHub App installation context already used by the package, using the repository owner, repository name, installation identifier, and **Pull Request** number supplied in the target DTO.
- Make the first version add-only. Every successful call creates one new **Pull Request Conversation Comment**.
- Do not persist GitHub comment identifiers in the first version.
- Do not implement upsert, update, delete, or comment-search behavior in the first version.
- Do not return published comment metadata in the first version. If future workflows need comment identifiers or URLs, that should be a deliberate contract expansion rather than implicit scope creep.
- Keep the design open for future growth by allowing new `TemplateKey` constants and new typed template-data types without changing the core publishing interface.

## Testing Decisions

- Good tests should assert external behavior rather than helper internals. The important questions are whether a comment can be published with valid input, whether invalid template input prevents publication, and whether the produced outbound request contains the expected rendered markdown.
- The comment-publishing module should be tested for template lookup, placeholder validation, markdown rendering, and the no-network-on-validation-failure rule.
- The GitHub publication path should be tested with focused HTTP-client stubs so the tests verify request method, endpoint shape, authentication flow integration, and request body content without relying on live GitHub calls.
- The template registry should be tested to ensure each registered `TemplateKey` resolves to a template and that missing keys fail clearly.
- The rendering contract should be tested against typed template-data implementations to verify that required placeholders are satisfied and that missing values produce errors.
- The add-only behavior should be tested so the publisher always issues a create-comment request and never attempts update semantics in this slice.
- Caller-side integration tests should verify that publication errors are returned upward for caller-controlled logging instead of being swallowed or logged internally.
- Tests should verify that the comment target DTO is sufficient for publication and that no persistence-layer dependency is required inside the publisher.
- Tests should avoid binding themselves to incidental helper decomposition inside the renderer or registry. If the external rendered markdown and publication behavior are correct, the test is doing its job.
- Prior art already exists in the repo for this style of testing, including focused GitHub client tests that stub transport behavior, package-level unit tests that validate external outputs and failure modes, and worker or API tests that prefer observable behavior over private helper call order.

## Out of Scope

- Editing, replacing, or deleting previously published **Pull Request Conversation Comments**.
- Searching GitHub for an earlier Toolshed comment.
- Persisting GitHub comment identifiers or comment URLs.
- Returning published comment metadata from `PublishComment`.
- Inline review comments on a diff instead of main-thread **Pull Request Conversation Comments**.
- Having `githubapp` load domain data from storage based on **Repository**, **Pull Request**, or **Operation Request** identifiers.
- A generalized notification or messaging framework outside GitHub comment publication.
- A rich general-purpose templating engine with control flow, functions, or arbitrary scripting.
- Automatic synchronization of internal **Operation Request History** into GitHub comments.
- Any schema change whose only purpose is to support add-only publication in this first slice.

## Further Notes

- This design deliberately separates internal audit language from external GitHub feedback. **Operation Request History** remains the durable operator-visible record inside Toolshed, while **Pull Request Conversation Comments** remain GitHub-facing messages posted back to one **Pull Request**.
- The first version is intentionally narrow. Its goal is not to solve the full lifecycle of bot-authored comments, but to create one dependable central publishing seam that future features can reuse.
- The decision to keep rendering data caller-owned is meant to preserve package boundaries: domain orchestration belongs to the caller, while `githubapp` owns template lookup, render validation, and GitHub publication.
- If a future use case requires comment replacement, deduplication, or operator-visible comment tracking, that should be handled in a separate follow-up design rather than complicating the first add-only interface.
