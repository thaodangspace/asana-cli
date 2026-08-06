package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

var validTagColors = map[string]bool{
	"none":      true,
	"dark-pink": true, "dark-green": true, "dark-blue": true, "dark-red": true,
	"dark-teal": true, "dark-brown": true, "dark-orange": true, "dark-purple": true, "dark-warm-gray": true,
	"light-pink": true, "light-green": true, "light-blue": true, "light-red": true,
	"light-teal": true, "light-brown": true, "light-orange": true, "light-purple": true, "light-warm-gray": true,
}

func validateTagColor(color string) (string, error) {
	color = strings.TrimSpace(color)
	if color == "" {
		return "", usageErrorf("--color must be a valid Asana tag color")
	}
	if !validTagColors[color] {
		return "", usageErrorf("invalid tag color %q", color)
	}
	return color, nil
}

func newGetTagCommand() *cobra.Command {
	var tagGID, optFields string
	return newGetResourceCommand("get-tag", "Get an Asana tag", "tag-gid", &tagGID, &optFields, "/tags/", "Asana tag")
}

func newListTagsCommand() *cobra.Command {
	var workspaceGID, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: "list-tags", Short: "List tags in an Asana workspace", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("list-tags does not accept positional arguments")
		}
		_, cfg, err := buildClient()
		if err != nil {
			return err
		}
		workspace, err := cfg.ResolveWorkspace(workspaceGID)
		if err != nil {
			return &usageError{err: err}
		}
		return resourceListCommand(cmd, "/workspaces/"+asana.EncodePathSegment(workspace)+"/tags", &pagination, optFields, "No tags found.", summarizeTag)
	}}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newCreateTagCommand() *cobra.Command {
	var workspaceGID, name, color string
	cmd := &cobra.Command{Use: "create-tag", Short: "Create an Asana tag", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("create-tag does not accept positional arguments")
		}
		workspace, err := requireFlag("workspace-gid", workspaceGID)
		if err != nil {
			return err
		}
		tagName, err := requireFlag("name", name)
		if err != nil {
			return err
		}
		data := map[string]any{"workspace": workspace, "name": tagName}
		if cmd.Flags().Changed("color") {
			validated, err := validateTagColor(color)
			if err != nil {
				return err
			}
			data["color"] = validated
		}
		return executeTagWrite(cmd, http.MethodPost, "/tags", data, "Created tag.")
	}}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "tag name (required)")
	cmd.Flags().StringVar(&color, "color", "", "Asana tag color")
	return cmd
}

func newUpdateTagCommand() *cobra.Command {
	var tagGID, name, color string
	cmd := &cobra.Command{Use: "update-tag", Short: "Update an Asana tag", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("update-tag does not accept positional arguments")
		}
		gid, err := requireFlag("tag-gid", tagGID)
		if err != nil {
			return err
		}
		if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("color") {
			return usageErrorf("at least one of --name or --color is required")
		}
		data := map[string]any{}
		if cmd.Flags().Changed("name") {
			name, err := requireFlag("name", name)
			if err != nil {
				return err
			}
			data["name"] = name
		}
		if cmd.Flags().Changed("color") {
			validated, err := validateTagColor(color)
			if err != nil {
				return err
			}
			data["color"] = validated
		}
		return executeTagWrite(cmd, http.MethodPut, "/tags/"+asana.EncodePathSegment(gid), data, "Updated tag.")
	}}
	cmd.Flags().StringVar(&tagGID, "tag-gid", "", "Asana tag GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "tag name")
	cmd.Flags().StringVar(&color, "color", "", "Asana tag color")
	return cmd
}

func newDeleteTagCommand() *cobra.Command {
	var tagGID string
	var confirm, yes bool
	cmd := &cobra.Command{Use: "delete-tag", Short: "Delete an Asana tag", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("delete-tag does not accept positional arguments")
		}
		gid, err := requireFlag("tag-gid", tagGID)
		if err != nil {
			return err
		}
		if !confirm && !yes {
			return usageErrorf("deleting a tag requires --confirm or --yes")
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		if _, err := c.Request(ctx, http.MethodDelete, "/tags/"+asana.EncodePathSegment(gid), nil); err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), nil, opts.human, fmt.Sprintf("Deleted tag %s.", gid))
	}}
	cmd.Flags().StringVar(&tagGID, "tag-gid", "", "Asana tag GID (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm this destructive operation")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm this destructive operation for automation")
	return cmd
}

func executeTagWrite(cmd *cobra.Command, method, path string, data map[string]any, human string) error {
	c, _, err := buildClient()
	if err != nil {
		return err
	}
	ctx, cancel := withTimeout(cmd)
	defer cancel()
	raw, err := c.Request(ctx, method, path, map[string]any{"data": data})
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return writeSuccess(cmd.OutOrStdout(), nil, opts.human, human)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return writeSuccess(cmd.OutOrStdout(), env.Data, opts.human, human)
}

func summarizeTag(raw json.RawMessage) string {
	var tag struct {
		GID  string `json:"gid"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &tag)
	if tag.Name == "" {
		tag.Name = "(unnamed tag)"
	}
	return fmt.Sprintf("%s %s", orUnknown(tag.GID), tag.Name)
}
