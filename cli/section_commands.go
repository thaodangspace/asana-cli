package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

func newListSectionsCommand() *cobra.Command {
	var project, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: "list-sections", Short: "List sections in an Asana project", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("list-sections does not accept positional arguments")
		}
		id, err := requireFlag("project-gid", project)
		if err != nil {
			return err
		}
		limit, err := pagination.validate(cmd, 100)
		if err != nil {
			return err
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("limit", strconv.Itoa(pageSize))
		if pagination.offset != "" {
			q.Set("offset", pagination.offset)
		}
		appendOptFields(q, optFields)
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		result, err := c.Paginate(ctx, "/projects/"+asana.EncodePathSegment(id)+"/sections?"+q.Encode(), limit, paginationPageLimit(cmd, &pagination))
		if err != nil {
			return err
		}
		return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, humanList(result.Items, summarizeSection, "No sections found."))
	}}
	cmd.Flags().StringVar(&project, "project-gid", "", "Asana project GID (required)")
	pagination.addFlags(cmd, 100)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newGetSectionCommand() *cobra.Command {
	var section, optFields string
	cmd := &cobra.Command{Use: "get-section", Short: "Get a single Asana section by GID", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("get-section does not accept positional arguments")
		}
		id, err := requireFlag("section-gid", section)
		if err != nil {
			return err
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		q := url.Values{}
		appendOptFields(q, optFields)
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		data, err := requestData(ctx, c, http.MethodGet, "/sections/"+asana.EncodePathSegment(id)+querySuffix(q), nil)
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), data, opts.human, summarizeSection(data))
	}}
	cmd.Flags().StringVar(&section, "section-gid", "", "Asana section GID (required)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newCreateSectionCommand() *cobra.Command {
	var project, name string
	cmd := &cobra.Command{Use: "create-section", Short: "Create a section in an Asana project", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("create-section does not accept positional arguments")
		}
		projectID, err := requireFlag("project-gid", project)
		if err != nil {
			return err
		}
		sectionName, err := requireNonEmptyName(name)
		if err != nil {
			return err
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		raw, err := requestData(ctx, c, http.MethodPost, "/projects/"+asana.EncodePathSegment(projectID)+"/sections", map[string]any{"data": map[string]any{"name": sectionName}})
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), raw, opts.human, "Created section: "+summarizeSection(raw))
	}}
	cmd.Flags().StringVar(&project, "project-gid", "", "Asana project GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "section name (required)")
	return cmd
}

func newUpdateSectionCommand() *cobra.Command {
	var section, name string
	cmd := &cobra.Command{Use: "update-section", Short: "Rename an Asana section", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("update-section does not accept positional arguments")
		}
		id, err := requireFlag("section-gid", section)
		if err != nil {
			return err
		}
		if !cmd.Flags().Changed("name") {
			return usageErrorf("--name is required")
		}
		value, err := requireNonEmptyName(name)
		if err != nil {
			return err
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		raw, err := requestData(ctx, c, http.MethodPut, "/sections/"+asana.EncodePathSegment(id), map[string]any{"data": map[string]any{"name": value}})
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), raw, opts.human, "Updated section: "+summarizeSection(raw))
	}}
	cmd.Flags().StringVar(&section, "section-gid", "", "Asana section GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "new section name (required)")
	return cmd
}

func newDeleteSectionCommand() *cobra.Command {
	var section string
	var confirm, yes bool
	cmd := &cobra.Command{Use: "delete-section", Short: "Delete an Asana section", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("delete-section does not accept positional arguments")
		}
		id, err := requireFlag("section-gid", section)
		if err != nil {
			return err
		}
		if !confirm && !yes {
			return usageErrorf("deleting a section requires --confirm or --yes")
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		if _, err := c.Request(ctx, http.MethodDelete, "/sections/"+asana.EncodePathSegment(id), nil); err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), nil, opts.human, fmt.Sprintf("Deleted section %s.", id))
	}}
	cmd.Flags().StringVar(&section, "section-gid", "", "Asana section GID (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm this destructive operation")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm this destructive operation for automation")
	return cmd
}

