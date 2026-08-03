package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func decodeTaskRequest(t *testing.T, r *http.Request) map[string]json.RawMessage {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode body: %v\n%s", err, raw)
	}
	return envelope.Data
}

func TestCreateTaskFullyPopulated(t *testing.T) {
	var body map[string]json.RawMessage
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tasks" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		body = decodeTaskRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"99","name":"Ship v2","completed":false}}`))
	}, "create-task", "--workspace-gid", "ws1", "--name", "Ship v2", "--project-gid", "p1", "--section-gid", "s1", "--assignee", "me", "--notes", "details", "--due-on", "2026-08-15", "--start-at", "2026-08-01T09:00:00Z", "--follower", "u1", "--follower", "u2", "--custom-field", "cf-text=hello", "--custom-field", "cf-number=42", "--custom-field", `cf-options=["o1","o2"]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body["name"]) != `"Ship v2"` || string(body["workspace"]) != `"ws1"` {
		t.Fatalf("basic fields = %s", body)
	}
	if string(body["projects"]) != `["p1"]` {
		t.Errorf("projects = %s", body["projects"])
	}
	if string(body["memberships"]) != `[{"project":"p1","section":"s1"}]` {
		t.Errorf("memberships = %s", body["memberships"])
	}
	if string(body["due_on"]) != `"2026-08-15"` || string(body["start_at"]) != `"2026-08-01T09:00:00Z"` {
		t.Errorf("dates = due %s start %s", body["due_on"], body["start_at"])
	}
	var fields map[string]any
	if err := json.Unmarshal(body["custom_fields"], &fields); err != nil {
		t.Fatal(err)
	}
	if fields["cf-number"] != float64(42) || fields["cf-text"] != "hello" {
		t.Errorf("custom fields = %#v", fields)
	}
	var result struct{ GID string }
	decodeData(t, out, &result)
	if result.GID != "99" {
		t.Errorf("result gid = %q", result.GID)
	}
}

func TestCreateTaskProjectOnlyAndParentContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"project", []string{"--name", "Child", "--project-gid", "p1"}, `"projects":["p1"]`},
		{"parent", []string{"--name", "Child", "--parent-task-gid", "parent1"}, `"parent":"parent1"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rawBody string
			_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				rawBody = string(b)
				w.Write([]byte(`{"data":{"gid":"1"}}`))
			}, append([]string{"create-task"}, tc.args...)...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(rawBody, tc.want) {
				t.Errorf("body %s does not contain %s", rawBody, tc.want)
			}
		})
	}
}

func TestCreateTaskRejectsInvalidDatesAndConflicts(t *testing.T) {
	for _, args := range [][]string{
		{"create-task", "--workspace-gid", "ws", "--name", "x", "--due-on", "2026-01-01", "--due-at", "2026-01-01T00:00:00Z"},
		{"create-task", "--workspace-gid", "ws", "--name", "x", "--start-at", "not-a-date"},
		{"create-task", "--workspace-gid", "ws", "--name", "x", "--notes", "a", "--html-notes", "<p>b</p>"},
	} {
		_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("server should not be called")
		}, args...)
		if exitCodeFor(err) != exitUsage {
			t.Errorf("args %v exit = %d, want usage", args, exitCodeFor(err))
		}
	}
}

func TestUpdateTaskExtendedFieldsAndClearing(t *testing.T) {
	var body map[string]json.RawMessage
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		body = decodeTaskRequest(t, r)
		w.Write([]byte(`{"data":{"gid":"1","completed":false}}`))
	}, "update-task", "--task-gid", "1", "--html-notes", "<b>x</b>", "--due-at", "2026-08-15T12:00:00+00:00", "--start-on", "", "--follower", "u1", "--follower", "u2", "--custom-field", "cf=["+`"opt"`+"]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var htmlNotes string
	if err := json.Unmarshal(body["html_notes"], &htmlNotes); err != nil {
		t.Fatal(err)
	}
	if htmlNotes != "<b>x</b>" || string(body["due_at"]) != `"2026-08-15T12:00:00+00:00"` || string(body["start_on"]) != "null" {
		t.Errorf("body = %v", body)
	}
	if string(body["followers"]) != `["u1","u2"]` || string(body["custom_fields"]) != `{"cf":["opt"]}` {
		t.Errorf("collections = followers %s custom %s", body["followers"], body["custom_fields"])
	}
}

func TestDeleteTaskRequiresConfirmationAndSupportsEmptyResponse(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "delete-task", "--task-gid", "1")
	if exitCodeFor(err) != exitUsage {
		t.Fatalf("exit = %d, want usage", exitCodeFor(err))
	}

	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/tasks/1" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}, "delete-task", "--task-gid", "1", "--yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"data": null`) {
		t.Errorf("output = %s", out)
	}
}

func TestDuplicateTask(t *testing.T) {
	var body map[string]json.RawMessage
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tasks/1/duplicate" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		body = decodeTaskRequest(t, r)
		w.Write([]byte(`{"data":{"gid":"2","name":"Copy"}}`))
	}, "duplicate-task", "--task-gid", "1", "--name", "Copy", "--include", "subtasks,stories", "--include", "followers")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body["include_subtasks"]) != "true" || string(body["include_stories"]) != "true" || string(body["include_followers"]) != "true" {
		t.Errorf("include fields = %v", body)
	}
	var task struct{ GID string }
	decodeData(t, out, &task)
	if task.GID != "2" {
		t.Errorf("gid = %q", task.GID)
	}
}
