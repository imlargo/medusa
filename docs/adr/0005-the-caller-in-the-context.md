# 0005: The caller is a Principal in the context

Date: 2026-11-28. Status: accepted.

## Context

`auth.SubjectFrom` read the subject from the Gin context, so code that receives a `context.Context` (services, repositories, jobs) could not see who was calling. There were no scopes and no tenant, and a route accepted one kind of credential.

Butter's backend, the first consumer, shows what a general answer has to hold. A survey of its authorization (2026-11-28) found:

- Callers that are not users: a worker with its own tokens, a partner workflow with a shared secret, internal services with API keys.
- No tenant in the request. A user's reach is worked out per resource, from ownership and assignments in the database.
- Facts that are checked again on every call: a subscription, a grant that can be revoked, a role.
- Services that take the user as an argument in 142 methods.
- Jobs that run with no caller at all.

## Decision

`auth` carries a `Principal` in the request's `context.Context`: a kind (user or service), a subject, an optional tenant, scopes, and how it authenticated.

- **It holds what authentication established, and nothing that has to be looked up to be trusted.** Roles, plans and per-resource permissions stay where the application checks them.
- **Authenticators are tried in order.** `Middleware(tokens, apiKeys)` lets users and services call the same routes. An authenticator answers "not my kind of credential" to pass to the next; a credential that is present and wrong ends the request.
- **The tenant and the scopes can be set per request**, by `Enrich`, from what the application reads at that moment: a header or the path, checked against memberships. A token may also carry them (`WithScopes`, `WithTenant`) for what does not change while it lives.
- **`Require(scopes...)` guards a route.** Scopes are the application's own words; Medusa defines none.
- **The subject is a string**, because a client ID and a user ID share the field. `SubjectFrom[S]` still returns it typed for applications keyed by `int64` or `string`.
- **API keys are one per client**, stored hashed and looked up by hash through a function the application provides.
- **A job runs as whoever caused it.** The queue stores the principal's kind, subject and tenant with the job (`Principal.Metadata`, `queue.WithMetadata`), and the worker puts them back in the handler's context. Scopes are not carried: they say what a request was allowed when it was made, and work that runs later decides again.

## Alternatives considered

- **A generic `Principal[S]`.** Typed subjects everywhere, but every function that reads the caller needs the type parameter, and a service principal has a different ID type than a user.
- **A tenant fixed in the token.** Wrong for Butter, where one user acts across workspaces and reach is per resource. The token can still carry one for applications where it is fixed.
- **Roles and permissions in Medusa** (Casbin, OpenFGA built in). Every application's model differs; a wrong shared one is worse than none. They remain recipes on top of `Enrich` and `Require`.
- **A map of arbitrary claims on the principal.** It becomes the place where everything is put. An application that needs more (a device binding, a plan) adds its own context value.
- **Carrying scopes into jobs.** A job that runs an hour later with the scopes of the request would act on a permission that may be gone.

## Consequences

- Breaking in `auth`: a request with an `Authorization` scheme other than Bearer now answers `auth.missing_credentials` instead of `auth.malformed_credentials`, since another authenticator may read that scheme.
- A handler may pass its own `ctx` to `PrincipalFrom` and `SubjectFrom`; they find the request's context inside it.
- Refresh tokens are not tokens a route accepts: they are opaque values kept by the application's store and rotated by `auth.Refresher` (D1), so one can never be used as an access token and a session can be revoked.
- Not built yet: password hashing and policy (D2), OIDC logins (D6), signed callbacks between services, and a session authenticator for cookies.
