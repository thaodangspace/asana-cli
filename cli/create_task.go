package cli

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

func newCreateTaskCommand() *cobra.Command {
	var (
		workspaceGID  string
		name          string
		projects      []string
		sectionGID    string
		assignee      string
		notes         string
		htmlNotes     string
		completed     bool
		dueOn         string
		dueAt         string
		startOn       string
		startAt       string
		followers     []string
		customFields  []string
		parentTaskGID string
	)
	cmd := &cobra.Command{
		Use:   "create-task",
		Short: "Create an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("create-task does not accept positional arguments")
			}
			taskName, err := requireNonEmptyName(name)
			if err != nil {
				return err
			}
			if err := validateGIDList(projects, "project-gid"); err != nil {
				return err
			}
			section := strings.TrimSpace(sectionGID)
			parent := strings.TrimSpace(parentTaskGID)

			data := map[string]any{"name": taskName}
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
			if err := addCustomFields(data, customFields); err != nil {
				return err
			}

			for i := range projects {
				projects[i] = strings.TrimSpace(projects[i])
			}
			if section != "" && len(projects) != 1 {
				return usageErrorf("--section-gid requires exactly one --project-gid")
			}

			c, cfg, err := buildClient()
			if err != nil {
				return err
			}
			workspace := ""
			if strings.TrimSpace(workspaceGID) != "" || (len(projects) == 0 && parent == "") {
				workspace, err = cfg.ResolveWorkspace(workspaceGID)
				if err != nil {
					return &usageError{err: err}
				}
			}
			if err := validateTaskContext(workspace, projects, parent, section); err != nil {
				return err
			}
			if workspace != "" {
				data["workspace"] = workspace
			}
			if len(projects) > 0 {
				data["projects"] = projects
			}
			if parent != "" {
				data["parent"] = parent
			}
			addSectionMembership(data, projects, section)

			ctx, cancel := withTimeout(cmd)
			defer cancel()
			raw, err := requestData(ctx, c, http.MethodPost, "/tasks", map[string]any{"data": data})
			if err != nil {
				return err
			}
			human := fmt.Sprintf("Created task: %s", summarizeTask(raw))
			return writeSuccess(cmd.OutOrStdout(), raw, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE when needed)")
	cmd.Flags().StringVar(&name, "name", "", "task name (required)")
	cmd.Flags().StringArrayVar(&projects, "project-gid", nil, "project GID (repeatable)")
	cmd.Flags().StringVar(&sectionGID, "section-gid", "", "initial section GID (requires exactly one --project-gid)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee user GID or me")
	cmd.Flags().StringVar(&notes, "notes", "", "plain-text task description")
	cmd.Flags().StringVar(&htmlNotes, "html-notes", "", "HTML task description (mutually exclusive with --notes)")
	cmd.Flags().BoolVar(&completed, "completed", false, "initial completion state (sent only when set)")
	cmd.Flags().StringVar(&dueOn, "due-on", "", "due date YYYY-MM-DD")
	cmd.Flags().StringVar(&dueAt, "due-at", "", "due date-time in RFC 3339")
	cmd.Flags().StringVar(&startOn, "start-on", "", "start date YYYY-MM-DD")
	cmd.Flags().StringVar(&startAt, "start-at", "", "start date-time in RFC 3339")
	cmd.Flags().StringArrayVar(&followers, "follower", nil, "follower user GID (repeatable; empty value clears followers)")
	cmd.Flags().StringArrayVar(&customFields, "custom-field", nil, "custom field assignment FIELD_GID=VALUE (repeatable; use json: for typed JSON)")
	cmd.Flags().StringVar(&parentTaskGID, "parent-task-gid", "", "parent task GID")
	return cmd
}
