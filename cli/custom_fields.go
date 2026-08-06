package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thaodangspace/asana-cli/asana"
)

// customFieldDefinitionFlags contains the fields accepted by Asana's custom
// field definition endpoints. Fields are added to a request only when their
// flag was explicitly provided, which makes update-custom-field safe for
// partial updates and preserves zero values such as precision=0.
type customFieldDefinitionFlags struct {
	name                  string
	description           string
	resourceSubtype       string
	fieldType             string
	precision             int
	currencyCode          string
	format                string
	representationOptions string
	enumOptions           string
	peopleValue           string
	customIDPrefix        string
	customLabel           string
	customLabelPosition   string
	inputRestrictions     string
	isGlobalToWorkspace   bool
	ownedByApp            bool
}

func (f *customFieldDefinitionFlags) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.name, "name", "", "custom field name")
	cmd.Flags().StringVar(&f.description, "description", "", "custom field description")
	cmd.Flags().StringVar(&f.resourceSubtype, "resource-subtype", "", "custom field subtype (text, enum, multi_enum, number, date, people, or formula)")
	cmd.Flags().StringVar(&f.fieldType, "type", "", "custom field type, when supported by Asana")
	cmd.Flags().IntVar(&f.precision, "precision", 0, "number precision")
	cmd.Flags().StringVar(&f.currencyCode, "currency-code", "", "ISO currency code for currency-formatted number fields")
	cmd.Flags().StringVar(&f.format, "format", "", "custom field representation format")
	cmd.Flags().StringVar(&f.representationOptions, "representation-options", "", "representation options as a JSON object or array")
	cmd.Flags().StringVar(&f.enumOptions, "enum-options", "", "enum options as a JSON array (where supported)")
	cmd.Flags().StringVar(&f.peopleValue, "people-value", "", "people configuration as a JSON value (where supported)")
	cmd.Flags().StringVar(&f.customIDPrefix, "custom-id-prefix", "", "custom ID prefix")
	cmd.Flags().StringVar(&f.customLabel, "custom-label", "", "custom label for number fields")
	cmd.Flags().StringVar(&f.customLabelPosition, "custom-label-position", "", "custom label position")
	cmd.Flags().StringVar(&f.inputRestrictions, "input-restrictions", "", "reference field input restrictions as a JSON array")
	cmd.Flags().BoolVar(&f.isGlobalToWorkspace, "is-global-to-workspace", false, "make the custom field global to its workspace")
	cmd.Flags().BoolVar(&f.ownedByApp, "owned-by-app", false, "mark the field as app-owned (allow-listed Asana feature)")
}

func (f *customFieldDefinitionFlags) data(cmd *cobra.Command, requireName bool) (map[string]any, error) {
	data := make(map[string]any)
	if cmd.Flags().Changed("name") {
		name, err := requireNonEmptyName(f.name)
		if err != nil {
			return nil, err
		}
		data["name"] = name
	} else if requireName {
		return nil, usageErrorf("--name is required")
	}
	if requireName && !cmd.Flags().Changed("resource-subtype") {
		return nil, usageErrorf("--resource-subtype is required")
	}

	for _, item := range []struct {
		flag  string
		key   string
		value string
	}{
		{"description", "description", f.description},
		{"resource-subtype", "resource_subtype", f.resourceSubtype},
		{"type", "type", f.fieldType},
		{"currency-code", "currency_code", f.currencyCode},
		{"format", "format", f.format},
		{"custom-id-prefix", "id_prefix", f.customIDPrefix},
		{"custom-label", "custom_label", f.customLabel},
		{"custom-label-position", "custom_label_position", f.customLabelPosition},
	} {
		if cmd.Flags().Changed(item.flag) {
			value := strings.TrimSpace(item.value)
			if value == "" && item.flag != "resource-subtype" && item.flag != "type" {
				data[item.key] = nil
			} else {
				data[item.key] = value
			}
		}
	}
	if cmd.Flags().Changed("precision") {
		if f.precision < 0 {
			return nil, usageErrorf("--precision must be zero or greater, got %d", f.precision)
		}
		data["precision"] = f.precision
	}
	if cmd.Flags().Changed("is-global-to-workspace") {
		data["is_global_to_workspace"] = f.isGlobalToWorkspace
	}
	if cmd.Flags().Changed("owned-by-app") {
		data["owned_by_app"] = f.ownedByApp
	}
	for _, item := range []struct {
		flag  string
		key   string
		value string
	}{
		{"representation-options", "representation_options", f.representationOptions},
		{"enum-options", "enum_options", f.enumOptions},
		{"people-value", "people_value", f.peopleValue},
		{"input-restrictions", "input_restrictions", f.inputRestrictions},
	} {
		if !cmd.Flags().Changed(item.flag) {
			continue
		}
		value, err := parseJSONFlag(item.flag, item.value)
		if err != nil {
			return nil, err
		}
		data[item.key] = value
	}
	return data, nil
}

