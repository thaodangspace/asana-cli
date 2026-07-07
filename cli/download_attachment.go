package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dtonair/asana-cli/asana"
)

type attachmentMetadata struct {
	GID         string `json:"gid"`
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
}

type downloadAttachmentResult struct {
	GID          string `json:"gid"`
	Name         string `json:"name,omitempty"`
	OutputPath   string `json:"output_path"`
	BytesWritten int64  `json:"bytes_written"`
}

func newDownloadAttachmentCommand() *cobra.Command {
	var (
		attachmentGID string
		outputPath    string
		overwrite     bool
	)
	cmd := &cobra.Command{
		Use:   "download-attachment",
		Short: "Download an Asana attachment to a file",
		RunE: func(cmd *cobra.Command, args []string) error {
			gid, err := requireFlag("attachment-gid", attachmentGID)
			if err != nil {
				return err
			}
			out, err := requireFlag("output", outputPath)
			if err != nil {
				return err
			}
			if !overwrite {
				if _, err := os.Stat(out); err == nil {
					return usageErrorf("output file %s already exists; pass --overwrite to replace it", out)
				} else if err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			q := url.Values{}
			q.Set("opt_fields", "gid,name,download_url")
			path := "/attachments/" + asana.EncodePathSegment(gid) + querySuffix(q)
			data, err := requestData(ctx, c, http.MethodGet, path, nil)
			if err != nil {
				return err
			}

			var meta attachmentMetadata
			if err := json.Unmarshal(data, &meta); err != nil {
				return fmt.Errorf("decode attachment metadata: %w", err)
			}
			if strings.TrimSpace(meta.GID) == "" {
				meta.GID = gid
			}
			if strings.TrimSpace(meta.DownloadURL) == "" {
				return fmt.Errorf("attachment %s has no download_url", meta.GID)
			}

			written, err := downloadToFile(ctx, c, meta.DownloadURL, out, overwrite)
			if err != nil {
				return err
			}
			result := downloadAttachmentResult{
				GID:          meta.GID,
				Name:         meta.Name,
				OutputPath:   out,
				BytesWritten: written,
			}
			return writeSuccess(cmd.OutOrStdout(), result, opts.human, summarizeDownloadAttachment(result))
		},
	}
	cmd.Flags().StringVar(&attachmentGID, "attachment-gid", "", "Asana attachment GID (required)")
	cmd.Flags().StringVar(&outputPath, "output", "", "path to write the downloaded attachment (required)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite the output file if it already exists")
	return cmd
}

func downloadToFile(ctx context.Context, c *asana.Client, downloadURL, outputPath string, overwrite bool) (int64, error) {
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	f, err := os.OpenFile(outputPath, flags, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return 0, usageErrorf("output file %s already exists; pass --overwrite to replace it", outputPath)
		}
		return 0, err
	}

	success := false
	defer func() {
		if !success {
			_ = os.Remove(outputPath)
		}
	}()

	written, err := c.Download(ctx, downloadURL, f)
	if err != nil {
		_ = f.Close()
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	success = true
	return written, nil
}

func summarizeDownloadAttachment(result downloadAttachmentResult) string {
	name := result.Name
	if name == "" {
		name = result.GID
	}
	if name == "" {
		name = "attachment"
	}
	return fmt.Sprintf("Downloaded %s to %s (%d bytes)", name, result.OutputPath, result.BytesWritten)
}
