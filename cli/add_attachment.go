package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newAddAttachmentCommand() *cobra.Command {
	var (
		parentGID  string
		taskGID    string
		parentType string
		filePath   string
		name       string
	)
	cmd := &cobra.Command{
		Use:   "add-attachment",
		Short: "Upload a local file as an attachment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("add-attachment does not accept positional arguments")
			}
			gid, err := resolveAttachmentParent(parentGID, taskGID)
			if err != nil {
				return err
			}
			typeName, err := validateAttachmentParentType(parentType)
			if err != nil {
				return err
			}
			path, err := requireFlag("file", filePath)
			if err != nil {
				return err
			}

			f, err := os.Open(path)
			if err != nil {
				if os.IsNotExist(err) {
					return usageErrorf("file %s does not exist", path)
				}
				return usageErrorf("cannot open file %s: %v", path, err)
			}
			defer f.Close()

			fileName := strings.TrimSpace(name)
			if fileName == "" {
				fileName = filepath.Base(path)
			}
			fields := map[string]string{"parent": gid}
			if fileName != "" {
				fields["name"] = fileName
			}

			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			raw, err := c.Upload(ctx, "/attachments", fields, "file", fileName, f)
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
	cmd.Flags().StringVar(&parentGID, "parent-gid", "", "Asana parent resource GID (required; use --task-gid for the legacy task-only alias)")
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "deprecated alias for --parent-gid (task parent only)")
	addAttachmentParentTypeFlag(cmd, &parentType)
	cmd.Flags().StringVar(&filePath, "file", "", "path to the local file to upload (required)")
	cmd.Flags().StringVar(&name, "name", "", "display name for the attachment (defaults to the file's base name)")
	return cmd
}
