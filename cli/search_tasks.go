package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

// Search query parameters whose API semantics permit more than one value.
// Keep this list explicit: repeated values for scalar parameters are usually a
// caller mistake and should not silently change the request.
var searchListQueryKeys = map[string]bool{
	"assignee.any":     true,
	"assignee.not":     true,
	"projects.any":     true,
	"projects.not":     true,
	"sections.any":     true,
	"sections.not":     true,
	"tags.any":         true,
	"tags.not":         true,
	"teams.any":        true,
	"followers.any":    true,
	"resource_subtype": true,
}

var searchSortValues = map[string]bool{
	"created_at":  true,
	"due_date":    true,
	"likes":       true,
	"modified_at": true,
	"name":        true,
}

func newSearchTasksCommand() *cobra.Command {
	var (
		workspaceGID    string
		text            string
		assignee        string // Deprecated, retained as an alias for assignee.any.
		assigneeAny     []string
		assigneeNot     []string
		projectAny      []string
		projectNot      []string
		sectionAny      []string
		sectionNot      []string
		tagAny          []string
		tagNot          []string
		teamAny         []string
		followerAny     []string
		resourceType    []string
		completed       bool
		dueOn           string
		dueBefore       string
		dueAfter        string
		startBefore     string
		startAfter      string
		createdBefore   string
		createdAfter    string
		modifiedBefore  string
		modifiedAfter   string
		completedBefore string
		completedAfter  string
		sortBy          string
		sortAscending   bool
		queries         []string
		pagination      paginationOptions
		optFields       string
	)
	cmd := &cobra.Command{
		Use:   "search-tasks",
		Short: "Search tasks in an Asana workspace (may require premium access)",
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, err := pagination.validate(cmd, 100)
			if err != nil {
				return err
			}
			if err := validateSearchDates(dueOn, dueBefore, dueAfter, startBefore, startAfter,
				createdBefore, createdAfter, modifiedBefore, modifiedAfter, completedBefore, completedAfter); err != nil {
				return err
			}
			sortValue := strings.TrimSpace(sortBy)
			if sortValue != "" && !searchSortValues[sortValue] {
				return usageErrorf("--sort-by must be one of created_at, due_date, likes, modified_at, or name, got %q", sortValue)
			}

			q := url.Values{}
			q.Set("limit", strconv.Itoa(pageSize))
			if pagination.offset != "" {
				q.Set("offset", pagination.offset)
			}
			if v := strings.TrimSpace(text); v != "" {
				q.Set("text", v)
			}
			appendSearchValues(q, "assignee.any", append([]string{assignee}, assigneeAny...), true)
			appendSearchValues(q, "assignee.not", assigneeNot, true)
			appendSearchValues(q, "projects.any", projectAny, true)
			appendSearchValues(q, "projects.not", projectNot, true)
			appendSearchValues(q, "sections.any", sectionAny, true)
			appendSearchValues(q, "sections.not", sectionNot, true)
			appendSearchValues(q, "tags.any", tagAny, true)
			appendSearchValues(q, "tags.not", tagNot, true)
			appendSearchValues(q, "teams.any", teamAny, true)
			appendSearchValues(q, "followers.any", followerAny, true)
			appendSearchValues(q, "resource_subtype", resourceType, true)
			if cmd.Flags().Changed("completed") {
				q.Set("completed", strconv.FormatBool(completed))
			}
			appendSearchValue(q, "due_on", dueOn)
			appendSearchValue(q, "due_on.before", dueBefore)
			appendSearchValue(q, "due_on.after", dueAfter)
			appendSearchValue(q, "start_on.before", startBefore)
			appendSearchValue(q, "start_on.after", startAfter)
			appendSearchValue(q, "created_at.before", createdBefore)
			appendSearchValue(q, "created_at.after", createdAfter)
			appendSearchValue(q, "modified_at.before", modifiedBefore)
			appendSearchValue(q, "modified_at.after", modifiedAfter)
			appendSearchValue(q, "completed_at.before", completedBefore)
			appendSearchValue(q, "completed_at.after", completedAfter)
			appendSearchValue(q, "sort_by", sortValue)
			if cmd.Flags().Changed("sort-ascending") {
				q.Set("sort_ascending", strconv.FormatBool(sortAscending))
			}
			appendOptFields(q, optFields)
			if err := mergeSearchQueries(q, queries); err != nil {
				return err
			}

			c, cfg, err := buildClient()
			if err != nil {
				return err
			}
			workspace, err := cfg.ResolveWorkspace(workspaceGID)
			if err != nil {
				return &usageError{err: err}
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()

			path := "/workspaces/" + asana.EncodePathSegment(workspace) + "/tasks/search?" + q.Encode()
			result, err := c.Paginate(ctx, path, limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			human := searchHumanText(result.Items, q)
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, human)
		},
	}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	cmd.Flags().StringVar(&text, "text", "", "text search query")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee GID, email, or me (alias for --assignee-any)")
	cmd.Flags().StringArrayVar(&assigneeAny, "assignee-any", nil, "assignee GID, email, or me (repeatable)")
	cmd.Flags().StringArrayVar(&assigneeNot, "assignee-not", nil, "exclude an assignee GID (repeatable)")
	cmd.Flags().StringArrayVar(&projectAny, "project-any", nil, "project GID to include (repeatable)")
	cmd.Flags().StringArrayVar(&projectNot, "project-not", nil, "project GID to exclude (repeatable)")
	cmd.Flags().StringArrayVar(&sectionAny, "section-any", nil, "section GID to include (repeatable)")
	cmd.Flags().StringArrayVar(&sectionNot, "section-not", nil, "section GID to exclude (repeatable)")
	cmd.Flags().StringArrayVar(&tagAny, "tag-any", nil, "tag GID to include (repeatable)")
	cmd.Flags().StringArrayVar(&tagNot, "tag-not", nil, "tag GID to exclude (repeatable)")
	cmd.Flags().StringArrayVar(&teamAny, "team-any", nil, "team GID to include (repeatable)")
	cmd.Flags().StringArrayVar(&followerAny, "follower-any", nil, "follower GID to include (repeatable)")
	cmd.Flags().StringArrayVar(&resourceType, "resource-subtype", nil, "task resource subtype (repeatable)")
	cmd.Flags().BoolVar(&completed, "completed", false, "filter by completion state (omitted unless set)")
	cmd.Flags().StringVar(&dueOn, "due-on", "", "due date YYYY-MM-DD")
	cmd.Flags().StringVar(&dueBefore, "due-before", "", "due date upper bound YYYY-MM-DD")
	cmd.Flags().StringVar(&dueAfter, "due-after", "", "due date lower bound YYYY-MM-DD")
	cmd.Flags().StringVar(&startBefore, "start-before", "", "start date upper bound YYYY-MM-DD")
	cmd.Flags().StringVar(&startAfter, "start-after", "", "start date lower bound YYYY-MM-DD")
	cmd.Flags().StringVar(&createdBefore, "created-before", "", "creation time upper bound RFC 3339")
	cmd.Flags().StringVar(&createdAfter, "created-after", "", "creation time lower bound RFC 3339")
	cmd.Flags().StringVar(&createdBefore, "created-at-before", "", "alias for --created-before")
	cmd.Flags().StringVar(&createdAfter, "created-at-after", "", "alias for --created-after")
	cmd.Flags().StringVar(&modifiedBefore, "modified-before", "", "modification time upper bound RFC 3339")
	cmd.Flags().StringVar(&modifiedAfter, "modified-after", "", "modification time lower bound RFC 3339")
	cmd.Flags().StringVar(&modifiedBefore, "modified-at-before", "", "alias for --modified-before")
	cmd.Flags().StringVar(&modifiedAfter, "modified-at-after", "", "alias for --modified-after")
	cmd.Flags().StringVar(&completedBefore, "completed-before", "", "completion time upper bound RFC 3339")
	cmd.Flags().StringVar(&completedAfter, "completed-after", "", "completion time lower bound RFC 3339")
	cmd.Flags().StringVar(&completedBefore, "completed-at-before", "", "alias for --completed-before")
	cmd.Flags().StringVar(&completedAfter, "completed-at-after", "", "alias for --completed-after")
	cmd.Flags().StringVar(&sortBy, "sort-by", "", "sort by created_at, due_date, likes, modified_at, or name")
	cmd.Flags().BoolVar(&sortAscending, "sort-ascending", false, "sort in ascending order")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "advanced search query key=value (repeatable)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func appendSearchValue(q url.Values, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		q.Set(key, value)
	}
}

