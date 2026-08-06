package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCommentOnTaskPostsBody(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"story99"}}`))
	}, "comment-on-task", "--task-gid", "42", "--text", "Taking a look.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q", gotMethod)
	}
	if gotPath != "/tasks/42/stories" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"text":"Taking a look."`) {
		t.Errorf("body = %q", gotBody)
	}
	var story struct{ GID string }
	decodeData(t, out, &story)
	if story.GID != "story99" {
		t.Errorf("gid = %q", story.GID)
	}
}

func TestCommentOnTaskHuman(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"story99"}}`))
	}, "comment-on-task", "--task-gid", "42", "--text", "hi", "--human")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Posted comment story99 on task 42.\n" {
		t.Errorf("got %q", out)
	}
}

func TestCommentOnTaskValidation(t *testing.T) {
	cases := [][]string{
		{"comment-on-task", "--text", "hi"},                    // missing task-gid
		{"comment-on-task", "--task-gid", "42"},                // missing text
		{"comment-on-task", "--task-gid", "42", "--text", " "}, // blank text
	}
	for _, args := range cases {
		_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("server should not be called for invalid input")
		}, args...)
		if exitCodeFor(err) != exitUsage {
			t.Errorf("args %v: exit code = %d, want %d", args, exitCodeFor(err), exitUsage)
		}
	}
}

func TestCommentOnTaskForbiddenIsRuntimeError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"errors":[]}`))
	}, "comment-on-task", "--task-gid", "42", "--text", "hi")
	if exitCodeFor(err) != exitRuntime {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitRuntime)
	}
}
