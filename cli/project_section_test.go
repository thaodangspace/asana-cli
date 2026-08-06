package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestMoveSectionUsesProjectScopedInsert(t *testing.T) {
	var gotPath, gotMethod string
	var body map[string]any
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		payload, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(payload, &body)
		w.WriteHeader(http.StatusNoContent)
	}, "move-section", "--project-gid", "p1", "--section-gid", "s1", "--before-section-gid", "s2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/projects/p1/sections/insert" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	data, ok := body["data"].(map[string]any)
	if !ok || data["section"] != "s1" || data["before_section"] != "s2" {
		t.Fatalf("body = %#v", body)
	}
}

func TestMoveSectionRequiresExactlyOnePosition(t *testing.T) {
	for _, args := range [][]string{
		{"move-section", "--project-gid", "p1", "--section-gid", "s1"},
		{"move-section", "--project-gid", "p1", "--section-gid", "s1", "--before-section-gid", "s2", "--after-section-gid", "s3"},
	} {
		_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("server should not be called")
		}, args...)
		if err == nil || exitCodeFor(err) != exitUsage {
			t.Fatalf("args %v: err=%v code=%d", args, err, exitCodeFor(err))
		}
	}
}

func TestSearchProjectsUsesSinglePageFilters(t *testing.T) {
	var requests int
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Errorf("limit = %q, want 100", got)
		}
		for key, want := range map[string]string{
			"owner.any":   "u1",
			"teams.any":   "t1",
			"members.any": "u2",
			"completed":   "false",
		} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"p1","name":"Project"}]}`))
	}, "search-projects", "--workspace-gid", "w1", "--limit", "100", "--owner", "u1", "--team", "t1", "--member", "u2", "--completed=false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	var projects []struct{ GID string }
	decodeData(t, out, &projects)
	if len(projects) != 1 || projects[0].GID != "p1" {
		t.Fatalf("projects = %#v", projects)
	}
}

func TestSearchProjectsRejectsCollectionPaginationFlags(t *testing.T) {
	for _, flag := range []string{"--offset", "--all", "--max-pages"} {
		_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("server should not be called")
		}, "search-projects", "--workspace-gid", "w1", flag, "1")
		if err == nil || exitCodeFor(err) != exitUsage {
			t.Fatalf("flag %s: err=%v code=%d", flag, err, exitCodeFor(err))
		}
	}
}

func TestUpdateProjectDoesNotExposeLocationFlags(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "update-project", "--project-gid", "p1", "--workspace-gid", "w1")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("err=%v code=%d, want usage error", err, exitCodeFor(err))
	}
}
