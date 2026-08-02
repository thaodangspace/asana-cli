package cli

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newSearchTasksCommand() *cobra.Command {
	var (
		workspaceGID string
		text         string
		assignee     string
		completed    bool
		pagination   paginationOptions
		optFields    string
	)
	cmd := &cobra.Command{
		Use:   "search-tasks",
		Short: "Search tasks in an Asana workspace (may require premium access)",
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
			if v := strings.TrimSpace(text); v != "" {
				q.Set("text", v)
			}
			if v := strings.TrimSpace(assignee); v != "" {
				q.Set("assignee.any", v)
			}
			// Tri-state: only send completed when the flag was explicitly set,
			// matching the extension's typeof === "boolean" check.
			if cmd.Flags().Changed("completed") {
				q.Set("completed", strconv.FormatBool(completed))
			}
			appendOptFields(q, optFields)

			path := "/workspaces/" + asana.EncodePathSegment(workspace) + "/tasks/search?" + q.Encode()
			result, err := c.Paginate(ctx, path, limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			human := humanList(result.Items, summarizeTask, "No tasks found.")
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, human)
		},
	}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	cmd.Flags().StringVar(&text, "text", "", "text search query")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee GID, email, or me")
	cmd.Flags().BoolVar(&completed, "completed", false, "filter by completion state (omitted unless set)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
