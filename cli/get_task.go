package cli

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newGetTaskCommand() *cobra.Command {
	var (
		taskGID   string
		optFields string
	)
	cmd := &cobra.Command{
		Use:   "get-task",
		Short: "Get a single Asana task by GID",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			appendOptFields(q, optFields)
			path := "/tasks/" + asana.EncodePathSegment(gid) + querySuffix(q)
			data, err := requestData(ctx, c, http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			return writeSuccess(cmd.OutOrStdout(), data, opts.human, summarizeTask(data))
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID (required)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
