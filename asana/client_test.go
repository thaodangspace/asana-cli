package asana

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient("secret-token", srv.Client(), WithBaseURL(srv.URL))
}

func TestRequestSuccessAndHeaders(t *testing.T) {
	var gotAuth, gotAccept string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"1"}}`))
	})

	raw, err := c.Request(context.Background(), http.MethodGet, "/users/me", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if !strings.Contains(string(raw), `"gid":"1"`) {
		t.Errorf("body = %s", raw)
	}
}

func TestRequestPostBody(t *testing.T) {
	var body, ctype string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"99"}}`))
	})

	_, err := c.Request(context.Background(), http.MethodPost, "/tasks/1/stories", map[string]any{"data": map[string]string{"text": "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctype != "application/json" {
		t.Errorf("Content-Type = %q", ctype)
	}
	if !strings.Contains(body, `"text":"hi"`) {
		t.Errorf("body = %q", body)
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{401, "Asana authentication failed. Check ASANA_ACCESS_TOKEN."},
		{402, "Asana API access requires a premium workspace or feature for this request."},
		{403, "Asana authorization failed. Check token scopes and resource permissions."},
		{404, "Asana resource not found. Check workspace_gid, task_gid, project_gid, and IDs."},
		{429, "Asana rate limit reached. Retry later."},
	}
	for _, tt := range tests {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
			w.Write([]byte(`{"errors":[{"message":"x"}]}`))
		})
		_, err := c.Request(context.Background(), http.MethodGet, "/x", nil)
		if err == nil || err.Error() != tt.want {
			t.Errorf("status %d: got %v, want %q", tt.status, err, tt.want)
		}
		var he *HTTPError
		if !asHTTPError(err, &he) || he.Status != tt.status {
			t.Errorf("expected *HTTPError with status %d, got %v", tt.status, err)
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Errorf("token leaked into error: %v", err)
		}
	}
}

func TestErrorGenericExcerpt(t *testing.T) {
	big := strings.Repeat("a", 600)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(big))
	})
	_, err := c.Request(context.Background(), http.MethodGet, "/x", nil)
	var he *HTTPError
	if !asHTTPError(err, &he) {
		t.Fatalf("expected HTTPError, got %v", err)
	}
	if len(he.ResponseExcerpt) != 503 || !strings.HasSuffix(he.ResponseExcerpt, "...") {
		t.Errorf("excerpt len = %d, suffix ok = %v", len(he.ResponseExcerpt), strings.HasSuffix(he.ResponseExcerpt, "..."))
	}
}

func TestPaginateFollowsNextPageAndCaps(t *testing.T) {
	hits := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/page1":
			w.Write([]byte(`{"data":[{"gid":"1"},{"gid":"2"}],"next_page":{"path":"/page2"}}`))
		case "/page2":
			w.Write([]byte(`{"data":[{"gid":"3"},{"gid":"4"}],"next_page":null}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})

	got, err := c.Paginate(context.Background(), "/page1", 3, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 3 {
		t.Fatalf("len = %d, want 3 (limit cap)", len(got.Items))
	}
	if !got.Truncated || got.NextPath != "" {
		t.Errorf("pagination = %+v, want truncated without a resumable next page", got)
	}
	if hits != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
}

func TestPaginateUsesRemainingLimit(t *testing.T) {
	var gotLimit int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotLimit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"1"}],"next_page":null}`))
	})
	got, err := c.Paginate(context.Background(), "/items?limit=50", 20, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != 20 || len(got.Items) != 1 {
		t.Errorf("request limit=%d result=%+v, want limit 20", gotLimit, got)
	}
}

func TestPaginateMaxPages(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// always advertises a next page → bounded by maxPages
		w.Write([]byte(`{"data":[{"gid":"x"}],"next_page":{"path":"/loop"}}`))
	})
	got, err := c.Paginate(context.Background(), "/loop", 100, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 3 {
		t.Errorf("len = %d, want 3 (maxPages)", len(got.Items))
	}
	if !got.Truncated || got.NextPath != "/loop" {
		t.Errorf("pagination = %+v, want truncated with /loop", got)
	}
}

