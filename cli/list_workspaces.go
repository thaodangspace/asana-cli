package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func newListWorkspacesCommand() *cobra.Command {
	var (
		pagination paginationOptions
		optFields  string
	)
	cmd := &cobra.Command{
		Use:   "list-workspaces",
		Short: "List workspaces visible to the authenticated user",
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, err := pagination.validate(cmd, 100)
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
			result, err := c.Paginate(ctx, "/workspaces"+querySuffix(q), limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			human := humanList(result.Items, summarizeWorkspace, "No workspaces found.")
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, human)
		},
	}
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
