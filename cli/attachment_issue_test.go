package cli

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListAttachmentsSupportsProjectBriefParent(t *testing.T) {
	var gotParent string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotParent = r.URL.Query().Get("parent")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"gid":"att1","name":"brief.pdf"}],"next_page":null}`)
	}, "list-attachments", "--parent-gid", "brief1", "--parent-type", "project-brief")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotParent != "brief1" {
		t.Fatalf("parent = %q", gotParent)
	}
	if !strings.Contains(out, `"gid": "att1"`) {
		t.Fatalf("output does not contain attachment: %s", out)
	}
}

func TestListAttachmentsRejectsUnknownParentType(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "list-attachments", "--parent-gid", "1", "--parent-type", "workspace")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestAddAttachmentParentGIDAndLegacyTaskAlias(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "report.txt")
	if err := writeTestFile(filePath, "report"); err != nil {
		t.Fatal(err)
	}
	var parent string
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		parent = r.FormValue("parent")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"gid":"att1","name":"report.txt"}}`)
	}, "add-attachment", "--parent-gid", "project1", "--parent-type", "project", "--file", filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parent != "project1" {
		t.Errorf("parent = %q", parent)
	}
}

func TestAddAttachmentURLValidatesAndSendsMultipartFields(t *testing.T) {
	var form url.Values
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		form = r.Form
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"gid":"att-url","name":"Design doc"}}`)
	}, "add-attachment-url", "--parent-gid", "project1", "--parent-type", "project", "--url", "https://example.com/design", "--name", "Design doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if form.Get("parent") != "project1" || form.Get("url") != "https://example.com/design" || form.Get("name") != "Design doc" {
		t.Errorf("form = %#v", form)
	}
	if !strings.Contains(out, "att-url") {
		t.Errorf("output = %s", out)
	}
}

func TestAddAttachmentURLRejectsNonHTTPS(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "add-attachment-url", "--parent-gid", "1", "--url", "http://example.com/file", "--name", "file")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestDeleteAttachmentAllowsEmptyResponse(t *testing.T) {
	var path string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}, "delete-attachment", "--attachment-gid", "att1", "--yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/attachments/att1" || !strings.Contains(out, `"data": null`) {
		t.Errorf("path=%q output=%s", path, out)
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
