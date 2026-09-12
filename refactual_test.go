package refactual

// Idempotency-Key behaviour of the request core.
//
// The platform bills /v1/chat/completions once per Idempotency-Key. These
// tests pin the client half of that contract: a key is minted before the
// first attempt, reused by every retry of the same call, never sent on GET,
// surfaced on errors, and a 409 idempotency_in_flight is waited out with the
// same key rather than failed or re-issued under a new one.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type recorded struct {
	Method string
	Key    string
	HasKey bool
	Body   string
}

// script drives a fake platform: step n answers the n-th request; the last
// step repeats. A nil step hijacks and drops the connection (a network error).
type step func(w http.ResponseWriter)

func jsonStep(status int, body any, headers ...string) step {
	return func(w http.ResponseWriter) {
		for i := 0; i+1 < len(headers); i += 2 {
			w.Header().Set(headers[i], headers[i+1])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}

func newFakePlatform(t *testing.T, steps ...step) (*Client, func() []recorded) {
	t.Helper()
	var mu sync.Mutex
	var calls []recorded
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		_, has := r.Header["Idempotency-Key"]
		calls = append(calls, recorded{Method: r.Method, Key: r.Header.Get("Idempotency-Key"), HasKey: has, Body: string(b)})
		i := n
		n++
		mu.Unlock()
		if i >= len(steps) {
			i = len(steps) - 1
		}
		if steps[i] == nil {
			c, _, _ := w.(http.Hijacker).Hijack()
			_ = c.Close()
			return
		}
		steps[i](w)
	}))
	t.Cleanup(srv.Close)
	// Waits are real time.Sleeps in the client; shrink them so the suite is instant.
	oldB, oldP := backoffBase, inflightPollBase
	backoffBase, inflightPollBase = time.Millisecond, time.Millisecond
	t.Cleanup(func() { backoffBase, inflightPollBase = oldB, oldP })
	client := New(WithAPIKey("rk_test_x"), WithBaseURL(srv.URL))
	return client, func() []recorded { mu.Lock(); defer mu.Unlock(); return append([]recorded(nil), calls...) }
}

var (
	completion = map[string]any{"id": "msg_1", "model": "claude-sonnet-4-6", "environment": "live", "content": "ok",
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "cost_cents": 0.1}}
	inFlight = map[string]any{"error": map[string]any{"type": "idempotency_in_flight",
		"message": "A request with this Idempotency-Key is still running. Retry shortly to receive its result."}}
	chatReq = &ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "hi"}}}
)

func TestMintsUUIDv4BeforeFirstAttempt(t *testing.T) {
	client, calls := newFakePlatform(t, jsonStep(200, completion))
	reply, err := client.Chat.Completions.Create(context.Background(), chatReq)
	if err != nil {
		t.Fatal(err)
	}
	c := calls()
	if len(c) != 1 || !uuidV4.MatchString(c[0].Key) {
		t.Fatalf("want one POST with a UUID v4 key, got %+v", c)
	}
	if reply.Content != "ok" || reply.Replayed {
		t.Fatalf("unexpected reply %+v", reply)
	}
}

func TestNoKeyOnGet(t *testing.T) {
	client, calls := newFakePlatform(t, jsonStep(200, map[string]any{"balanceCents": 1}))
	if _, err := client.Credits.Retrieve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c := calls(); c[0].Method != "GET" || c[0].HasKey {
		t.Fatalf("GET must not carry a key: %+v", c)
	}
}

func TestSameKeyAcross5xxRetry(t *testing.T) {
	client, calls := newFakePlatform(t,
		jsonStep(503, map[string]any{"error": map[string]any{"type": "server_error", "message": "x"}}),
		jsonStep(200, completion))
	if _, err := client.Chat.Completions.Create(context.Background(), chatReq); err != nil {
		t.Fatal(err)
	}
	c := calls()
	if len(c) != 2 || c[1].Key != c[0].Key || !uuidV4.MatchString(c[0].Key) || c[1].Body != c[0].Body {
		t.Fatalf("retry must resend the same key and body: %+v", c)
	}
}

