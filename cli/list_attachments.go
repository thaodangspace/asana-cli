package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func newListAttachmentsCommand() *cobra.Command {
	var (
		parentGID  string
		parentType string
		pagination paginationOptions
		optFields  string
	)
	cmd := &cobra.Command{
		Use:   "list-attachments",
		Short: "List attachments on an Asana task, project, or project brief",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("list-attachments does not accept positional arguments")
			}
			limit, err := pagination.validate(cmd, 100)
			if err != nil {
				return err
			}
			gid, err := requireFlag("parent-gid", parentGID)
			if err != nil {
				return err
			}
			typeName, err := validateAttachmentParentType(parentType)
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
			result, err := c.Paginate(ctx, "/attachments?"+q.Encode(), limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			human := humanList(result.Items, summarizeAttachment, "No attachments found.")
			if len(result.Items) > 0 {
				human = attachmentParentLabel(typeName, gid) + ":\n" + human
			} else {
				human = "No attachments found on " + attachmentParentLabel(typeName, gid) + "."
			}
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, human)
		},
	}
	cmd.Flags().StringVar(&parentGID, "parent-gid", "", "Asana parent resource GID (required)")
	addAttachmentParentTypeFlag(cmd, &parentType)
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
