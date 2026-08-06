package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func newAddAttachmentURLCommand() *cobra.Command {
	var (
		parentGID  string
		parentType string
		remoteURL  string
		name       string
	)
	cmd := &cobra.Command{
		Use:   "add-attachment-url",
		Short: "Attach an external HTTPS URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("add-attachment-url does not accept positional arguments")
			}
			gid, err := requireFlag("parent-gid", parentGID)
			if err != nil {
				return err
			}
			typeName, err := validateAttachmentParentType(parentType)
			if err != nil {
				return err
			}
			remote, err := validateAttachmentURL(remoteURL)
			if err != nil {
				return err
			}
			displayName, err := requireFlag("name", name)
			if err != nil {
				return err
			}

			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()
			raw, err := c.UploadURL(ctx, "/attachments", map[string]string{
				"parent":           gid,
				"url":              remote,
				"name":             displayName,
				"resource_subtype": "external",
			})
			if err != nil {
				return err
			}
			var env struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
			attachment := parseResource(env.Data)
			human := fmt.Sprintf("Attached %s (%s) to %s.", orUnknown(attachment.Name), orUnknown(attachment.GID), attachmentParentLabel(typeName, gid))
			return writeSuccess(cmd.OutOrStdout(), env.Data, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&parentGID, "parent-gid", "", "Asana parent resource GID (required)")
	addAttachmentParentTypeFlag(cmd, &parentType)
	cmd.Flags().StringVar(&remoteURL, "url", "", "external HTTPS URL to attach (required)")
	cmd.Flags().StringVar(&name, "name", "", "display name for the attachment (required)")
	return cmd
}

func validateAttachmentURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", usageErrorf("--url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || !strings.EqualFold(u.Scheme, "https") || u.Host == "" {
		return "", usageErrorf("--url must be a valid HTTPS URL")
	}
	return raw, nil
}
