package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/greyone/yango-client-go/errors"
	"github.com/greyone/yango-client-go/internal/httpx"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Option configures the Client.
type Option func(*config)

type config struct {
	apiKey           string
	bearerToken      string
	authHeaderFunc   func(context.Context) (string, error)
	userHeadersFunc  func(context.Context) (userID, customerID string)
	timeout          time.Duration
	httpClient       *http.Client
	maxRetries       int
	retryDelay       time.Duration
	useOtel          bool
	retryableStatus  map[int]bool // status codes that trigger retry (in addition to 5xx, 429)
}

func defaultConfig() *config {
	return &config{
		timeout:     30 * time.Second,
		maxRetries: 3,
		retryDelay: 300 * time.Millisecond,
		useOtel:    true,
		retryableStatus: map[int]bool{
			429: true,
		},
	}
}

// WithAPIKey sets the API key (sent as Authorization Bearer and X-API-Key).
func WithAPIKey(key string) Option {
	return func(c *config) {
		c.apiKey = key
	}
}

// WithStaticBearerToken sets a static bearer token (overrides API key for Authorization).
func WithStaticBearerToken(token string) Option {
	return func(c *config) {
		c.bearerToken = token
	}
}

// WithAuthHeaderFunc sets a function that returns the Authorization header value per request.
func WithAuthHeaderFunc(fn func(context.Context) (string, error)) Option {
	return func(c *config) {
		c.authHeaderFunc = fn
	}
}

// WithDefaultUserHeaders sets a function to inject X-User-Id and X-Customer-Id from context.
func WithDefaultUserHeaders(fn func(context.Context) (userID, customerID string)) Option {
	return func(c *config) {
		c.userHeadersFunc = fn
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		c.timeout = d
	}
}

// WithHTTPClient sets a custom *http.Client (overrides timeout/transport from other options).
func WithHTTPClient(cl *http.Client) Option {
	return func(c *config) {
		c.httpClient = cl
	}
}

// WithRetries sets max retry attempts and delay between retries.
func WithRetries(maxAttempts int, delay time.Duration) Option {
	return func(c *config) {
		c.maxRetries = maxAttempts
		c.retryDelay = delay
	}
}

// WithOpenTelemetry enables or disables otelhttp transport wrapping.
func WithOpenTelemetry(enable bool) Option {
	return func(c *config) {
		c.useOtel = enable
	}
}

// RequestOption applies per-request overrides (e.g. idempotency key, extra headers).
type RequestOption func(*RequestConfig)

// RequestConfig holds per-request options.
type RequestConfig struct {
	Headers         http.Header
	IdempotencyKey  string
}

// WithHeaders merges additional headers into the request.
func WithHeaders(h http.Header) RequestOption {
	return func(rc *RequestConfig) {
		rc.Headers = h
	}
}

// WithIdempotencyKey sets the idempotency key header (Idempotency-Key by convention).
func WithIdempotencyKey(key string) RequestOption {
	return func(rc *RequestConfig) {
		rc.IdempotencyKey = key
	}
}

// Client is the Yango API client with retries and error handling.
type Client struct {
	baseURL string
	cfg     *config
	http    *http.Client
}

// NewClient creates a new Client. baseURL must not include a trailing slash.
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("yango client: baseURL is required")
	}
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	transport := http.DefaultTransport
	if cfg.useOtel {
		transport = otelhttp.NewTransport(http.DefaultTransport)
	}
	client := &Client{
		baseURL: baseURL,
		cfg:     cfg,
		http: &http.Client{
			Timeout:   cfg.timeout,
			Transport: transport,
		},
	}
	if cfg.httpClient != nil {
		client.http = cfg.httpClient
	}
	return client, nil
}

// Do executes the request with retries and returns a typed error on non-2xx.
// If v is non-nil and response is JSON, the body is decoded into v.
func (c *Client) Do(ctx context.Context, req *http.Request, v any) error {
	op := req.Method + " " + req.URL.Path
	return c.doWithRetries(ctx, req, v, op)
}

func (c *Client) doWithRetries(ctx context.Context, req *http.Request, v any, op string) error {
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return err
		}
	}
	var lastErr error
	delays := httpx.FixedBackoff(c.cfg.retryDelay, c.cfg.maxRetries)
	for attempt := 0; attempt <= c.cfg.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delays[attempt-1]):
			}
		}
		reqCopy := req.Clone(ctx)
		if len(bodyBytes) > 0 {
			reqCopy.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
		lastErr = c.doOne(ctx, reqCopy, v, op)
		if lastErr == nil {
			return nil
		}
		if !errors.IsRetryableError(lastErr) {
			return lastErr
		}
		// For non-idempotent methods, only retry if idempotency key is set
		if reqCopy.Method != http.MethodGet && reqCopy.Method != http.MethodHead && reqCopy.Method != http.MethodPut {
			if reqCopy.Header.Get("Idempotency-Key") == "" {
				return lastErr
			}
		}
	}
	return lastErr
}

