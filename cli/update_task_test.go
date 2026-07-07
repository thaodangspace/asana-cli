package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// decodeRequestBody reads and unwraps the {"data": {...}} request body.
func decodeRequestBody(t *testing.T, r *http.Request) map[string]json.RawMessage {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var env struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode body: %v\n%s", err, raw)
	}
	return env.Data
}

func TestUpdateTaskHappyPath(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]json.RawMessage
	)
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody = decodeRequestBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"123","name":"New","completed":true}}`))
	}, "update-task", "--task-gid", "123", "--name", "New", "--completed")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/tasks/123" {
		t.Errorf("path = %q", gotPath)
	}
	if string(gotBody["name"]) != `"New"` {
		t.Errorf("body name = %s", gotBody["name"])
	}
	if string(gotBody["completed"]) != "true" {
		t.Errorf("body completed = %s", gotBody["completed"])
	}
	var task struct{ Name string }
	decodeData(t, out, &task)
	if task.Name != "New" {
		t.Errorf("name = %q", task.Name)
	}
}

func TestUpdateTaskOnlySetFieldsSent(t *testing.T) {
	var gotBody map[string]json.RawMessage
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeRequestBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"123"}}`))
	}, "update-task", "--task-gid", "123", "--notes", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := gotBody["notes"]; !ok {
		t.Errorf("expected notes key present, body=%v", gotBody)
	}
	if string(gotBody["notes"]) != `""` {
		t.Errorf("notes = %s, want empty string", gotBody["notes"])
	}
	for _, k := range []string{"name", "completed", "due_on", "assignee"} {
		if _, ok := gotBody[k]; ok {
			t.Errorf("unexpected key %q in body", k)
		}
	}
}

func TestUpdateTaskClearSemantics(t *testing.T) {
	var gotBody map[string]json.RawMessage
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeRequestBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"123"}}`))
	}, "update-task", "--task-gid", "123", "--due-on", "", "--assignee", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(gotBody["due_on"]) != "null" {
		t.Errorf("due_on = %s, want null", gotBody["due_on"])
	}
	if string(gotBody["assignee"]) != "null" {
		t.Errorf("assignee = %s, want null", gotBody["assignee"])
	}
}

func TestUpdateTaskNoFieldsIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "update-task", "--task-gid", "123")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestUpdateTaskEmptyNameIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "update-task", "--task-gid", "123", "--name", "")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestUpdateTaskBadDueOnIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "update-task", "--task-gid", "123", "--due-on", "2026-13-40")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestUpdateTaskHTTPErrorPassthrough(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errors":[{"message":"forbidden"}]}`))
	}, "update-task", "--task-gid", "123", "--completed")
	if err == nil {
		t.Fatal("expected error")
	}
	if exitCodeFor(err) != exitRuntime {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitRuntime)
	}
	if strings.Contains(out, "tok") {
		t.Errorf("token leaked into output: %s", out)
	}
}