func parseJSONFlag(flag, raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, usageErrorf("--%s must be valid JSON: %v", flag, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, usageErrorf("--%s must contain exactly one JSON value", flag)
		}
		return nil, usageErrorf("--%s must be valid JSON: %v", flag, err)
	}
	return value, nil
}

func customFieldPath(gid string) string {
	return "/custom_fields/" + asana.EncodePathSegment(gid)
}

func enumOptionPath(gid string) string {
	return "/enum_options/" + asana.EncodePathSegment(gid)
}

func newGetCustomFieldCommand() *cobra.Command {
	var gid, optFields string
	cmd := &cobra.Command{
		Use:   "get-custom-field",
		Short: "Get an Asana custom field definition",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("get-custom-field does not accept positional arguments")
			}
			gid, err := requireFlag("custom-field-gid", gid)
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
			data, err := requestData(ctx, c, http.MethodGet, customFieldPath(gid)+querySuffix(q), nil)
			if err != nil {
				return err
			}
			return writeSuccess(cmd.OutOrStdout(), data, opts.human, "Custom field: "+summarizeCustomField(data))
		},
	}
	cmd.Flags().StringVar(&gid, "custom-field-gid", "", "custom field GID (required)")
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newListWorkspaceCustomFieldsCommand() *cobra.Command {
	var workspace, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{
		Use:   "list-workspace-custom-fields",
		Short: "List custom fields in an Asana workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("list-workspace-custom-fields does not accept positional arguments")
			}
			ws, err := requireFlag("workspace-gid", workspace)
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
			result, err := c.Paginate(ctx, "/workspaces/"+asana.EncodePathSegment(ws)+"/custom_fields"+querySuffix(q), limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, humanList(result.Items, summarizeCustomField, "No custom fields found."))
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace-gid", "", "workspace GID (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func customFieldMutation(cmd *cobra.Command, method, path string, data map[string]any, human string) error {
	c, _, err := buildClient()
	if err != nil {
		return err
	}
	ctx, cancel := withTimeout(cmd)
	defer cancel()
	if method == http.MethodDelete {
		if _, err := c.Request(ctx, method, path, nil); err != nil {
			return err
		}
		return writeSuccess(cmd.OutOrStdout(), nil, opts.human, human)
	}
	raw, err := requestData(ctx, c, method, path, map[string]any{"data": data})
	if err != nil {
		return err
	}
	return writeSuccess(cmd.OutOrStdout(), raw, opts.human, human)
}

func customFieldDefinitionMutation(cmd *cobra.Command, method, path string, data map[string]any, action string) error {
	c, _, err := buildClient()
	if err != nil {
		return err
	}
	ctx, cancel := withTimeout(cmd)
	defer cancel()
	raw, err := requestData(ctx, c, method, path, map[string]any{"data": data})
	if err != nil {
		return err
	}
	return writeSuccess(cmd.OutOrStdout(), raw, opts.human, action+": "+summarizeCustomField(raw))
}

func newCreateCustomFieldCommand() *cobra.Command {
	var workspace string
	var fields customFieldDefinitionFlags
	cmd := &cobra.Command{
		Use:   "create-custom-field",
		Short: "Create a custom field in an Asana workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("create-custom-field does not accept positional arguments")
			}
			ws, err := requireFlag("workspace-gid", workspace)
			if err != nil {
				return err
			}
			data, err := fields.data(cmd, true)
			if err != nil {
				return err
			}
			return customFieldDefinitionMutation(cmd, http.MethodPost, "/workspaces/"+asana.EncodePathSegment(ws)+"/custom_fields", data, "Created custom field")
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace-gid", "", "workspace GID (required)")
	fields.addFlags(cmd)
	return cmd
}

func newUpdateCustomFieldCommand() *cobra.Command {
	var gid string
	var fields customFieldDefinitionFlags
	cmd := &cobra.Command{
		Use:   "update-custom-field",
		Short: "Update an Asana custom field definition",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("update-custom-field does not accept positional arguments")
			}
			gid, err := requireFlag("custom-field-gid", gid)
			if err != nil {
				return err
			}
			data, err := fields.data(cmd, false)
			if err != nil {
				return err
			}
			if len(data) == 0 {
				return usageErrorf("at least one custom field definition flag must be set")
			}
			return customFieldDefinitionMutation(cmd, http.MethodPut, customFieldPath(gid), data, "Updated custom field")
		},
	}
	cmd.Flags().StringVar(&gid, "custom-field-gid", "", "custom field GID (required)")
	fields.addFlags(cmd)
	return cmd
}

