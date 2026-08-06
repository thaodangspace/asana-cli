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
	} {
		if _, err := parseCustomFields([]string{assignment}); err == nil || exitCodeFor(err) != exitUsage {
			t.Errorf("%q: error = %v, want usage error", assignment, err)
		}
	}
}

func TestLegacyColonCustomFieldValueRemainsText(t *testing.T) {
	fields, err := parseCustomFields([]string{"field=https://example.com/a:b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fields["field"]; got != "https://example.com/a:b" {
		t.Errorf("value = %#v", got)
	}
}

func TestCreateCustomFieldCommand(t *testing.T) {
	var body map[string]map[string]any
	out, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/custom_fields" {
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
	if data["name"] != "Priority" || data["resource_subtype"] != "enum" || data["workspace"] != "ws1" {
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
	if len(paths) != 2 || paths[0] != "/workspaces/ws1/custom_fields?limit=2" || paths[1] != "/workspaces/ws1/custom_fields?limit=1&offset=next" {
		t.Errorf("paths = %#v", paths)
	}
	var fields []json.RawMessage
	decodeData(t, out, &fields)
	if len(fields) != 2 {
		t.Fatalf("got %d fields", len(fields))
	}
}

func TestReorderEnumOptionUsesDocumentedPayload(t *testing.T) {
	var body map[string]map[string]any
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/custom_fields/cf1/enum_options/insert" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"opt1"}}`))
	}, "reorder-enum-option", "--custom-field-gid", "cf1", "--enum-option-gid", "opt1", "--before", "opt2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["data"]["enum_option"] != "opt1" || body["data"]["before_enum_option"] != "opt2" {
		t.Errorf("payload = %#v", body)
	}
	if _, ok := body["data"]["before_value"]; ok {
		t.Errorf("legacy before_value key present: %#v", body)
	}
}

func TestDisableEnumOptionUsesUpdate(t *testing.T) {
	var body map[string]map[string]any
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/enum_options/opt1" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"gid":"opt1","enabled":false}}`))
	}, "disable-enum-option", "--enum-option-gid", "opt1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled, ok := body["data"]["enabled"].(bool); !ok || enabled {
		t.Errorf("payload = %#v", body)
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
		if r.Method != http.MethodPost || r.URL.Path != "/portfolios/portfolio1/addCustomFieldSetting" {
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

func TestRemoveCustomFieldSettingUsesOperationEndpoint(t *testing.T) {
	var body map[string]map[string]any
	_, err := runWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/projects/project1/removeCustomFieldSetting" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":null}`))
	}, "remove-custom-field-setting", "--parent-gid", "project1", "--parent-type", "project", "--custom-field-gid", "cf1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["data"]["custom_field"] != "cf1" {
		t.Errorf("payload = %#v", body)
	}
}
