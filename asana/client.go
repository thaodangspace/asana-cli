// Package asana is a thin HTTP client for the Asana REST API, ported from the
// Pi extension's AsanaClient (request, error mapping, pagination).
package asana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Asana REST API v1.0 root.
const DefaultBaseURL = "https://app.asana.com/api/1.0"

// Retry defaults used by clients and the CLI.
const (
	// DefaultMaxRetries is the number of retries after the initial request.
	DefaultMaxRetries = 3
	// DefaultRetryMaxWait bounds one retry delay, including Retry-After delays.
	DefaultRetryMaxWait = 30 * time.Second
	retryBaseDelay      = 100 * time.Millisecond
)

// RetryConfig controls bounded retries for replayable requests. Only GET and
// DELETE requests are retried: retrying mutating JSON requests could duplicate
// an Asana operation, and multipart uploads are never replayed.
type RetryConfig struct {
	MaxRetries int
	MaxWait    time.Duration
	Disabled   bool
}

type sleeper func(context.Context, time.Duration) error

// Client performs authenticated requests against the Asana API.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	verbose    bool
	logw       io.Writer
	retry      RetryConfig
	sleep      sleeper
	now        func() time.Time
}

func defaultRetryConfig() RetryConfig {
	return RetryConfig{MaxRetries: DefaultMaxRetries, MaxWait: DefaultRetryMaxWait}
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (used by tests against httptest).
func WithBaseURL(base string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(base, "/") }
}

// WithVerbose logs each request's method and path (never the token) to w.
func WithVerbose(w io.Writer) Option {
	return func(c *Client) {
		c.verbose = true
		c.logw = w
	}
}

// WithRetryConfig overrides the client's retry policy.
func WithRetryConfig(config RetryConfig) Option {
	return func(c *Client) {
		if config.MaxWait <= 0 {
			config.MaxWait = DefaultRetryMaxWait
		}
		c.retry = config
	}
}

// WithSleeper replaces the context-aware retry sleeper. It is primarily useful
// for tests; production callers should use the default sleeper.
func WithSleeper(fn func(context.Context, time.Duration) error) Option {
	return func(c *Client) {
		if fn != nil {
			c.sleep = fn
		}
	}
}

// WithClock replaces the clock used to parse HTTP-date Retry-After values.
// It is primarily useful for tests.
func WithClock(now func() time.Time) Option {
	return func(c *Client) {
		if now != nil {
			c.now = now
		}
	}
}

