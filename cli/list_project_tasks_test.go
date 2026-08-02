package cli

import (
	"encoding/json"
	"net/http"
	"strings"
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

func TestListProjectTasksAllAndOffset(t *testing.T) {
	var queries []string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if len(queries) == 1 {
			w.Write([]byte(`{"data":[{"gid":"1"}],"next_page":{"offset":"resume me"}}`))
			return
		}
		w.Write([]byte(`{"data":[{"gid":"2"}],"next_page":null}`))
	}, "list-project-tasks", "--project-gid", "p1", "--all", "--offset", "start here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(queries) != 2 || !strings.Contains(queries[0], "offset=start+here") {
		t.Errorf("queries = %v, want encoded initial offset", queries)
	}
	var env struct {
		Data       []json.RawMessage `json:"data"`
		Pagination struct {
			PagesFetched int  `json:"pages_fetched"`
			Truncated    bool `json:"truncated"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(env.Data) != 2 || env.Pagination.PagesFetched != 2 || env.Pagination.Truncated {
		t.Errorf("result = %+v, want two complete items", env)
	}
}

func TestListProjectTasksAllWithLimitIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not be made for conflicting pagination flags")
	}, "list-project-tasks", "--project-gid", "p1", "--all", "--limit", "10")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Errorf("error = %v, exit = %d; want usage error", err, exitCodeFor(err))
	}
}

func TestListProjectTasksMaxPagesMarksTruncated(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"1"}],"next_page":{"path":"/projects/p1/tasks?page=2"}}`))
	}, "list-project-tasks", "--project-gid", "p1", "--all", "--max-pages", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Pagination struct {
			Truncated bool   `json:"truncated"`
			NextPath  string `json:"next_path"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if !env.Pagination.Truncated || env.Pagination.NextPath != "/projects/p1/tasks?page=2" {
		t.Errorf("pagination = %+v, want resumable truncation", env.Pagination)
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
