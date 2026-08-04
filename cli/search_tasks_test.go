package cli

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestSearchTasksBuildsQuery(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"t1","name":"Release","completed":false}],"next_page":null}`))
	}, "search-tasks", "--workspace-gid", "ws1", "--text", "release", "--assignee", "me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/workspaces/ws1/tasks/search" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery.Get("text") != "release" {
		t.Errorf("text = %q", gotQuery.Get("text"))
	}
	if gotQuery.Get("assignee.any") != "me" {
		t.Errorf("assignee.any = %q", gotQuery.Get("assignee.any"))
	}
	if _, ok := gotQuery["completed"]; ok {
		t.Errorf("completed should be omitted when unset, got %q", gotQuery.Get("completed"))
	}
	var tasks []json.RawMessage
	decodeData(t, out, &tasks)
	if len(tasks) != 1 {
		t.Errorf("got %d tasks", len(tasks))
	}
}

func TestSearchTasksCompletedTriState(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string // "" means key absent
	}{
		{"unset", []string{"search-tasks", "--workspace-gid", "ws1"}, ""},
		{"true", []string{"search-tasks", "--workspace-gid", "ws1", "--completed=true"}, "true"},
		{"false", []string{"search-tasks", "--workspace-gid", "ws1", "--completed=false"}, "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotQuery url.Values
			_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"data":[],"next_page":null}`))
			}, tc.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, present := gotQuery["completed"]
			if tc.want == "" {
				if present {
					t.Errorf("completed present = %v, want absent", got)
				}
				return
			}
			if !present || got[0] != tc.want {
				t.Errorf("completed = %v, want %q", got, tc.want)
			}
		})
	}
}

func TestSearchTasksFirstClassFiltersAndQueries(t *testing.T) {
	var gotQuery url.Values
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"next_page":null}`))
	}, "search-tasks", "--workspace-gid", "ws1",
		"--assignee-any", "me", "--assignee-any", "u2", "--assignee-not", "u3",
		"--project-any", "p1", "--project-any", "p2", "--section-not", "s1",
		"--tag-any", "tag1", "--team-any", "team1", "--follower-any", "u4",
		"--due-on", "2026-08-20", "--due-before", "2026-08-31", "--start-after", "2026-08-01",
		"--created-after", "2026-08-01T00:00:00Z", "--modified-before", "2026-08-31T00:00:00Z",
		"--completed=true", "--sort-by", "due_date", "--sort-ascending",
		"--query", "custom_fields.111.value=222")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for key, want := range map[string]string{
		"assignee.any": "me", "assignee.not": "u3", "projects.any": "p1",
		"sections.not": "s1", "tags.any": "tag1", "teams.any": "team1",
		"followers.any": "u4", "due_on": "2026-08-20", "due_on.before": "2026-08-31",
		"start_on.after": "2026-08-01", "created_at.after": "2026-08-01T00:00:00Z",
		"modified_at.before": "2026-08-31T00:00:00Z", "completed": "true",
		"sort_by": "due_date", "sort_ascending": "true", "custom_fields.111.value": "222",
	} {
		if gotQuery.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, gotQuery.Get(key), want)
		}
	}
	if got := gotQuery["projects.any"]; len(got) != 2 || got[1] != "p2" {
		t.Errorf("projects.any = %v, want [p1 p2]", got)
	}
}

func TestSearchTasksRejectsConflictingAndPaginationQueries(t *testing.T) {
	cases := [][]string{
		{"--query", "completed=true", "--query", "completed=false"},
		{"--completed=false", "--query", "completed=true"},
		{"--query", "limit=10"},
		{"--query", "offset=next"},
	}
	for _, extra := range cases {
		_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":[]}`))
		}, append([]string{"search-tasks", "--workspace-gid", "ws1"}, extra...)...)
		if err == nil || exitCodeFor(err) != exitUsage {
			t.Errorf("args %v: err = %v, want usage error", extra, err)
		}
	}
}

func TestSearchTasksValidatesDateRangesAndSort(t *testing.T) {
	cases := [][]string{
		{"--due-after", "2026-09-01", "--due-before", "2026-08-01"},
		{"--created-after", "not-a-time"},
		{"--sort-by", "random"},
	}
	for _, extra := range cases {
		_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":[]}`))
		}, append([]string{"search-tasks", "--workspace-gid", "ws1"}, extra...)...)
		if err == nil || exitCodeFor(err) != exitUsage {
			t.Errorf("args %v: err = %v, want usage error", extra, err)
		}
	}
}

func TestSearchTasksPremiumRequiredIsRuntimeError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(402)
		w.Write([]byte(`{"errors":[]}`))
	}, "search-tasks", "--workspace-gid", "ws1", "--text", "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if exitCodeFor(err) != exitRuntime {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitRuntime)
	}
	if err.Error() != "Asana API access requires a premium workspace or feature for this request." {
		t.Errorf("message = %q", err.Error())
	}
}
