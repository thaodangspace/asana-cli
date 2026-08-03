package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestAPICommandSupportsAllMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var gotMethod, gotPath, gotAuth string
			out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"data":{"method":"` + method + `"}}`))
			}, "api", method, "/tasks/123")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMethod != method || gotPath != "/tasks/123" || gotAuth != "Bearer tok" {
				t.Errorf("request = %s %s auth=%q", gotMethod, gotPath, gotAuth)
			}
			var data struct {
				Method string `json:"method"`
			}
			decodeData(t, out, &data)
			if data.Method != method {
				t.Errorf("data method = %q", data.Method)
			}
		})
	}
}

func TestAPICommandEncodesDuplicateQueries(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q["filter"]; len(got) != 2 || got[0] != "one two" || got[1] != "two&three" {
			t.Errorf("filter query = %#v", got)
		}
		if q.Get("redirect") != "https://example.test/a?b=c" {
			t.Errorf("redirect query = %q", q.Get("redirect"))
		}
		w.Write([]byte(`{"data":null}`))
	}, "api", "GET", "/tasks/search", "--query", "filter=one two", "--query", "filter=two&three", "--query", "redirect=https://example.test/a?b=c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var data any
	decodeData(t, out, &data)
}

func TestAPICommandInlineAndFileData(t *testing.T) {
	file := t.TempDir() + "/body.json"
	if err := os.WriteFile(file, []byte(`{"data":{"name":"from-file"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	bodies := []string{`{"data":{"name":"inline"}}`, "@" + file}
	for _, bodyArg := range bodies {
		t.Run(bodyArg, func(t *testing.T) {
			var got map[string]any
			out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("content type = %q", r.Header.Get("Content-Type"))
				}
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
				}
				w.Write([]byte(`{"data":{"ok":true}}`))
			}, "api", "POST", "/tasks", "--data", bodyArg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got["data"] == nil {
				t.Errorf("request body = %#v", got)
			}
			var data struct {
				OK bool `json:"ok"`
			}
			decodeData(t, out, &data)
			if !data.OK {
				t.Error("response data ok = false")
			}
		})
	}
}

func TestAPICommandRejectsInvalidDataBeforeRequest(t *testing.T) {
	hits := 0
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
	}, "api", "POST", "/tasks", "--data", `{"data":`)
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("error = %v, exit = %d", err, exitCodeFor(err))
	}
	if hits != 0 {
		t.Fatalf("server received %d requests", hits)
	}
}

func TestAPICommandRejectsMissingDataFile(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "api", "POST", "/tasks", "--data", "@missing.json")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("error = %v, exit = %d", err, exitCodeFor(err))
	}
}

func TestAPICommandRejectsAbsoluteAndUnsafePaths(t *testing.T) {
	paths := []string{"https://evil.example/tasks", "//evil.example/tasks", "../tasks", "/tasks/../users"}
	for _, apiPath := range paths {
		t.Run(apiPath, func(t *testing.T) {
			_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("server should not be called")
			}, "api", "GET", apiPath)
			if err == nil || exitCodeFor(err) != exitUsage {
				t.Fatalf("path %q: error = %v, exit = %d", apiPath, err, exitCodeFor(err))
			}
		})
	}
}

func TestAPICommandEmptyAndRawResponses(t *testing.T) {
	t.Run("204", func(t *testing.T) {
		out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}, "api", "DELETE", "/tasks/123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"data": null`) {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("raw", func(t *testing.T) {
		out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":{"gid":"1"},"next_page":{"offset":"next"}}`))
		}, "api", "GET", "/tasks", "--raw-response")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var envelope struct {
			Data struct {
				Data     map[string]string `json:"data"`
				NextPage map[string]string `json:"next_page"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Data.Data["gid"] != "1" || envelope.Data.NextPage["offset"] != "next" {
			t.Errorf("raw output = %s", out)
		}
	})
}

func TestAPICommandDoesNotEchoErrorResponseBody(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"super-secret-request-value"}`))
	}, "api", "POST", "/tasks", "--data", `{"secret":"super-secret-request-value"}`)
	if err == nil {
		t.Fatal("expected error")
	}
	var output bytes.Buffer
	writeError(&output, err, false)
	if strings.Contains(output.String(), "super-secret-request-value") {
		t.Errorf("error output leaked response body: %s", output.String())
	}
}

func TestAPICommandPreservesPathQueries(t *testing.T) {
	var got url.Values
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{"data":{}}`))
	}, "api", "GET", "/tasks?existing=one", "--query", "existing=two")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Get("existing") != "one" || len(got["existing"]) != 2 || got["existing"][1] != "two" {
		t.Errorf("query = %#v", got)
	}
}
