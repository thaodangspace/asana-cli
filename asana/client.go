// Package asana is a thin HTTP client for the Asana REST API, ported from the
// Pi extension's AsanaClient (request, error mapping, pagination).
package asana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DefaultBaseURL is the Asana REST API v1.0 root.
const DefaultBaseURL = "https://app.asana.com/api/1.0"

// Client performs authenticated requests against the Asana API.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	verbose    bool
	logw       io.Writer
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

// Request performs an HTTP request and returns the raw response body. A non-2xx
// status yields an *HTTPError. body, when non-nil, is JSON-encoded.
func (c *Client) Request(ctx context.Context, method, pathOrURL string, body any) (json.RawMessage, error) {
	fullURL := c.buildURL(pathOrURL)

	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.verbose && c.logw != nil {
		fmt.Fprintf(c.logw, "%s %s\n", method, redactSecret(displayPath(pathOrURL), c.token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{
			Method:          method,
			URL:             redactSecret(safeURL(fullURL), c.token),
			Path:            redactSecret(displayPath(pathOrURL), c.token),
			Status:          resp.StatusCode,
			StatusText:      http.StatusText(resp.StatusCode),
			ResponseExcerpt: redactSecret(excerpt(payload), c.token),
		}
	}

	return json.RawMessage(payload), nil
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return 0, redactError(err, c.token)
	}
	req.Header.Set("Accept", "application/octet-stream")
	if c.shouldAuthenticate(pathOrURL) {
		req.Header.Set("Authorization", "Bearer "+c.token)
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

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, redactError(err, c.token)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return 0, readErr
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

	return io.Copy(w, resp.Body)
}

// Upload performs an authenticated multipart/form-data POST, streaming file
// content from r as the form field named fileField (with the given fileName).
// Additional simple text fields are sent from the fields map. It returns the
// raw response body; a non-2xx status yields an *HTTPError. The token is never
// logged.
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
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	if c.verbose && c.logw != nil {
		fmt.Fprintf(c.logw, "%s %s\n", http.MethodPost, redactSecret(displayPath(pathOrURL), c.token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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

// page is the envelope returned by Asana collection endpoints.
type page struct {
	Data     []json.RawMessage `json:"data"`
	NextPage *struct {
		Offset string `json:"offset"`
		Path   string `json:"path"`
		URI    string `json:"uri"`
	} `json:"next_page"`
}

// Paginate follows next_page links, accumulating up to limit elements across at
// most maxPages requests. Mirrors the extension's paginate semantics.
func (c *Client) Paginate(ctx context.Context, pathOrURL string, limit, maxPages int) ([]json.RawMessage, error) {
	var values []json.RawMessage
	next := pathOrURL
	pages := 0

	for next != "" && len(values) < limit && pages < maxPages {
		raw, err := c.Request(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		var p page
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode page: %w", err)
		}
		values = append(values, p.Data...)

		next = ""
		if p.NextPage != nil {
			if p.NextPage.URI != "" {
				next = p.NextPage.URI
			} else {
				next = p.NextPage.Path
			}
		}
		pages++
	}

	if len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}