func newDeleteCustomFieldCommand() *cobra.Command {
	var gid string
	var confirm, yes bool
	cmd := &cobra.Command{
		Use:   "delete-custom-field",
		Short: "Delete an Asana custom field",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("delete-custom-field does not accept positional arguments")
			}
			gid, err := requireFlag("custom-field-gid", gid)
			if err != nil {
				return err
			}
			if !confirm && !yes {
				return usageErrorf("deleting a custom field requires --confirm or --yes")
			}
			return customFieldMutation(cmd, http.MethodDelete, customFieldPath(gid), nil, "Deleted custom field "+gid+".")
		},
	}
	cmd.Flags().StringVar(&gid, "custom-field-gid", "", "custom field GID (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm this destructive operation")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm this destructive operation for automation")
	return cmd
}

func newCreateEnumOptionCommand() *cobra.Command {
	var fieldGID, name, color string
	cmd := &cobra.Command{
		Use:   "create-enum-option",
		Short: "Create an enum option for an Asana custom field",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("create-enum-option does not accept positional arguments")
			}
			field, err := requireFlag("custom-field-gid", fieldGID)
			if err != nil {
				return err
			}
			option, err := requireNonEmptyName(name)
			if err != nil {
				return err
			}
			data := map[string]any{"name": option}
			if cmd.Flags().Changed("color") {
				data["color"] = strings.TrimSpace(color)
			}
			return customFieldMutation(cmd, http.MethodPost, customFieldPath(field)+"/enum_options", data, "Created enum option.")
		},
	}
	cmd.Flags().StringVar(&fieldGID, "custom-field-gid", "", "custom field GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "enum option name (required)")
	cmd.Flags().StringVar(&color, "color", "", "enum option color")
	return cmd
}

func newUpdateEnumOptionCommand() *cobra.Command {
	var gid, name, color string
	var enabled bool
	cmd := &cobra.Command{
		Use:   "update-enum-option",
		Short: "Update an Asana enum option",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("update-enum-option does not accept positional arguments")
			}
			gid, err := requireFlag("enum-option-gid", gid)
			if err != nil {
				return err
			}
			data := map[string]any{}
			if cmd.Flags().Changed("name") {
				name, err := requireNonEmptyName(name)
				if err != nil {
					return err
				}
				data["name"] = name
			}
			if cmd.Flags().Changed("color") {
				data["color"] = strings.TrimSpace(color)
			}
			if cmd.Flags().Changed("enabled") {
				data["enabled"] = enabled
			}
			if len(data) == 0 {
				return usageErrorf("at least one of --name, --color, or --enabled must be set")
			}
			return customFieldMutation(cmd, http.MethodPut, enumOptionPath(gid), data, "Updated enum option "+gid+".")
		},
	}
	cmd.Flags().StringVar(&gid, "enum-option-gid", "", "enum option GID (required)")
	cmd.Flags().StringVar(&name, "name", "", "new enum option name")
	cmd.Flags().StringVar(&color, "color", "", "new enum option color")
	cmd.Flags().BoolVar(&enabled, "enabled", false, "whether the enum option is enabled")
	return cmd
}

