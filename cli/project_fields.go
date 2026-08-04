package cli

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// addProjectFields adds only explicitly supplied project fields. Empty values
// for nullable fields are encoded as JSON null so callers can clear them.
func addProjectFields(data map[string]any, cmd *cobra.Command, name, notes, htmlNotes, color, owner string, archived, public bool, defaultView, dueOn, dueAt, startOn string, members, followers []string) error {
	if cmd.Flags().Changed("name") {
		value, err := requireNonEmptyName(name)
		if err != nil {
			return err
		}
		data["name"] = value
	}
	if err := addNotes(data, cmd, notes, htmlNotes); err != nil {
		return err
	}
	// Project descriptions are nullable in Asana. Preserve the task command's
	// historical empty-string behavior, but make project clears explicit null.
	if cmd.Flags().Changed("notes") && strings.TrimSpace(notes) == "" {
		data["notes"] = nil
	}
	if cmd.Flags().Changed("html-notes") && strings.TrimSpace(htmlNotes) == "" {
		data["html_notes"] = nil
	}
	if cmd.Flags().Changed("color") {
		if strings.TrimSpace(color) == "" {
			data["color"] = nil
		} else {
			data["color"] = strings.TrimSpace(color)
		}
	}
	if cmd.Flags().Changed("archived") {
		data["archived"] = archived
	}
	if cmd.Flags().Changed("public") {
		data["public"] = public
	}
	if cmd.Flags().Changed("default-view") {
		if strings.TrimSpace(defaultView) == "" {
			data["default_view"] = nil
		} else {
			data["default_view"] = strings.TrimSpace(defaultView)
		}
	}
	if cmd.Flags().Changed("owner") {
		if strings.TrimSpace(owner) == "" {
			data["owner"] = nil
		} else {
			data["owner"] = strings.TrimSpace(owner)
		}
	}
	if err := validateStartDependency(cmd); err != nil {
		return err
	}
	if err := addDatePair(cmd, data, "due-on", dueOn, "due_on", "due-at", dueAt, "due_at"); err != nil {
		return err
	}
	if cmd.Flags().Changed("start-on") {
		value, err := validateDate(startOn, "start-on")
		if err != nil {
			return err
		}
		data["start_on"] = value
	}
	if err := addProjectUserList(data, cmd, "member", "members", members); err != nil {
		return err
	}
	if err := addProjectUserList(data, cmd, "follower", "followers", followers); err != nil {
		return err
	}
	return nil
}

func appendProjectSearchValue(q url.Values, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		q.Set(key, value)
	}
}

func validateProjectSearchDates(dueBefore, dueAfter, startBefore, startAfter, createdBefore, createdAfter string) error {
	for _, item := range []struct{ name, value, layout string }{
		{"due-before", dueBefore, "2006-01-02"}, {"due-after", dueAfter, "2006-01-02"},
		{"start-before", startBefore, "2006-01-02"}, {"start-after", startAfter, "2006-01-02"},
		{"created-before", createdBefore, time.RFC3339}, {"created-after", createdAfter, time.RFC3339},
	} {
		if strings.TrimSpace(item.value) == "" {
			continue
		}
		if _, err := time.Parse(item.layout, strings.TrimSpace(item.value)); err != nil {
			return usageErrorf("--%s has an invalid date value %q", item.name, item.value)
		}
	}
	for _, item := range []struct{ name, after, before, layout string }{
		{"due", dueAfter, dueBefore, "2006-01-02"}, {"start", startAfter, startBefore, "2006-01-02"},
		{"created", createdAfter, createdBefore, time.RFC3339},
	} {
		if strings.TrimSpace(item.after) == "" || strings.TrimSpace(item.before) == "" {
			continue
		}
		a, _ := time.Parse(item.layout, strings.TrimSpace(item.after))
		b, _ := time.Parse(item.layout, strings.TrimSpace(item.before))
		if a.After(b) {
			return usageErrorf("--%s-after must not be later than --%s-before", item.name, item.name)
		}
	}
	return nil
}

func mergeProjectQueries(q url.Values, queries []string) error {
	for _, item := range queries {
		at := strings.IndexByte(item, '=')
		if at <= 0 {
			return usageErrorf("--query must use key=value form, got %q", item)
		}
		key, value := strings.TrimSpace(item[:at]), item[at+1:]
		if key == "" {
			return usageErrorf("--query key must not be empty")
		}
		if key == "limit" || key == "offset" {
			return usageErrorf("--query cannot override pagination parameter %q; use the corresponding flag", key)
		}
		if existing, ok := q[key]; ok && existing[0] != value {
			return usageErrorf("conflicting values for scalar query parameter %q", key)
		}
		q.Set(key, value)
	}
	return nil
}

func anyFlagChanged(cmd *cobra.Command, names ...string) bool {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func addProjectUserList(data map[string]any, cmd *cobra.Command, flag, key string, values []string) error {
	aliases := []string{flag}
	if flag == "member" {
		aliases = append(aliases, "members")
	}
	if flag == "follower" {
		aliases = append(aliases, "followers")
	}
	if !anyFlagChanged(cmd, aliases...) {
		return nil
	}
	if len(values) == 1 && strings.TrimSpace(values[0]) == "" {
		data[key] = []string{}
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return usageErrorf("--%s values cannot be empty (use --%s \"\" to clear)", flag, flag)
		}
		result = append(result, value)
	}
	data[key] = result
	return nil
}

func parseProjectOptions(values []string) (map[string]any, error) {
	result := make(map[string]any, len(values))
	for _, item := range values {
		at := strings.IndexByte(item, '=')
		if at <= 0 {
			return nil, usageErrorf("--option must use key=value form, got %q", item)
		}
		key := strings.TrimSpace(item[:at])
		if key == "" {
			return nil, usageErrorf("--option key must not be empty")
		}
		if _, exists := result[key]; exists {
			return nil, usageErrorf("duplicate --option %q", key)
		}
		value := strings.TrimSpace(item[at+1:])
		if strings.HasPrefix(value, "json:") {
			raw := strings.TrimSpace(strings.TrimPrefix(value, "json:"))
			if raw == "" || !json.Valid([]byte(raw)) {
				return nil, usageErrorf("--option JSON value for %q must be valid JSON", key)
			}
			var decoded any
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				return nil, usageErrorf("invalid --option value for %q: %v", key, err)
			}
			result[key] = decoded
		} else {
			result[key] = value
		}
	}
	return result, nil
}
