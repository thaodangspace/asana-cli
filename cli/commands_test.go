package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// runWithServer runs the root command against an httptest server, with a fake
// token and the API base pointed at the server. It returns stdout and the
// error returned by command execution (nil on success).
func runWithServer(t *testing.T, h http.HandlerFunc, args ...string) (string, error) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("ASANA_ACCESS_TOKEN", "tok")
	t.Setenv("ASANA_DEFAULT_WORKSPACE", "")
	t.Setenv("ASANA_API_BASE", srv.URL)
	t.Setenv("ASANA_CONFIG", t.TempDir()+"/absent.yaml")

	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// decodeData unmarshals the success envelope's data field.
func decodeData(t *testing.T, stdout string, v any) {
	t.Helper()
	var env struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if !env.OK {
		t.Fatalf("ok=false: %s", stdout)
	}
	if err := json.Unmarshal(env.Data, v); err != nil {
		t.Fatalf("decode data: %v\n%s", err, env.Data)
	}
}

func TestMeCommand(t *testing.T) {
	var gotPath string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"1","name":"Sam"}}`))
	}, "me", "--opt-fields", "name,email")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/users/me" {
		t.Errorf("path = %q", gotPath)
	}
	var user struct{ Name string }
	decodeData(t, out, &user)
	if user.Name != "Sam" {
		t.Errorf("name = %q", user.Name)
	}
}

func TestMeCommandHuman(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"1","name":"Sam"}}`))
	}, "me", "--human")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out; got != "Asana user: Sam\n" {
		t.Errorf("human out = %q", got)
	}
}

func TestListWorkspacesCommand(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"1","name":"A"},{"gid":"2","name":"B"}],"next_page":null}`))
	}, "list-workspaces")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var ws []struct{ GID string }
	decodeData(t, out, &ws)
	if len(ws) != 2 {
		t.Errorf("got %d workspaces", len(ws))
	}
}

func TestListProjectsResolvesWorkspaceFlag(t *testing.T) {
	var gotPath string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"p1","name":"Proj"}],"next_page":null}`))
	}, "list-projects", "--workspace-gid", "ws9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/workspaces/ws9/projects" {
		t.Errorf("path = %q", gotPath)
	}
	var projs []json.RawMessage
	decodeData(t, out, &projs)
	if len(projs) != 1 {
		t.Errorf("got %d projects", len(projs))
	}
}

func TestListProjectsMissingWorkspaceIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "list-projects")
	if err == nil {
		t.Fatal("expected error")
	}
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestGetTaskCommand(t *testing.T) {
	var gotPath string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"42","name":"Ship it","completed":false}}`))
	}, "get-task", "--task-gid", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/tasks/42" {
		t.Errorf("path = %q", gotPath)
	}
	var task struct{ Name string }
	decodeData(t, out, &task)
	if task.Name != "Ship it" {
		t.Errorf("name = %q", task.Name)
	}
}

func TestGetTaskMissingGIDIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "get-task")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestListTaskStoriesCommand(t *testing.T) {
	var gotPath string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"s1","text":"hi","created_by":{"name":"Sam"}}],"next_page":null}`))
	}, "list-task-stories", "--task-gid", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/tasks/42/stories" {
		t.Errorf("path = %q", gotPath)
	}
	var stories []json.RawMessage
	decodeData(t, out, &stories)
	if len(stories) != 1 {
		t.Errorf("got %d stories", len(stories))
	}
}

func TestListWorkspacesEmptyHuman(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[],"next_page":null}`))
	}, "list-workspaces", "--human")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "No workspaces found.\n" {
		t.Errorf("got %q", out)
	}
}

func TestLimitOutOfRangeIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "list-workspaces", "--limit", "0")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestHTTPErrorIsRuntimeError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"errors":[]}`))
	}, "get-task", "--task-gid", "999")
	if err == nil {
		t.Fatal("expected error")
	}
	if exitCodeFor(err) != exitRuntime {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitRuntime)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"bogus-cmd"})
	if got := exitCodeFor(root.Execute()); got != exitUsage {
		t.Errorf("unknown command exit = %d, want %d", got, exitUsage)
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	t.Setenv("ASANA_ACCESS_TOKEN", "tok")
	root := newRootCommand()
	root.SetArgs([]string{"me", "--nope"})
	if got := exitCodeFor(root.Execute()); got != exitUsage {
		t.Errorf("unknown flag exit = %d, want %d", got, exitUsage)
	}
}

func TestMissingTokenIsUsageError(t *testing.T) {
	t.Setenv("ASANA_ACCESS_TOKEN", "")
	t.Setenv("ASANA_API_BASE", "")
	t.Setenv("ASANA_CONFIG", t.TempDir()+"/absent.yaml")
	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"me"})
	err := root.Execute()
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}
