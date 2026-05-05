# PRD — HTTP Route Logging Integration

## Problem Statement

prout currently emits structured application logs for exceptional events inside handlers and workers, but it does not emit a consistent request-lifecycle log for HTTP traffic. From the operator's perspective, this makes the server harder to observe: there is no single record showing which route was hit, how long it took, what status it returned, how many bytes were written, or whether a failure came from routing, authorization, handler logic, or readiness checks. The current state also lacks a clean integration point for future route-specific enrichment, which means new logging behavior would tend to leak into individual handlers instead of remaining a reusable server capability.

## Solution

Introduce an always-on HTTP request logging middleware mounted on the root router. The middleware will live in the shared logging module and emit one structured completion log per request. It will provide a stable extension surface for future route-specific enrichment through request-scoped attributes, while preserving handler-level logs for exceptional domain events and failures. The middleware will use the existing structured logger, inherit the existing server log configuration, and align identifier naming with the project glossary by distinguishing the internal Repository identifier from GitHub's repository identifier.

## User Stories

1. As an operator, I want every HTTP request to produce one structured completion log, so that I can audit traffic without piecing together multiple records.
2. As an operator, I want request logs to include the matched route and the raw path, so that I can distinguish endpoint behavior from individual request URLs.
3. As an operator, I want request logs to include status, duration, and bytes written, so that I can quickly spot slow or failing routes.
4. As an operator, I want request logs to inherit the existing request identifier, so that I can correlate request-lifecycle logs with handler and worker logs.
5. As an operator, I want unauthorized API requests to be logged by the same middleware as successful requests, so that I can observe authentication failures at the edge.
6. As an operator, I want health and readiness requests logged through the same mechanism as other routes, so that server liveness behavior remains observable.
7. As an operator, I want request log severity to reflect the response class, so that failures stand out without requiring separate alerting logic in each handler.
8. As a developer, I want request logging owned by one reusable module, so that route observability can evolve without duplicating middleware logic.
9. As a developer, I want route-specific code to add request log attributes through a request-scoped API, so that webhook and API routes can enrich logs without emitting their own generic request records.
10. As a developer, I want the root router to be the single mounting point for request logging, so that future route groups inherit consistent behavior by default.
11. As a developer, I want handler-level logs to remain focused on exceptional domain events, so that request logging does not drown out meaningful business failures.
12. As a developer, I want the internal Repository identifier and GitHub repository identifier logged under distinct field names, so that logs do not silently change meaning between routes.
13. As a developer, I want the request logging middleware to work with recovery middleware, so that panic-driven `500` responses still produce a completion log.
14. As a developer, I want the request logging surface to avoid logging bearer tokens, headers, and bodies by default, so that the baseline is safe to leave enabled in all environments.
15. As a future contributor, I want a simple enrichment interface for request logs, so that new HTTP routes can add domain context without understanding the internals of log emission.
16. As a future contributor, I want the request logging contract to rely on stable field names, so that downstream consumers can build dashboards or filters without chasing implementation churn.
17. As a maintainer, I want request logging to reuse the existing log configuration, so that observability improves without introducing new server-level knobs before they are needed.
18. As a maintainer, I want the request logging behavior covered by focused tests, so that future changes to router wiring or middleware internals do not silently degrade observability.

## Implementation Decisions

- Build a deep request-logging module in the shared logging package that owns middleware construction, response instrumentation, and request-scoped attribute enrichment.
- Mount the request logging middleware once on the root router, after request identification and real IP extraction, and before recovery middleware.
- Emit exactly one completion log per HTTP request rather than a start/finish pair.
- Keep the request log always on in the first version and reuse the existing log level, format, and source configuration instead of adding request-specific config.
- Use a canonical base field set for request logs: method, matched route, raw path, status, duration in milliseconds, bytes written, remote IP, and user agent.
- Continue to rely on the existing contextual logger behavior to attach the request identifier automatically.
- Preserve handler-level logs for exceptional conditions and domain outcomes rather than moving business logging into the request middleware.
- Provide a request-scoped enrichment API so downstream handlers or nested middleware can append attributes before the completion log is emitted.
- Reserve `repository_id` for the internal prout Repository identifier and use `github_repository_id` when only the GitHub repository identifier is available.
- Prefer the matched route pattern for the canonical route field and keep the raw URL path as a separate field.
- Map response classes to log levels in the middleware: normal completions at info, client errors at warn, and server errors at error.
- Include health, readiness, webhook, and operator API traffic in the same request logging flow instead of introducing endpoint suppression in the first version.
- Avoid logging sensitive request contents such as authorization headers, request bodies, or response bodies in the baseline contract.

## Testing Decisions

- A good test asserts externally visible behavior: the HTTP response still behaves correctly and the emitted log record contains the expected fields and severity. Tests should not depend on private helper structure or implementation-specific control flow.
- The primary module to test is the shared request-logging module, including completion logging, status and byte capture, route extraction, level selection, and request-scoped attribute enrichment.
- Add a thin router wiring test to confirm the server mounts the middleware at the root and logs requests that fail before reaching business handlers, such as unauthorized operator API requests.
- Prefer focused `net/http/httptest`-based tests with an in-memory log sink over heavier end-to-end server startup.
- Follow the repo's existing prior art of using the standard library `testing` package and table-driven unit tests, as seen in the webhook parsing and trigger catalog tests.
- Do not add tests that merely restate the internals of helper types if the same confidence can be obtained by observing emitted logs and HTTP responses.

## Out of Scope

- Metrics, tracing spans, or OpenTelemetry integration.
- Request or response body logging.
- Logging of authorization headers, bearer tokens, or other secrets.
- Endpoint-specific skip lists, sampling policies, or separate request-log configuration knobs.
- Converting existing domain logs into generic request logs.
- Background job logging changes outside the shared contextual field naming cleanup already implied by the identifier decision.
- Multi-destination log shipping, retention policy, or dashboard work.

## Further Notes

- The logging contract is intended to be the stable integration point for future HTTP observability work; route-specific behavior should extend it, not bypass it.
- The repo currently has structured application logging and contextual request identifiers, which reduces the implementation surface to request-lifecycle capture and enrichment.
- The current codebase does not yet include dedicated HTTP middleware tests, so this work should establish that pattern in a way that future route and middleware features can reuse.
