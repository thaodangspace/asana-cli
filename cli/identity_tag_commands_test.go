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

func TestListWorkspaceUsersUsesWorkspaceEndpoint(t *testing.T) {
	var gotPath string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"u1","name":"Sam"}],"next_page":null}`))
	}, "list-workspace-users", "--workspace-gid", "ws1", "--opt-fields", "name,email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/workspaces/ws1/users" {
		t.Errorf("path = %q", gotPath)
	}
	var users []json.RawMessage
	decodeData(t, out, &users)
	if len(users) != 1 {
		t.Fatalf("users = %d, want 1", len(users))
	}
}

func TestFindUserRejectsAmbiguousExactMatches(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"u1","email":"sam@example.com"},{"gid":"u2","email":"SAM@example.com"}],"next_page":null}`))
	}, "find-user", "--workspace-gid", "ws1", "--email", "sam@example.com")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("error = %v, exit = %d", err, exitCodeFor(err))
	}
}

func TestListTeamMembershipsEndpoint(t *testing.T) {
	var gotPath string
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"next_page":null}`))
	}, "list-team-memberships", "--team-gid", "team/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/teams/team/1/memberships" {
		// URL.Path is decoded by net/http; the request still uses an escaped
		// path segment on the wire.
		t.Errorf("path = %q", gotPath)
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

func TestDeleteTagRequiresConfirmation(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "delete-tag", "--tag-gid", "tag1")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("error = %v, exit = %d", err, exitCodeFor(err))
	}
}
