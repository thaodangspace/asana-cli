package cli

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestListTagTasksBuildsQuery(t *testing.T) {
	var gotPath string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"t1","name":"Fix bug","completed":false}],"next_page":null}`))
	}, "list-tag-tasks", "--tag-gid", "tag1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/tags/tag1/tasks" {
		t.Errorf("path = %q", gotPath)
	}
	var tasks []json.RawMessage
	decodeData(t, out, &tasks)
	if len(tasks) != 1 {
		t.Errorf("got %d tasks", len(tasks))
	}
}

func TestListTagTasksRequiresTagGID(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request should not be made without --tag-gid")
	}, "list-tag-tasks")
	if err == nil {
		t.Fatal("expected error for missing --tag-gid")
	}
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want usage error", exitCodeFor(err))
	}
}

func TestListTagTasksPaginatesPastHundred(t *testing.T) {
	hits := 0
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tags/tag1/tasks":
			w.Write([]byte(`{"data":[{"gid":"1"},{"gid":"2"}],"next_page":{"path":"/tags/tag1/tasks/page2"}}`))
		case "/tags/tag1/tasks/page2":
			w.Write([]byte(`{"data":[{"gid":"3"}],"next_page":null}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}, "list-tag-tasks", "--tag-gid", "tag1", "--limit", "200")
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

func TestListTagTasksLimitBounds(t *testing.T) {
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
			}, "list-tag-tasks", "--tag-gid", "tag1", "--limit", tc.limit)
			if err == nil {
				t.Fatal("expected error for out-of-range limit")
			}
			if exitCodeFor(err) != exitUsage {
				t.Errorf("exit code = %d, want usage error", exitCodeFor(err))
			}
		})
	}
}
