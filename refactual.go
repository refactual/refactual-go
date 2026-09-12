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
	crand "crypto/rand"
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
	Version        = "0.1.2"
	DefaultBaseURL = "https://app.refactual.com"
)

// Client is the top-level API client. Create one and reuse it.
type Client struct {
	Chat         *ChatResource
	Credits      *CreditsResource
	Usage        *UsageResource
	Sessions     *SessionsResource
	Invoices     *InvoicesResource
	Models       *ModelsResource
	OAuth        *OAuthResource
	Intelligence *IntelligenceResource
	Documents    *DocumentsResource
	Autofix      *AutofixResource
	Refactor     *RefactorResource
	Commerce     *CommerceResource

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
	c.Intelligence = &IntelligenceResource{c: c}
	c.Documents = &DocumentsResource{c: c}
	c.Autofix = &AutofixResource{c: c}
	c.Refactor = &RefactorResource{c: c}
	c.Commerce = &CommerceResource{c: c}
	return c
}

// ── Commerce (Shopify) ────────────────────────────────────────────
//
// Wraps /api/commerce/* — same surface the in-app chat tools use.
// Multi-store callers pass `store` (the .myshopify.com domain) via
// the variadic options helpers below; single-store users can omit it.
type CommerceResource struct{ c *Client }

// CommerceQueryOpt is a setter applied to the query map for commerce
// helpers. Use Store("foo.myshopify.com"), Days(30), Limit(10), etc.
type CommerceQueryOpt func(map[string]any)

func Store(s string) CommerceQueryOpt { return func(q map[string]any) { if s != "" { q["store"] = s } } }
func Days(n int) CommerceQueryOpt     { return func(q map[string]any) { q["days"] = n } }
func Limit(n int) CommerceQueryOpt    { return func(q map[string]any) { q["limit"] = n } }

func collectCommerceQuery(opts ...CommerceQueryOpt) map[string]any {
	q := map[string]any{}
	for _, o := range opts { o(q) }
	return q
}

// GetOrder fetches one Shopify order by numeric ID or order name (#1234).
func (r *CommerceResource) GetOrder(ctx context.Context, idOrName string, opts ...CommerceQueryOpt) (map[string]any, error) {
	var out map[string]any
	err := r.c.request(ctx, http.MethodGet, "/api/commerce/orders/"+idOrName, nil, collectCommerceQuery(opts...), &out)
	return out, err
}

// SearchProducts searches the catalog by title fragment.
func (r *CommerceResource) SearchProducts(ctx context.Context, q string, opts ...CommerceQueryOpt) (map[string]any, error) {
	query := collectCommerceQuery(opts...)
	query["q"] = q
	var out map[string]any
	err := r.c.request(ctx, http.MethodGet, "/api/commerce/products/search", nil, query, &out)
	return out, err
}

// GetProduct fetches one product by handle or numeric ID.
func (r *CommerceResource) GetProduct(ctx context.Context, idOrHandle string, opts ...CommerceQueryOpt) (map[string]any, error) {
	var out map[string]any
	err := r.c.request(ctx, http.MethodGet, "/api/commerce/products/"+idOrHandle, nil, collectCommerceQuery(opts...), &out)
	return out, err
}

// GetCustomer fetches one customer by email or numeric ID.
func (r *CommerceResource) GetCustomer(ctx context.Context, idOrEmail string, opts ...CommerceQueryOpt) (map[string]any, error) {
	var out map[string]any
	err := r.c.request(ctx, http.MethodGet, "/api/commerce/customers/"+idOrEmail, nil, collectCommerceQuery(opts...), &out)
	return out, err
}

// Overview returns revenue/AOV/orders for a window.
func (r *CommerceResource) Overview(ctx context.Context, opts ...CommerceQueryOpt) (map[string]any, error) {
	var out map[string]any
	err := r.c.request(ctx, http.MethodGet, "/api/commerce/overview", nil, collectCommerceQuery(opts...), &out)
	return out, err
}

// Insights returns the AI action plan.
func (r *CommerceResource) Insights(ctx context.Context, opts ...CommerceQueryOpt) (map[string]any, error) {
	var out map[string]any
	err := r.c.request(ctx, http.MethodGet, "/api/commerce/insights", nil, collectCommerceQuery(opts...), &out)
	return out, err
}

// ErrTypeIdempotencyInFlight is the Error.Type the platform answers (with
// HTTP 409) when a request carries an Idempotency-Key whose first attempt is
// still running. The client waits it out; it only surfaces when that wait
// exceeds the request budget.
const ErrTypeIdempotencyInFlight = "idempotency_in_flight"

