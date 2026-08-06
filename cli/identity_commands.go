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

// resourceListCommand implements the common bounded pagination behavior used
// by the identity and membership discovery commands.
func resourceListCommand(cmd *cobra.Command, path string, pagination *paginationOptions, optFields, empty string, summarize func(json.RawMessage) string) error {
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
	if strings.Contains(path, "?") {
		path += "&" + q.Encode()
	} else {
		path += "?" + q.Encode()
	}
	result, err := c.Paginate(ctx, path, limit, paginationPageLimit(cmd, pagination))
	if err != nil {
		return err
	}
	return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human,
		humanList(result.Items, summarize, empty))
}

func newGetUserCommand() *cobra.Command {
	var userGID, optFields string
	cmd := &cobra.Command{
		Use:   "get-user",
		Short: "Get an Asana user",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("get-user does not accept positional arguments")
			}
			gid, err := requireFlag("user-gid", userGID)
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
			appendOptFields(q, optFields)
			data, err := requestData(ctx, c, http.MethodGet, "/users/"+asana.EncodePathSegment(gid)+querySuffix(q), nil)
			if err != nil {
				return err
			}
			return writeSuccess(cmd.OutOrStdout(), data, opts.human, summarizeUserWithGID(data))
		},
	}
	cmd.Flags().StringVar(&userGID, "user-gid", "", "Asana user GID or me (required)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newListWorkspaceUsersCommand() *cobra.Command {
	var workspaceGID, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{
		Use:   "list-workspace-users",
		Short: "List users in an Asana workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("list-workspace-users does not accept positional arguments")
			}
			_, cfg, err := buildClient()
			if err != nil {
				return err
			}
			workspace, err := cfg.ResolveWorkspace(workspaceGID)
			if err != nil {
				return &usageError{err: err}
			}
			return resourceListCommand(cmd, "/users?workspace="+url.QueryEscape(workspace), &pagination, optFields, "No users found.", summarizeIdentityUser)
		},
	}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newListTeamUsersCommand() *cobra.Command {
	var teamGID, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{
		Use:   "list-team-users",
		Short: "List users in an Asana team",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("list-team-users does not accept positional arguments")
			}
			gid, err := requireFlag("team-gid", teamGID)
			if err != nil {
				return err
			}
			return resourceListCommand(cmd, "/users?team="+url.QueryEscape(gid), &pagination, optFields, "No users found.", summarizeIdentityUser)
		},
	}
	cmd.Flags().StringVar(&teamGID, "team-gid", "", "Asana team GID (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newFindUserCommand() *cobra.Command {
	var workspaceGID, email, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{
		Use:   "find-user",
		Short: "Find a user by exact email in a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("find-user does not accept positional arguments")
			}
			email, err := requireFlag("email", email)
			if err != nil {
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
			limit, err := pagination.validate(cmd, 100)
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()
			q := url.Values{"limit": []string{strconv.Itoa(pageSize)}}
			if pagination.offset != "" {
				q.Set("offset", pagination.offset)
			}
			fields := strings.TrimSpace(optFields)
			if fields == "" {
				q.Set("opt_fields", "email")
			} else if !containsOptField(fields, "email") {
				q.Set("opt_fields", "email,"+fields)
			} else {
				q.Set("opt_fields", fields)
			}
			q.Set("workspace", workspace)
			result, err := c.Paginate(ctx, "/users?"+q.Encode(), limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			if result.Truncated {
				return usageErrorf("user search was truncated; rerun with --all to guarantee an exact result")
			}
			matches := make([]json.RawMessage, 0, 1)
			for _, raw := range result.Items {
				var user struct {
					Email string `json:"email"`
				}
				if json.Unmarshal(raw, &user) == nil && strings.EqualFold(strings.TrimSpace(user.Email), email) {
					matches = append(matches, raw)
				}
			}
			if len(matches) == 0 {
				return usageErrorf("no user found with exact email %q", email)
			}
			if len(matches) > 1 {
				return usageErrorf("multiple users found with exact email %q; refusing to choose one", email)
			}
			return writeSuccessWithPagination(cmd.OutOrStdout(), matches[0], pageMetadata(result), opts.human, summarizeUserWithGID(matches[0]))
		},
	}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	cmd.Flags().StringVar(&email, "email", "", "exact user email (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newGetTeamCommand() *cobra.Command {
	var teamGID, optFields string
	return newGetResourceCommand("get-team", "Get an Asana team", "team-gid", &teamGID, &optFields, "/teams/", "Asana team")
}

func newListWorkspaceTeamsCommand() *cobra.Command {
	var workspaceGID, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: "list-workspace-teams", Short: "List teams in an Asana workspace", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("list-workspace-teams does not accept positional arguments")
		}
		_, cfg, err := buildClient()
		if err != nil {
			return err
		}
		workspace, err := cfg.ResolveWorkspace(workspaceGID)
		if err != nil {
			return &usageError{err: err}
		}
		return resourceListCommand(cmd, "/workspaces/"+asana.EncodePathSegment(workspace)+"/teams", &pagination, optFields, "No teams found.", summarizeIdentityResource)
	}}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newListUserTeamsCommand() *cobra.Command {
	var userGID, organizationGID, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: "list-user-teams", Short: "List teams for an Asana user", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("list-user-teams does not accept positional arguments")
		}
		user, err := requireFlag("user-gid", userGID)
		if err != nil {
			return err
		}
		org, err := requireFlag("organization-gid", organizationGID)
		if err != nil {
			return err
		}
		return resourceListCommand(cmd, "/users/"+asana.EncodePathSegment(user)+"/teams?organization="+url.QueryEscape(org), &pagination, optFields, "No teams found.", summarizeIdentityResource)
	}}
	cmd.Flags().StringVar(&userGID, "user-gid", "", "Asana user GID or me (required)")
	cmd.Flags().StringVar(&organizationGID, "organization-gid", "", "Asana organization/workspace GID (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newListTeamMembershipsCommand() *cobra.Command {
	return newMembershipListCommand("list-team-memberships", "List memberships for an Asana team", "team-gid", "/teams/", "/team_memberships")
}

