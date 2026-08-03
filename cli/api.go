package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

// maxAPIDataBytes bounds --data files and inline request bodies so an escape
// hatch cannot accidentally buffer an unbounded local file in memory.
const maxAPIDataBytes = 10 << 20

var apiMethods = map[string]string{
	http.MethodGet:    http.MethodGet,
	http.MethodPost:   http.MethodPost,
	http.MethodPut:    http.MethodPut,
	http.MethodPatch:  http.MethodPatch,
	http.MethodDelete: http.MethodDelete,
}

func newAPICommand() *cobra.Command {
	var (
		queries   []string
		data      string
		rawResult bool
	)

	cmd := &cobra.Command{
		Use:   "api METHOD PATH",
		Short: "Call an Asana API endpoint not covered by a first-class command",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageErrorf("api requires METHOD and PATH")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			method, ok := apiMethods[strings.ToUpper(args[0])]
			if !ok {
				return usageErrorf("unsupported API method %q (must be GET, POST, PUT, PATCH, or DELETE)", args[0])
			}

			apiPath, err := apiPathWithQuery(args[1], queries)
			if err != nil {
				return err
			}
			body, err := apiRequestBody(cmd, data)
			if err != nil {
				return err
			}

			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			raw, err := c.Request(ctx, method, apiPath, body)
			if err != nil {
				return sanitizeAPIError(err)
			}
			response, err := decodeAPIResponse(raw, rawResult)
			if err != nil {
				return err
			}
			return writeSuccess(cmd.OutOrStdout(), response, opts.human, "API request succeeded.")
		},
	}
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query parameter in key=value form (repeatable)")
	cmd.Flags().StringVar(&data, "data", "", "JSON request body, or @FILE (maximum 10 MiB)")
	cmd.Flags().BoolVar(&rawResult, "raw-response", false, "return the complete decoded Asana response envelope")
	return cmd
}

// apiPathWithQuery accepts only a path relative to the configured API base.
// Query values are rebuilt with net/url so duplicate keys and reserved
// characters are encoded correctly.
func apiPathWithQuery(rawPath string, queries []string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", usageErrorf("API PATH is required")
	}
	if strings.Contains(rawPath, "\\") {
		return "", usageErrorf("API PATH must use forward slashes")
	}

	u, err := url.Parse(rawPath)
	if err != nil {
		return "", usageErrorf("invalid API PATH: %v", err)
	}
	if u.IsAbs() || u.Host != "" || u.User != nil || u.Opaque != "" {
		return "", usageErrorf("API PATH must be a relative path, not an absolute URL")
	}
	if u.Fragment != "" {
		return "", usageErrorf("API PATH must not contain a fragment")
	}

	decodedPath, err := url.PathUnescape(u.EscapedPath())
	if err != nil {
		return "", usageErrorf("invalid API PATH escaping: %v", err)
	}
	// Dot segments could escape the configured /api/1.0 base when the URL is
	// resolved by a proxy. Reject them rather than relying on URL cleaning.
	for _, segment := range strings.Split(decodedPath, "/") {
		if segment == "." || segment == ".." {
			return "", usageErrorf("API PATH must not contain dot segments")
		}
	}

	path := u.EscapedPath()
	if path == "" {
		path = "/"
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", usageErrorf("invalid API query: %v", err)
	}
	for _, item := range queries {
		separator := strings.IndexByte(item, '=')
		if separator <= 0 {
			return "", usageErrorf("--query must use key=value form, got %q", item)
		}
		key := strings.TrimSpace(item[:separator])
		if key == "" {
			return "", usageErrorf("--query key must not be empty")
		}
		q.Add(key, item[separator+1:])
	}
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path, nil
}

func apiRequestBody(cmd *cobra.Command, value string) (json.RawMessage, error) {
	if !cmd.Flags().Changed("data") {
		return nil, nil
	}

	var body []byte
	if strings.HasPrefix(value, "@") {
		filename := strings.TrimPrefix(value, "@")
		if filename == "" {
			return nil, usageErrorf("--data @FILE requires a file path")
		}
		f, err := os.Open(filename)
		if err != nil {
			return nil, usageErrorf("read --data file: %v", err)
		}
		body, err = io.ReadAll(io.LimitReader(f, maxAPIDataBytes+1))
		closeErr := f.Close()
		if err != nil {
			return nil, usageErrorf("read --data file: %v", err)
		}
		if closeErr != nil {
			return nil, usageErrorf("read --data file: %v", closeErr)
		}
	} else {
		body = []byte(value)
	}
	if len(body) > maxAPIDataBytes {
		return nil, usageErrorf("--data must be at most %d MiB", maxAPIDataBytes/(1<<20))
	}
	if !json.Valid(body) {
		return nil, usageErrorf("--data must contain valid JSON")
	}
	return json.RawMessage(body), nil
}

// sanitizeAPIError keeps the standard HTTP error metadata but omits the
// response excerpt. A low-level endpoint may echo a request body containing
// secrets, so API errors must not turn that body into CLI output.
func sanitizeAPIError(err error) error {
	var httpErr *asana.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}
	safe := *httpErr
	safe.ResponseExcerpt = ""
	return &safe
}

func decodeAPIResponse(raw json.RawMessage, rawResult bool) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("decode response: invalid JSON")
	}
	if rawResult {
		return raw, nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if envelope.Data == nil {
		return nil, nil
	}
	return envelope.Data, nil
}
