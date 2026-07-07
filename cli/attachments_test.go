package cli

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestListTaskAttachmentsCommand(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"att1","name":"Screenshot.png","host":"asana"}],"next_page":null}`))
	}, "list-task-attachments", "--task-gid", "42", "--opt-fields", "name,download_url")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/attachments" {
		t.Errorf("path = %q", gotPath)
	}
	if got := gotQuery.Get("parent"); got != "42" {
		t.Errorf("parent query = %q", got)
	}
	if got := gotQuery.Get("limit"); got != "50" {
		t.Errorf("limit query = %q", got)
	}
	if got := gotQuery.Get("opt_fields"); got != "name,download_url" {
		t.Errorf("opt_fields query = %q", got)
	}
	var attachments []json.RawMessage
	decodeData(t, out, &attachments)
	if len(attachments) != 1 {
		t.Errorf("got %d attachments", len(attachments))
	}
}

func TestListTaskAttachmentsMissingGIDIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "list-task-attachments")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestListTaskAttachmentsLimitOutOfRangeIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "list-task-attachments", "--task-gid", "42", "--limit", "101")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestListTaskAttachmentsHuman(t *testing.T) {
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"gid":"att1","name":"Screenshot.png","host":"asana"}],"next_page":null}`))
	}, "list-task-attachments", "--task-gid", "42", "--human")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "att1 Screenshot.png [asana]\n" {
		t.Errorf("human out = %q", out)
	}
}

func TestGetAttachmentCommand(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"att1","name":"Screenshot.png","host":"asana","download_url":"https://example.test/download"}}`))
	}, "get-attachment", "--attachment-gid", "att1", "--opt-fields", "name,download_url")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/attachments/att1" {
		t.Errorf("path = %q", gotPath)
	}
	if got := gotQuery.Get("opt_fields"); got != "name,download_url" {
		t.Errorf("opt_fields query = %q", got)
	}
	var attachment struct {
		GID  string `json:"gid"`
		Name string `json:"name"`
	}
	decodeData(t, out, &attachment)
	if attachment.GID != "att1" || attachment.Name != "Screenshot.png" {
		t.Errorf("attachment = %+v", attachment)
	}
}

func TestGetAttachmentMissingGIDIsUsageError(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "get-attachment")
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestDownloadAttachmentCommand(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "Screenshot.png")
	var calls []string
	var metadataOptFields, downloadAuth string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/attachments/att1":
			calls = append(calls, "metadata")
			metadataOptFields = r.URL.Query().Get("opt_fields")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"gid":"att1","name":"Screenshot.png","download_url":"http://` + r.Host + `/download/att1"}}`))
		case "/download/att1":
			calls = append(calls, "download")
			downloadAuth = r.Header.Get("Authorization")
			w.Write([]byte("png-bytes"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}, "download-attachment", "--attachment-gid", "att1", "--output", outPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 || calls[0] != "metadata" || calls[1] != "download" {
		t.Errorf("calls = %v", calls)
	}
	if metadataOptFields != "gid,name,download_url" {
		t.Errorf("metadata opt_fields = %q", metadataOptFields)
	}
	if downloadAuth != "Bearer tok" {
		t.Errorf("download auth = %q", downloadAuth)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(body) != "png-bytes" {
		t.Errorf("output body = %q", body)
	}
	var result struct {
		GID          string `json:"gid"`
		Name         string `json:"name"`
		OutputPath   string `json:"output_path"`
		BytesWritten int64  `json:"bytes_written"`
	}
	decodeData(t, out, &result)
	if result.GID != "att1" || result.Name != "Screenshot.png" || result.OutputPath != outPath || result.BytesWritten != int64(len("png-bytes")) {
		t.Errorf("result = %+v", result)
	}
}

func TestDownloadAttachmentRefusesOverwriteByDefault(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "Screenshot.png")
	if err := os.WriteFile(outPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called")
	}, "download-attachment", "--attachment-gid", "att1", "--output", outPath)
	if exitCodeFor(err) != exitUsage {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
	body, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatalf("read existing: %v", readErr)
	}
	if string(body) != "old" {
		t.Errorf("existing file body = %q", body)
	}
}

func TestDownloadAttachmentOverwrite(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "Screenshot.png")
	if err := os.WriteFile(outPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/attachments/att1":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"gid":"att1","name":"Screenshot.png","download_url":"http://` + r.Host + `/download/att1"}}`))
		case "/download/att1":
			w.Write([]byte("new"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}, "download-attachment", "--attachment-gid", "att1", "--output", outPath, "--overwrite")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(body) != "new" {
		t.Errorf("output body = %q", body)
	}
}

func TestDownloadAttachmentMissingDownloadURLIsRuntimeError(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "Screenshot.png")
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"att1","name":"Screenshot.png"}}`))
	}, "download-attachment", "--attachment-gid", "att1", "--output", outPath)
	if exitCodeFor(err) != exitRuntime {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitRuntime)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Errorf("output exists after missing download_url: %v", statErr)
	}
}

func TestDownloadAttachmentFailureRemovesPartialOutput(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "Screenshot.png")
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/attachments/att1":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"gid":"att1","name":"Screenshot.png","download_url":"http://` + r.Host + `/download/att1"}}`))
		case "/download/att1":
			w.WriteHeader(500)
			w.Write([]byte("nope"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}, "download-attachment", "--attachment-gid", "att1", "--output", outPath)
	if exitCodeFor(err) != exitRuntime {
		t.Errorf("exit code = %d, want %d", exitCodeFor(err), exitRuntime)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Errorf("output exists after failed download: %v", statErr)
	}
}

func TestDownloadAttachmentMissingFlagsAreUsageErrors(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "Screenshot.png")
	tests := [][]string{
		{"download-attachment", "--output", outPath},
		{"download-attachment", "--attachment-gid", "att1"},
	}
	for _, args := range tests {
		_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("server should not be called")
		}, args...)
		if exitCodeFor(err) != exitUsage {
			t.Errorf("%v exit code = %d, want %d", args, exitCodeFor(err), exitUsage)
		}
	}
}
