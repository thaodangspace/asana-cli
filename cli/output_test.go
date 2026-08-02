package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/thaodangspace/asana-cli/asana"
)

func TestWriteSuccessJSON(t *testing.T) {
	var buf bytes.Buffer
	data := []json.RawMessage{json.RawMessage(`{"gid":"1"}`)}
	if err := writeSuccess(&buf, data, false, "ignored"); err != nil {
		t.Fatal(err)
	}
	var env struct {
		OK   bool              `json:"ok"`
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if !env.OK || len(env.Data) != 1 {
		t.Errorf("unexpected envelope: %s", buf.String())
	}
}

func TestWriteSuccessHuman(t *testing.T) {
	var buf bytes.Buffer
	if err := writeSuccess(&buf, nil, true, "1 Hello [open]"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "1 Hello [open]" {
		t.Errorf("got %q", buf.String())
	}
	if strings.Contains(buf.String(), "{") {
		t.Errorf("human output should not be JSON: %q", buf.String())
	}
}

func TestWriteErrorJSONHTTPError(t *testing.T) {
	var buf bytes.Buffer
	he := &asana.HTTPError{Method: "GET", Path: "/tasks/1", Status: 404, StatusText: "Not Found"}
	writeError(&buf, he, false)
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Message string `json:"message"`
			Status  int    `json:"status"`
			Method  string `json:"method"`
			Path    string `json:"path"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if env.OK || env.Error.Status != 404 || env.Error.Method != "GET" || env.Error.Path != "/tasks/1" {
		t.Errorf("unexpected error envelope: %s", buf.String())
	}
}

func TestWriteErrorHuman(t *testing.T) {
	var buf bytes.Buffer
	writeError(&buf, errors.New("boom"), true)
	if strings.TrimSpace(buf.String()) != "boom" {
		t.Errorf("got %q", buf.String())
	}
}

func TestSummarizers(t *testing.T) {
	if got := summarizeWorkspace(json.RawMessage(`{"gid":"7","name":"Acme"}`)); got != "7 Acme" {
		t.Errorf("workspace: %q", got)
	}
	if got := summarizeProject(json.RawMessage(`{"gid":"9"}`)); got != "9 (unnamed project)" {
		t.Errorf("project: %q", got)
	}
	if got := summarizeTask(json.RawMessage(`{"gid":"3","name":"Ship","completed":true}`)); got != "3 Ship [completed]" {
		t.Errorf("task completed: %q", got)
	}
	if got := summarizeTask(json.RawMessage(`{"gid":"4","name":"WIP"}`)); got != "4 WIP [open]" {
		t.Errorf("task open: %q", got)
	}
	if got := summarizeTask(json.RawMessage(`{}`)); got != "unknown (unnamed task) [open]" {
		t.Errorf("task empty: %q", got)
	}
	if got := summarizeStory(json.RawMessage(`{"gid":"5","text":"hi","created_by":{"name":"Sam"}}`)); got != "5 Sam: hi" {
		t.Errorf("story: %q", got)
	}
	if got := summarizeStory(json.RawMessage(`{"gid":"6","resource_subtype":"assigned"}`)); got != "6 assigned" {
		t.Errorf("story subtype: %q", got)
	}
}

func TestHumanListEmpty(t *testing.T) {
	if got := humanList(nil, summarizeTask, "No tasks found."); got != "No tasks found." {
		t.Errorf("got %q", got)
	}
}

func TestHumanListJoins(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"gid":"1","name":"A"}`),
		json.RawMessage(`{"gid":"2","name":"B"}`),
	}
	got := humanList(items, summarizeWorkspace, "none")
	if got != "1 A\n2 B" {
		t.Errorf("got %q", got)
	}
}