func TestSameKeyAcrossNetworkErrorRetryAndReplayedFlag(t *testing.T) {
	client, calls := newFakePlatform(t, nil, jsonStep(200, completion, "Idempotency-Replayed", "true"))
	reply, err := client.Chat.Completions.Create(context.Background(), chatReq)
	if err != nil {
		t.Fatal(err)
	}
	c := calls()
	if len(c) != 2 || c[1].Key != c[0].Key {
		t.Fatalf("retry after a dropped connection must resend the same key: %+v", c)
	}
	if !reply.Replayed || reply.Content != "ok" {
		t.Fatalf("expected the replayed answer, got %+v", reply)
	}
}

func TestCallerSuppliedKey(t *testing.T) {
	client, calls := newFakePlatform(t, jsonStep(200, completion))
	ctx := context.Background()
	if _, err := client.Chat.Completions.Create(ctx, chatReq, WithIdempotencyKey("job-42")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Refactor.Code(ctx, &RefactorCodeRequest{SourceLang: "COBOL", TargetLang: "Go", Code: "x"}, WithIdempotencyKey("rf-7")); err != nil {
		t.Fatal(err)
	}
	c := calls()
	if c[0].Key != "job-42" || c[1].Key != "rf-7" {
		t.Fatalf("caller keys must be sent verbatim: %+v", c)
	}
}

func TestWaitsOutInFlightWithSameKeyAndReturnsReplay(t *testing.T) {
	client, calls := newFakePlatform(t, jsonStep(409, inFlight), jsonStep(409, inFlight),
		jsonStep(200, completion, "Idempotency-Replayed", "true"))
	client.retries = 0 // polls are not retries
	reply, err := client.Chat.Completions.Create(context.Background(), chatReq)
	if err != nil {
		t.Fatal(err)
	}
	c := calls()
	if len(c) != 3 || c[1].Key != c[0].Key || c[2].Key != c[0].Key {
		t.Fatalf("in-flight polls must reuse the key: %+v", c)
	}
	if !reply.Replayed || reply.Content != "ok" {
		t.Fatalf("expected the replayed answer, got %+v", reply)
	}
}

func TestGivesUpOnInFlightAfterBudgetAndSurfacesKey(t *testing.T) {
	client, calls := newFakePlatform(t, jsonStep(409, inFlight))
	client.httpClient.Timeout = 20 * time.Millisecond // the in-flight wait budget
	_, err := client.Chat.Completions.Create(context.Background(), chatReq)
	var rerr *Error
	if !errors.As(err, &rerr) || rerr.Status != 409 || rerr.Type != ErrTypeIdempotencyInFlight {
		t.Fatalf("want 409 idempotency_in_flight, got %v", err)
	}
	c := calls()
	if rerr.IdempotencyKey == "" || rerr.IdempotencyKey != c[0].Key {
		t.Fatalf("error must carry the key that was sent: %+v vs %+v", rerr, c[0])
	}
	if len(c) < 3 {
		t.Fatalf("expected several polls inside the budget, got %d", len(c))
	}
	for _, x := range c {
		if x.Key != c[0].Key {
			t.Fatalf("key rotated during polling: %+v", c)
		}
	}
}

func TestStopsRetryingAReplayed5xx(t *testing.T) {
	client, calls := newFakePlatform(t,
		jsonStep(502, map[string]any{"error": map[string]any{"type": "upstream_error", "message": "x"}}, "Idempotency-Replayed", "true"))
	_, err := client.Chat.Completions.Create(context.Background(), chatReq)
	var rerr *Error
	if !errors.As(err, &rerr) || rerr.Status != 502 {
		t.Fatalf("want the replayed 502, got %v", err)
	}
	if len(calls()) != 1 {
		t.Fatalf("a replayed 5xx is final for its key; got %d attempts", len(calls()))
	}
}

func TestErrorCarriesKey(t *testing.T) {
	client, calls := newFakePlatform(t, jsonStep(402, map[string]any{"error": map[string]any{"type": "insufficient_credits", "message": "x"}}))
	_, err := client.Chat.Completions.Create(context.Background(), chatReq)
	var rerr *Error
	if !errors.As(err, &rerr) || rerr.Type != "insufficient_credits" || rerr.IdempotencyKey != calls()[0].Key {
		t.Fatalf("API errors must carry the key: %v", err)
	}
}

func TestContextCancelStopsTheWait(t *testing.T) {
	client, _ := newFakePlatform(t, jsonStep(409, inFlight))
	inflightPollBase = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := client.Chat.Completions.Create(ctx, chatReq)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want context deadline, got %v", err)
	}
}
