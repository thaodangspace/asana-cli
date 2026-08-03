package cli

import "testing"

func TestParseCustomFieldsPreservesScalarGIDs(t *testing.T) {
	fields, err := parseCustomFields([]string{
		"enum=1201234567890123",
		"people=json:[\"1201234567890123\"]",
		"number=json:42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := fields["enum"].(string); !ok || got != "1201234567890123" {
		t.Errorf("enum = %#v (%T)", fields["enum"], fields["enum"])
	}
	if got, ok := fields["number"].(float64); !ok || got != 42 {
		t.Errorf("number = %#v (%T)", fields["number"], fields["number"])
	}
}

func TestParseCustomFieldsRejectsDuplicateAssignments(t *testing.T) {
	if _, err := parseCustomFields([]string{"field=one", "field=two"}); err == nil {
		t.Fatal("expected duplicate assignment error")
	}
}
