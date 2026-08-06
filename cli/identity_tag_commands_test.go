package cli

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestGetUserCommandAcceptsMe(t *testing.T) {
	var gotPath, gotFields string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotFields = r.URL.Path, r.URL.Query().Get("opt_fields")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"1","name":"Sam","email":"sam@example.com"}}`))
	}, "get-user", "--user-gid", "me", "--opt-fields", "name,email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/users/me" || gotFields != "name,email" {
		t.Fatalf("request = %s?opt_fields=%s", gotPath, gotFields)
	}
	var user struct {
		GID string `json:"gid"`
	}
	decodeData(t, out, &user)
	if user.GID != "1" {
		t.Errorf("gid = %q", user.GID)
	}
}

func TestListWorkspaceUsersUsesUsersEndpoint(t *testing.T) {
	var gotPath string
	var gotQuery map[string]string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = map[string]string{
			"workspace":  r.URL.Query().Get("workspace"),
			"opt_fields": r.URL.Query().Get("opt_fields"),
			"limit":      r.URL.Query().Get("limit"),
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"u1","name":"Sam"}],"next_page":null}`))
	}, "list-workspace-users", "--workspace-gid", "ws1", "--opt-fields", "name,email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/users" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery["workspace"] != "ws1" || gotQuery["opt_fields"] != "name,email" || gotQuery["limit"] != "20" {
		t.Errorf("query = %#v", gotQuery)
	}
	var users []json.RawMessage
	decodeData(t, out, &users)
	if len(users) != 1 {
		t.Fatalf("users = %d, want 1", len(users))
	}
}

func TestListTeamUsersUsesUsersEndpoint(t *testing.T) {
	var gotPath string
	var gotTeam, gotFields string
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotTeam, gotFields = r.URL.Path, r.URL.Query().Get("team"), r.URL.Query().Get("opt_fields")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"next_page":null}`))
	}, "list-team-users", "--team-gid", "team1", "--opt-fields", "name,email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/users" || gotTeam != "team1" || gotFields != "name,email" {
		t.Errorf("request = %s?team=%s&opt_fields=%s", gotPath, gotTeam, gotFields)
	}
}

func TestFindUserFindsMatchOnSecondPage(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("offset") == "page2" {
			w.Write([]byte(`{"data":[{"gid":"u2","email":"sam@example.com"}],"next_page":null}`))
			return
		}
		if r.URL.Query().Get("workspace") != "ws1" || r.URL.Query().Get("opt_fields") != "email" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"data":[{"gid":"u1","email":"other@example.com"}],"next_page":{"offset":"page2"}}`))
	}, "find-user", "--workspace-gid", "ws1", "--email", "sam@example.com", "--all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var user struct {
		GID string `json:"gid"`
	}
	decodeData(t, out, &user)
	if user.GID != "u2" {
		t.Errorf("gid = %q, want u2", user.GID)
	}
}

func TestFindUserRejectsAmbiguousMatchesAcrossPages(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("offset") == "page2" {
			w.Write([]byte(`{"data":[{"gid":"u2","email":"SAM@example.com"}],"next_page":null}`))
			return
		}
		w.Write([]byte(`{"data":[{"gid":"u1","email":"sam@example.com"}],"next_page":{"offset":"page2"}}`))
	}, "find-user", "--workspace-gid", "ws1", "--email", "sam@example.com", "--all")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("error = %v, exit = %d", err, exitCodeFor(err))
	}
}

func TestListTeamMembershipsEndpoint(t *testing.T) {
	var gotPath, gotFields string
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotFields = r.URL.Path, r.URL.Query().Get("opt_fields")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"next_page":null}`))
	}, "list-team-memberships", "--team-gid", "team/1", "--opt-fields", "user,team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/teams/team/1/team_memberships" || gotFields != "user,team" {
		t.Errorf("request = %s?opt_fields=%s", gotPath, gotFields)
	}
}

func TestWorkspaceTeamAndMembershipEndpoints(t *testing.T) {
	calls := 0
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/workspaces/ws1/teams":
			if r.URL.Query().Get("opt_fields") != "name" {
				t.Errorf("team query = %s", r.URL.RawQuery)
			}
		case "/workspaces/ws1/workspace_memberships":
			if r.URL.Query().Get("limit") != "20" {
				t.Errorf("membership query = %s", r.URL.RawQuery)
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Write([]byte(`{"data":[],"next_page":null}`))
	}, "list-workspace-teams", "--workspace-gid", "ws1", "--opt-fields", "name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	_, err = runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workspaces/ws1/workspace_memberships" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"next_page":null}`))
	}, "list-workspace-memberships", "--workspace-gid", "ws1")
	if err != nil {
		t.Fatalf("unexpected membership error: %v", err)
	}
}

func TestCreateTagPayload(t *testing.T) {
	var gotMethod, gotPath string
	var body struct {
		Data map[string]any `json:"data"`
	}
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"tag1","name":"Ready"}}`))
	}, "create-tag", "--workspace-gid", "ws1", "--name", "Ready", "--color", "light-green")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/tags" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if body.Data["workspace"] != "ws1" || body.Data["name"] != "Ready" || body.Data["color"] != "light-green" {
		t.Errorf("data = %#v", body.Data)
	}
	if out == "" {
		t.Fatal("expected output")
	}
}

func TestTagColorNoneIsRejected(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "create-tag", "--workspace-gid", "ws1", "--name", "Ready", "--color", "none")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("error = %v, exit = %d", err, exitCodeFor(err))
	}
}

func TestDeleteTagRequiresConfirmation(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "delete-tag", "--tag-gid", "tag1")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("error = %v, exit = %d", err, exitCodeFor(err))
	}
}
