package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

// relationshipListCommand constructs the three task-graph collection commands
// that all use the same bounded pagination behavior as the other list commands.
func relationshipListCommand(use, short, suffix, label string) *cobra.Command {
	var taskGID, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("%s does not accept positional arguments", use)
			}
			gid, err := requireFlag("task-gid", taskGID)
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
			ctx, cancel := withTimeout(cmd)
			defer cancel()
			q := url.Values{}
			q.Set("limit", strconv.Itoa(pageSize))
			if pagination.offset != "" {
				q.Set("offset", pagination.offset)
			}
			appendOptFields(q, optFields)
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/" + suffix + querySuffix(q)
			result, err := c.Paginate(ctx, path, limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			human := humanRelationshipList(result.Items, gid, label)
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "Asana task GID (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func humanRelationshipList(items []json.RawMessage, taskGID, label string) string {
	plural := map[string]string{"subtask": "subtasks", "dependency": "dependencies", "dependent": "dependents"}[label]
	if plural == "" {
		plural = label + "s"
	}
	if len(items) == 0 {
		return "No " + plural + " found."
	}
	display := strings.ToUpper(label[:1]) + label[1:]
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("%s of task %s: %s", display, taskGID, summarizeTask(item)))
	}
	return strings.Join(lines, "\n")
}

func executeRelationshipMutation(cmd *cobra.Command, method, path string, data, result map[string]any, human string) error {
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
	// Asana uses both 204/empty responses and normal data envelopes for
	// relationship endpoints. Preserve a returned resource; otherwise emit a
	// deterministic operation result rather than an ambiguous null.
	output := any(result)
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &envelope) == nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		output = envelope.Data
	}
	return writeSuccess(cmd.OutOrStdout(), output, opts.human, human)
}

func newListSubtasksCommand() *cobra.Command {
	return relationshipListCommand("list-subtasks", "List subtasks of an Asana task (GET /tasks/{gid}/subtasks)", "subtasks", "subtask")
}

func newListDependenciesCommand() *cobra.Command {
	return relationshipListCommand("list-dependencies", "List dependencies of an Asana task (GET /tasks/{gid}/dependencies)", "dependencies", "dependency")
}

func newListDependentsCommand() *cobra.Command {
	return relationshipListCommand("list-dependents", "List tasks dependent on an Asana task (GET /tasks/{gid}/dependents)", "dependents", "dependent")
}

func newCreateSubtaskCommand() *cobra.Command {
	var (
		parentGID  string
		name       string
		projects   []string
		sectionGID string
		fields     commonTaskFields
	)
	cmd := &cobra.Command{
		Use:   "create-subtask",
		Short: "Create a subtask under an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("create-subtask does not accept positional arguments")
			}
			parent, err := requireFlag("task-gid", parentGID)
			if err != nil {
				return err
			}
			taskName, err := requireNonEmptyName(name)
			if err != nil {
				return err
			}
			if err := validateGIDList(projects, "project-gid"); err != nil {
				return err
			}
			for i := range projects {
				projects[i] = strings.TrimSpace(projects[i])
			}
			section := strings.TrimSpace(sectionGID)
			if section != "" && len(projects) != 1 {
				return usageErrorf("--section-gid requires exactly one --project-gid")
			}
			data := map[string]any{"name": taskName}
			if err := fields.addTo(cmd, data); err != nil {
				return err
			}
			if len(projects) > 0 {
				data["projects"] = projects
			}
			addSectionMembership(data, projects, section)

			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()
			path := "/tasks/" + asana.EncodePathSegment(parent) + "/subtasks"
			raw, err := requestData(ctx, c, http.MethodPost, path, map[string]any{"data": data})
			if err != nil {
				return err
			}
			return writeSuccess(cmd.OutOrStdout(), raw, opts.human, fmt.Sprintf("Created subtask of %s: %s", parent, summarizeTask(raw)))
		},
	}
	cmd.Flags().StringVar(&parentGID, "task-gid", "", "parent task GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "subtask name (required)")
	cmd.Flags().StringArrayVar(&projects, "project-gid", nil, "project GID (repeatable)")
	cmd.Flags().StringVar(&sectionGID, "section-gid", "", "initial section GID (requires exactly one --project-gid)")
	fields.addFlags(cmd)
	return cmd
}

