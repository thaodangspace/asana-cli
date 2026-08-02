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
		pagination paginationOptions
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
			limit, err := pagination.validate(cmd, 500)
			if err != nil {
				return err
			}
			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			q := url.Values{}
			q.Set("limit", strconv.Itoa(pageSize))
			if pagination.offset != "" {
				q.Set("offset", pagination.offset)
			}
			appendOptFields(q, optFields)
			path := "/projects/" + asana.EncodePathSegment(gid) + "/tasks" + querySuffix(q)
			result, err := c.Paginate(ctx, path, limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			human := humanList(result.Items, summarizeTask, "No tasks found.")
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, human)
		},
	}
	cmd.Flags().StringVar(&projectGID, "project-gid", "", "Asana project GID (required)")
	pagination.addFlags(cmd, 100)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
