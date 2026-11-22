# AGENTS.md

# Rombay Gateway — AI Agent Instructions

## 1. Project Overview

`rombay-gateway` is the merchant-facing API gateway for the Rombay payment platform.

It works alongside `rombay-core`, which owns the core payment business logic and transaction lifecycle.

The gateway is responsible for:
- Merchant authentication and authorization.
- Merchant-level rate limiting.
- Request idempotency.
- Routing requests to `rombay-core`.
- Consuming payment status events.
- Dispatching merchant webhooks.
- API observability and operational monitoring.

**Important:** `rombay-core` is an existing sibling repository. Its implementation and API contracts must be inspected before implementing any integration.

## 2. Workspace Structure

Expected workspace:

```text
rombay/
├── rombay-core/
└── rombay-gateway/
```

Repository boundaries:
- `rombay-gateway/`: Current repository. The agent may modify files here.
- `rombay-core/`: Existing dependency and integration reference. Treat as read-only unless explicitly instructed otherwise.

Do not modify `rombay-core` as part of gateway implementation without explicit approval.

## 3. Source of Truth and Integration Rules

Before implementing or changing an integration, inspect `rombay-core`.

Use its existing implementation to determine:

- API endpoint paths and HTTP methods.
- Request and response DTOs.
- Authentication and authorization requirements.
- Merchant identification and ownership rules.
- Error response structure and HTTP status codes.
- Payment status definitions and transitions.
- Idempotency behavior.
- Kafka topics, event schemas, and serialization format.
- Webhook configuration APIs.
- Internal service communication conventions.

### Contract rules

1. Never invent an API contract when an equivalent implementation already exists in `rombay-core`.
2. Do not assume field names, enum values, or status transitions.
3. Reuse existing DTO definitions conceptually, but avoid creating a direct build-time dependency between Go and Java.
4. Document any necessary Go representation of Java DTOs.
5. If a contract is missing or ambiguous, document the uncertainty and ask for clarification.
6. Do not silently introduce breaking changes to the core integration.
7. Keep gateway-specific concerns separate from core business logic.

If an API contract changes in `rombay-core`, assess its impact on `rombay-gateway`.

## 4. Architecture Boundaries

### rombay-gateway owns

- HTTP entry point for merchants.
- JWT validation.
- Merchant-level rate limiting.
- Idempotency request handling.
- Reverse proxy and request forwarding.
- Kafka event consumption.
- Durable webhook delivery scheduling.
- Webhook HMAC signing.
- Retry and dead-letter handling.
- Gateway-specific metrics and logs.

### rombay-core owns

- Payment business logic.
- Transaction creation and lifecycle.
- Payment validation and processing.
- Merchant business rules.
- Transaction persistence.
- Payment status transitions.
- Core-side enforcement of transaction idempotency.

### Other services

Ledger and other platform services own their respective domains. Do not duplicate their responsibilities inside the gateway.

## 5. Technology Stack

- Go 1.23+
- `net/http`
- Redis
- Kafka
- Prometheus
- `log/slog`
- Docker
- Go standard testing package

Prefer standard-library functionality where practical.

## 6. Code Organization

Keep the implementation modular and maintainable.

Suggested structure:

```text
rombay-gateway/
├── cmd/
│   └── gateway/
│       └── main.go
├── internal/
│   ├── config/
│   ├── auth/
│   ├── middleware/
│   ├── ratelimit/
│   ├── idempotency/
│   ├── proxy/
│   ├── coreclient/
│   ├── events/
│   ├── webhook/
│   ├── observability/
│   └── server/
├── deployments/
├── configs/
├── tests/
├── go.mod
├── go.sum
├── AGENTS.md
└── README.md
```

Adjust the structure if the existing repository or integration requirements justify a different approach.

## 7. Core API Integration

Create a dedicated `coreclient` package.

Responsibilities:
- Communicate with `rombay-core`.
- Apply configured connection and response timeouts.
- Propagate request IDs and supported correlation headers.
- Forward authenticated merchant context using a trusted internal mechanism.
- Map core responses and errors without losing relevant information.
- Handle connection failures and upstream timeouts.

Do not trust merchant identity supplied through arbitrary client headers.

The gateway must establish merchant identity through validated authentication and propagate it using the internal contract supported by `rombay-core`.

Do not create a second implementation of core payment business logic.

## 8. Authentication and Authorization

- Validate JWT signatures and claims.
- Use the signing algorithm configured by the actual platform contract.
- Validate issuer, audience, expiration, and other required claims.
- Extract merchant identity from trusted claims.
- Reject malformed or expired tokens.
- Never log JWTs or secrets.
- Do not assume HS256 if the core implementation uses another authentication mechanism.

## 9. Rate Limiting

Use Redis for distributed merchant-level rate limiting.

Requirements:
- Atomic operations.
- Configurable limits.
- Merchant isolation.
- Appropriate HTTP 429 responses.
- Optional `Retry-After` support.
- Avoid unbounded Redis keys.

Rate-limit behavior should be configurable rather than hardcoded.

## 10. Idempotency

Support the `Idempotency-Key` request header for payment creation.

Requirements:
- Scope keys by merchant.
- Calculate a deterministic request fingerprint.
- Same key and same payload should return the original result.
- Same key with a different payload should return HTTP 422.
- Concurrent requests must not create duplicate operations.
- Handle pending and completed states safely.
- Implement expiration and recovery for abandoned pending requests.