func appendSearchValues(q url.Values, key string, values []string, list bool) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if list {
			q.Add(key, value)
		} else {
			q.Set(key, value)
		}
	}
}

// mergeSearchQueries adds --query values without allowing callers to replace
// pagination. Scalar duplicates with the same value are harmless; conflicting
// duplicates are rejected so a typo cannot silently alter a search.
func mergeSearchQueries(q url.Values, queries []string) error {
	for _, item := range queries {
		separator := strings.IndexByte(item, '=')
		if separator <= 0 {
			return usageErrorf("--query must use key=value form, got %q", item)
		}
		key := strings.TrimSpace(item[:separator])
		if key == "" {
			return usageErrorf("--query key must not be empty")
		}
		value := item[separator+1:]
		if key == "limit" || key == "offset" {
			return usageErrorf("--query cannot override pagination parameter %q; use the corresponding flag", key)
		}
		if searchListQueryKeys[key] {
			q.Add(key, value)
			continue
		}
		if existing, ok := q[key]; ok {
			for _, previous := range existing {
				if previous != value {
					return usageErrorf("conflicting values for scalar query parameter %q", key)
				}
			}
			continue
		}
		q.Set(key, value)
	}
	return nil
}

func validateSearchDates(dueOn, dueBefore, dueAfter, startBefore, startAfter,
	createdBefore, createdAfter, modifiedBefore, modifiedAfter, completedBefore, completedAfter string) error {
	dateValues := []struct {
		name  string
		value string
	}{
		{"due-on", dueOn}, {"due-before", dueBefore}, {"due-after", dueAfter},
		{"start-before", startBefore}, {"start-after", startAfter},
	}
	for _, item := range dateValues {
		if strings.TrimSpace(item.value) == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(item.value)); err != nil {
			return usageErrorf("--%s must be YYYY-MM-DD, got %q", item.name, item.value)
		}
	}
	if err := validateSearchDateRange("due", dueAfter, dueBefore, false); err != nil {
		return err
	}
	if err := validateSearchDateRange("start", startAfter, startBefore, false); err != nil {
		return err
	}
	for _, item := range []struct{ name, value string }{
		{"created-before", createdBefore}, {"created-after", createdAfter},
		{"modified-before", modifiedBefore}, {"modified-after", modifiedAfter},
		{"completed-before", completedBefore}, {"completed-after", completedAfter},
	} {
		if strings.TrimSpace(item.value) == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, strings.TrimSpace(item.value)); err != nil {
			return usageErrorf("--%s must be RFC 3339, got %q", item.name, item.value)
		}
	}
	if err := validateSearchDateRange("created", createdAfter, createdBefore, true); err != nil {
		return err
	}
	if err := validateSearchDateRange("modified", modifiedAfter, modifiedBefore, true); err != nil {
		return err
	}
	return validateSearchDateRange("completed", completedAfter, completedBefore, true)
}

func validateSearchDateRange(name, after, before string, dateTime bool) error {
	after, before = strings.TrimSpace(after), strings.TrimSpace(before)
	if after == "" || before == "" {
		return nil
	}
	layout := "2006-01-02"
	if dateTime {
		layout = time.RFC3339
	}
	a, _ := time.Parse(layout, after)
	b, _ := time.Parse(layout, before)
	if a.After(b) {
		return usageErrorf("--%s-after must not be later than --%s-before", name, name)
	}
	return nil
}

func searchHumanText(items []json.RawMessage, q url.Values) string {
	filters := make([]string, 0, len(q))
	keys := make([]string, 0, len(q))
	for key := range q {
		if key != "limit" && key != "offset" && key != "opt_fields" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		filters = append(filters, fmt.Sprintf("%s=%s", key, strings.Join(q[key], ",")))
	}
	prefix := "Filters: none"
	if len(filters) > 0 {
		prefix = "Filters: " + strings.Join(filters, ", ")
	}
	return prefix + "\n" + humanList(items, summarizeTask, "No tasks found.")
}