func newSetTaskParentCommand() *cobra.Command {
	var taskGID, parentGID string
	cmd := &cobra.Command{
		Use:   "set-task-parent",
		Short: "Set the parent of an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("set-task-parent does not accept positional arguments")
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			parent, err := requireFlag("parent-task-gid", parentGID)
			if err != nil {
				return err
			}
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/setParent"
			result := map[string]any{"operation": "set-task-parent", "task_gid": gid, "parent_task_gid": parent}
			return executeRelationshipMutation(cmd, http.MethodPost, path, map[string]any{"parent": parent}, result, fmt.Sprintf("Set parent of task %s to %s.", gid, parent))
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "child task GID (required)")
	cmd.Flags().StringVar(&parentGID, "parent-task-gid", "", "parent task GID (required)")
	return cmd
}

func newRemoveTaskParentCommand() *cobra.Command {
	var taskGID string
	cmd := &cobra.Command{
		Use:   "remove-task-parent",
		Short: "Remove the parent from an Asana task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("remove-task-parent does not accept positional arguments")
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/setParent"
			result := map[string]any{"operation": "remove-task-parent", "task_gid": gid, "parent_task_gid": nil}
			return executeRelationshipMutation(cmd, http.MethodPost, path, map[string]any{"parent": nil}, result, fmt.Sprintf("Removed parent from task %s.", gid))
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "task GID (required)")
	return cmd
}

func newAddDependencyCommand() *cobra.Command {
	return newDependencyMutationCommand("add-dependency", "Add a dependency to an Asana task", "addDependencies", "add-dependency")
}

func newRemoveDependencyCommand() *cobra.Command {
	return newDependencyMutationCommand("remove-dependency", "Remove a dependency from an Asana task", "removeDependencies", "remove-dependency")
}

func newDependencyMutationCommand(use, short, endpoint, operation string) *cobra.Command {
	var taskGID, dependencyGID string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("%s does not accept positional arguments", use)
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			dependency, err := requireFlag("dependency-task-gid", dependencyGID)
			if err != nil {
				return err
			}
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/" + endpoint
			result := map[string]any{"operation": operation, "task_gid": gid, "dependency_task_gid": dependency}
			humanVerb, relation := "Added", "to"
			if operation == "remove-dependency" {
				humanVerb, relation = "Removed", "from"
			}
			human := fmt.Sprintf("%s dependency %s %s task %s.", humanVerb, dependency, relation, gid)
			return executeRelationshipMutation(cmd, http.MethodPost, path, map[string]any{"dependencies": []string{dependency}}, result, human)
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "task GID (required)")
	cmd.Flags().StringVar(&dependencyGID, "dependency-task-gid", "", "dependency task GID (required)")
	return cmd
}

func newAddTaskToProjectCommand() *cobra.Command {
	var taskGID, projectGID, sectionGID, insertBefore, insertAfter string
	cmd := &cobra.Command{
		Use:   "add-task-to-project",
		Short: "Add an Asana task to a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("add-task-to-project does not accept positional arguments")
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			project, err := requireFlag("project-gid", projectGID)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("insert-before") && cmd.Flags().Changed("insert-after") {
				return usageErrorf("--insert-before cannot be combined with --insert-after")
			}
			data := map[string]any{"project": project}
			for _, item := range [][3]string{{"section-gid", "section", sectionGID}, {"insert-before", "insert_before", insertBefore}, {"insert-after", "insert_after", insertAfter}} {
				flag, key, value := item[0], item[1], item[2]
				if cmd.Flags().Changed(flag) {
					v, err := requireFlag(flag, value)
					if err != nil {
						return err
					}
					data[key] = v
				}
			}
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/addProject"
			result := map[string]any{"operation": "add-task-to-project", "task_gid": gid, "project_gid": project}
			if sectionGID != "" {
				result["section_gid"] = sectionGID
			}
			return executeRelationshipMutation(cmd, http.MethodPost, path, data, result, fmt.Sprintf("Added task %s to project %s.", gid, project))
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "task GID (required)")
	cmd.Flags().StringVar(&projectGID, "project-gid", "", "project GID (required)")
	cmd.Flags().StringVar(&sectionGID, "section-gid", "", "section GID")
	cmd.Flags().StringVar(&insertBefore, "insert-before", "", "task GID to insert before")
	cmd.Flags().StringVar(&insertAfter, "insert-after", "", "task GID to insert after")
	return cmd
}

