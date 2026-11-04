# Error codes

Every error the API answers with, as the `code` of its RFC 9457 problem. Generated from the declared errors; regenerate with `go test ./cmd/api/bootstrap -run TestErrorCatalog -update`.

| Code | Status | Kind | Message |
|---|---|---|---|
| `account.invalid_credentials` | 401 | unauthenticated | invalid email or password |
| `account.password_hash_failed` | 500 | internal | could not process the password |
| `account.password_too_long` | 400 | invalid | the password must be at most 72 bytes long |
| `account.token_issue_failed` | 500 | internal | could not issue the session tokens |
| `auth.invalid_api_key` | 401 | unauthenticated | invalid API key |
| `auth.invalid_token` | 401 | unauthenticated | invalid or expired token |
| `auth.malformed_credentials` | 401 | unauthenticated | the authorization header must be 'Bearer &lt;token&gt;' |
| `auth.missing_credentials` | 401 | unauthenticated | the authorization header is missing |
| `events.publish_failed` | 500 | internal | could not publish the event |
| `request.body_too_large` | 413 | too_large | the request body is too large |
| `request.invalid_parameter` | 400 | invalid | a path or query parameter is invalid |
| `request.malformed_body` | 400 | invalid | the request body could not be parsed |
| `request.rate_limited` | 429 | resource_exhausted | rate limit exceeded |
| `request.validation_failed` | 400 | invalid | some fields are invalid |
| `server.canceled` | 499 | canceled | the request was canceled |
| `server.internal` | 500 | internal | an internal error occurred |
| `server.timeout` | 503 | unavailable | the request took too long, retry later |
| `user.email_taken` | 409 | conflict | a user with this email already exists |
| `user.not_found` | 404 | not_found | user not found |