func newDisableEnumOptionCommand() *cobra.Command {
	var gid string
	cmd := &cobra.Command{
		Use:   "disable-enum-option",
		Short: "Disable an Asana enum option",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("disable-enum-option does not accept positional arguments")
			}
			gid, err := requireFlag("enum-option-gid", gid)
			if err != nil {
				return err
			}
			return customFieldMutation(cmd, http.MethodPost, enumOptionPath(gid)+"/disable", map[string]any{}, "Disabled enum option "+gid+".")
		},
	}
	cmd.Flags().StringVar(&gid, "enum-option-gid", "", "enum option GID (required)")
	return cmd
}

func newReorderEnumOptionCommand() *cobra.Command {
	var fieldGID, optionGID, before, after string
	cmd := &cobra.Command{
		Use:   "reorder-enum-option",
		Short: "Reorder an enum option",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("reorder-enum-option does not accept positional arguments")
			}
			field, err := requireFlag("custom-field-gid", fieldGID)
			if err != nil {
				return err
			}
			option, err := requireFlag("enum-option-gid", optionGID)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("before") == cmd.Flags().Changed("after") {
				return usageErrorf("exactly one of --before or --after is required")
			}
			data := map[string]any{"enum_option": option}
			if cmd.Flags().Changed("before") {
				value, err := requireFlag("before", before)
				if err != nil {
					return err
				}
				data["before_value"] = value
			} else {
				value, err := requireFlag("after", after)
				if err != nil {
					return err
				}
				data["after_value"] = value
			}
			return customFieldMutation(cmd, http.MethodPost, customFieldPath(field)+"/enum_options/insert", data, "Reordered enum option "+option+".")
		},
	}
	cmd.Flags().StringVar(&fieldGID, "custom-field-gid", "", "custom field GID (required)")
	cmd.Flags().StringVar(&optionGID, "enum-option-gid", "", "enum option GID (required)")
	cmd.Flags().StringVar(&before, "before", "", "place before enum option GID")
	cmd.Flags().StringVar(&after, "after", "", "place after enum option GID")
	return cmd
}

func customFieldSettingsBase(parentType, parentGID string) (string, error) {
	parentType = strings.ToLower(strings.TrimSpace(parentType))
	if parentType != "project" && parentType != "portfolio" {
		return "", usageErrorf("--parent-type must be project or portfolio, got %q", parentType)
	}
	return "/" + parentType + "s/" + asana.EncodePathSegment(parentGID) + "/custom_field_settings", nil
}

func newListCustomFieldSettingsCommand() *cobra.Command {
	var parentGID, parentType, optFields string
	var pagination paginationOptions
	cmd := &cobra.Command{
		Use:   "list-custom-field-settings",
		Short: "List custom field settings on a project or portfolio",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("list-custom-field-settings does not accept positional arguments")
			}
			parent, err := requireFlag("parent-gid", parentGID)
			if err != nil {
				return err
			}
			base, err := customFieldSettingsBase(parentType, parent)
			if err != nil {
				return err
			}
			limit, err := pagination.validate(cmd, 100)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("limit", strconv.Itoa(pageSize))
			if pagination.offset != "" {
				q.Set("offset", pagination.offset)
			}
			appendOptFields(q, optFields)
			c, _, err := buildClient()
			if err != nil {
				return err
			}
			ctx, cancel := withTimeout(cmd)
			defer cancel()
			result, err := c.Paginate(ctx, base+querySuffix(q), limit, paginationPageLimit(cmd, &pagination))
			if err != nil {
				return err
			}
			return writeSuccessWithPagination(cmd.OutOrStdout(), result.Items, pageMetadata(result), opts.human, humanList(result.Items, summarizeCustomFieldSetting, "No custom field settings found."))
		},
	}
	cmd.Flags().StringVar(&parentGID, "parent-gid", "", "project or portfolio GID (required)")
	cmd.Flags().StringVar(&parentType, "parent-type", "", "parent type: project or portfolio (required)")
	pagination.addFlags(cmd, 20)
	cmd.Flags().StringVar(&optFields, "opt-fields", "", "comma-separated Asana opt_fields")
	return cmd
}