// NewClient builds a Client. httpClient may be nil to use http.DefaultClient's
// transport with the given client (callers typically pass one with a Timeout).
func NewClient(token string, httpClient *http.Client, opts ...Option) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	c := &Client{
		httpClient: httpClient,
		token:      token,
		baseURL:    DefaultBaseURL,
		retry:      defaultRetryConfig(),
		sleep:      sleepContext,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HTTPError describes a non-2xx Asana response. It never carries the token.
type HTTPError struct {
	Method          string `json:"method"`
	URL             string `json:"url"`
	Path            string `json:"path"`
	Status          int    `json:"status"`
	StatusText      string `json:"statusText"`
	ResponseExcerpt string `json:"responseExcerpt"`
}

func (e *HTTPError) Error() string { return formatHTTPError(e) }

func formatHTTPError(e *HTTPError) string {
	switch e.Status {
	case http.StatusUnauthorized:
		return "Asana authentication failed. Check ASANA_ACCESS_TOKEN."
	case http.StatusPaymentRequired:
		return "Asana API access requires a premium workspace or feature for this request."
	case http.StatusForbidden:
		return "Asana authorization failed. Check token scopes and resource permissions."
	case http.StatusNotFound:
		return "Asana resource not found. Check workspace_gid, task_gid, project_gid, and IDs."
	case http.StatusTooManyRequests:
		return "Asana rate limit reached. Retry later."
	default:
		if e.ResponseExcerpt == "" {
			return fmt.Sprintf("Asana request failed with %d %s", e.Status, e.StatusText)
		}
		return fmt.Sprintf("Asana request failed with %d %s: %s", e.Status, e.StatusText, e.ResponseExcerpt)
	}
}

// EncodePathSegment escapes a value for safe use in a URL path segment.
func EncodePathSegment(value string) string {
	return url.PathEscape(value)
}

// buildURL resolves a path or absolute URL against the base, mirroring the
// extension's buildUrl behavior.
func (c *Client) buildURL(pathOrURL string) string {
	if u, err := url.Parse(pathOrURL); err == nil && u.IsAbs() && u.Host != "" {
		return pathOrURL
	}
	if strings.HasPrefix(pathOrURL, "/") {
		return c.baseURL + pathOrURL
	}
	return c.baseURL + "/" + pathOrURL
}

// urlOrigin is the part of a URL that determines whether credentials may be
// sent. URL paths, queries, and fragments are deliberately excluded.
type urlOrigin struct {
	scheme   string
	hostname string
	port     string
}

func originOf(u *url.URL) urlOrigin {
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port != "" {
		if number, err := strconv.Atoi(port); err == nil {
			port = strconv.Itoa(number)
		}
	} else {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return urlOrigin{
		scheme:   scheme,
		hostname: strings.ToLower(u.Hostname()),
		port:     port,
	}
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || a.Host == "" || b.Host == "" {
		return false
	}
	return originOf(a) == originOf(b)
}

// shouldAuthenticate reports whether targetURL is trusted to receive the
// Asana bearer token. Relative API paths are trusted by definition; absolute
// URLs must have the same normalized origin as the configured API base URL.
func (c *Client) shouldAuthenticate(targetURL string) bool {
	target, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	if !target.IsAbs() {
		return true
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return false
	}
	return sameOrigin(base, target)
}

func isHTTPS(u *url.URL) bool {
	return u != nil && strings.EqualFold(u.Scheme, "https")
}

// displayPath strips query strings and fragments before a URL is logged or
// included in an error. Attachment URLs commonly contain signed credentials.
func displayPath(pathOrURL string) string {
	u, err := url.Parse(pathOrURL)
	if err == nil {
		path := u.EscapedPath()
		if path == "" {
			return "/"
		}
		return path
	}
	return pathOrURL
}

func safeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return displayPath(raw)
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	if u.IsAbs() {
		return u.String()
	}
	return displayPath(raw)
}

func redactSecret(value, secret string) string {
	if secret != "" {
		return strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	return value
}

type redactedError struct {
	err     error
	message string
}

func (e *redactedError) Error() string { return e.message }
func (e *redactedError) Unwrap() error { return e.err }

func redactError(err error, secret string) error {
	if err == nil || secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	return &redactedError{err: err, message: redactSecret(err.Error(), secret)}
}

func excerpt(body []byte) string {
	s := string(body)
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) retriesEnabled(method string) bool {
	if c.retry.Disabled || c.retry.MaxRetries <= 0 {
		return false
	}
	return method == http.MethodGet || method == http.MethodDelete
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusInternalServerError ||
		status == http.StatusBadGateway || status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func retryableNetworkError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

func (c *Client) retryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		if seconds == 0 {
			return 0, true
		}
		if seconds > int64((time.Duration(1<<63-1))/time.Second) {
			return c.retry.MaxWait, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	return maxDuration(0, when.Sub(c.now())), true
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func (c *Client) retryDelay(retryNumber int, retryAfter string) time.Duration {
	if delay, ok := c.retryAfter(retryAfter); ok {
		return capDuration(delay, c.retry.MaxWait)
	}
	if retryNumber < 1 {
		retryNumber = 1
	}
	backoff := retryBaseDelay
	for i := 1; i < retryNumber && backoff < c.retry.MaxWait; i++ {
		if backoff > time.Duration(1<<62) {
			break
		}
		backoff *= 2
	}
	backoff = capDuration(backoff, c.retry.MaxWait)
	if backoff <= 1 {
		return backoff
	}
	// Full jitter is deliberately bounded away from zero so retries do not
	// turn into a tight loop while still spreading concurrent clients.
	half := backoff / 2
	return half + time.Duration(rand.Int63n(int64(backoff-half)+1))
}

func capDuration(delay, cap time.Duration) time.Duration {
	if cap > 0 && delay > cap {
		return cap
	}
	if delay < 0 {
		return 0
	}
	return delay
}

func (c *Client) waitForRetry(ctx context.Context, method, path string, retryNumber int, retryAfter string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delay := c.retryDelay(retryNumber, retryAfter)
	if c.verbose && c.logw != nil {
		fmt.Fprintf(c.logw, "retry %s %s attempt %d/%d wait %s\n", method,
			redactSecret(displayPath(path), c.token), retryNumber, c.retry.MaxRetries, delay)
	}
	return c.sleep(ctx, delay)
}

// Request performs an HTTP request and returns the raw response body. A non-2xx
// status yields an *HTTPError. body, when non-nil, is JSON-encoded. Replayable
// GET and DELETE requests retry selected transient failures; JSON mutations do
// not retry because repeating them could duplicate an operation.
func (c *Client) Request(ctx context.Context, method, pathOrURL string, body any) (json.RawMessage, error) {
	fullURL := c.buildURL(pathOrURL)
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		var reqBody io.Reader
		if encoded != nil {
			reqBody = bytes.NewReader(encoded)
		}
		req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
		if err != nil {
			return nil, redactError(err, c.token)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		if attempt == 0 && c.verbose && c.logw != nil {
			fmt.Fprintf(c.logw, "%s %s\n", method, redactSecret(displayPath(pathOrURL), c.token))
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			if c.retriesEnabled(method) && retryableNetworkError(err) && attempt < c.retry.MaxRetries {
				if retryErr := c.waitForRetry(ctx, method, pathOrURL, attempt+1, ""); retryErr != nil {
					return nil, retryErr
				}
				continue
			}
			return nil, redactError(err, c.token)
		}

		payload, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if c.retriesEnabled(method) && retryableNetworkError(readErr) && attempt < c.retry.MaxRetries {
				if retryErr := c.waitForRetry(ctx, method, pathOrURL, attempt+1, ""); retryErr != nil {
					return nil, retryErr
				}
				continue
			}
			return nil, redactError(readErr, c.token)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return json.RawMessage(payload), nil
		}
		if c.retriesEnabled(method) && retryableStatus(resp.StatusCode) && attempt < c.retry.MaxRetries {
			if retryErr := c.waitForRetry(ctx, method, pathOrURL, attempt+1, resp.Header.Get("Retry-After")); retryErr != nil {
				return nil, retryErr
			}
			continue
		}
		return nil, &HTTPError{
			Method:          method,
			URL:             redactSecret(safeURL(fullURL), c.token),
			Path:            redactSecret(displayPath(pathOrURL), c.token),
			Status:          resp.StatusCode,
			StatusText:      http.StatusText(resp.StatusCode),
			ResponseExcerpt: redactSecret(excerpt(payload), c.token),
		}
	}
}

// Download performs a GET and streams the response body to w. It authenticates
// only relative paths and absolute URLs sharing the configured API origin. It
// is intended for attachment download_url values and does not JSON-decode
// successful responses.
func (c *Client) Download(ctx context.Context, pathOrURL string, w io.Writer) (int64, error) {
	fullURL := c.buildURL(pathOrURL)
	target, err := url.Parse(fullURL)
	if err != nil {
		return 0, redactError(err, c.token)
	}
	if !c.shouldAuthenticate(pathOrURL) && !isHTTPS(target) {
		return 0, fmt.Errorf("refusing download from an external non-HTTPS URL")
	}

	safePath := displayPath(pathOrURL)
	if c.verbose && c.logw != nil {
		fmt.Fprintf(c.logw, "%s %s\n", http.MethodGet, redactSecret(safePath, c.token))
	}

	// net/http normally strips Authorization on cross-origin redirects. Wrap
	// the client's redirect policy as defense in depth, including for clients
	// with a custom CheckRedirect callback.
	httpClient := *c.httpClient
	previousRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(redirectReq *http.Request, via []*http.Request) error {
		trusted := c.shouldAuthenticate(redirectReq.URL.String())
		if !trusted && !isHTTPS(redirectReq.URL) {
			return fmt.Errorf("refusing redirect to an external non-HTTPS URL")
		}
		if !trusted {
			redirectReq.Header.Del("Authorization")
		}
		var redirectErr error
		if previousRedirect != nil {
			redirectErr = previousRedirect(redirectReq, via)
		}
		// A caller-supplied callback must not be able to accidentally restore
		// credentials on an untrusted redirect.
		if !trusted {
			redirectReq.Header.Del("Authorization")
		}
		return redirectErr
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			return 0, redactError(err, c.token)
		}
		req.Header.Set("Accept", "application/octet-stream")
		if c.shouldAuthenticate(pathOrURL) {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			if c.retriesEnabled(http.MethodGet) && retryableNetworkError(err) && attempt < c.retry.MaxRetries {
				if retryErr := c.waitForRetry(ctx, http.MethodGet, pathOrURL, attempt+1, ""); retryErr != nil {
					return 0, retryErr
				}
				continue
			}
			return 0, redactError(err, c.token)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			written, copyErr := io.Copy(w, resp.Body)
			resp.Body.Close()
			return written, copyErr
		}

		payload, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if c.retriesEnabled(http.MethodGet) && retryableNetworkError(readErr) && attempt < c.retry.MaxRetries {
				if retryErr := c.waitForRetry(ctx, http.MethodGet, pathOrURL, attempt+1, ""); retryErr != nil {
					return 0, retryErr
				}
				continue
			}
			return 0, redactError(readErr, c.token)
		}
		if c.retriesEnabled(http.MethodGet) && retryableStatus(resp.StatusCode) && attempt < c.retry.MaxRetries {
			if retryErr := c.waitForRetry(ctx, http.MethodGet, pathOrURL, attempt+1, resp.Header.Get("Retry-After")); retryErr != nil {
				return 0, retryErr
			}
			continue
		}
		return 0, &HTTPError{
			Method:          http.MethodGet,
			URL:             redactSecret(safeURL(fullURL), c.token),
			Path:            redactSecret(safePath, c.token),
			Status:          resp.StatusCode,
			StatusText:      http.StatusText(resp.StatusCode),
			ResponseExcerpt: redactSecret(excerpt(payload), c.token),
		}
	}
}

// Upload performs an authenticated multipart/form-data POST, streaming file
// content from r as the form field named fileField (with the given fileName).
// Additional simple text fields are sent from the fields map. It returns the
// raw response body; a non-2xx status yields an *HTTPError. Multipart uploads
// are intentionally one-shot and are never retried because the reader may not
// be replayable and repeating an upload can create duplicate attachments.
func (c *Client) Upload(ctx context.Context, pathOrURL string, fields map[string]string, fileField, fileName string, r io.Reader) (json.RawMessage, error) {
	fullURL := c.buildURL(pathOrURL)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("write form field %s: %w", k, err)
		}
	}
	part, err := mw.CreateFormFile(fileField, fileName)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, fmt.Errorf("copy file content: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("finalize multipart body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, &buf)
	if err != nil {
		return nil, redactError(err, c.token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if c.verbose && c.logw != nil {
		fmt.Fprintf(c.logw, "%s %s\n", http.MethodPost, redactSecret(displayPath(pathOrURL), c.token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, redactError(err, c.token)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, redactError(err, c.token)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{
			Method:          http.MethodPost,
			URL:             redactSecret(safeURL(fullURL), c.token),
			Path:            redactSecret(displayPath(pathOrURL), c.token),
			Status:          resp.StatusCode,
			StatusText:      http.StatusText(resp.StatusCode),
			ResponseExcerpt: redactSecret(excerpt(payload), c.token),
		}
	}

	return json.RawMessage(payload), nil
}

// PageResult contains collection items and enough state to inspect or resume
// pagination. NextPath and NextOffset describe the next page advertised by
// Asana when traversal stopped before the collection was exhausted.
type PageResult struct {
	Items        []json.RawMessage
	PagesFetched int
	NextOffset   string
	NextPath     string
	Truncated    bool
}

// page is the envelope returned by Asana collection endpoints.
type page struct {
	Data     []json.RawMessage `json:"data"`
	NextPage *struct {
		Offset string `json:"offset"`
		Path   string `json:"path"`
		URI    string `json:"uri"`
	} `json:"next_page"`
}

// withOffset adds or replaces an offset query parameter without discarding
// query parameters already present on a page URL.
func withOffset(pathOrURL, offset string) string {
	if offset == "" {
		return pathOrURL
	}
	u, err := url.Parse(pathOrURL)
	if err != nil {
		return pathOrURL
	}
	q := u.Query()
	q.Set("offset", offset)
	u.RawQuery = q.Encode()
	return u.String()
}

func withLimit(pathOrURL string, limit int) string {
	if limit <= 0 {
		return pathOrURL
	}
	u, err := url.Parse(pathOrURL)
	if err != nil {
		return pathOrURL
	}
	q := u.Query()
	q.Set("limit", strconv.Itoa(limit))
	u.RawQuery = q.Encode()
	return u.String()
}

func nextPage(current string, p *struct {
	Offset string `json:"offset"`
	Path   string `json:"path"`
	URI    string `json:"uri"`
}) (path, offset string) {
	if p == nil {
		return "", ""
	}
	offset = p.Offset
	switch {
	case p.URI != "":
		path = p.URI
	case p.Path != "":
		path = p.Path
	case offset != "":
		path = withOffset(current, offset)
	}
	return path, offset
}

// Paginate follows next_page links. A non-positive limit means unlimited
// items, and a non-positive maxPages means unlimited pages. The result marks
// intentional or safety-bound partial results as truncated and exposes a
// resumable next path/offset when Asana supplied one.
func (c *Client) Paginate(ctx context.Context, pathOrURL string, limit, maxPages int) (PageResult, error) {
	const requestPageSize = 50
	result := PageResult{Items: make([]json.RawMessage, 0)}
	next := pathOrURL

	for next != "" && (limit <= 0 || len(result.Items) < limit) && (maxPages <= 0 || result.PagesFetched < maxPages) {
		requestPath := next
		if limit > 0 {
			remaining := limit - len(result.Items)
			requestSize := requestPageSize
			if remaining < requestSize {
				requestSize = remaining
			}
			requestPath = withLimit(next, requestSize)
		}
		raw, err := c.Request(ctx, http.MethodGet, requestPath, nil)
		if err != nil {
			return PageResult{}, err
		}
		var p page
		if err := json.Unmarshal(raw, &p); err != nil {
			return PageResult{}, fmt.Errorf("decode page: %w", err)
		}

		result.Items = append(result.Items, p.Data...)
		result.PagesFetched++
		next, result.NextOffset = nextPage(requestPath, p.NextPage)
		hasNext := next != ""

		if limit > 0 && len(result.Items) >= limit {
			if len(result.Items) > limit || hasNext {
				result.Truncated = true
			}
			result.Items = result.Items[:limit]
			if !hasNext {
				result.NextOffset = ""
			}
			break
		}
		if maxPages > 0 && result.PagesFetched >= maxPages && hasNext {
			result.Truncated = true
			break
		}
		if !hasNext {
			break
		}
	}

	if !result.Truncated {
		result.NextOffset = ""
		result.NextPath = ""
	} else {
		result.NextPath = next
	}
	return result, nil
}
