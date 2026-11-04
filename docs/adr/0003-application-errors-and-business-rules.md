# 0003: Application errors and business rules

Date: 2026-11-02. Status: accepted.

## Context

Services returned `httperr` values, so business logic chose HTTP statuses. The same service called from a job or a queue consumer would still answer with a 409. Error codes were generic (`CONFLICT`, `NOT_FOUND`), so a client could not tell "email taken" from any other conflict without parsing the message. Rules were scattered: email normalization was written three times, and login lowercased the email but did not trim it, unlike registration.

The app was organized by layer (`dto`, `models`, `services`, `handlers`, `repositories`), so one feature lived in five packages.

## Decision

**Errors are declared once, as application errors, independent of HTTP.**

- New core package `apperr`, standard library only. An `*apperr.Error` has:
  - a `Kind`, the category that decides how each transport reports it;
  - a stable code in the form `domain.rule` (`user.email_taken`), checked when the error is declared;
  - a client-safe message;
  - optional invalid fields, metadata for the client (`retry_after_seconds`) and a cause for the log.
- Each domain declares its errors as package variables next to its rules: `var ErrEmailTaken = apperr.Conflict("user.email_taken", "...")`. Callers match with `errors.Is`, which compares codes. `WithCause`, `WithFields` and `WithMeta` return copies, so a declared error is never modified.
- Any error that is not an `*apperr.Error` is a bug nobody classified, and it becomes `server.internal`.

**Kinds and HTTP statuses** (`httperr.Status`):

| Kind | When | Status |
|---|---|---|
| `Invalid` | The input breaks a rule, whatever the state | 400 |
| `NotFound` | The thing referred to does not exist | 404 |
| `Conflict` | Clashes with existing state (duplicate, concurrent change) | 409 |
| `FailedPrecondition` | Valid input, but a business rule forbids it in the current state | 422 |
| `Unauthenticated` | Identity missing or invalid | 401 |
| `PermissionDenied` | Known caller, not allowed | 403 |
| `TooLarge` | Input over a size limit | 413 |
| `ResourceExhausted` | Quota or rate limit | 429 |
| `Unavailable` | Dependency down, deadline expired | 503 |
| `Canceled` | The caller went away | 499 |
| `Internal` | Anything else | 500 |

**Errors reach clients as RFC 9457 problem details** (`application/problem+json`):

- `type` is `about:blank` until there is a page per code.
- `title` is the status text, `status` the status, `detail` the message.
- `code` and `request_id` are extension members. Invalid fields go in `errors`, and metadata becomes extra members.

Success responses keep their envelope.

**The app is organized by feature.** A feature package holds its model, rules, errors, repository, service and handler: `internal/user` (the account record, `NormalizeEmail`, its storage) and `internal/account` (registration, login, the signed-in user). A repository speaks its domain's errors (`user.ErrNotFound`, `user.ErrEmailTaken`), so driver errors and generic sentinels never reach services. `internal/store` stays as the place for transactions across features.

## Alternatives considered

- **Keep the current envelope**, only with domain codes. It would not break clients, but it is a format of our own while RFC 9457 is the standard every client library and gateway understands.
- **Domain errors only in the app**, mapped to `httperr` there. Every new app would rebuild the same type and mapping.
- **Codes in upper case** (`EMAIL_TAKEN`). They collide across domains as the app grows.
- **Keep layer packages** and only change the errors. A feature would still be spread over five packages.

## Consequences

- This breaks the error contract. Clients reading `error`, `details` or the old codes must move to `detail`, `errors` and the new codes. The `Content-Type` of errors is now `application/problem+json`.
- Every error a client can see has a code in the code, so the list of codes can be generated or checked by a test later.
- Login now trims the email like registration does. Registration validates `email` and `password` as required, which it did not before.
- The `httperr.Error` type and its constructors are gone.
