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

type projectFlags struct {
	name, workspace, team, notes, htmlNotes, color, defaultView string
	dueOn, dueAt, startOn, owner                                string
	archived, public                                            bool
	members, followers                                          []string
}

func (f *projectFlags) bind(cmd *cobra.Command, create bool) {
	cmd.Flags().StringVar(&f.name, "name", "", "project name")
	if create {
		cmd.Flags().StringVar(&f.workspace, "workspace-gid", "", "Asana workspace GID")
		cmd.Flags().StringVar(&f.team, "team-gid", "", "Asana team GID")
	}
	cmd.Flags().StringVar(&f.notes, "notes", "", "plain-text project description; empty clears it")
	cmd.Flags().StringVar(&f.htmlNotes, "html-notes", "", "HTML project description; empty clears it")
	cmd.Flags().StringVar(&f.color, "color", "", "project color; empty clears it")
	cmd.Flags().BoolVar(&f.archived, "archived", false, "archive state (sent only when set)")
	cmd.Flags().BoolVar(&f.public, "public", false, "visibility (sent only when set)")
	cmd.Flags().StringVar(&f.defaultView, "default-view", "", "default project view; empty clears it")
	cmd.Flags().StringVar(&f.dueOn, "due-on", "", "due date YYYY-MM-DD; empty clears it")
	cmd.Flags().StringVar(&f.dueAt, "due-at", "", "due date-time RFC 3339; empty clears it")
	cmd.Flags().StringVar(&f.startOn, "start-on", "", "start date YYYY-MM-DD; empty clears it")
	cmd.Flags().StringVar(&f.owner, "owner", "", "owner user GID; empty clears it")
	cmd.Flags().StringArrayVar(&f.members, "member", nil, "project member user GID (repeatable; empty clears members)")
	cmd.Flags().StringArrayVar(&f.members, "members", nil, "alias for --member")
	cmd.Flags().StringArrayVar(&f.followers, "follower", nil, "project follower user GID (repeatable; empty clears followers)")
	cmd.Flags().StringArrayVar(&f.followers, "followers", nil, "alias for --follower")
}

func addBoundProjectFields(data map[string]any, cmd *cobra.Command, f *projectFlags) error {
	return addProjectFields(data, cmd, f.name, f.notes, f.htmlNotes, f.color, f.owner, f.archived, f.public, f.defaultView, f.dueOn, f.dueAt, f.startOn, f.members, f.followers)
}

func newGetProjectCommand() *cobra.Command {
	var gid, optFields string
	cmd := &cobra.Command{Use: "get-project", Short: "Get a single Asana project by GID", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("get-project does not accept positional arguments")
		}
		id, err := requireFlag("project-gid", gid)
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
		data, err := requestData(ctx, c, http.MethodGet, "/projects/"+asana.EncodePathSegment(id)+querySuffix(q), nil)
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), data, opts.human, summarizeProject(data))
	}}
	cmd.Flags().StringVar(&gid, "project-gid", "", "Asana project GID (required)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newCreateProjectCommand() *cobra.Command {
	var f projectFlags
	cmd := &cobra.Command{Use: "create-project", Short: "Create an Asana project", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("create-project does not accept positional arguments")
		}
		name, err := requireNonEmptyName(f.name)
		if err != nil {
			return err
		}
		data := map[string]any{"name": name}
		if err := addBoundProjectFields(data, cmd, &f); err != nil {
			return err
		}
		// A project can be located by workspace or team. Resolve the configured
		// workspace only when neither explicit location flag supplies one.
		c, cfg, err := buildClient()
		if err != nil {
			return err
		}
		workspace, team := strings.TrimSpace(f.workspace), strings.TrimSpace(f.team)
		if workspace == "" && team == "" {
			workspace, err = cfg.ResolveWorkspace("")
			if err != nil {
				return &usageError{err: err}
			}
		}
		if workspace != "" {
			data["workspace"] = workspace
		}
		if team != "" {
			data["team"] = team
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		raw, err := requestData(ctx, c, http.MethodPost, "/projects", map[string]any{"data": data})
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), raw, opts.human, "Created project: "+summarizeProject(raw))
	}}
	f.bind(cmd, true)
	return cmd
}

