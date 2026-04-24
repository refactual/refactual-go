// Package refactual is the official Refactual REST API client for Go.
//
// Example:
//
//	client := refactual.New(refactual.WithAPIKey(os.Getenv("REFACTUAL_API_KEY")))
//	reply, err := client.Chat.Completions.Create(ctx, &refactual.ChatCompletionRequest{
//		Model: "claude-sonnet-4-6",
//		Messages: []refactual.ChatMessage{{Role: "user", Content: "Hello"}},
//	})
//
// OpenAPI-generated types emitted by `npm run build-sdks` live in the
// `generated/` subpackage and can be imported for advanced use.
package refactual

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	Version        = "1.0.0"
	DefaultBaseURL = "https://app.refactual.com"
)

// Client is the top-level API client. Create one and reuse it.
type Client struct {
	Chat     *ChatResource
	Credits  *CreditsResource
	Usage    *UsageResource
	Sessions *SessionsResource
	Invoices *InvoicesResource
	Models   *ModelsResource
	OAuth    *OAuthResource

	baseURL    string
	token      string
	httpClient *http.Client
	headers    map[string]string
	retries    int
}

// Option configures a Client.
type Option func(*Client)

func WithAPIKey(k string) Option         { return func(c *Client) { c.token = k } }
func WithAccessToken(t string) Option    { return func(c *Client) { c.token = t } }
func WithBaseURL(u string) Option        { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }
func WithMaxRetries(n int) Option        { return func(c *Client) { c.retries = n } }
func WithHeader(k, v string) Option {
	return func(c *Client) {
		if c.headers == nil {
			c.headers = map[string]string{}
		}
		c.headers[k] = v
	}
}

// New constructs a client. At minimum, supply WithAPIKey or WithAccessToken.
func New(opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		retries:    2,
	}
	for _, o := range opts {
		o(c)
	}
	c.Chat = &ChatResource{c: c, Completions: &ChatCompletionsResource{c: c}}
	c.Credits = &CreditsResource{c: c}
	c.Usage = &UsageResource{c: c}
	c.Sessions = &SessionsResource{c: c}
	c.Invoices = &InvoicesResource{c: c}
	c.Models = &ModelsResource{c: c}
	c.OAuth = &OAuthResource{c: c}
	return c
}

// Error is returned on non-2xx responses.
type Error struct {
	Status  int    `json:"-"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("refactual: %s (status %d, type %s)", e.Message, e.Status, e.Type) }

type errorEnvelope struct {
	Error Error `json:"error"`
}

func (c *Client) request(ctx context.Context, method, path string, body any, query map[string]any, out any) error {
	if c.token == "" {
		return errors.New("refactual: token required (WithAPIKey or WithAccessToken)")
	}
	u := c.baseURL + path
	if len(query) > 0 {
		v := url.Values{}
		for k, x := range query {
			if x == nil {
				continue
			}
			v.Set(k, fmt.Sprint(x))
		}
		if s := v.Encode(); s != "" {
			u += "?" + s
		}
	}
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = b
	}

	var resp *http.Response
	var reqErr error
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("User-Agent", "refactual-go/"+Version)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		resp, reqErr = c.httpClient.Do(req)
		if reqErr != nil {
			if attempt < c.retries {
				time.Sleep(backoff(attempt + 1))
				continue
			}
			return reqErr
		}
		if resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
			if attempt < c.retries {
				ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
				_ = resp.Body.Close()
				if ra > 0 {
					time.Sleep(time.Duration(ra) * time.Second)
				} else {
					time.Sleep(backoff(attempt + 1))
				}
				continue
			}
		}
		break
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var env errorEnvelope
		_ = json.Unmarshal(data, &env)
		env.Error.Status = resp.StatusCode
		if env.Error.Message == "" {
			env.Error.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		if env.Error.Type == "" {
			env.Error.Type = "api_error"
		}
		return &env.Error
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func backoff(attempt int) time.Duration {
	base := math.Pow(2, float64(attempt)) * 250
	jitter := rand.Intn(250)
	return time.Duration(int(base)+jitter) * time.Millisecond
}

// ── Resources & models ───────────────────────────────────────────

type ChatResource struct {
	c           *Client
	Completions *ChatCompletionsResource
}

type ChatCompletionsResource struct{ c *Client }
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ChatCompletionRequest struct {
	Messages     []ChatMessage `json:"messages"`
	Model        string        `json:"model,omitempty"`
	SystemPrompt string        `json:"systemPrompt,omitempty"`
	MaxTokens    int           `json:"maxTokens,omitempty"`
}
type ChatCompletion struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	Environment string `json:"environment"`
	Content     string `json:"content"`
	Usage       struct {
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		CostCents    float64 `json:"cost_cents"`
	} `json:"usage"`
	Note string `json:"note,omitempty"`
}

func (r *ChatCompletionsResource) Create(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletion, error) {
	var out ChatCompletion
	if err := r.c.request(ctx, "POST", "/v1/chat/completions", req, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type CreditsResource struct{ c *Client }
type CreditBalance struct {
	BalanceCents        int    `json:"balanceCents"`
	TotalAllocatedCents int    `json:"totalAllocatedCents"`
	Currency            string `json:"currency"`
	Plan                string `json:"plan"`
	AutoReload          struct {
		Enabled        bool `json:"enabled"`
		ThresholdCents int  `json:"thresholdCents"`
		AmountCents    int  `json:"amountCents"`
	} `json:"autoReload"`
}

func (r *CreditsResource) Retrieve(ctx context.Context) (*CreditBalance, error) {
	var out CreditBalance
	return &out, r.c.request(ctx, "GET", "/v1/credits", nil, nil, &out)
}

type UsageResource struct{ c *Client }
type UsageDay struct {
	Date         string `json:"date"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	TotalTokens  int    `json:"totalTokens"`
	RequestCount int    `json:"requestCount"`
	CostCents    int    `json:"costCents"`
}
type UsageList struct {
	Days []UsageDay `json:"days"`
}

