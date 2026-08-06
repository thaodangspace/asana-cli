package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCreateSubtaskUsesParentEndpointAndCommonFields(t *testing.T) {
	var body map[string]json.RawMessage
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tasks/parent/subtasks" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		body = decodeTaskRequest(t, r)
		w.Write([]byte(`{"data":{"gid":"child","name":"Child"}}`))
	}, "create-subtask", "--task-gid", "parent", "--name", "Child", "--due-on", "2026-08-15", "--follower", "u1", "--custom-field", "cf=hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body["name"]) != `"Child"` || string(body["due_on"]) != `"2026-08-15"` || string(body["followers"]) != `["u1"]` {
		t.Fatalf("body = %v", body)
	}
	var task struct{ GID string }
	decodeData(t, out, &task)
	if task.GID != "child" {
		t.Errorf("gid = %q", task.GID)
	}
}

func TestTaskGraphListUsesPagination(t *testing.T) {
	calls := 0
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		expectedLimit := "2"
		if calls > 1 {
			expectedLimit = "1"
		}
		if r.URL.Path != "/tasks/task/dependencies" || r.URL.Query().Get("limit") != expectedLimit {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if calls == 1 {
			w.Write([]byte(`{"data":[{"gid":"dep1"}],"next_page":{"offset":"next"}}`))
		} else {
			if r.URL.Query().Get("offset") != "next" {
				t.Errorf("offset = %q", r.URL.Query().Get("offset"))
			}
			w.Write([]byte(`{"data":[{"gid":"dep2"}],"next_page":null}`))
		}
	}, "list-dependencies", "--task-gid", "task", "--limit", "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 || !strings.Contains(out, `"dep2"`) {
		t.Fatalf("calls = %d, output = %s", calls, out)
	}
}

func TestRelationshipMutationBodiesAndEscapedPaths(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tasks/a/b/addDependencies" {
			t.Errorf("request = %s %s (%s)", r.Method, r.URL.Path, r.RequestURI)
		}
		if !strings.Contains(r.RequestURI, "a%2Fb") {
			t.Errorf("path was not escaped: %s", r.RequestURI)
		}
		body := decodeTaskRequest(t, r)
		if string(body["dependencies"]) != `["dep"]` {
			t.Errorf("body = %s", body["dependencies"])
		}
		w.Write([]byte(`{"data":{}}`))
	}, "add-dependency", "--task-gid", "a/b", "--dependency-task-gid", "dep")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"task_gid": "a/b"`) || !strings.Contains(out, `"dependency_task_gid": "dep"`) {
		t.Errorf("stable result = %s", out)
	}
}

func TestProjectPlacementFlagsAreMutuallyExclusive(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "add-task-to-project", "--task-gid", "task", "--project-gid", "project", "--insert-before", "a", "--insert-after", "b")
	if exitCodeFor(err) != exitUsage {
		t.Fatalf("exit = %d, want usage", exitCodeFor(err))
	}
}

func TestFollowerMutationPreservesOrder(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := decodeTaskRequest(t, r)
		if string(body["followers"]) != `["u2","me","u1"]` {
			t.Errorf("followers = %s", body["followers"])
		}
		w.WriteHeader(http.StatusNoContent)
	}, "add-task-followers", "--task-gid", "task", "--follower", "u2", "--follower", "me", "--follower", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
