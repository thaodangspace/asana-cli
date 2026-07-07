package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func newListTaskAttachmentsCommand() *cobra.Command {
	var (
		taskGID   string
		limit     int
		optFields string
	)
	cmd := &cobra.Command{
		Use:   "list-task-attachments",
		Short: "List attachments on an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateLimit(limit); err != nil {
				return err
			}
			gid, err := requireFlag("task-gid", taskGID)
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
			q.Set("parent", gid)
			q.Set("limit", strconv.Itoa(pageSize))
			appendOptFields(q, optFields)
			path := "/attachments" + querySuffix(q)
			items, err := c.Paginate(ctx, path, limit, maxPages)
			if err != nil {
				return err
			}
			human := humanList(items, summarizeAttachment, "No attachments found.")
			return writeSuccess(cmd.OutOrStdout(), items, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID (required)")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum items to return (1-100)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