func newListWorkspaceMembershipsCommand() *cobra.Command {
	return newWorkspaceMembershipListCommand("list-workspace-memberships", "List memberships for an Asana workspace")
}

func newListUserWorkspaceMembershipsCommand() *cobra.Command {
	var userGID, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: "list-user-workspace-memberships", Short: "List workspace memberships for an Asana user", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("list-user-workspace-memberships does not accept positional arguments")
		}
		gid, err := requireFlag("user-gid", userGID)
		if err != nil {
			return err
		}
		return resourceListCommand(cmd, "/users/"+asana.EncodePathSegment(gid)+"/workspace_memberships", &pagination, optFields, "No workspace memberships found.", summarizeIdentityResource)
	}}
	cmd.Flags().StringVar(&userGID, "user-gid", "", "Asana user GID or me (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newGetTeamMembershipCommand() *cobra.Command {
	var gid, optFields string
	return newGetResourceCommand("get-team-membership", "Get an Asana team membership", "team-membership-gid", &gid, &optFields, "/team_memberships/", "Asana team membership")
}

func newGetWorkspaceMembershipCommand() *cobra.Command {
	var gid, optFields string
	return newGetResourceCommand("get-workspace-membership", "Get an Asana workspace membership", "workspace-membership-gid", &gid, &optFields, "/workspace_memberships/", "Asana workspace membership")
}

func newGetResourceCommand(use, short, flag string, value, optFields *string, prefix, label string) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("%s does not accept positional arguments", use)
		}
		gid, err := requireFlag(flag, *value)
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
		appendOptFields(q, *optFields)
		data, err := requestData(ctx, c, http.MethodGet, prefix+asana.EncodePathSegment(gid)+querySuffix(q), nil)
		if err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), data, opts.human, fmt.Sprintf("%s: %s", label, summarizeIdentityResource(data)))
	}}
	cmd.Flags().StringVar(value, flag, "", flag+" (required)")
	cmd.Flags().StringVar(optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newMembershipListCommand(use, short, flag, prefix, suffix string) *cobra.Command {
	var gid, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("%s does not accept positional arguments", use)
		}
		id, err := requireFlag(flag, gid)
		if err != nil {
			return err
		}
		return resourceListCommand(cmd, prefix+asana.EncodePathSegment(id)+suffix, &pagination, optFields, "No memberships found.", summarizeIdentityResource)
	}}
	cmd.Flags().StringVar(&gid, flag, "", flag+" (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newWorkspaceMembershipListCommand(use, short string) *cobra.Command {
	var workspaceGID, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{Use: use, Short: short, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("%s does not accept positional arguments", use)
		}
		_, cfg, err := buildClient()
		if err != nil {
			return err
		}
		workspace, err := cfg.ResolveWorkspace(workspaceGID)
		if err != nil {
			return &usageError{err: err}
		}
		return resourceListCommand(cmd, "/workspaces/"+asana.EncodePathSegment(workspace)+"/workspace_memberships", &pagination, optFields, "No workspace memberships found.", summarizeIdentityResource)
	}}
	cmd.Flags().StringVar(&workspaceGID, "workspace-gid", "", "Asana workspace GID (defaults to ASANA_DEFAULT_WORKSPACE)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func containsOptField(fields, wanted string) bool {
	for _, field := range strings.Split(fields, ",") {
		if strings.TrimSpace(field) == wanted {
			return true
		}
	}
	return false
}

func summarizeIdentityResource(raw json.RawMessage) string {
	var r struct {
		GID  string `json:"gid"`
		Name string `json:"name"`
		User struct {
			Name string `json:"name"`
		} `json:"user"`
		Team struct {
			Name string `json:"name"`
		} `json:"team"`
	}
	_ = json.Unmarshal(raw, &r)
	name := r.Name
	if name == "" {
		name = r.User.Name
	}
	if name == "" {
		name = r.Team.Name
	}
	if name == "" {
		name = "(unnamed)"
	}
	return fmt.Sprintf("%s %s", orUnknown(r.GID), name)
}

func summarizeIdentityUser(raw json.RawMessage) string { return summarizeIdentityResource(raw) }

func summarizeUserWithGID(raw json.RawMessage) string { return summarizeIdentityResource(raw) }