func newAddCustomFieldSettingCommand() *cobra.Command {
	var parentGID, parentType, fieldGID string
	var important bool
	cmd := &cobra.Command{
		Use:   "add-custom-field-setting",
		Short: "Attach a custom field to a project or portfolio",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("add-custom-field-setting does not accept positional arguments")
			}
			parent, err := requireFlag("parent-gid", parentGID)
			if err != nil {
				return err
			}
			field, err := requireFlag("custom-field-gid", fieldGID)
			if err != nil {
				return err
			}
			base, err := customFieldSettingsBase(parentType, parent)
			if err != nil {
				return err
			}
			data := map[string]any{"custom_field": field}
			if cmd.Flags().Changed("is-important") {
				data["is_important"] = important
			}
			return customFieldMutation(cmd, http.MethodPost, base, data, "Added custom field "+field+" to "+parent+".")
		},
	}
	cmd.Flags().StringVar(&parentGID, "parent-gid", "", "project or portfolio GID (required)")
	cmd.Flags().StringVar(&parentType, "parent-type", "", "parent type: project or portfolio (required)")
	cmd.Flags().StringVar(&fieldGID, "custom-field-gid", "", "custom field GID (required)")
	cmd.Flags().BoolVar(&important, "is-important", false, "mark the field as important")
	return cmd
}

func newRemoveCustomFieldSettingCommand() *cobra.Command {
	var parentGID, parentType, fieldGID string
	cmd := &cobra.Command{
		Use:   "remove-custom-field-setting",
		Short: "Remove a custom field from a project or portfolio",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageErrorf("remove-custom-field-setting does not accept positional arguments")
			}
			parent, err := requireFlag("parent-gid", parentGID)
			if err != nil {
				return err
			}
			field, err := requireFlag("custom-field-gid", fieldGID)
			if err != nil {
				return err
			}
			base, err := customFieldSettingsBase(parentType, parent)
			if err != nil {
				return err
			}
			return customFieldMutation(cmd, http.MethodDelete, base+"/"+asana.EncodePathSegment(field), nil, "Removed custom field "+field+" from "+parent+".")
		},
	}
	cmd.Flags().StringVar(&parentGID, "parent-gid", "", "project or portfolio GID (required)")
	cmd.Flags().StringVar(&parentType, "parent-type", "", "parent type: project or portfolio (required)")
	cmd.Flags().StringVar(&fieldGID, "custom-field-gid", "", "custom field GID (required)")
	return cmd
}

// parseCustomFields accepts FIELD_GID=VALUE and its typed form
// FIELD_GID=type:value. Typed values avoid ambiguous JSON while keeping the
// older untyped syntax compatible. json.Number is used for number values so
// decimal precision is not changed by an intermediate float64 conversion.
func parseCustomFields(assignments []string) (map[string]any, error) {
	if len(assignments) == 0 {
		return nil, nil
	}
	fields := make(map[string]any, len(assignments))
	for _, assignment := range assignments {
		separator := strings.IndexByte(assignment, '=')
		if separator <= 0 {
			return nil, usageErrorf("--custom-field must use FIELD_GID=VALUE form, got %q", assignment)
		}
		gid := strings.TrimSpace(assignment[:separator])
		if gid == "" {
			return nil, usageErrorf("--custom-field field GID must not be empty")
		}
		if _, exists := fields[gid]; exists {
			return nil, usageErrorf("duplicate --custom-field assignment for %q", gid)
		}
		value, err := parseCustomFieldValue(strings.TrimSpace(assignment[separator+1:]), gid)
		if err != nil {
			return nil, err
		}
		fields[gid] = value
	}
	return fields, nil
}

