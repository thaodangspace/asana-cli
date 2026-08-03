package cli

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newDeleteTaskCommand() *cobra.Command {
	var (
		taskGID string
		confirm bool
		yes     bool
	)
	cmd := &cobra.Command{
		Use:   "delete-task",
		Short: "Delete an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("delete-task does not accept positional arguments")
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			if !confirm && !yes {
				return usageErrorf("deleting a task requires --confirm or --yes")
			}

			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()
			path := "/tasks/" + asana.EncodePathSegment(gid)
			raw, err := c.Request(ctx, http.MethodDelete, path, nil)
			if err != nil {
				return err
			}
			// Asana normally answers DELETE with 204 and an empty body. The
			// stable result is deliberately independent of that response body.
			_ = raw
			return writeSuccess(cmd.OutOrStdout(), nil, opts.human, fmt.Sprintf("Deleted task %s.", gid))
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm this destructive operation")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm this destructive operation for automation")
	return cmd
}
