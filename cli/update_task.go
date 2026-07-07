package cli

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dtonair/asana-cli/asana"
)

func newUpdateTaskCommand() *cobra.Command {
	var (
		taskGID   string
		name      string
		notes     string
		dueOn     string
		assignee  string
		completed bool
	)
	cmd := &cobra.Command{
		Use:   "update-task",
		Short: "Update fields on an Asana task (PUT /tasks/{gid})",
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}

			data := map[string]any{}

			if cmd.Flags().Changed("name") {
				v := strings.TrimSpace(name)
				if v == "" {
					return usageErrorf("--name cannot be empty")
				}
				data["name"] = v
			}
			if cmd.Flags().Changed("notes") {
				data["notes"] = notes
			}
			if cmd.Flags().Changed("completed") {
				data["completed"] = completed
			}
			if cmd.Flags().Changed("due-on") {
				v := strings.TrimSpace(dueOn)
				if v == "" {
					data["due_on"] = nil
				} else {
					if _, perr := time.Parse("2006-01-02", v); perr != nil {
						return usageErrorf("--due-on must be YYYY-MM-DD, got %q", dueOn)
					}
					data["due_on"] = v
				}
			}
			if cmd.Flags().Changed("assignee") {
				if v := strings.TrimSpace(assignee); v == "" {
					data["assignee"] = nil
				} else {
					data["assignee"] = v
				}
			}

			if len(data) == 0 {
				return usageErrorf("at least one field flag must be set (--name, --notes, --completed, --due-on, --assignee)")
			}

			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			path := "/tasks/" + asana.EncodePathSegment(gid)
			payload := map[string]any{"data": data}
			raw, err := requestData(ctx, c, http.MethodPut, path, payload)
			if err != nil {
				return err
			}
			human := fmt.Sprintf("Updated task %s: %s", gid, summarizeTask(raw))
			return writeSuccess(cmd.OutOrStdout(), raw, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "new task name (non-empty)")
	cmd.Flags().StringVar(&notes, "notes", "", "new task description; empty string clears it")
	cmd.Flags().BoolVar(&completed, "completed", false, "completion state (sent only when set)")
	cmd.Flags().StringVar(&dueOn, "due-on", "", "due date YYYY-MM-DD; empty string clears it")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee user GID or me; empty string unassigns")
	return cmd
}