func newUpdateProjectCommand() *cobra.Command {
	var gid string
	var f projectFlags
	cmd := &cobra.Command{Use: "update-project", Short: "Update fields on an Asana project", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("update-project does not accept positional arguments")
		}
		id, err := requireFlag("project-gid", gid)
		if err != nil {
			return err
		}
		data := map[string]any{}
		if err := addBoundProjectFields(data, cmd, &f); err != nil {
			return err
		}
		if len(data) == 0 {
			return usageErrorf("at least one project field flag must be set")
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		raw, err := requestData(ctx, c, http.MethodPut, "/projects/"+asana.EncodePathSegment(id), map[string]any{"data": data})
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), raw, opts.human, "Updated project: "+summarizeProject(raw))
	}}
	cmd.Flags().StringVar(&gid, "project-gid", "", "Asana project GID (required)")
	f.bind(cmd, false)
	return cmd
}

func newDeleteProjectCommand() *cobra.Command {
	var gid string
	var confirm, yes bool
	cmd := &cobra.Command{Use: "delete-project", Short: "Delete an Asana project", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("delete-project does not accept positional arguments")
		}
		id, err := requireFlag("project-gid", gid)
		if err != nil {
			return err
		}
		if !confirm && !yes {
			return usageErrorf("deleting a project requires --confirm or --yes")
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		if _, err := c.Request(ctx, http.MethodDelete, "/projects/"+asana.EncodePathSegment(id), nil); err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), nil, opts.human, fmt.Sprintf("Deleted project %s.", id))
	}}
	cmd.Flags().StringVar(&gid, "project-gid", "", "Asana project GID (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm this destructive operation")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm this destructive operation for automation")
	return cmd
}

func newDuplicateProjectCommand() *cobra.Command {
	var gid, name string
	var include, options []string
	cmd := &cobra.Command{Use: "duplicate-project", Short: "Duplicate an Asana project", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("duplicate-project does not accept positional arguments")
		}
		id, err := requireFlag("project-gid", gid)
		if err != nil {
			return err
		}
		data := map[string]any{}
		if cmd.Flags().Changed("name") {
			value, e := requireNonEmptyName(name)
			if e != nil {
				return e
			}
			data["name"] = value
		}
		if len(include) > 0 {
			items := []string{}
			for _, value := range include {
				for _, item := range strings.Split(value, ",") {
					item = strings.TrimSpace(item)
					if item == "" {
						return usageErrorf("--include option cannot be empty")
					}
					items = append(items, item)
				}
			}
			data["include"] = items
		}
		parsed, err := parseProjectOptions(options)
		if err != nil {
			return err
		}
		for key, value := range parsed {
			if _, exists := data[key]; exists {
				return usageErrorf("duplicate project duplication option %q", key)
			}
			data[key] = value
		}
		c, _, err := buildClient()
		if err != nil {
			return err
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		raw, err := requestData(ctx, c, http.MethodPost, "/projects/"+asana.EncodePathSegment(id)+"/duplicate", map[string]any{"data": data})
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), raw, opts.human, "Started project duplication job: "+summarizeProject(raw))
	}}
	cmd.Flags().StringVar(&gid, "project-gid", "", "source project GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "name for duplicated project")
	cmd.Flags().StringArrayVar(&include, "include", nil, "duplication include option (repeatable or comma-separated)")
	cmd.Flags().StringArrayVar(&options, "option", nil, "documented duplication option key=value (repeatable; use json: for typed JSON)")
	return cmd
}

