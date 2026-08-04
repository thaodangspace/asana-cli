package cli

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// parseCustomFields accepts FIELD_GID=VALUE assignments. Scalar values remain
// strings so numeric Asana enum/people GIDs cannot be accidentally converted to
// numbers. Prefix a value with json: to send a JSON number, boolean, null,
// array, object, or quoted string. This covers Asana text, number, enum,
// multi-enum, date, and people custom fields without requiring field metadata.
func parseCustomFields(assignments []string) (map[string]any, error) {
	if len(assignments) == 0 {
		return nil, nil
	}
	fields := make(map[string]any, len(assignments))
	for _, assignment := range assignments {
		separator := strings.IndexByte(assignment, '=')
		if separator <= 0 {
			return nil, usageErrorf("--custom-field must use FIELD_GID=VALUE form, got %q", assignment)
		}
		gid := strings.TrimSpace(assignment[:separator])
		if gid == "" {
			return nil, usageErrorf("--custom-field field GID must not be empty")
		}
		if _, exists := fields[gid]; exists {
			return nil, usageErrorf("duplicate --custom-field assignment for %q", gid)
		}

		raw := strings.TrimSpace(assignment[separator+1:])
		if raw == "" {
			// An empty value is the convenient command-line spelling for clearing
			// a custom field; null is the representation accepted by Asana.
			fields[gid] = nil
			continue
		}
		if !strings.HasPrefix(raw, "json:") {
			fields[gid] = raw
			continue
		}
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "json:"))
		if raw == "" || !json.Valid([]byte(raw)) {
			return nil, usageErrorf("--custom-field json value for %q must be valid JSON", gid)
		}
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, usageErrorf("invalid --custom-field value for %q: %v", gid, err)
		}
		fields[gid] = value
	}
	return fields, nil
}

func validateDate(value, flag string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return nil, usageErrorf("--%s must be YYYY-MM-DD, got %q", flag, value)
	}
	return value, nil
}

func validateDateTime(value, flag string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return nil, usageErrorf("--%s must be RFC 3339, got %q", flag, value)
	}
	return value, nil
}

// validateStartDependency enforces Asana's requirement that a start date and
// its corresponding due date are supplied in the same request.
func validateStartDependency(cmd *cobra.Command) error {
	if cmd.Flags().Changed("start-at") && !cmd.Flags().Changed("due-at") {
		return usageErrorf("--start-at requires --due-at in the same invocation")
	}
	if cmd.Flags().Changed("start-on") && !cmd.Flags().Changed("due-on") && !cmd.Flags().Changed("due-at") {
		return usageErrorf("--start-on requires --due-on or --due-at in the same invocation")
	}
	return nil
}

func addDatePair(cmd *cobra.Command, data map[string]any, dateFlag, dateValue, dateKey, timeFlag, timeValue, timeKey string) error {
	dateSet := cmd.Flags().Changed(dateFlag)
	timeSet := cmd.Flags().Changed(timeFlag)
	if dateSet && timeSet {
		return usageErrorf("--%s cannot be combined with --%s", dateFlag, timeFlag)
	}
	if dateSet {
		value, err := validateDate(dateValue, dateFlag)
		if err != nil {
			return err
		}
		data[dateKey] = value
	}
	if timeSet {
		value, err := validateDateTime(timeValue, timeFlag)
		if err != nil {
			return err
		}
		data[timeKey] = value
	}
	return nil
}

func addNotes(data map[string]any, cmd *cobra.Command, notes, htmlNotes string) error {
	notesSet := cmd.Flags().Changed("notes")
	htmlSet := cmd.Flags().Changed("html-notes")
	if notesSet && htmlSet {
		return usageErrorf("--notes cannot be combined with --html-notes")
	}
	if notesSet {
		data["notes"] = notes
	}
	if htmlSet {
		data["html_notes"] = htmlNotes
	}
	return nil
}

func addAssignee(data map[string]any, cmd *cobra.Command, assignee string) {
	if !cmd.Flags().Changed("assignee") {
		return
	}
	if strings.TrimSpace(assignee) == "" {
		data["assignee"] = nil
	} else {
		data["assignee"] = strings.TrimSpace(assignee)
	}
}

func addFollowers(data map[string]any, cmd *cobra.Command, followers []string) error {
	if !cmd.Flags().Changed("follower") {
		return nil
	}
	if len(followers) == 1 && strings.TrimSpace(followers[0]) == "" {
		data["followers"] = []string{}
		return nil
	}
	result := make([]string, 0, len(followers))
	for _, follower := range followers {
		follower = strings.TrimSpace(follower)
		if follower == "" {
			return usageErrorf("--follower values cannot be empty (use --follower \"\" to clear followers)")
		}
		result = append(result, follower)
	}
	data["followers"] = result
	return nil
}

