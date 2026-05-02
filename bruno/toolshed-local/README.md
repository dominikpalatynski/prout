Bruno collection for the phase 4 Toolshed HTTP surface.

Open `bruno/toolshed-local` as a collection in Bruno, select the `local` environment, and update the placeholder values in `environments/local.bru`.

Webhook requests support two modes:
- Preferred: switch the collection to Bruno Developer Mode so the pre-request script can compute `X-Hub-Signature-256` from `webhookSecret`.
- Fallback: stay in Safe Mode and set `webhookSignature` in `environments/local.bru` to a precomputed hex digest without the `sha256=` prefix.

The collection persists `repositoryId`, `eventFamilyKey`, runtime-settings values, repository environment-variable values, `repositoryTriggerId`, `webhookEventId`, `operationRequestId`, and `runtimeEnvironmentId` back into the environment after successful requests so the follow-up requests keep working without manual edits.