func (r *UsageResource) List(ctx context.Context, days int) (*UsageList, error) {
	var out UsageList
	return &out, r.c.request(ctx, "GET", "/v1/usage", nil, map[string]any{"days": days}, &out)
}

type SessionsResource struct{ c *Client }
type SessionSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Model        string `json:"model"`
	LastUpdated  string `json:"lastUpdated"`
	MessageCount int    `json:"messageCount"`
}
type Session struct {
	SessionSummary
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		Ts      string `json:"ts"`
	} `json:"messages"`
}
type SessionsList struct {
	Sessions []SessionSummary `json:"sessions"`
}

func (r *SessionsResource) List(ctx context.Context, limit int) (*SessionsList, error) {
	var out SessionsList
	return &out, r.c.request(ctx, "GET", "/v1/sessions", nil, map[string]any{"limit": limit}, &out)
}
func (r *SessionsResource) Retrieve(ctx context.Context, id string) (*Session, error) {
	var out Session
	return &out, r.c.request(ctx, "GET", "/v1/sessions/"+url.PathEscape(id), nil, nil, &out)
}

type InvoicesResource struct{ c *Client }
type Invoice struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	AmountCents int    `json:"amountCents"`
	Status      string `json:"status"`
	PDFURL      string `json:"pdfUrl"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}
type InvoicesList struct {
	Invoices []Invoice `json:"invoices"`
}

func (r *InvoicesResource) List(ctx context.Context, limit int) (*InvoicesList, error) {
	var out InvoicesList
	return &out, r.c.request(ctx, "GET", "/v1/invoices", nil, map[string]any{"limit": limit}, &out)
}

type ModelsResource struct{ c *Client }
type Model struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Context   int    `json:"context"`
	MaxOutput int    `json:"maxOutput"`
}
type ModelsList struct {
	Models []Model `json:"models"`
}

func (r *ModelsResource) List(ctx context.Context) (*ModelsList, error) {
	var out ModelsList
	return &out, r.c.request(ctx, "GET", "/v1/models", nil, nil, &out)
}

type OAuthResource struct{ c *Client }
type OAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}
type OAuthTokenRequest struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"-"`
}

func (r *OAuthResource) Token(ctx context.Context, req *OAuthTokenRequest) (*OAuthTokenResponse, error) {
	body := map[string]any{
		"grant_type":    "client_credentials",
		"client_id":     req.ClientID,
		"client_secret": req.ClientSecret,
	}
	if len(req.Scopes) > 0 {
		body["scope"] = strings.Join(req.Scopes, " ")
	}
	var out OAuthTokenResponse
	if err := r.c.request(ctx, "POST", "/oauth/token", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// VerifyWebhook checks the Refactual-Signature header against the given secret.
// Returns nil on success. Tolerance defaults to 5 minutes when zero.
func VerifyWebhook(rawBody []byte, signatureHeader, secret string, tolerance time.Duration) error {
	if signatureHeader == "" {
		return errors.New("missing Refactual-Signature header")
	}
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	parts := map[string]string{}
	for _, p := range strings.Split(signatureHeader, ",") {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 {
			parts[kv[0]] = kv[1]
		}
	}
	ts, err := strconv.ParseInt(parts["t"], 10, 64)
	if err != nil {
		return errors.New("malformed signature header")
	}
	if d := time.Since(time.Unix(ts, 0)).Abs(); d > tolerance {
		return fmt.Errorf("stale signature timestamp (%s)", d)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", ts, rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts["v1"]), []byte(expected)) {
		return errors.New("bad signature")
	}
	return nil
}
