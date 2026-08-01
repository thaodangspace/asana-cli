package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dtonair/asana-cli/asana"
)

func newListProjectTasksCommand() *cobra.Command {
	var (
		projectGID string
		limit      int
		optFields  string
	)
	cmd := &cobra.Command{
		Use:   "list-project-tasks",
		Short: "List tasks in an Asana project (GET /projects/{project_gid}/tasks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, err := requireFlag("project-gid", projectGID)
			if err != nil {
				return err
			}
			if limit < 1 || limit > maxPages*pageSize {
				return usageErrorf("--limit must be between 1 and %d, got %d", maxPages*pageSize, limit)
			}
			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			q := url.Values{}
			q.Set("limit", strconv.Itoa(pageSize))
			appendOptFields(q, optFields)
			path := "/projects/" + asana.EncodePathSegment(gid) + "/tasks" + querySuffix(q)
			items, err := c.Paginate(ctx, path, limit, maxPages)
			if err != nil {
				return err
			}
			human := humanList(items, summarizeTask, "No tasks found.")
			return writeSuccess(cmd.OutOrStdout(), items, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&projectGID, "project-gid", "", "Asana project GID (required)")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum items to return (1-500)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