**Critical:** Gateway-level idempotency is not sufficient.

Forward the idempotency key to `rombay-core` and ensure the core provides durable idempotency enforcement. Do not claim exactly-once payment execution based only on Redis.

## 11. Reverse Proxy

The gateway must forward payment requests to `rombay-core`.

Requirements:
- Preserve the intended HTTP method and request body.
- Forward supported headers.
- Propagate request and correlation IDs.
- Remove untrusted internal identity headers.
- Apply upstream timeouts.
- Handle upstream errors consistently.
- Avoid leaking internal infrastructure details.

Do not expose internal core endpoints through unrestricted proxying.

## 12. Kafka Event Consumer

The gateway consumes payment status events produced by the core or the platform's configured event publisher.

Before implementation, inspect the actual Kafka configuration and event contract.

Requirements:
- Use the correct topic names.
- Follow the existing serialization format.
- Validate event structure.
- Deduplicate using the event identifier when available.
- Persist webhook delivery work before acknowledging the event.
- Handle consumer shutdown gracefully.
- Support retryable and non-retryable processing errors.

Do not acknowledge an event before its required processing is durably recorded.

## 13. Webhook Dispatcher

Responsibilities:
- Retrieve merchant webhook configuration through the actual core contract.
- Dispatch payment events to configured merchant endpoints.
- Sign payloads using HMAC-SHA256 if required by the platform contract.
- Include a timestamp and event identifier where supported.
- Implement delivery timeout and response handling.
- Persist delivery attempts and statuses.

Default retry schedule:

1. 1 minute
2. 5 minutes
3. 30 minutes
4. 2 hours

After exhausting retries, move the delivery to a dead-letter state.

Use durable storage for scheduled delivery jobs. Redis may support coordination, but do not assume volatile Redis data is sufficient for reliable webhook delivery.

### Webhook security

- Validate destination URLs.
- Prevent SSRF to private, loopback, link-local, and internal addresses.
- Consider DNS rebinding.
- Do not blindly follow redirects.
- Never log signing secrets.
- Apply request timeouts and payload-size limits.

Merchant webhook delivery is at-least-once. Consumers should be able to deduplicate events using a stable event identifier.

## 14. API Endpoints

The initial gateway endpoints include:

- `POST /payments`
- `GET /payments/{id}`
- `GET /healthz`
- `GET /metrics`

Health and metrics endpoints must not require merchant JWT authentication.

Restrict metrics exposure to trusted monitoring infrastructure.

Add readiness checks separately from liveness checks where appropriate.

The final payment request and response contracts must match `rombay-core`.

## 15. Observability

Use:
- `log/slog` for structured logging.
- Prometheus for metrics.
- Request IDs and correlation IDs.
- Upstream latency metrics.
- Redis operation metrics.
- Kafka consumer lag and processing metrics.
- Webhook delivery success/failure metrics.

Avoid high-cardinality metric labels such as transaction IDs, merchant IDs, and request IDs.

Never log:
- Authorization headers.
- JWT secrets.
- Webhook signing secrets.
- Sensitive payment information.

## 16. Configuration

Use environment variables.

Initial configuration may include:

- `HTTP_PORT`
- `CORE_URL`
- `REDIS_ADDR`
- `KAFKA_BROKERS`
- `JWT_SECRET`

Add further configuration only when justified by the actual integration.

Never commit production credentials or secrets.

Validate required configuration during startup.

## 17. Testing Requirements

Write tests for:
- Configuration validation.
- JWT validation.
- Rate limiting.
- Idempotency and concurrent requests.
- Core client behavior.
- Proxy behavior.
- Kafka event processing.
- Webhook signatures.
- Webhook retries.
- SSRF protection.
- Health and readiness endpoints.

Use mocks for unit tests and integration tests with real Redis/Kafka-compatible infrastructure where appropriate.

Run:

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

Do not claim a feature is production-ready without appropriate tests.

## 18. Engineering Standards

- Follow idiomatic Go.
- Prefer small, focused packages.
- Use dependency injection where it improves testability.
- Pass `context.Context` through I/O operations.
- Handle errors explicitly.
- Avoid global mutable state.
- Implement graceful shutdown.
- Apply sensible HTTP timeouts.
- Avoid unnecessary abstractions and dependencies.
- Document non-obvious architectural decisions.

## 19. Development Workflow

Before implementing a feature:

1. Inspect the existing gateway code.
2. Inspect the relevant `rombay-core` implementation.
3. Identify the integration contract.
4. Explain the proposed approach.
5. Implement incrementally.
6. Add or update tests.
7. Run formatting, vetting, and tests.
8. Update documentation.

Keep changes scoped to the current repository unless explicitly authorized.

## 20. AI Agent Behavior

Act as a senior Go backend engineer working on a payment platform.

Prioritize:
- Correctness.
- Security.
- Consistency with `rombay-core`.
- Reliability of payment-related operations.
- Maintainability.
- Testability.

Do not:
- Invent integration contracts.
- Duplicate core business logic.
- Change `rombay-core` without permission.
- Implement the entire platform in one step.
- Hide assumptions.
- Claim exactly-once delivery or execution without a valid architectural guarantee.

When information is missing, explain what is missing and propose the safest next step.