func newSearchProjectsCommand() *cobra.Command {
	var workspace, optFields string
	var owner, team, member string
	var completed bool
	var dueBefore, dueAfter, startBefore, startAfter string
	var createdBefore, createdAfter string
	var sortBy string
	var sortAscending bool
	var queries []string
	var limit int
	cmd := &cobra.Command{Use: "search-projects", Short: "Search projects in an Asana workspace", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("search-projects does not accept positional arguments")
		}
		if limit < 1 || limit > 100 {
			return usageErrorf("--limit must be between 1 and 100, got %d", limit)
		}
		if err := validateProjectSearchDates(dueBefore, dueAfter, startBefore, startAfter, createdBefore, createdAfter); err != nil {
			return err
		}
		q := url.Values{}
		q.Set("limit", strconv.Itoa(limit))
		appendOptFields(q, optFields)
		appendProjectSearchValue(q, "owner.any", owner)
		appendProjectSearchValue(q, "teams.any", team)
		appendProjectSearchValue(q, "members.any", member)
		if cmd.Flags().Changed("completed") {
			q.Set("completed", strconv.FormatBool(completed))
		}
		appendProjectSearchValue(q, "due_on.before", dueBefore)
		appendProjectSearchValue(q, "due_on.after", dueAfter)
		appendProjectSearchValue(q, "start_on.before", startBefore)
		appendProjectSearchValue(q, "start_on.after", startAfter)
		appendProjectSearchValue(q, "created_at.before", createdBefore)
		appendProjectSearchValue(q, "created_at.after", createdAfter)
		if strings.TrimSpace(sortBy) != "" {
			q.Set("sort_by", strings.TrimSpace(sortBy))
		}
		if cmd.Flags().Changed("sort-ascending") {
			q.Set("sort_ascending", strconv.FormatBool(sortAscending))
		}
		if err := mergeProjectQueries(q, queries); err != nil {
			return err
		}
		c, cfg, err := buildClient()
		if err != nil {
			return err
		}
		ws, err := cfg.ResolveWorkspace(workspace)
		if err != nil {
			return &usageError{err: err}
		}
		ctx, cancel := withTimeout(cmd)
		defer cancel()
		path := "/workspaces/" + asana.EncodePathSegment(ws) + "/projects/search?" + q.Encode()
		data, err := requestData(ctx, c, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		items := make([]json.RawMessage, 0)
		if err := json.Unmarshal(data, &items); err != nil {
			return fmt.Errorf("decode project search response: %w", err)
		}
		result := asana.PageResult{Items: items, PagesFetched: 1}
		return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, humanList(result.Items, summarizeProject, "No projects found."))
	}}
	cmd.Flags().StringVar(&workspace, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	cmd.Flags().StringVar(&owner, "owner", "", "owner GID (sent as owner.any)")
	cmd.Flags().StringVar(&team, "team", "", "team GID (sent as teams.any)")
	cmd.Flags().StringVar(&member, "member", "", "member GID (sent as members.any)")
	cmd.Flags().BoolVar(&completed, "completed", false, "filter by completion state (omitted unless set)")
	cmd.Flags().StringVar(&dueBefore, "due-before", "", "due date upper bound YYYY-MM-DD")
	cmd.Flags().StringVar(&dueAfter, "due-after", "", "due date lower bound YYYY-MM-DD")
	cmd.Flags().StringVar(&startBefore, "start-before", "", "start date upper bound YYYY-MM-DD")
	cmd.Flags().StringVar(&startAfter, "start-after", "", "start date lower bound YYYY-MM-DD")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "creation time upper bound RFC 3339")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "creation time lower bound RFC 3339")
	cmd.Flags().StringVar(&sortBy, "sort-by", "", "sort by due_date, created_at, or modified_at")
	cmd.Flags().BoolVar(&sortAscending, "sort-ascending", false, "sort in ascending order")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "advanced project query key=value (repeatable)")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum results to return (1-100)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newListTeamProjectsCommand() *cobra.Command {
	var team, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: "list-team-projects", Short: "List projects for an Asana team", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("list-team-projects does not accept positional arguments")
		}
		id, err := requireFlag("team-gid", team)
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
		result, err := c.Paginate(ctx, "/teams/"+asana.EncodePathSegment(id)+"/projects?"+q.Encode(), limit, paginationPageLimit(cmd, &pagination))
		if err != nil {
			return err
		}
		return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, humanList(result.Items, summarizeProject, "No projects found."))
	}}
	cmd.Flags().StringVar(&team, "team-gid", "", "Asana team GID (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}