func addProjects(data map[string]any, cmd *cobra.Command, projects []string) error {
	if !cmd.Flags().Changed("project-gid") {
		return nil
	}
	if len(projects) == 1 && strings.TrimSpace(projects[0]) == "" {
		data["projects"] = []string{}
		return nil
	}
	result := make([]string, 0, len(projects))
	for _, project := range projects {
		project = strings.TrimSpace(project)
		if project == "" {
			return usageErrorf("--project-gid values cannot be empty (use --project-gid \"\" to clear projects)")
		}
		result = append(result, project)
	}
	data["projects"] = result
	return nil
}

func addCustomFields(data map[string]any, assignments []string) error {
	fields, err := parseCustomFields(assignments)
	if err != nil {
		return err
	}
	if len(fields) > 0 {
		data["custom_fields"] = fields
	}
	return nil
}

// commonTaskFields contains the task fields shared by task creation commands.
// Keeping the parsing here prevents create-task and create-subtask from
// drifting apart as Asana adds task field options.
type commonTaskFields struct {
	notes        string
	htmlNotes    string
	completed    bool
	dueOn        string
	dueAt        string
	startOn      string
	startAt      string
	assignee     string
	followers    []string
	customFields []string
}

func (f *commonTaskFields) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.notes, "notes", "", "plain-text task description")
	cmd.Flags().StringVar(&f.htmlNotes, "html-notes", "", "HTML task description (mutually exclusive with --notes)")
	cmd.Flags().BoolVar(&f.completed, "completed", false, "initial completion state (sent only when set)")
	cmd.Flags().StringVar(&f.dueOn, "due-on", "", "due date YYYY-MM-DD")
	cmd.Flags().StringVar(&f.dueAt, "due-at", "", "due date-time in RFC 3339")
	cmd.Flags().StringVar(&f.startOn, "start-on", "", "start date YYYY-MM-DD")
	cmd.Flags().StringVar(&f.startAt, "start-at", "", "start date-time in RFC 3339")
	cmd.Flags().StringVar(&f.assignee, "assignee", "", "assignee user GID or me")
	cmd.Flags().StringArrayVar(&f.followers, "follower", nil, "follower user GID (repeatable; empty value clears followers)")
	cmd.Flags().StringArrayVar(&f.customFields, "custom-field", nil, "custom field assignment FIELD_GID=VALUE (repeatable; use json: for typed JSON)")
}

func (f *commonTaskFields) addTo(cmd *cobra.Command, data map[string]any) error {
	if err := addNotes(data, cmd, f.notes, f.htmlNotes); err != nil {
		return err
	}
	if cmd.Flags().Changed("completed") {
		data["completed"] = f.completed
	}
	if err := validateStartDependency(cmd); err != nil {
		return err
	}
	if err := addDatePair(cmd, data, "due-on", f.dueOn, "due_on", "due-at", f.dueAt, "due_at"); err != nil {
		return err
	}
	if err := addDatePair(cmd, data, "start-on", f.startOn, "start_on", "start-at", f.startAt, "start_at"); err != nil {
		return err
	}
	addAssignee(data, cmd, f.assignee)
	if err := addFollowers(data, cmd, f.followers); err != nil {
		return err
	}
	return addCustomFields(data, f.customFields)
}

func requireNonEmptyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", usageErrorf("--name cannot be empty")
	}
	return name, nil
}

func validateTaskContext(workspace string, projects []string, parent, section string) error {
	if workspace == "" && len(projects) == 0 && parent == "" {
		return usageErrorf("one task location is required: --workspace-gid, --project-gid, or --parent-task-gid")
	}
	if section != "" && len(projects) != 1 {
		return usageErrorf("--section-gid requires exactly one --project-gid")
	}
	return nil
}

func addSectionMembership(data map[string]any, projects []string, section string) {
	if section == "" {
		return
	}
	data["memberships"] = []map[string]string{{"project": projects[0], "section": section}}
}

func validateGIDList(values []string, flag string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return usageErrorf("--%s values cannot be empty", flag)
		}
	}
	return nil
}

func parseIncludeOptions(values []string) (map[string]bool, error) {
	allowed := map[string]string{
		"subtasks":         "include_subtasks",
		"dependencies":     "include_dependencies",
		"task-subtasks":    "include_task_subtasks",
		"task_subtasks":    "include_task_subtasks",
		"parent":           "include_parent",
		"assignee":         "include_assignee",
		"followers":        "include_followers",
		"notes":            "include_notes",
		"tags":             "include_tags",
		"stories":          "include_stories",
		"custom-fields":    "include_custom_fields",
		"custom_fields":    "include_custom_fields",
		"task-memberships": "include_task_memberships",
		"task_memberships": "include_task_memberships",
	}
	result := make(map[string]bool)
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.ToLower(strings.TrimSpace(item))
			if item == "" {
				return nil, usageErrorf("--include option cannot be empty")
			}
			key, ok := allowed[item]
			if !ok {
				return nil, usageErrorf("unsupported --include option %q", item)
			}
			if result[key] {
				return nil, usageErrorf("duplicate --include option %q", item)
			}
			result[key] = true
		}
	}
	return result, nil
}
