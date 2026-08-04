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
		parentTaskGID string
		fields        commonTaskFields
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
			if err := fields.addTo(cmd, data); err != nil {
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
	fields.addFlags(cmd)
	cmd.Flags().StringVar(&parentTaskGID, "parent-task-gid", "", "parent task GID")
	return cmd
}
