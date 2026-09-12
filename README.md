# refactual-go

Idiomatic Go client for the [Refactual](https://app.refactual.com)
platform. One small package, standard-library HTTP, `context.Context`
first. Drop it into a microservice, a CI binary, or a Lambda; ship modern
code, structured JSON, or modernization scores in or out.

> **Preview release** (`0.x`) — the API is stable but minor versions may add
> fields before `1.0.0`. Pin exact versions with `go.mod` and watch the
> [changelog](https://github.com/refactual/refactual/releases).

## What is Refactual?

Refactual is a developer platform that turns AI agents into a working stack
for legacy modernization, document parsing, and ecommerce intelligence:

- **Refactor** — paste COBOL, Fortran, Perl, VB6, PHP, Java 7, jQuery (40+
  source languages); receive idiomatic modern code in your target language
  with a reviewed diff.
- **Repos** — point at a GitHub URL or upload a directory; receive a
  modernization score, ranked tech-debt hotspots, and Auto-Fix recipes
  with a dev-weeks effort estimate.
- **Parser** — upload PDFs or freeform text; receive structured JSON,
  field-level diffs, or a generated API schema.
- **Stores** — connect Shopify; receive a ranked AI action plan with
  dollar-impact estimates for revenue, AOV, and CAC moves.

For Go developers, this SDK is the obvious shape: a `*Client` with
resource fields (`Chat`, `Credits`, `Sessions`, ...), idiomatic
`func(ctx, *Request) (*Response, error)` method signatures, and an `Error`
type that satisfies `error` and carries the status + type code so you can
`errors.As` against it.

## Why use this SDK?

- **Zero non-stdlib runtime deps.** The whole client is `net/http`,
  `encoding/json`, and `crypto/hmac`. Easy to vendor, easy to audit.
- **`context.Context` everywhere.** Cancellation, deadlines, and request
  tracing propagate the way you expect.
- **Functional options pattern.** `refactual.New(WithAPIKey(k), WithMaxRetries(3), WithHeader("X-Tenant", t))`.
- **Typed error.** `*refactual.Error` exposes `Status` (HTTP code) and
  `Type` (e.g. `insufficient_credits`, `rate_limited`); pair with
  `errors.As`.
- **Auto retry with backoff.** 429s and 5xx retry up to twice with
  exponential backoff and `Retry-After` honoring; you don't write the loop.
- **Retries are billed once.** Every call carries an idempotency key that
  its retries reuse, so a timed-out request that is retried can't be charged
  twice. See [Retries are billed once](#retries-are-billed-once).
- **Bring your own `*http.Client`.** Wire up a custom transport, OTLP
  middleware, or service mesh sidecar via `WithHTTPClient`.
- **Webhook verification.** `refactual.VerifyWebhook(rawBody, hdr, secret, tolerance)`
  ships in the package.

## Install

```bash
go get github.com/refactual/refactual-go
```

Requires Go 1.21+.

## Quickstart

Get a key at [app.refactual.com/developer](https://app.refactual.com/developer).
Use a `rk_test_*` key in CI — it returns deterministic sandbox responses
and consumes zero credits.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/refactual/refactual-go"
)

func main() {
    client := refactual.New(
        refactual.WithAPIKey(os.Getenv("REFACTUAL_API_KEY")),
        refactual.WithMaxRetries(3),
    )

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    reply, err := client.Chat.Completions.Create(ctx, &refactual.ChatCompletionRequest{
        Model:        "claude-sonnet-4-6",
        SystemPrompt: "You are Refactor. Output only modernized Go code; no commentary.",
        Messages: []refactual.ChatMessage{{
            Role:    "user",
            Content: "function reverse(s) { return s.split('').reverse().join(''); }",
        }},
        MaxTokens: 2048,
    })
    if err != nil {
        var rerr *refactual.Error
        log.Fatalf("refactor failed: %v (status=%d type=%s)", err, rerr.Status, rerr.Type)
    }

    fmt.Println(reply.Content)               // modernized Go source
    fmt.Println(reply.Usage.CostCents)       // 0.42
}
```

Response shape is fully typed — `reply.Content string`,
`reply.Usage.InputTokens int`, etc.

## What you can build

**CI binary that posts modernization scores on PRs.** Compile a small Go
binary that runs in your GitHub Action, calls
`Chat.Completions.Create` against the diff, and writes a markdown comment
back. Single static binary, no Node.js install in your runner.

**gRPC microservice that exposes Refactor to internal teams.** Wrap the
client in your service handler. Carry trace IDs into Refactual via
`WithHeader("X-Correlation-Id", traceID)`. Use `client.Credits.Retrieve()`
for capacity-based admission control.

**Sidecar that drains a Slack queue into Refactual.** Pull `/refactor`
messages from Slack, run them through the SDK, post replies back. Use
`oauth.Token` per-workspace so each team authenticates with its own
Refactual account.

**Cron job that ingests PDFs from S3 and writes structured JSON to
Postgres.** Goroutine pool calls `Chat.Completions.Create` with a strict
JSON-schema system prompt; results land in `pgx`. `client.Usage.List`
backs your monthly cost dashboard.

## Features

- Single small file (`refactual.go`); zero non-stdlib runtime deps.
- `context.Context` on every method.
- Functional options for client config.
- Auto retry on 429 + 5xx with exponential backoff and jitter.
- One idempotency key per call, minted before the first attempt and reused
  by every retry — a retried request is billed once.
- Configurable per-request timeout via the supplied `*http.Client`.
- Per-request and per-client custom headers.
- Typed `*Error` that satisfies the `error` interface.
- OAuth 2.0 `client_credentials` grant with scoped tokens.
- Webhook signature verification (HMAC-SHA256, 5-minute tolerance).

## API surface

| Method                                    | Description                          |
|-------------------------------------------|--------------------------------------|
| `client.Chat.Completions.Create(ctx, req, opts...)` | Submit messages, receive a reply. |
| `client.Credits.Retrieve(ctx)`            | Current balance, plan, auto-reload.  |
| `client.Usage.List(ctx, days)`            | Daily token + cost rollup.           |
| `client.Sessions.List(ctx, limit)`        | Recent conversations.                |
| `client.Sessions.Retrieve(ctx, id)`       | Full transcript for one session.     |
| `client.Invoices.List(ctx, limit)`        | Invoices and credit grants.          |
| `client.Models.List(ctx)`                 | Available models.                    |
| `client.OAuth.Token(ctx, req)`            | Exchange `client_credentials` for a token. |
| `refactual.VerifyWebhook(...)`            | Validate a webhook signature.        |

Full reference: <https://app.refactual.com/developer/docs>.

## Configuration

```go
client := refactual.New(
    refactual.WithAPIKey("rk_live_..."),
    refactual.WithBaseURL("https://app.refactual.com"), // override for self-hosted
    refactual.WithMaxRetries(2),
    refactual.WithHeader("X-Correlation-Id", correlationID),
    refactual.WithHTTPClient(&http.Client{
        Timeout: 90 * time.Second,
        Transport: otelhttp.NewTransport(http.DefaultTransport),
    }),
)
```

## OAuth 2.0 (`client_credentials`)

```go
tok, err := client.OAuth.Token(ctx, &refactual.OAuthTokenRequest{
    ClientID:     os.Getenv("REFACTUAL_CLIENT_ID"),
    ClientSecret: os.Getenv("REFACTUAL_CLIENT_SECRET"),
    Scopes:       []string{"chat:write", "chat:read"},
})
if err != nil { return err }

scoped := refactual.New(refactual.WithAccessToken(tok.AccessToken))
// tok.ExpiresIn is 3600 (1 hour). Call OAuth.Token again to rotate.
```

## Webhook verification

```go
http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(r.Body)
    if err := refactual.VerifyWebhook(
        body,
        r.Header.Get("Refactual-Signature"),
        os.Getenv("WEBHOOK_SECRET"),
        0, // tolerance: 0 means default 5 minutes
    ); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    var event struct {
        Type string          `json:"type"`
        Data json.RawMessage `json:"data"`
    }
    _ = json.Unmarshal(body, &event)
    switch event.Type {
    case "chat.completed":  /* ... */
    case "credit.low":      /* ... */
    case "invoice.paid":    /* ... */
    }
    w.WriteHeader(http.StatusOK)
})
```

## Error handling

```go
import "errors"

reply, err := client.Chat.Completions.Create(ctx, req)
if err != nil {
    var rerr *refactual.Error
    if errors.As(err, &rerr) {
        switch rerr.Type {
        case "insufficient_credits": topUp()
        case "rate_limited":         backoffAndRetry()
        case "authentication_error": rotateKey()
        case refactual.ErrTypeIdempotencyInFlight:
            resubmitLater(rerr.IdempotencyKey) // same key → the same single answer
        default:                     return err
        }
    }
    return err
}
```

## Retries are billed once

Every non-GET request the client sends carries an `Idempotency-Key`: a UUID
minted before the first attempt and reused by every retry of that call —
timeouts, 429s, 5xx. The platform runs a keyed `Chat.Completions.Create`
once and remembers its answer for 24 hours, so a retry receives the stored
reply instead of starting a second, separately billed completion.
`reply.Replayed` is `true` when that happened.

If a retry lands while the original attempt is still running, the platform
answers `409 idempotency_in_flight` and the client waits for the original to
finish — polling with the same key for up to the `*http.Client` timeout
(60s by default) — then returns its result. Only if that budget also runs
out does the 409 surface as a `*refactual.Error` (`Type ==
refactual.ErrTypeIdempotencyInFlight`), with `IdempotencyKey` set so you can
resubmit the same call later and still receive the one answer. In-flight
polls don't count against `WithMaxRetries`, and cancelling the `ctx` stops
the wait.

Supply your own key when the same logical call may be issued again by a
*new* process — a queue that can deliver a job twice, a cron that reruns
after a crash:

```go
reply, err := client.Chat.Completions.Create(ctx, req,
    refactual.WithIdempotencyKey("job-"+job.ID), // any string, up to 200 characters
)
if err == nil && reply.Replayed {
    log.Println("answered from the earlier attempt — not billed again")
}
```

Keys are scoped to your API key. A call that ended in an error is remembered
under its key too, so a resubmission returns that same error; start a new
call (which mints a new key) to try again.

## MCP integration

Refactual ships an MCP server that exposes the same surface as this SDK to
Claude, Cursor, Continue, and any MCP-aware agent. The OAuth flow above is
how third-party MCP clients authenticate against a user's Refactual
account. See <https://app.refactual.com/developer/docs/mcp>.

## Versioning + support

- **Semver.** Breaking API changes bump the major version.
- **Issues:** <https://github.com/refactual/refactual/issues>
- **Email:** <support@refactual.com>
- **Status:** <https://status.refactual.com>

## License

Apache-2.0. See [`LICENSE`](./LICENSE).