// Error is returned on non-2xx responses.
type Error struct {
	Status  int    `json:"-"`
	Type    string `json:"type"`
	Message string `json:"message"`
	// IdempotencyKey is the Idempotency-Key the failed call was sent with
	// (non-GET requests only). Resubmitting with WithIdempotencyKey(key) is
	// safe: the platform runs a keyed call at most once.
	IdempotencyKey string `json:"-"`
}

// RequestOption configures a single call (as opposed to Option, which
// configures the Client).
type RequestOption func(*requestConfig)

type requestConfig struct {
	idempotencyKey string
}

// WithIdempotencyKey sets the Idempotency-Key for one call. The client mints
// a UUID v4 for every non-GET request automatically and reuses it across that
// call's retries, so this is only needed when the same logical call may be
// issued again by a *new* process — a queue that can deliver a job twice, a
// cron that reruns after a crash. Keys are scoped to your API key and
// remembered for 24 hours; up to 200 characters.
func WithIdempotencyKey(k string) RequestOption {
	return func(rc *requestConfig) { rc.idempotencyKey = k }
}

func (e *Error) Error() string { return fmt.Sprintf("refactual: %s (status %d, type %s)", e.Message, e.Status, e.Type) }

type errorEnvelope struct {
	Error Error `json:"error"`
}

func (c *Client) request(ctx context.Context, method, path string, body any, query map[string]any, out any) error {
	_, err := c.do(ctx, method, path, body, query, out)
	return err
}

// do issues one logical call — with retries — and returns the response
// headers of the attempt that produced the answer.
func (c *Client) do(ctx context.Context, method, path string, body any, query map[string]any, out any, opts ...RequestOption) (http.Header, error) {
	if c.token == "" {
		return nil, errors.New("refactual: token required (WithAPIKey or WithAccessToken)")
	}
	var rc requestConfig
	for _, o := range opts {
		o(&rc)
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
			return nil, err
		}
		payload = b
	}

	// One idempotency key per logical call, minted BEFORE the first attempt
	// and sent unchanged by every retry of it. The platform runs a keyed
	// request once: a retry that lands after the first attempt finished gets
	// the stored answer back (Idempotency-Replayed: true); one that lands
	// while it is still running gets 409 idempotency_in_flight, which is
	// waited out below. Net effect: a timed-out or 5xx'd POST that this loop
	// retries is billed once. The key is never rotated here — a fresh key on
	// a 5xx could re-run an attempt the platform had in fact completed. An
	// explicit WithIdempotencyKey or Idempotency-Key header wins.
	idemKey := ""
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		idemKey = rc.idempotencyKey
		if idemKey == "" {
			for k, v := range c.headers {
				if strings.EqualFold(k, "Idempotency-Key") {
					idemKey = v
				}
			}
		}
		if idemKey == "" {
			idemKey = newIdempotencyKey()
		}
	}
	budget := c.httpClient.Timeout
	if budget <= 0 {
		budget = 60 * time.Second
	}

	var resp *http.Response
	var reqErr error
	var inflightDeadline time.Time
	inflightPolls := 0
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("User-Agent", "refactual-go/"+Version)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		if idemKey != "" {
			req.Header.Set("Idempotency-Key", idemKey)
		}
		resp, reqErr = c.httpClient.Do(req)
		if reqErr != nil {
			if attempt < c.retries {
				if err := sleepCtx(ctx, backoff(attempt+1)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, reqErr
		}
		replayed := strings.EqualFold(resp.Header.Get("Idempotency-Replayed"), "true")
		if resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
			// A replayed 5xx is the stored outcome of this very call — the
			// same key can only ever return it again, so retrying is pointless.
			if !replayed && attempt < c.retries {
				ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
				_ = resp.Body.Close()
				wait := backoff(attempt + 1)
				if ra > 0 {
					wait = time.Duration(ra) * time.Second
				}
				if err := sleepCtx(ctx, wait); err != nil {
					return nil, err
				}
				continue
			}
		}
		if resp.StatusCode == http.StatusConflict && idemKey != "" {
			data, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			apiErr := newAPIError(resp.StatusCode, data, idemKey)
			if apiErr.Type == ErrTypeIdempotencyInFlight {
				// Our own earlier attempt is still running. Poll with the same
				// key — cheap for the platform, and the answer comes back
				// replayed — for up to the client timeout; past that, surface
				// the 409 with the key attached so the caller can resubmit
				// later and still receive the one answer.
				now := time.Now()
				if inflightDeadline.IsZero() {
					inflightDeadline = now.Add(budget)
				}
				if remaining := inflightDeadline.Sub(now); remaining > 0 {
					wait := inflightPoll(inflightPolls)
					inflightPolls++
					if wait > remaining {
						wait = remaining
					}
					if err := sleepCtx(ctx, wait); err != nil {
						return nil, err
					}
					continue
				}
			}
			return resp.Header, apiErr
		}
		break
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return resp.Header, newAPIError(resp.StatusCode, data, idemKey)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.Header, err
		}
	}
	return resp.Header, nil
}

