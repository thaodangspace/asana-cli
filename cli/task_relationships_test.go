package cli

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestUpdateTaskRelationshipsUseDedicatedEndpoints(t *testing.T) {
	var calls []string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tasks/1":
			if r.Method == http.MethodGet {
				if r.URL.Query().Get("opt_fields") == "projects" {
					w.Write([]byte(`{"data":{"gid":"1","projects":[{"gid":"p1"},{"gid":"p2"}]}}`))
				} else {
					w.Write([]byte(`{"data":{"gid":"1","name":"Updated"}}`))
				}
				return
			}
			t.Errorf("unexpected PUT to task endpoint")
		case "/tasks/1/addProject":
			body := decodeTaskRequest(t, r)
			if string(body["project"]) != `"p3"` {
				t.Errorf("add project body = %v", body)
			}
			w.Write([]byte(`{}`))
		case "/tasks/1/removeProject":
			body := decodeTaskRequest(t, r)
			if string(body["project"]) != `"p2"` {
				t.Errorf("remove project body = %v", body)
			}
			w.Write([]byte(`{}`))
		case "/sections/s1/addTask":
			body := decodeTaskRequest(t, r)
			if string(body["task"]) != `"1"` {
				t.Errorf("section body = %v", body)
			}
			w.Write([]byte(`{}`))
		case "/tasks/1/setParent":
			body := decodeTaskRequest(t, r)
			if string(body["parent"]) != `"parent"` {
				t.Errorf("parent body = %v", body)
			}
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Path == "/tasks/1" && r.Method == http.MethodGet {
			return
		}
		if r.URL.Path == "/tasks/1" {
			w.Write([]byte(`{"data":{"gid":"1"}}`))
		}
	}, "update-task", "--task-gid", "1", "--project-gid", "p1", "--project-gid", "p3", "--section-gid", "s1", "--parent-task-gid", "parent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = out
	want := []string{
		"GET /tasks/1",
		"POST /tasks/1/removeProject",
		"POST /tasks/1/addProject",
		"POST /sections/s1/addTask",
		"POST /tasks/1/setParent",
		"GET /tasks/1",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}

func TestTaskProjectGIDsRequiresProjectsField(t *testing.T) {
	if _, err := taskProjectGIDs(json.RawMessage(`{"gid":"1"}`)); err == nil {
		t.Fatal("expected missing projects error")
	}
}
