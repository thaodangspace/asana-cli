package cli

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestListProjectTasksBuildsQuery(t *testing.T) {
	var gotPath string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"t1","name":"Fix bug","completed":false}],"next_page":null}`))
	}, "list-project-tasks", "--project-gid", "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/projects/p1/tasks" {
		t.Errorf("path = %q", gotPath)
	}
	var tasks []json.RawMessage
	decodeData(t, out, &tasks)
	if len(tasks) != 1 {
		t.Errorf("got %d tasks", len(tasks))
	}
}

func TestListProjectTasksRequiresProjectGID(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request should not be made without --project-gid")
	}, "list-project-tasks")
	if err == nil {
		t.Fatal("expected error for missing --project-gid")
	}
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want usage error", exitCodeFor(err))
	}
}

func TestListProjectTasksPaginatesPastHundred(t *testing.T) {
	hits := 0
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/projects/p1/tasks":
			w.Write([]byte(`{"data":[{"gid":"1"},{"gid":"2"}],"next_page":{"path":"/projects/p1/tasks/page2"}}`))
		case "/projects/p1/tasks/page2":
			w.Write([]byte(`{"data":[{"gid":"3"}],"next_page":null}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}, "list-project-tasks", "--project-gid", "p1", "--limit", "200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
	var tasks []json.RawMessage
	decodeData(t, out, &tasks)
	if len(tasks) != 3 {
		t.Errorf("got %d tasks, want 3", len(tasks))
	}
}

func TestListProjectTasksLimitBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit string
	}{
		{"zero", "0"},
		{"tooHigh", "501"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("request should not be made for out-of-range limit")
			}, "list-project-tasks", "--project-gid", "p1", "--limit", tc.limit)
			if err == nil {
				t.Fatal("expected error for out-of-range limit")
			}
			if exitCodeFor(err) != exitUsage {
				t.Errorf("exit code = %d, want usage error", exitCodeFor(err))
			}
		})
	}
}