func newRemoveTaskFromProjectCommand() *cobra.Command {
	var taskGID, projectGID string
	cmd := &cobra.Command{
		Use:   "remove-task-from-project",
		Short: "Remove an Asana task from a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("remove-task-from-project does not accept positional arguments")
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			project, err := requireFlag("project-gid", projectGID)
			if err != nil {
				return err
			}
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/removeProject"
			result := map[string]any{"operation": "remove-task-from-project", "task_gid": gid, "project_gid": project}
			return executeRelationshipMutation(cmd, http.MethodPost, path, map[string]any{"project": project}, result, fmt.Sprintf("Removed task %s from project %s.", gid, project))
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "task GID (required)")
	cmd.Flags().StringVar(&projectGID, "project-gid", "", "project GID (required)")
	return cmd
}

func newAddTagToTaskCommand() *cobra.Command {
	return newTagMutationCommand("add-tag-to-task", "Add a tag to an Asana task", "addTag", "add-tag-to-task", "Added tag %s to task %s.")
}

func newRemoveTagFromTaskCommand() *cobra.Command {
	return newTagMutationCommand("remove-tag-from-task", "Remove a tag from an Asana task", "removeTag", "remove-tag-from-task", "Removed tag %s from task %s.")
}

func newTagMutationCommand(use, short, endpoint, operation, humanFormat string) *cobra.Command {
	var taskGID, tagGID string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("%s does not accept positional arguments", use)
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			tag, err := requireFlag("tag-gid", tagGID)
			if err != nil {
				return err
			}
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/" + endpoint
			result := map[string]any{"operation": operation, "task_gid": gid, "tag_gid": tag}
			return executeRelationshipMutation(cmd, http.MethodPost, path, map[string]any{"tag": tag}, result, fmt.Sprintf(humanFormat, tag, gid))
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "task GID (required)")
	cmd.Flags().StringVar(&tagGID, "tag-gid", "", "tag GID (required)")
	return cmd
}

func newAddTaskFollowersCommand() *cobra.Command {
	return newFollowerMutationCommand("add-task-followers", "Add followers to an Asana task", "addFollowers", "add-task-followers", "Added followers %s to task %s.")
}

func newRemoveTaskFollowersCommand() *cobra.Command {
	return newFollowerMutationCommand("remove-task-followers", "Remove followers from an Asana task", "removeFollowers", "remove-task-followers", "Removed followers %s from task %s.")
}

func newFollowerMutationCommand(use, short, endpoint, operation, humanFormat string) *cobra.Command {
	var taskGID string
	var followers []string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("%s does not accept positional arguments", use)
			}
			gid, err := requireFlag("task-gid", taskGID)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("follower") || len(followers) == 0 {
				return usageErrorf("at least one --follower is required")
			}
			if err := validateGIDList(followers, "follower"); err != nil {
				return err
			}
			normalized := make([]string, len(followers))
			for i, follower := range followers {
				normalized[i] = strings.TrimSpace(follower)
			}
			path := "/tasks/" + asana.EncodePathSegment(gid) + "/" + endpoint
			result := map[string]any{"operation": operation, "task_gid": gid, "followers": normalized}
			return executeRelationshipMutation(cmd, http.MethodPost, path, map[string]any{"followers": normalized}, result, fmt.Sprintf(humanFormat, strings.Join(normalized, ", "), gid))
		},
	}
	cmd.Flags().StringVar(&taskGID, "task-gid", "", "task GID (required)")
	cmd.Flags().StringArrayVar(&followers, "follower", nil, "follower user GID or me (repeatable; order preserved)")
	return cmd
}