func newAPIError(status int, data []byte, idemKey string) *Error {
	var env errorEnvelope
	_ = json.Unmarshal(data, &env)
	env.Error.Status = status
	env.Error.IdempotencyKey = idemKey
	if env.Error.Message == "" {
		env.Error.Message = fmt.Sprintf("HTTP %d", status)
	}
	if env.Error.Type == "" {
		env.Error.Type = "api_error"
	}
	return &env.Error
}

// sleepCtx waits for d or until ctx is done, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Tunable in tests so the retry paths run without real waits.
var (
	backoffBase      = 250 * time.Millisecond
	inflightPollBase = 500 * time.Millisecond
)

// inflightPoll is the wait before the n-th poll while an earlier attempt is
// still in flight: 0.5s, 1s, 2s, 4s, 8s, 8s… plus jitter.
func inflightPoll(n int) time.Duration {
	d := inflightPollBase * time.Duration(1<<uint(n))
	if limit := 16 * inflightPollBase; d > limit {
		d = limit
	}
	return d + time.Duration(rand.Int63n(int64(inflightPollBase/2)+1)) // up to half a base step of jitter
}

// newIdempotencyKey returns a UUID v4 from crypto/rand.
func newIdempotencyKey() string {
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		// Practically unreachable; a dedupe token, not a secret.
		return fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func backoff(attempt int) time.Duration {
	base := time.Duration(math.Pow(2, float64(attempt))) * backoffBase
	jitter := time.Duration(rand.Int63n(int64(backoffBase) + 1)) // up to one base step
	return base + jitter
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
	// Replayed is true when the platform answered from an earlier attempt
	// that carried the same idempotency key — nothing new was billed.
	Replayed bool `json:"-"`
}

// Create submits messages and returns the reply. Billed once per call: the
// client sends an idempotency key with the request and reuses it on every
// retry, so a timeout or 5xx that gets retried cannot produce a second
// charge. Pass WithIdempotencyKey to make the call re-runnable across
// processes.
func (r *ChatCompletionsResource) Create(ctx context.Context, req *ChatCompletionRequest, opts ...RequestOption) (*ChatCompletion, error) {
	var out ChatCompletion
	h, err := r.c.do(ctx, "POST", "/v1/chat/completions", req, nil, &out, opts...)
	if err != nil {
		return nil, err
	}
	out.Replayed = strings.EqualFold(h.Get("Idempotency-Replayed"), "true")
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

// ── Intelligence (Repo Analysis) ───────────────────────────────────

// IntelligenceResource is Refactual's Repo Analysis surface.
//
// Reports run async; create one and poll until status == "completed":
//
//	resp, _ := client.Intelligence.Create(ctx, &refactual.CreateReportRequest{
//		RepoURL: "https://github.com/owner/repo",
//	})
//	report, _ := client.Intelligence.WaitForCompletion(ctx, resp.ReportID, 0)
type IntelligenceResource struct{ c *Client }

type ModernizationReport struct {
	ID                   string    `json:"_id"`
	UserID               string    `json:"userId"`
	Source               string    `json:"source"`
	RepoURL              string    `json:"repoUrl"`
	Title                string    `json:"title"`
	Status               string    `json:"status"`
	ModernizationScore   int       `json:"modernizationScore,omitempty"`
	EstimatedEffortWeeks int       `json:"estimatedEffortWeeks,omitempty"`
	Summary              string    `json:"summary,omitempty"`
	Hotspots             []Hotspot `json:"hotspots,omitempty"`
	CreatedAt            string    `json:"createdAt"`
	CompletedAt          string    `json:"completedAt,omitempty"`
	Error                string    `json:"error,omitempty"`
}

type Hotspot struct {
	Path            string `json:"path"`
	Language        string `json:"language,omitempty"`
	Severity        string `json:"severity,omitempty"`
	SuggestedAction string `json:"suggestedAction,omitempty"`
	Effort          string `json:"effort,omitempty"`
}

type ReportsList struct {
	Reports []ModernizationReport `json:"reports"`
}

type CreateReportRequest struct {
	RepoURL  string `json:"repoUrl,omitempty"`
	Snapshot string `json:"snapshot,omitempty"`
	Title    string `json:"title,omitempty"`
	Source   string `json:"source,omitempty"`
}

type CreateReportResponse struct {
	OK       bool                `json:"ok"`
	ReportID string              `json:"reportId"`
	Report   ModernizationReport `json:"report"`
}

type UploadReportRequest struct {
	Title string         `json:"title,omitempty"`
	Files []UploadedFile `json:"files"`
}
type UploadedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
type UploadReportResponse struct {
	OK       bool   `json:"ok"`
	ReportID string `json:"reportId"`
	URL      string `json:"url"`
}

func (r *IntelligenceResource) List(ctx context.Context, limit int) (*ReportsList, error) {
	var out ReportsList
	q := map[string]any{}
	if limit > 0 {
		q["limit"] = limit
	}
	return &out, r.c.request(ctx, http.MethodGet, "/v1/mi/reports", nil, q, &out)
}

func (r *IntelligenceResource) Retrieve(ctx context.Context, id string) (*ModernizationReport, error) {
	var out struct {
		Report ModernizationReport `json:"report"`
	}
	if err := r.c.request(ctx, http.MethodGet, "/v1/mi/reports/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out.Report, nil
}

func (r *IntelligenceResource) Create(ctx context.Context, req *CreateReportRequest) (*CreateReportResponse, error) {
	if req.Source == "" {
		if req.Snapshot != "" {
			req.Source = "paste"
		} else {
			req.Source = "github"
		}
	}
	var out CreateReportResponse
	return &out, r.c.request(ctx, http.MethodPost, "/v1/mi/reports", req, nil, &out)
}

func (r *IntelligenceResource) Upload(ctx context.Context, req *UploadReportRequest) (*UploadReportResponse, error) {
	var out UploadReportResponse
	return &out, r.c.request(ctx, http.MethodPost, "/v1/mi/reports/upload", req, nil, &out)
}

func (r *IntelligenceResource) Delete(ctx context.Context, id string) error {
	return r.c.request(ctx, http.MethodDelete, "/v1/mi/reports/"+url.PathEscape(id), nil, nil, nil)
}

// WaitForCompletion polls Retrieve every 4 seconds until the report is
// either completed or failed. Pass 0 for default timeout (5 minutes).
func (r *IntelligenceResource) WaitForCompletion(ctx context.Context, id string, timeout time.Duration) (*ModernizationReport, error) {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rep, err := r.Retrieve(ctx, id)
		if err != nil {
			return nil, err
		}
		switch rep.Status {
		case "completed":
			return rep, nil
		case "failed":
			return rep, &Error{Type: "analysis_failed", Message: rep.Error, Status: 0}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(4 * time.Second):
		}
	}
	return nil, errors.New("refactual: timed out waiting for analysis to complete")
}

// ── Auto-Fix ───────────────────────────────────────────────────────

// AutofixResource is Refactual's hotspot-fix surface (read-only via the
// SDK; jobs are kicked off from the web app today).
type AutofixResource struct{ c *Client }

type AutofixJob struct {
	ID              string `json:"_id"`
	UserID          string `json:"userId"`
	ReportID        string `json:"reportId"`
	HotspotPath     string `json:"hotspotPath"`
	HotspotLanguage string `json:"hotspotLanguage"`
	Mode            string `json:"mode"`
	Status          string `json:"status"`
	PRURL           string `json:"prUrl,omitempty"`
	PatchDiff       string `json:"patchDiff,omitempty"`
	RefactoredBody  string `json:"refactoredBody,omitempty"`
	CostCents       int    `json:"costCents,omitempty"`
	CreatedAt       string `json:"createdAt"`
	CompletedAt     string `json:"completedAt,omitempty"`
	Error           string `json:"error,omitempty"`
}

type AutofixList struct {
	Jobs []AutofixJob `json:"jobs"`
}

func (r *AutofixResource) List(ctx context.Context, reportID string) (*AutofixList, error) {
	var out AutofixList
	return &out, r.c.request(ctx, http.MethodGet, "/v1/mi/reports/"+url.PathEscape(reportID)+"/autofix", nil, nil, &out)
}

func (r *AutofixResource) Retrieve(ctx context.Context, jobID string) (*AutofixJob, error) {
	var out struct {
		Job AutofixJob `json:"job"`
	}
	if err := r.c.request(ctx, http.MethodGet, "/v1/mi/autofix/"+url.PathEscape(jobID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out.Job, nil
}

func (r *AutofixResource) WaitForCompletion(ctx context.Context, jobID string, timeout time.Duration) (*AutofixJob, error) {
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := r.Retrieve(ctx, jobID)
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case "completed":
			return job, nil
		case "failed":
			return job, &Error{Type: "autofix_failed", Message: job.Error}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(4 * time.Second):
		}
	}
	return nil, errors.New("refactual: timed out waiting for Auto-Fix")
}

// ── Documents (Parser) ─────────────────────────────────────────────

type DocumentsResource struct{ c *Client }

type DocumentJob struct {
	ID          string             `json:"_id"`
	UserID      string             `json:"userId"`
	Kind        string             `json:"kind"`
	Title       string             `json:"title"`
	Status      string             `json:"status"`
	Inputs      []DocumentJobInput `json:"inputs,omitempty"`
	Output      json.RawMessage    `json:"output,omitempty"`
	Summary     string             `json:"summary,omitempty"`
	CreatedAt   string             `json:"createdAt"`
	CompletedAt string             `json:"completedAt,omitempty"`
	Error       string             `json:"error,omitempty"`
}

type DocumentJobInput struct {
	Name    string `json:"name"`
	Mime    string `json:"mime"`
	Bytes   int    `json:"bytes,omitempty"`
	Content string `json:"content,omitempty"`
}

type DocumentJobList struct {
	Jobs []DocumentJob `json:"jobs"`
}

type CreateDocumentJobRequest struct {
	Kind    string             `json:"kind"`
	Title   string             `json:"title,omitempty"`
	Inputs  []DocumentJobInput `json:"inputs"`
	Options map[string]any     `json:"options,omitempty"`
}

type CreateDocumentJobResponse struct {
	OK    bool        `json:"ok"`
	JobID string      `json:"jobId"`
	Job   DocumentJob `json:"job"`
}

func (r *DocumentsResource) List(ctx context.Context, limit int) (*DocumentJobList, error) {
	var out DocumentJobList
	q := map[string]any{}
	if limit > 0 {
		q["limit"] = limit
	}
	return &out, r.c.request(ctx, http.MethodGet, "/v1/documents/jobs", nil, q, &out)
}

func (r *DocumentsResource) Retrieve(ctx context.Context, id string) (*DocumentJob, error) {
	var out struct {
		Job DocumentJob `json:"job"`
	}
	if err := r.c.request(ctx, http.MethodGet, "/v1/documents/jobs/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out.Job, nil
}

func (r *DocumentsResource) Create(ctx context.Context, req *CreateDocumentJobRequest) (*CreateDocumentJobResponse, error) {
	var out CreateDocumentJobResponse
	return &out, r.c.request(ctx, http.MethodPost, "/v1/documents/jobs", req, nil, &out)
}

func (r *DocumentsResource) Delete(ctx context.Context, id string) error {
	return r.c.request(ctx, http.MethodDelete, "/v1/documents/jobs/"+url.PathEscape(id), nil, nil, nil)
}

func (r *DocumentsResource) WaitForCompletion(ctx context.Context, id string, timeout time.Duration) (*DocumentJob, error) {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := r.Retrieve(ctx, id)
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case "completed":
			return job, nil
		case "failed":
			return job, &Error{Type: "parser_failed", Message: job.Error}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return nil, errors.New("refactual: timed out waiting for Parser job")
}

// ── Refactor (sync code translation) ────────────────────────────────

// RefactorResource wraps Chat.Completions.Create with a curated system
// prompt for code translation. Saves callers from rolling their own
// prompt for the most common case.
type RefactorResource struct{ c *Client }

type RefactorCodeRequest struct {
	SourceLang   string
	TargetLang   string
	Code         string
	Instructions string
	Model        string
	MaxTokens    int
}

func (r *RefactorResource) Code(ctx context.Context, req *RefactorCodeRequest, opts ...RequestOption) (*ChatCompletion, error) {
	system := fmt.Sprintf(
		"You are Refactual's code refactoring engine. Translate %s to idiomatic %s. "+
			"Preserve numeric precision (decimal types for money, never floats). "+
			"Output ONLY the modernized code as a single fenced code block. "+
			"No commentary, no explanations, no markdown headers.",
		req.SourceLang, req.TargetLang,
	)
	if req.Instructions != "" {
		system += " Additional constraints: " + req.Instructions
	}
	model := req.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	return r.c.Chat.Completions.Create(ctx, &ChatCompletionRequest{
		Model:        model,
		Messages:     []ChatMessage{{Role: "user", Content: req.Code}},
		SystemPrompt: system,
		MaxTokens:    maxTokens,
	}, opts...)
}