func (c *Client) doOne(ctx context.Context, req *http.Request, v any, op string) error {
	c.applyConfigToRequest(ctx, req)
	resp, err := c.http.Do(req)
	if err != nil {
		return &errors.Err{Message: err.Error(), Op: op}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &errors.Err{Message: err.Error(), Op: op}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if v != nil && len(body) > 0 {
			if err := json.Unmarshal(body, v); err != nil {
				return &errors.Err{StatusCode: resp.StatusCode, Message: "decode response: " + err.Error(), RawBody: body, Op: op}
			}
		}
		return nil
	}
	// Parse optional API error body
	apiErr := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{}
	_ = json.Unmarshal(body, &apiErr)
	msg := apiErr.Message
	if msg == "" {
		msg = string(body)
	}
	if msg == "" {
		msg = resp.Status
	}
	return &errors.Err{
		StatusCode: resp.StatusCode,
		Code:       apiErr.Code,
		Message:    msg,
		RawBody:    body,
		Op:         op,
	}
}

func (c *Client) applyConfigToRequest(ctx context.Context, req *http.Request) {
	cfg := c.cfg
	if cfg.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.bearerToken)
	} else if cfg.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
		req.Header.Set("X-API-Key", cfg.apiKey)
	}
	if cfg.authHeaderFunc != nil {
		if auth, err := cfg.authHeaderFunc(ctx); err == nil && auth != "" {
			req.Header.Set("Authorization", auth)
		}
	}
	if cfg.userHeadersFunc != nil {
		uid, cid := cfg.userHeadersFunc(ctx)
		if uid != "" {
			req.Header.Set("X-User-Id", uid)
		}
		if cid != "" {
			req.Header.Set("X-Customer-Id", cid)
		}
	}
	if req.Header.Get("Content-Type") == "" && req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
}

// applyRequestOptions applies per-request options (headers, idempotency key).
func (c *Client) applyRequestOptions(req *http.Request, ropts []RequestOption) {
	var rc RequestConfig
	for _, o := range ropts {
		o(&rc)
	}
	if rc.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", rc.IdempotencyKey)
	}
	for k, v := range rc.Headers {
		for _, vv := range v {
			req.Header.Set(k, vv)
		}
	}
}

// buildURL appends path to baseURL.
func (c *Client) buildURL(p string) string {
	if p == "" {
		return c.baseURL
	}
	return c.baseURL + path.Join("/", p)
}

// GetJSON performs a GET and decodes the response into v.
func (c *Client) GetJSON(ctx context.Context, path string, v any, ropts ...RequestOption) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.buildURL(path), nil)
	if err != nil {
		return err
	}
	c.applyRequestOptions(req, ropts)
	return c.Do(ctx, req, v)
}

// PostJSON sends body as JSON and decodes the response into v.
func (c *Client) PostJSON(ctx context.Context, path string, body any, v any, ropts ...RequestOption) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL(path), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyRequestOptions(req, ropts)
	return c.Do(ctx, req, v)
}

// PutJSON sends body as JSON and decodes the response into v.
func (c *Client) PutJSON(ctx context.Context, path string, body any, v any, ropts ...RequestOption) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.buildURL(path), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyRequestOptions(req, ropts)
	return c.Do(ctx, req, v)
}

// RawDo executes a request without retries and returns body, status, and a typed error.
// Use this when you need to forward raw responses (e.g. proxy). For normal API calls use Do/GetJSON/PostJSON.
func (c *Client) RawDo(ctx context.Context, method, path string, body []byte, forwardHeaders http.Header) ([]byte, int, error) {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path), bodyReader)
	if err != nil {
		return nil, 0, err
	}
	c.applyConfigToRequest(ctx, req)
	for _, k := range []string{"Authorization", "X-User-Id", "X-Customer-Id"} {
		if v := forwardHeaders.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return out, resp.StatusCode, nil
	}
	return out, resp.StatusCode, &errors.Err{
		StatusCode: resp.StatusCode,
		Message:    string(out),
		RawBody:    out,
		Op:         method + " " + path,
	}
}