func positionData(cmd *cobra.Command, before, after, beforeFlag, afterFlag, beforeKey, afterKey string) (map[string]any, error) {
	beforeSet, afterSet := cmd.Flags().Changed(beforeFlag), cmd.Flags().Changed(afterFlag)
	if beforeSet && afterSet {
		return nil, usageErrorf("--%s cannot be combined with --%s", beforeFlag, afterFlag)
	}
	data := map[string]any{}
	if beforeSet {
		value, err := requireFlag(beforeFlag, before)
		if err != nil {
			return nil, err
		}
		data[beforeKey] = value
	}
	if afterSet {
		value, err := requireFlag(afterFlag, after)
		if err != nil {
			return nil, err
		}
		data[afterKey] = value
	}
	return data, nil
}

func newMoveSectionCommand() *cobra.Command {
	var section, before, after string
	cmd := &cobra.Command{Use: "move-section", Short: "Move an Asana section", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("move-section does not accept positional arguments")
		}
		id, err := requireFlag("section-gid", section)
		if err != nil {
			return err
		}
		data, err := positionData(cmd, before, after, "before-section-gid", "after-section-gid", "before_section", "after_section")
		if err != nil {
			return err
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		raw, err := requestDataAllowEmpty(ctx, c, http.MethodPost, "/sections/"+asana.EncodePathSegment(id)+"/insert", map[string]any{"data": data}, true)
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), raw, opts.human, "Moved section: "+summarizeSection(raw))
	}}
	cmd.Flags().StringVar(&section, "section-gid", "", "Asana section GID (required)")
	cmd.Flags().StringVar(&before, "before-section-gid", "", "place before this section")
	cmd.Flags().StringVar(&after, "after-section-gid", "", "place after this section")
	return cmd
}

func newAddTaskToSectionCommand() *cobra.Command {
	var section, task, before, after string
	cmd := &cobra.Command{Use: "add-task-to-section", Short: "Add and optionally position a task in a section", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("add-task-to-section does not accept positional arguments")
		}
		sectionID, err := requireFlag("section-gid", section)
		if err != nil {
			return err
		}
		taskID, err := requireFlag("task-gid", task)
		if err != nil {
			return err
		}
		data, err := positionData(cmd, before, after, "before-task-gid", "after-task-gid", "insert_before", "insert_after")
		if err != nil {
			return err
		}
		data["task"] = taskID
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		if _, err := c.Request(ctx, http.MethodPost, "/sections/"+asana.EncodePathSegment(sectionID)+"/addTask", map[string]any{"data": data}); err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), nil, opts.human, fmt.Sprintf("Added task %s to section %s.", taskID, sectionID))
	}}
	cmd.Flags().StringVar(&section, "section-gid", "", "Asana section GID (required)")
	cmd.Flags().StringVar(&task, "task-gid", "", "Asana task GID (required)")
	cmd.Flags().StringVar(&before, "before-task-gid", "", "place before this task")
	cmd.Flags().StringVar(&after, "after-task-gid", "", "place after this task")
	return cmd
}

func newListSectionTasksCommand() *cobra.Command {
	var section, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: "list-section-tasks", Short: "List tasks in an Asana section", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("list-section-tasks does not accept positional arguments")
		}
		id, err := requireFlag("section-gid", section)
		if err != nil {
			return err
		}
		limit, err := pagination.validate(cmd, 100)
		if err != nil {
			return err
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("limit", strconv.Itoa(pageSize))
		if pagination.offset != "" {
			q.Set("offset", pagination.offset)
		}
		appendOptFields(q, optFields)
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		result, err := c.Paginate(ctx, "/sections/"+asana.EncodePathSegment(id)+"/tasks?"+q.Encode(), limit, paginationPageLimit(cmd, &pagination))
		if err != nil {
			return err
		}
		return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, humanList(result.Items, summarizeTask, "No tasks found."))
	}}
	cmd.Flags().StringVar(&section, "section-gid", "", "Asana section GID (required)")
	pagination.addFlags(cmd, 100)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
