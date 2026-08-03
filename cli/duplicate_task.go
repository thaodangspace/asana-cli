package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newDuplicateTaskCommand() *cobra.Command {
	var (
		taskGID string
		name    string
		include []string
	)
	cmd := &cobra.Command{
		Use:   "duplicate-task",
		Short: "Duplicate an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("duplicate-task does not accept positional arguments")
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			newName, err := requireNonEmptyName(name)
			if err != nil {
				return err
			}
			options, err := parseIncludeOptions(include)
			if err != nil {
				return err
			}
			data := map[string]any{"name": newName}
			for key, value := range options {
				data[key] = value
			}

			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/duplicate"
			raw, err := requestData(ctx, c, http.MethodPost, path, map[string]any{"data": data})
			if err != nil {
				return err
			}
			var job struct {
				GID    string `json:"gid"`
				Status string `json:"status"`
			}
			_ = json.Unmarshal(raw, &job)
			human := fmt.Sprintf("Started duplication job %s", orUnknown(job.GID))
			if job.Status != "" {
				human += " [" + job.Status + "]"
			}
			human += "."
			return writeSuccess(cmd.OutOrStdout(), raw, opts.human, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "source task GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "name for the duplicated task (required)")
	cmd.Flags().StringArrayVar(&include, "include", nil, "duplicate option (repeatable or comma-separated; e.g. subtasks,stories)")
	return cmd
}
