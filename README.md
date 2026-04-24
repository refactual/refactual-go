# refactual-go

Official Refactual REST API client for Go.

> **Preview release** (`0.x`) — APIs may shift before `1.0.0`. Pin
> exact versions in production and follow the
> [changelog](https://github.com/refactual/refactual/releases) for
> breaking notes.

```bash
go get github.com/refactual/refactual-go
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/refactual/refactual-go"
)

func main() {
    client := refactual.New(refactual.WithAPIKey(os.Getenv("REFACTUAL_API_KEY")))
    reply, err := client.Chat.Completions.Create(context.Background(), &refactual.ChatCompletionRequest{
        Model: "claude-sonnet-4-6",
        Messages: []refactual.ChatMessage{{Role: "user", Content: "Explain async/await"}},
    })
    if err != nil { panic(err) }
    fmt.Println(reply.Content)
}
```

## OAuth 2.0 (client_credentials)

```go
tok, err := client.OAuth.Token(ctx, &refactual.OAuthTokenRequest{
    ClientID:     os.Getenv("REFACTUAL_CLIENT_ID"),
    ClientSecret: os.Getenv("REFACTUAL_CLIENT_SECRET"),
    Scopes:       []string{"chat:write", "chat:read"},
})
```

## Webhook verification

```go
if err := refactual.VerifyWebhook(rawBody, r.Header.Get("Refactual-Signature"), secret, 0); err != nil {
    http.Error(w, err.Error(), 400); return
}
```
