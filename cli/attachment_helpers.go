package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const defaultAttachmentParentType = "task"

var attachmentParentTypes = map[string]struct{}{
	"task":          {},
	"project":       {},
	"project-brief": {},
}

func addAttachmentParentTypeFlag(cmd *cobra.Command, parentType *string) {
	cmd.Flags().StringVar(parentType, "parent-type", defaultAttachmentParentType,
		"parent resource type: task, project, or project-brief")
}

func validateAttachmentParentType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if _, ok := attachmentParentTypes[value]; !ok {
		return "", usageErrorf("--parent-type must be task, project, or project-brief, got %q", value)
	}
	return value, nil
}

func resolveAttachmentParent(parentGID, taskGID string) (string, error) {
	parentGID = strings.TrimSpace(parentGID)
	taskGID = strings.TrimSpace(taskGID)
	if parentGID != "" && taskGID != "" {
		return "", usageErrorf("--parent-gid and deprecated --task-gid cannot be combined")
	}
	if parentGID != "" {
		return parentGID, nil
	}
	if taskGID != "" {
		return taskGID, nil
	}
	return "", usageErrorf("--parent-gid is required")
}

func attachmentParentLabel(parentType, parentGID string) string {
	return fmt.Sprintf("%s %s", parentType, parentGID)
}
