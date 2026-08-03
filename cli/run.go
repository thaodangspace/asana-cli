package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
	"github.com/thaodangspace/asana-cli/config"
)

// pageSize is the per-request page size used for paginated endpoints.
const pageSize = 50

// maxPages is the default safety bound for the existing bounded behavior.
const maxPages = 10

type paginationOptions struct {
	limit    int
	all      bool
	offset   string
	maxPages int
}

func (p *paginationOptions) addFlags(cmd *cobra.Command, defaultLimit int) {
	cmd.Flags().IntVar(&p.limit, "limit", defaultLimit, "maximum items to return")
	cmd.Flags().BoolVar(&p.all, "all", false, "return all items (cannot be combined with an explicit --limit)")
	cmd.Flags().StringVar(&p.offset, "offset", "", "start from an Asana pagination offset")
	cmd.Flags().IntVar(&p.maxPages, "max-pages", maxPages, "maximum pages to fetch (1 or more; --all is unlimited unless set)")
}

func (p *paginationOptions) validate(cmd *cobra.Command, maximum int) (int, error) {
	if p.all && cmd.Flags().Changed("limit") {
		return 0, usageErrorf("--all cannot be combined with --limit")
	}
	if p.maxPages < 1 {
		return 0, usageErrorf("--max-pages must be at least 1, got %d", p.maxPages)
	}
	if p.all {
		return 0, nil
	}
	if p.limit < 1 || p.limit > maximum {
		return 0, usageErrorf("--limit must be between 1 and %d, got %d", maximum, p.limit)
	}
	return p.limit, nil
}

func paginationPageLimit(cmd *cobra.Command, p *paginationOptions) int {
	if p.all && !cmd.Flags().Changed("max-pages") {
		return 0
	}
	return p.maxPages
}

// buildClient loads config and constructs an Asana client honoring the
// persistent flags. Config failures are usage errors (exit code 2).
//
// ASANA_API_BASE overrides the API base URL; it exists only to point tests at
// an httptest server and is not a documented user-facing flag.
func buildClient() (*asana.Client, config.Config, error) {
	if opts.maxRetries < 0 {
		return nil, config.Config{}, usageErrorf("--max-retries must be zero or greater, got %d", opts.maxRetries)
	}
	if opts.retryMaxWait <= 0 {
		return nil, config.Config{}, usageErrorf("--retry-max-wait must be greater than zero, got %s", opts.retryMaxWait)
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, config.Config{}, &usageError{err: err}
	}

	httpClient := &http.Client{Timeout: opts.timeout}
	var copts []asana.Option
	if base := strings.TrimSpace(os.Getenv("ASANA_API_BASE")); base != "" {
		copts = append(copts, asana.WithBaseURL(base))
	}
	if opts.verbose {
		copts = append(copts, asana.WithVerbose(os.Stderr))
	}
	copts = append(copts, asana.WithRetryConfig(asana.RetryConfig{
		MaxRetries: opts.maxRetries,
		MaxWait:    opts.retryMaxWait,
		Disabled:   opts.noRetry,
	}))
	return asana.NewClient(cfg.AccessToken, httpClient, copts...), cfg, nil
}

// withTimeout derives a context bounded by the --timeout flag.
func withTimeout(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	return context.WithTimeout(cmd.Context(), opts.timeout)
}

// requireFlag returns a trimmed required string value or a usage error.
func requireFlag(name, value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", usageErrorf("--%s is required", name)
	}
	return v, nil
}

// query helpers

func appendOptFields(q url.Values, optFields string) {
	if v := strings.TrimSpace(optFields); v != "" {
		q.Set("opt_fields", v)
	}
}

func querySuffix(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// requestData performs a request and unwraps the top-level "data" field.
func requestData(ctx context.Context, c *asana.Client, method, path string, body any) (json.RawMessage, error) {
	raw, err := c.Request(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return env.Data, nil
}
