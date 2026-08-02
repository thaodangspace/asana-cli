package cli

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newCommentOnTaskCommand() *cobra.Command {
	var (
		taskGID string
		text    string
	)
	cmd := &cobra.Command{
		Use:   "comment-on-task",
		Short: "Create a plain-text comment story on an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			body, err := requireFlag("text", text)
			if err != nil {
				return err
			}
			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			path := "/tasks/" + asana.EncodePathSegment(gid) + "/stories"
			payload := map[string]any{"data": map[string]string{"text": body}}
			data, err := requestData(ctx, c, http.MethodPost, path, payload)
			if err != nil {
				return err
			}
			story := parseResource(data)
			human := fmt.Sprintf("Posted comment %s on task %s.", orUnknown(story.GID), gid)
			return writeSuccess(cmd.OutOrStdout(), data, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID (required)")
	cmd.Flags().StringVar(&text, "text", "", "plain-text comment body to post (required)")
	return cmd
}
