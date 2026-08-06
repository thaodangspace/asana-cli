package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestParseTypedCustomFields(t *testing.T) {
	fields, err := parseCustomFields([]string{
		"text=text:hello",
		"number=number:12345678901234567890.125",
		"enum=enum:option-1",
		"multi=multi-enum:option-1,option-2",
		"date=date:2026-08-15",
		"people=people:user-1,user-2",
		"clear=null",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fields["text"]; got != "hello" {
		t.Errorf("text = %#v", got)
	}
	if got := fields["number"].(json.Number).String(); got != "12345678901234567890.125" {
		t.Errorf("number = %q", got)
	}
	if got := fields["multi"].([]string); len(got) != 2 || got[1] != "option-2" {
		t.Errorf("multi = %#v", got)
	}
	if fields["clear"] != nil {
		t.Errorf("clear = %#v, want nil", fields["clear"])
	}
}

func TestParseTypedCustomFieldsRejectsInvalidValues(t *testing.T) {
	for _, assignment := range []string{
		"field=number:not-a-number",
		"field=people:user-1,user-1",
		"field=multi-enum:user-1,",
		"field=unsupported:value",
	} {
		if _, err := parseCustomFields([]string{assignment}); err == nil || exitCodeFor(err) != exitUsage {
			t.Errorf("%q: error = %v, want usage error", assignment, err)
		}
	}
}

func TestCreateCustomFieldCommand(t *testing.T) {
	var body map[string]map[string]any
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/workspaces/ws1/custom_fields" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"cf1","name":"Priority","resource_subtype":"enum"}}`))
	}, "create-custom-field", "--workspace-gid", "ws1", "--name", "Priority", "--resource-subtype", "enum", "--precision", "0", "--representation-options", `{"foo":"bar"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := body["data"]
	if data["name"] != "Priority" || data["resource_subtype"] != "enum" {
		t.Errorf("definition = %#v", data)
	}
	if got, ok := data["precision"].(float64); !ok || got != 0 {
		t.Errorf("precision = %#v", data["precision"])
	}
	if !strings.Contains(out, `"gid": "cf1"`) {
		t.Errorf("output = %s", out)
	}
}

func TestListWorkspaceCustomFieldsPaginates(t *testing.T) {
	var paths []string
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("offset") == "next" {
			w.Write([]byte(`{"data":[{"gid":"cf2","name":"Second"}],"next_page":null}`))
			return
		}
		w.Write([]byte(`{"data":[{"gid":"cf1","name":"First"}],"next_page":{"offset":"next"}}`))
	}, "list-workspace-custom-fields", "--workspace-gid", "ws1", "--limit", "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 2 || !strings.HasPrefix(paths[0], "/workspaces/ws1/custom_fields?") || paths[1] != "/workspaces/ws1/custom_fields?limit=50&offset=next" {
		t.Errorf("paths = %#v", paths)
	}
	var fields []json.RawMessage
	decodeData(t, out, &fields)
	if len(fields) != 2 {
		t.Fatalf("got %d fields", len(fields))
	}
}

func TestReorderEnumOptionRequiresExactlyOnePosition(t *testing.T) {
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}, "reorder-enum-option", "--custom-field-gid", "cf1", "--enum-option-gid", "opt1")
	if err == nil || exitCodeFor(err) != exitUsage {
		t.Fatalf("error = %v, want usage error", err)
	}
}

func TestAddCustomFieldSettingPortfolio(t *testing.T) {
	var body map[string]map[string]any
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/portfolios/portfolio1/custom_field_settings" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"setting1"}}`))
	}, "add-custom-field-setting", "--parent-gid", "portfolio1", "--parent-type", "portfolio", "--custom-field-gid", "cf1", "--is-important=false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := body["data"]["is_important"].(bool); !ok || got {
		t.Errorf("is_important = %#v", body["data"]["is_important"])
	}
}