func TestPaginateFollowsMoreThanTenPagesWhenUnlimited(t *testing.T) {
	hits := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits < 12 {
			w.Write([]byte(`{"data":[{"gid":"x"}],"next_page":{"offset":"next-` + strconv.Itoa(hits) + `"}}`))
			return
		}
		w.Write([]byte(`{"data":[{"gid":"last"}],"next_page":null}`))
	})

	got, err := c.Paginate(context.Background(), "/items?limit=50&filter=x", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 12 || got.PagesFetched != 12 || len(got.Items) != 12 || got.Truncated {
		t.Errorf("hits=%d result=%+v, want 12 complete pages", hits, got)
	}
}

func TestPaginateOffsetOnlyPreservesQuery(t *testing.T) {
	var gotQuery string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"1"}],"next_page":{"offset":"a token"}}`))
	})

	got, err := c.Paginate(context.Background(), "/items?limit=50&filter=x", 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	initial, initialErr := url.ParseQuery(gotQuery)
	if initialErr != nil || initial.Get("filter") != "x" || initial.Get("limit") != "50" {
		t.Errorf("initial query = %q", gotQuery)
	}
	u, parseErr := url.Parse(got.NextPath)
	if !got.Truncated || got.NextOffset != "a token" || parseErr != nil || u.Query().Get("filter") != "x" || u.Query().Get("limit") != "50" || u.Query().Get("offset") != "a token" {
		t.Errorf("pagination = %+v, want preserved query and offset", got)
	}
}

func TestBuildURL(t *testing.T) {
	c := NewClient("t", nil, WithBaseURL("https://api.example/x"))
	cases := map[string]string{
		"/users/me":                 "https://api.example/x/users/me",
		"users/me":                  "https://api.example/x/users/me",
		"https://other.example/abs": "https://other.example/abs",
		"http://other.example/abs":  "http://other.example/abs",
	}
	for in, want := range cases {
		if got := c.buildURL(in); got != want {
			t.Errorf("buildURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShouldAuthenticateUsesNormalizedOrigin(t *testing.T) {
	c := NewClient("secret-token", nil, WithBaseURL("https://API.example:443/api/1.0"))
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"relative", "/attachments/1", true},
		{"same origin", "https://api.example/download?signature=secret", true},
		{"different port", "https://api.example:444/download", false},
		{"different scheme", "http://api.example/download", false},
		{"lookalike hostname", "https://api.example.evil/download", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.shouldAuthenticate(tt.target); got != tt.want {
				t.Errorf("shouldAuthenticate(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestDownloadAbsoluteSameOriginIncludesAuth(t *testing.T) {
	var gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte("file-bytes"))
	})

	var out bytes.Buffer
	url := c.baseURL + "/download?signature=not-logged"
	if _, err := c.Download(context.Background(), url, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

func TestDownloadCrossOriginHTTPSOmitsAuth(t *testing.T) {
	var gotAuth string
	fileServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte("file-bytes"))
	}))
	t.Cleanup(fileServer.Close)

	apiServer := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(apiServer.Close)
	c := NewClient("secret-token", fileServer.Client(), WithBaseURL(apiServer.URL))

	var out bytes.Buffer
	if _, err := c.Download(context.Background(), fileServer.URL+"/download?signature=not-logged", &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("cross-origin Authorization = %q", gotAuth)
	}
	if out.String() != "file-bytes" {
		t.Errorf("body = %q", out.String())
	}
}

func TestDownloadExternalHTTPRejectedBeforeRequest(t *testing.T) {
	hits := 0
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	t.Cleanup(external.Close)
	c := NewClient("secret-token", external.Client(), WithBaseURL("https://api.example"))

	var out bytes.Buffer
	if _, err := c.Download(context.Background(), external.URL+"/download", &out); err == nil {
		t.Fatal("expected external HTTP URL to be rejected")
	}
	if hits != 0 {
		t.Errorf("external server received %d requests", hits)
	}
}

func TestDownloadRedirectOmitsAuthOnCrossOrigin(t *testing.T) {
	var gotAuth string
	external := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte("file-bytes"))
	}))
	t.Cleanup(external.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Errorf("source Authorization = %q", got)
		}
		http.Redirect(w, r, external.URL+"/download", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	c := NewClient("secret-token", external.Client(), WithBaseURL(source.URL))
	var out bytes.Buffer
	if _, err := c.Download(context.Background(), "/download", &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("redirected cross-origin Authorization = %q", gotAuth)
	}
}

func TestDownloadVerboseLogOmitsQuery(t *testing.T) {
	var log bytes.Buffer
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("failed"))
	})
	c.verbose = true
	c.logw = &log

	var out bytes.Buffer
	_, _ = c.Download(context.Background(), "/download?signature=secret-token", &out)
	if strings.Contains(log.String(), "?") || strings.Contains(log.String(), "secret-token") {
		t.Errorf("unsafe verbose log = %q", log.String())
	}
}

func TestDownloadSuccessAndHeaders(t *testing.T) {
	var gotAuth, gotAccept string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		w.Write([]byte("file-bytes"))
	})

	var out bytes.Buffer
	written, err := c.Download(context.Background(), "/download", &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if written != int64(len("file-bytes")) || out.String() != "file-bytes" {
		t.Errorf("written = %d body = %q", written, out.String())
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotAccept != "application/octet-stream" {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestDownloadHTTPErrorUsesSafePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("failed"))
	}))
	t.Cleanup(srv.Close)
	c := NewClient("secret-token", srv.Client(), WithBaseURL(srv.URL))

	var out bytes.Buffer
	_, err := c.Download(context.Background(), srv.URL+"/download?signature=secret", &out)
	var he *HTTPError
	if !asHTTPError(err, &he) {
		t.Fatalf("expected HTTPError, got %v", err)
	}
	if he.Path != "/download" {
		t.Errorf("path = %q", he.Path)
	}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(he.Path, "signature=secret") || strings.Contains(he.URL, "signature=secret") {
		t.Errorf("sensitive value leaked into error/path: %v url=%q path=%q", err, he.URL, he.Path)
	}
}

func TestEncodePathSegment(t *testing.T) {
	if got := EncodePathSegment("12345"); got != "12345" {
		t.Errorf("got %q", got)
	}
	if got := EncodePathSegment("a/b"); !strings.Contains(got, "%2F") && got == "a/b" {
		t.Errorf("expected slash to be escaped, got %q", got)
	}
}

// helpers

func asHTTPError(err error, target **HTTPError) bool {
	he, ok := err.(*HTTPError)
	if ok {
		*target = he
	}
	return ok
}

func TestRequestRetries429AndHonorsCappedRetryAfter(t *testing.T) {
	hits := 0
	var waits []time.Duration
	c := NewClient("secret-token", nil,
		WithBaseURL("http://example.test"),
		WithRetryConfig(RetryConfig{MaxRetries: 2, MaxWait: 500 * time.Millisecond}),
		WithSleeper(func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		}),
	)
	c.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		hits++
		if hits == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"10"}},
				Body:       io.NopCloser(strings.NewReader("busy")),
				Request:    req,
			}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{}}`)), Request: req}, nil
	})}

	if _, err := c.Request(context.Background(), http.MethodGet, "/items?signature=secret-token", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 2 || len(waits) != 1 || waits[0] != 500*time.Millisecond {
		t.Errorf("hits=%d waits=%v, want one capped retry", hits, waits)
	}
}

