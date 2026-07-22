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
		taskGID  string
		filePath string
		name     string
	)
	cmd := &cobra.Command{
		Use:   "add-attachment",
		Short: "Upload a local file as an attachment on an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, err := requireFlag("task-gid", taskGID)
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
			human := fmt.Sprintf("Attached %s (%s) to task %s.", orUnknown(attachment.Name), orUnknown(attachment.GID), gid)
			return writeSuccess(cmd.OutOrStdout(), env.Data, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID to attach the file to (required)")
	cmd.Flags().StringVar(&filePath, "file", "", "path to the local file to upload (required)")
	cmd.Flags().StringVar(&name, "name", "", "display name for the attachment (defaults to the file's base name)")
	return cmd
}
