package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/thaodangspace/asana-cli/asana"
)

func taskProjectGIDs(raw json.RawMessage) ([]string, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode task projects: %w", err)
	}
	projectsRaw, ok := envelope["projects"]
	if !ok {
		return nil, fmt.Errorf("decode task projects: response did not include projects")
	}
	var projects []struct {
		GID string `json:"gid"`
	}
	if err := json.Unmarshal(projectsRaw, &projects); err != nil {
		return nil, fmt.Errorf("decode task projects: %w", err)
	}
	result := make([]string, 0, len(projects))
	for _, project := range projects {
		if strings.TrimSpace(project.GID) == "" {
			return nil, fmt.Errorf("decode task projects: project is missing gid")
		}
		result = append(result, project.GID)
	}
	return result, nil
}

func relationshipMutation(ctx context.Context, c *asana.Client, path string, data map[string]any) error {
	_, err := c.Request(ctx, http.MethodPost, path, map[string]any{"data": data})
	return err
}

// replaceTaskProjects implements update-task's replacement semantics using
// Asana's dedicated add/remove project endpoints. The initial GET is required
// because the API does not accept projects in PUT /tasks/{gid}.
func replaceTaskProjects(ctx context.Context, c *asana.Client, taskGID string, desired []string) error {
	q := url.Values{}
	q.Set("opt_fields", "projects")
	path := "/tasks/" + asana.EncodePathSegment(taskGID) + "?" + q.Encode()
	raw, err := requestData(ctx, c, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	current, err := taskProjectGIDs(raw)
	if err != nil {
		return err
	}

	desiredSet := make(map[string]bool, len(desired))
	for _, project := range desired {
		desiredSet[project] = true
	}
	currentSet := make(map[string]bool, len(current))
	for _, project := range current {
		currentSet[project] = true
		if !desiredSet[project] {
			path := "/tasks/" + asana.EncodePathSegment(taskGID) + "/removeProject"
			if err := relationshipMutation(ctx, c, path, map[string]any{"project": project}); err != nil {
				return err
			}
		}
	}
	for _, project := range desired {
		if currentSet[project] {
			continue
		}
		path := "/tasks/" + asana.EncodePathSegment(taskGID) + "/addProject"
		if err := relationshipMutation(ctx, c, path, map[string]any{"project": project}); err != nil {
			return err
		}
	}
	return nil
}

func addTaskToSection(ctx context.Context, c *asana.Client, taskGID, sectionGID string) error {
	path := "/sections/" + asana.EncodePathSegment(sectionGID) + "/addTask"
	return relationshipMutation(ctx, c, path, map[string]any{"task": taskGID})
}

func setTaskParent(ctx context.Context, c *asana.Client, taskGID, parentGID string) error {
	var parent any
	if strings.TrimSpace(parentGID) != "" {
		parent = strings.TrimSpace(parentGID)
	}
	path := "/tasks/" + asana.EncodePathSegment(taskGID) + "/setParent"
	return relationshipMutation(ctx, c, path, map[string]any{"parent": parent})
}

func getTaskAfterRelationships(ctx context.Context, c *asana.Client, taskGID string) (json.RawMessage, error) {
	path := "/tasks/" + asana.EncodePathSegment(taskGID)
	return requestData(ctx, c, http.MethodGet, path, nil)
}
