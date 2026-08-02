package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func newListTaskAttachmentsCommand() *cobra.Command {
	var (
		taskGID    string
		pagination paginationOptions
		optFields  string
	)
	cmd := &cobra.Command{
		Use:   "list-task-attachments",
		Short: "List attachments on an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, err := pagination.validate(cmd, 100)
			if err != nil {
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
			if pagination.offset != "" {
				q.Set("offset", pagination.offset)
			}
			appendOptFields(q, optFields)
			path := "/attachments" + querySuffix(q)
			result, err := c.Paginate(ctx, path, limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			human := humanList(result.Items, summarizeAttachment, "No attachments found.")
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
