package cli

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newDeleteAttachmentCommand() *cobra.Command {
	var (
		attachmentGID string
		confirm       bool
		yes           bool
	)
	cmd := &cobra.Command{
		Use:   "delete-attachment",
		Short: "Delete an Asana attachment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("delete-attachment does not accept positional arguments")
			}
			gid, err := requireFlag("attachment-gid", attachmentGID)
			if err != nil {
				return err
			}
			if !confirm && !yes {
				return usageErrorf("deleting an attachment requires --confirm or --yes")
			}
			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()
			path := "/attachments/" + asana.EncodePathSegment(gid)
			_, err = c.Request(ctx, http.MethodDelete, path, nil)
			if err != nil {
				return err
			}
			return writeSuccess(cmd.OutOrStdout(), nil, opts.human, fmt.Sprintf("Deleted attachment %s.", gid))
		},
	}
	cmd.Flags().StringVar(&attachmentGID, "attachment-gid", "", "Asana attachment GID (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm this destructive operation")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm this destructive operation for automation")
	return cmd
}