func parseCustomFieldValue(raw, gid string) (any, error) {
	if raw == "" || raw == "null" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "json:") {
		// Keep the pre-typed json: escape hatch backwards compatible. Typed
		// number: values below use json.Number to preserve their spelling.
		legacy := strings.TrimSpace(strings.TrimPrefix(raw, "json:"))
		if legacy == "" || !json.Valid([]byte(legacy)) {
			return nil, usageErrorf("--custom-field json value for %q must be valid JSON", gid)
		}
		var value any
		if err := json.Unmarshal([]byte(legacy), &value); err != nil {
			return nil, usageErrorf("invalid --custom-field value for %q: %v", gid, err)
		}
		return value, nil
	}
	colon := strings.IndexByte(raw, ':')
	if colon < 0 {
		return raw, nil
	}
	typ, value := strings.ToLower(strings.TrimSpace(raw[:colon])), raw[colon+1:]
	switch typ {
	case "text":
		return value, nil
	case "number":
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, usageErrorf("custom field %q number value cannot be empty", gid)
		}
		if !json.Valid([]byte(value)) || strings.ContainsAny(value, "{}[]\"tfru") {
			return nil, usageErrorf("custom field %q number value must be a JSON number", gid)
		}
		return json.Number(value), nil
	case "enum":
		return requireTypedGID(value, gid, typ)
	case "date":
		value = strings.TrimSpace(value)
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return nil, usageErrorf("custom field %q date value must be YYYY-MM-DD, got %q", gid, value)
		}
		return value, nil
	case "multi-enum", "multi_enum", "people":
		parts := strings.Split(value, ",")
		if len(parts) == 1 && strings.TrimSpace(parts[0]) == "" {
			return nil, usageErrorf("custom field %q %s value must contain at least one GID", gid, typ)
		}
		result := make([]string, len(parts))
		seen := make(map[string]bool, len(parts))
		for i, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				return nil, usageErrorf("custom field %q %s value contains an empty GID", gid, typ)
			}
			if seen[part] {
				return nil, usageErrorf("custom field %q %s value contains duplicate GID %q", gid, typ, part)
			}
			seen[part] = true
			result[i] = part
		}
		return result, nil
	default:
		return nil, usageErrorf("custom field %q has unsupported value type %q (use text, number, enum, multi-enum, date, people, or null)", gid, typ)
	}
}

func requireTypedGID(raw, gid, typ string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.Contains(value, ",") {
		return "", usageErrorf("custom field %q %s value must be one GID", gid, typ)
	}
	return value, nil
}

func summarizeCustomField(raw json.RawMessage) string {
	var value struct {
		GID             string `json:"gid"`
		Name            string `json:"name"`
		ResourceSubtype string `json:"resource_subtype"`
		Type            string `json:"type"`
	}
	_ = json.Unmarshal(raw, &value)
	kind := value.ResourceSubtype
	if kind == "" {
		kind = value.Type
	}
	if kind == "" {
		kind = "unknown"
	}
	name := value.Name
	if name == "" {
		name = "(unnamed custom field)"
	}
	return fmt.Sprintf("%s %s [%s]", orUnknown(value.GID), name, kind)
}

func summarizeCustomFieldSetting(raw json.RawMessage) string {
	var value struct {
		GID         string          `json:"gid"`
		IsImportant bool            `json:"is_important"`
		CustomField json.RawMessage `json:"custom_field"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	if len(value.CustomField) > 0 && !bytes.Equal(bytes.TrimSpace(value.CustomField), []byte("null")) {
		return fmt.Sprintf("%s important=%t: %s", orUnknown(value.GID), value.IsImportant, summarizeCustomField(value.CustomField))
	}
	return fmt.Sprintf("%s important=%t", orUnknown(value.GID), value.IsImportant)
}
