package cli

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newGetAttachmentCommand() *cobra.Command {
	var (
		attachmentGID string
		optFields     string
	)
	cmd := &cobra.Command{
		Use:   "get-attachment",
		Short: "Get a single Asana attachment by GID",
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, err := requireFlag("attachment-gid", attachmentGID)
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
			path := "/attachments/" + asana.EncodePathSegment(gid) + querySuffix(q)
			data, err := requestData(ctx, c, http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			return writeSuccess(cmd.OutOrStdout(), data, opts.human, summarizeAttachment(data))
		},
	}
	cmd.Flags().StringVar(&attachmentGID, "attachment-gid", "", "Asana attachment GID (required)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