func TestRequestRetries503WithBackoffAndExhausts(t *testing.T) {
	hits := 0
	var waits []time.Duration
	c := NewClient("secret-token", nil,
		WithBaseURL("http://example.test"),
		WithRetryConfig(RetryConfig{MaxRetries: 2, MaxWait: time.Second}),
		WithSleeper(func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		}),
	)
	c.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		hits++
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("down")), Request: req}, nil
	})}

	_, err := c.Request(context.Background(), http.MethodGet, "/items", nil)
	var httpErr *HTTPError
	if !asHTTPError(err, &httpErr) || httpErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("expected final 503, got %v", err)
	}
	if hits != 3 || len(waits) != 2 {
		t.Errorf("hits=%d waits=%v, want initial plus two retries", hits, waits)
	}
	if waits[1] <= waits[0] {
		t.Errorf("backoff did not increase: %v", waits)
	}
}

func TestRequestStopsWhenCanceledDuringBackoff(t *testing.T) {
	hits := 0
	c := NewClient("secret-token", nil,
		WithBaseURL("http://example.test"),
		WithRetryConfig(RetryConfig{MaxRetries: 3, MaxWait: time.Second}),
		WithSleeper(func(ctx context.Context, _ time.Duration) error { return ctx.Err() }),
	)
	c.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		hits++
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("down")), Request: req}, nil
	})}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Request(ctx, http.MethodGet, "/items", nil)
	if err != context.Canceled {
		t.Errorf("error = %v, want context canceled", err)
	}
	if hits != 1 {
		t.Errorf("hits = %d, want one request", hits)
	}
}

