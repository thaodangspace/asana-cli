package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dtonair/asana-cli/asana"
)

func newListProjectsCommand() *cobra.Command {
	var (
		workspaceGID string
		pagination   paginationOptions
		optFields    string
	)
	cmd := &cobra.Command{
		Use:   "list-projects",
		Short: "List projects in an Asana workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, err := pagination.validate(cmd, 100)
			if err != nil {
				return err
			}
			c, cfg, err := buildClient()
			if err != nil {
				return err
			}
			workspace, err := cfg.ResolveWorkspace(workspaceGID)
			if err != nil {
				return &usageError{err: err}
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			q := url.Values{}
			q.Set("limit", strconv.Itoa(pageSize))
			if pagination.offset != "" {
				q.Set("offset", pagination.offset)
			}
			appendOptFields(q, optFields)
			path := "/workspaces/" + asana.EncodePathSegment(workspace) + "/projects" + querySuffix(q)
			result, err := c.Paginate(ctx, path, limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			human := humanList(result.Items, summarizeProject, "No projects found.")
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, human)
		},
	}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
