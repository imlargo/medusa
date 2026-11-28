# 0008: The API contract is built on Huma

Date: 2026-11-28. Status: accepted.

## Context

A handler received its JSON body already typed and validated, and nothing else: path, query and headers were read by hand. The OpenAPI document came from swag comments above each handler, which nothing ties to the code, and it was Swagger 2.0. Success answers were wrapped in `{success, message, data}` while errors, since ADR 0003, were not wrapped at all.

Medusa's model is FastAPI: the types of a route are the single source of truth for binding, validation and documentation. imlargo chose Huma for it (D3) and no envelope (D8).

## Decision

A battery, `api`, wraps Huma and keeps Gin as the router.

- **A handler is `func(ctx context.Context, in *Input) (*Resource, error)`.** The input's tags say where each value comes from (path, query, header, and a `Body` field) and what is valid. Huma binds and validates before the handler runs and builds an OpenAPI 3.1 document from the same types at startup. There is nothing to generate and nothing to drift.
- **The resource is the answer.** A handler returns it directly and it is sent as it is: no envelope, and none of Huma's `$schema` members. The status says how it went: 200 by default, 204 for a handler that returns `api.Empty`, anything else with `api.Status`. A list answers `{items, next_cursor}` through `api.List`.
- **One error vocabulary.** A handler's error is rendered as its `apperr` problem, as `medusa.WriteError` does. What Huma refuses itself is mapped to the errors Medusa already declares: failed validation is `request.validation_failed` with status 400 and fields named as the client sent them, an unreadable body is `request.malformed_body`. A client cannot tell which layer refused it.
- **Routes go on Gin groups.** One document covers routes behind different middleware, which Huma's own Gin adapter cannot do: it builds one API per group. The battery has its own adapter, about thirty lines, that puts each operation on the Gin group it was declared for. Gin middleware (auth, rate limits) keeps working unchanged, and plain Gin handlers live on the same groups.
- **Security is declared once.** `api.Secured(scheme, middleware)` both installs the middleware that guards a group and documents the scheme. `api.Scopes(guard, scopes...)` does the same for one route. `api` does not import `auth`: it receives the middleware.
- **A body field the type does not have is refused**, which is Huma's default and is kept.
- **Escape hatches.** An output written the way Huma reads one (a `Body`, a `Status` or a header field) is passed through, for redirects and response headers. `RawBody []byte` in an input gives the exact bytes, for signed webhooks. `api.Document` lists a route a plain Gin handler serves, such as a stream.

Routes written with `medusa.Handle` keep working, on the same groups. They are not in the document and they keep the envelope.

## Alternatives considered

- **swag v2.** Still comments that can say anything.
- **oapi-codegen** (the document first, code generated from it). Two artifacts to keep in step, and a build step, where here there is one source.
- **Extending `medusa.Handle`** to bind path and query. The document would still need its own source.
- **Huma's Gin adapter as it is.** One OpenAPI document per Gin group, so an API with a public and a protected group has two.
- **Keeping the envelope.** Every schema in the document would be nested under `data`, for a `success` flag the status already gives.
- **Huma's 422 for validation.** Medusa already answers 400 for invalid input everywhere else, and 422 means a business rule refused a valid request (ADR 0003). Two statuses for one mistake would be the contract leaking its implementation.

## Consequences

- Huma is a dependency of the battery, with one main maintainer. What ties an application to it is the tags on input types and the handler signature, which is plain Go; the battery's own surface is small enough to reimplement on another generator.
- Validation tags are Huma's (`minLength`, `format`, `enum`), not Gin's `binding`.
- Huma's error constructors are process-wide, so the first `api.New` in a process sets them for every Huma API in it.
- The success envelope is gone from typed routes: a client of the example app reads `user` and `tokens` at the top of the body.
- A 406 or a 415 from content negotiation answers a problem with the codes `request.not_acceptable` and `request.unsupported_media_type`, which have no `apperr` kind.
- Not built yet: stored results for `Idempotency-Key` (the header is documented with `api.Idempotent`), and several documents for several audiences (tags and `Hidden` for now).