func TestRequestDoesNotRetryNonRetryableOrMutatingFailures(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		hits := 0
		c := NewClient("secret-token", nil,
			WithBaseURL("http://example.test"),
			WithRetryConfig(RetryConfig{MaxRetries: 3, MaxWait: time.Second}),
			WithSleeper(func(context.Context, time.Duration) error { t.Fatal("unexpected sleep"); return nil }),
		)
		c.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			hits++
			return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("bad")), Request: req}, nil
		})}
		_, err := c.Request(context.Background(), method, "/items", nil)
		if err == nil || hits != 1 {
			t.Errorf("method=%s err=%v hits=%d, want one request", method, err, hits)
		}
	}
}

func TestRetryAfterParsesSecondsAndHTTPDate(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c := NewClient("t", nil, WithRetryConfig(RetryConfig{MaxRetries: 1, MaxWait: time.Minute}), WithClock(func() time.Time { return now }))
	if got, ok := c.retryAfter("7"); !ok || got != 7*time.Second {
		t.Errorf("seconds = %v, %v", got, ok)
	}
	if got, ok := c.retryAfter(now.Add(9 * time.Second).Format(http.TimeFormat)); !ok || got != 9*time.Second {
		t.Errorf("date = %v, %v", got, ok)
	}
	if _, ok := c.retryAfter("not-a-delay"); ok {
		t.Error("invalid Retry-After parsed successfully")
	}
}

func TestUploadIsNotRetried(t *testing.T) {
	hits := 0
	c := NewClient("secret-token", nil, WithBaseURL("http://example.test"), WithRetryConfig(RetryConfig{MaxRetries: 3, MaxWait: time.Second}))
	c.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		hits++
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("down")), Request: req}, nil
	})}
	_, err := c.Upload(context.Background(), "/attachments", map[string]string{"parent": "1"}, "file", "x.txt", strings.NewReader("data"))
	if err == nil || hits != 1 {
		t.Errorf("err=%v hits=%d, want one upload request", err, hits)
	}
}

func TestVerboseRetryLogIsSafe(t *testing.T) {
	var log bytes.Buffer
	hits := 0
	c := NewClient("secret-token", nil,
		WithBaseURL("http://example.test"), WithVerbose(&log),
		WithRetryConfig(RetryConfig{MaxRetries: 1, MaxWait: time.Second}),
		WithSleeper(func(context.Context, time.Duration) error { return nil }),
	)
	c.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		hits++
		if hits == 1 {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("busy")), Request: req}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	})}
	_, _ = c.Request(context.Background(), http.MethodGet, "/items?token=secret-token", nil)
	if strings.Contains(log.String(), "secret-token") || strings.Contains(log.String(), "?") || !strings.Contains(log.String(), "attempt 1/1") {
		t.Errorf("unsafe or incomplete retry log: %q", log.String())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
