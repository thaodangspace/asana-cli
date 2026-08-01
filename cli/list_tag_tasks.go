package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dtonair/asana-cli/asana"
)

func newListTagTasksCommand() *cobra.Command {
	var (
		tagGID    string
		limit     int
		optFields string
	)
	cmd := &cobra.Command{
		Use:   "list-tag-tasks",
		Short: "List tasks with an Asana tag (GET /tags/{tag_gid}/tasks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, err := requireFlag("tag-gid", tagGID)
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
			path := "/tags/" + asana.EncodePathSegment(gid) + "/tasks" + querySuffix(q)
			items, err := c.Paginate(ctx, path, limit, maxPages)
			if err != nil {
				return err
			}
			human := humanList(items, summarizeTask, "No tasks found.")
			return writeSuccess(cmd.OutOrStdout(), items, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&tagGID, "tag-gid", "", "Asana tag GID (required)")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum items to return (1-500)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
