package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newUpdateTaskCommand() *cobra.Command {
	var (
		taskGID       string
		name          string
		notes         string
		htmlNotes     string
		dueOn         string
		dueAt         string
		startOn       string
		startAt       string
		assignee      string
		completed     bool
		followers     []string
		projects      []string
		sectionGID    string
		parentTaskGID string
		customFields  []string
	)
	cmd := &cobra.Command{
		Use:   "update-task",
		Short: "Update fields on an Asana task (PUT /tasks/{gid})",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("update-task does not accept positional arguments")
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}

			data := map[string]any{}
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
			if cmd.Flags().Changed("completed") {
				data["completed"] = completed
			}
			if err := validateStartDependency(cmd); err != nil {
				return err
			}
			if err := addDatePair(cmd, data, "due-on", dueOn, "due_on", "due-at", dueAt, "due_at"); err != nil {
				return err
			}
			if err := addDatePair(cmd, data, "start-on", startOn, "start_on", "start-at", startAt, "start_at"); err != nil {
				return err
			}
			addAssignee(data, cmd, assignee)
			if err := addFollowers(data, cmd, followers); err != nil {
				return err
			}
			if !(len(projects) == 1 && strings.TrimSpace(projects[0]) == "") {
				if err := validateGIDList(projects, "project-gid"); err != nil {
					return err
				}
			}
			if err := addCustomFields(data, customFields); err != nil {
				return err
			}

			section := strings.TrimSpace(sectionGID)
			if cmd.Flags().Changed("section-gid") && section == "" {
				return usageErrorf("--section-gid cannot be empty")
			}
			relationshipChanged := cmd.Flags().Changed("project-gid") ||
				cmd.Flags().Changed("section-gid") || cmd.Flags().Changed("parent-task-gid")

			if len(data) == 0 && !relationshipChanged {
				return usageErrorf("at least one field flag must be set (--name, --notes, --html-notes, --completed, --due-on, --due-at, --start-on, --start-at, --assignee, --follower, --project-gid, --section-gid, --parent-task-gid, --custom-field)")
			}

			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			var raw json.RawMessage
			if len(data) > 0 {
				path := "/tasks/" + asana.EncodePathSegment(gid)
				raw, err = requestData(ctx, c, http.MethodPut, path, map[string]any{"data": data})
				if err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("project-gid") {
				desired := make([]string, 0, len(projects))
				for _, project := range projects {
					desired = append(desired, strings.TrimSpace(project))
				}
				if len(desired) == 1 && desired[0] == "" {
					desired = nil
				}
				if err := replaceTaskProjects(ctx, c, gid, desired); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("section-gid") {
				if err := addTaskToSection(ctx, c, gid, section); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("parent-task-gid") {
				if err := setTaskParent(ctx, c, gid, parentTaskGID); err != nil {
					return err
				}
			}
			if relationshipChanged {
				raw, err = getTaskAfterRelationships(ctx, c, gid)
				if err != nil {
					return err
				}
			}
			human := fmt.Sprintf("Updated task: %s", summarizeTask(raw))
			return writeSuccess(cmd.OutOrStdout(), raw, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "new task name (non-empty)")
	cmd.Flags().StringVar(&notes, "notes", "", "new plain-text task description; empty string clears it")
	cmd.Flags().StringVar(&htmlNotes, "html-notes", "", "new HTML task description; empty string clears it")
	cmd.Flags().BoolVar(&completed, "completed", false, "completion state (sent only when set)")
	cmd.Flags().StringVar(&dueOn, "due-on", "", "due date YYYY-MM-DD; empty string clears it")
	cmd.Flags().StringVar(&dueAt, "due-at", "", "due date-time RFC 3339; empty string clears it")
	cmd.Flags().StringVar(&startOn, "start-on", "", "start date YYYY-MM-DD; empty string clears it")
	cmd.Flags().StringVar(&startAt, "start-at", "", "start date-time RFC 3339; empty string clears it")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee user GID or me; empty string unassigns")
	cmd.Flags().StringArrayVar(&followers, "follower", nil, "follower user GID (repeatable; empty value clears followers)")
	cmd.Flags().StringArrayVar(&projects, "project-gid", nil, "replace project memberships (repeatable; empty value clears projects)")
	cmd.Flags().StringVar(&sectionGID, "section-gid", "", "move into a section using Asana's section endpoint")
	cmd.Flags().StringVar(&parentTaskGID, "parent-task-gid", "", "parent task GID; empty string clears the parent")
	cmd.Flags().StringArrayVar(&customFields, "custom-field", nil, "custom field assignment FIELD_GID=VALUE (repeatable; use json: for typed JSON)")
	return cmd
}
