package teammcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TeamSkillListArgs are the inputs for the team_skill_list tool.
type TeamSkillListArgs struct {
	Channel    string `json:"channel,omitempty" jsonschema:"Optional channel slug to filter visible skills. Defaults to the active conversation channel."`
	MySlug     string `json:"my_slug,omitempty" jsonschema:"Agent slug requesting the list. Defaults to WUPHF_AGENT_SLUG."`
	Query      string `json:"query,omitempty" jsonschema:"Optional text filter over name, title, description, trigger, tags, capabilities, and plugin metadata."`
	Capability string `json:"capability,omitempty" jsonschema:"Optional capability filter, e.g. skill.invoke or knowledge.reuse."`
	PluginID   string `json:"plugin_id,omitempty" jsonschema:"Optional plugin/source id filter, e.g. dunderia-learning."`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum number of skills to return. Defaults to 12."`
}

// TeamSkillViewArgs are the inputs for the team_skill_view tool.
type TeamSkillViewArgs struct {
	SkillName string `json:"skill_name" jsonschema:"Name of the skill to inspect (slug, e.g. 'investigate', 'daily-digest')"`
	Channel   string `json:"channel,omitempty" jsonschema:"Optional channel slug used to resolve visible skills. Defaults to the active conversation channel."`
	MySlug    string `json:"my_slug,omitempty" jsonschema:"Agent slug requesting the skill. Defaults to WUPHF_AGENT_SLUG."`
}

// TeamSkillRunArgs are the inputs for the team_skill_run tool.
type TeamSkillRunArgs struct {
	SkillName string `json:"skill_name" jsonschema:"Name of the skill to run (slug, e.g. 'investigate', 'daily-digest')"`
	Channel   string `json:"channel,omitempty" jsonschema:"Optional channel slug to log the invocation into. Defaults to the active conversation channel."`
	MySlug    string `json:"my_slug,omitempty" jsonschema:"Agent slug invoking the skill. Defaults to WUPHF_AGENT_SLUG."`
}

type brokerSkillsResponse struct {
	Skills []brokerSkillSummary `json:"skills"`
}

type brokerSkillSummary struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Title               string   `json:"title"`
	Description         string   `json:"description"`
	Content             string   `json:"content"`
	PluginID            string   `json:"plugin_id"`
	PluginKind          string   `json:"plugin_kind"`
	Capabilities        []string `json:"capabilities"`
	HealthStatus        string   `json:"health_status"`
	HealthSummary       string   `json:"health_summary"`
	SourceType          string   `json:"source_type"`
	SourceRef           string   `json:"source_ref"`
	SourceHash          string   `json:"source_hash"`
	InstalledAt         string   `json:"installed_at"`
	LastScannedAt       string   `json:"last_scanned_at"`
	ScanStatus          string   `json:"scan_status"`
	ScanSummary         string   `json:"scan_summary"`
	Channel             string   `json:"channel"`
	Tags                []string `json:"tags"`
	Trigger             string   `json:"trigger"`
	WorkflowProvider    string   `json:"workflow_provider"`
	WorkflowKey         string   `json:"workflow_key"`
	LastExecutionAt     string   `json:"last_execution_at"`
	LastExecutionStatus string   `json:"last_execution_status"`
	UsageCount          int      `json:"usage_count"`
	Status              string   `json:"status"`
	UpdatedAt           string   `json:"updated_at"`
}

// brokerSkillResponse mirrors the JSON shape returned by
// POST /skills/<name>/invoke on the broker.
type brokerSkillResponse struct {
	Skill struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Content     string   `json:"content"`
		Channel     string   `json:"channel"`
		Tags        []string `json:"tags"`
		Trigger     string   `json:"trigger"`
		UsageCount  int      `json:"usage_count"`
		Status      string   `json:"status"`
	} `json:"skill"`
}

func handleTeamSkillList(ctx context.Context, _ *mcp.CallToolRequest, args TeamSkillListArgs) (*mcp.CallToolResult, any, error) {
	slug, err := resolveSlug(args.MySlug)
	if err != nil {
		return toolError(err), nil, nil
	}
	channel := resolveConversationChannel(ctx, slug, args.Channel)
	resp, err := fetchVisibleSkills(ctx, channel, args.Capability, args.PluginID)
	if err != nil {
		return toolError(fmt.Errorf("list skills: %w", err)), nil, nil
	}

	query := strings.ToLower(strings.TrimSpace(args.Query))
	limit := args.Limit
	if limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}
	skills := make([]map[string]any, 0, len(resp.Skills))
	for _, skill := range resp.Skills {
		if query != "" && !skillMatchesQuery(skill, query) {
			continue
		}
		skills = append(skills, compactSkillPayload(skill))
		if len(skills) >= limit {
			break
		}
	}
	payload := map[string]any{
		"ok":           true,
		"channel":      channel,
		"count":        len(skills),
		"skills":       skills,
		"instructions": "Use team_skill_view to inspect full content for a candidate skill. Use team_skill_run only when you are actually invoking the playbook and want the office audit trail.",
	}
	return textResult(prettyObject(payload)), nil, nil
}

func handleTeamSkillView(ctx context.Context, _ *mcp.CallToolRequest, args TeamSkillViewArgs) (*mcp.CallToolResult, any, error) {
	name := strings.TrimSpace(args.SkillName)
	if name == "" {
		return toolError(fmt.Errorf("skill_name is required")), nil, nil
	}
	slug, err := resolveSlug(args.MySlug)
	if err != nil {
		return toolError(err), nil, nil
	}
	channel := resolveConversationChannel(ctx, slug, args.Channel)
	resp, err := fetchVisibleSkills(ctx, channel, "", "")
	if err != nil {
		return toolError(fmt.Errorf("view skill %q: %w", name, err)), nil, nil
	}
	for _, skill := range resp.Skills {
		if skillPathSegment(skill.Name) != skillPathSegment(name) {
			continue
		}
		payload := map[string]any{
			"ok":                    true,
			"skill_name":            skill.Name,
			"title":                 skill.Title,
			"description":           skill.Description,
			"trigger":               skill.Trigger,
			"channel":               skill.Channel,
			"tags":                  skill.Tags,
			"plugin_id":             skill.PluginID,
			"plugin_kind":           skill.PluginKind,
			"capabilities":          skill.Capabilities,
			"health_status":         skill.HealthStatus,
			"health_summary":        skill.HealthSummary,
			"source_type":           skill.SourceType,
			"source_ref":            skill.SourceRef,
			"source_hash":           skill.SourceHash,
			"installed_at":          skill.InstalledAt,
			"last_scanned_at":       skill.LastScannedAt,
			"scan_status":           skill.ScanStatus,
			"scan_summary":          skill.ScanSummary,
			"usage_count":           skill.UsageCount,
			"last_execution_at":     skill.LastExecutionAt,
			"last_execution_status": skill.LastExecutionStatus,
			"content":               skill.Content,
			"instructions":          "This is a read-only inspection. If the request matches this playbook, call team_skill_run before acting so usage and audit trail are recorded.",
		}
		return textResult(prettyObject(payload)), nil, nil
	}
	return toolError(fmt.Errorf("skill %q not found in visible skills for channel %q", name, channel)), nil, nil
}

// handleTeamSkillRun invokes a named skill through the broker, mirroring the
// HTTP endpoint humans hit from the UI. The broker bumps UsageCount and
// appends a `skill_invocation` message to the channel so the office sees
// that the agent actually followed the playbook rather than freelancing.
func handleTeamSkillRun(ctx context.Context, _ *mcp.CallToolRequest, args TeamSkillRunArgs) (*mcp.CallToolResult, any, error) {
	name := strings.TrimSpace(args.SkillName)
	if name == "" {
		return toolError(fmt.Errorf("skill_name is required")), nil, nil
	}
	slug, err := resolveSlug(args.MySlug)
	if err != nil {
		return toolError(err), nil, nil
	}
	channel := resolveConversationChannel(ctx, slug, args.Channel)

	var resp brokerSkillResponse
	path := "/skills/" + skillPathSegment(name) + "/invoke"
	if err := brokerPostJSON(ctx, path, map[string]any{
		"invoked_by": slug,
		"channel":    channel,
	}, &resp); err != nil {
		return toolError(fmt.Errorf("invoke skill %q: %w", name, err)), nil, nil
	}

	payload := map[string]any{
		"ok":           true,
		"skill_name":   resp.Skill.Name,
		"title":        resp.Skill.Title,
		"description":  resp.Skill.Description,
		"trigger":      resp.Skill.Trigger,
		"usage_count":  resp.Skill.UsageCount,
		"channel":      resp.Skill.Channel,
		"content":      resp.Skill.Content,
		"instructions": "Follow the steps in `content` exactly. Do NOT freelance — this skill is the canonical playbook for this request.",
	}
	return textResult(prettyObject(payload)), nil, nil
}

func fetchVisibleSkills(ctx context.Context, channel string, capability string, pluginID string) (brokerSkillsResponse, error) {
	values := url.Values{}
	if strings.TrimSpace(channel) != "" {
		values.Set("channel", strings.TrimSpace(channel))
	}
	if strings.TrimSpace(capability) != "" {
		values.Set("capability", strings.TrimSpace(capability))
	}
	if strings.TrimSpace(pluginID) != "" {
		values.Set("plugin_id", strings.TrimSpace(pluginID))
	}
	path := "/skills"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var resp brokerSkillsResponse
	if err := brokerGetJSON(ctx, path, &resp); err != nil {
		return brokerSkillsResponse{}, err
	}
	return resp, nil
}

func compactSkillPayload(skill brokerSkillSummary) map[string]any {
	return map[string]any{
		"name":           skill.Name,
		"title":          skill.Title,
		"description":    skill.Description,
		"trigger":        skill.Trigger,
		"channel":        skill.Channel,
		"tags":           skill.Tags,
		"plugin_id":      skill.PluginID,
		"plugin_kind":    skill.PluginKind,
		"capabilities":   skill.Capabilities,
		"health_status":  skill.HealthStatus,
		"health_summary": skill.HealthSummary,
		"source_type":    skill.SourceType,
		"source_ref":     skill.SourceRef,
		"source_hash":    skill.SourceHash,
		"scan_status":    skill.ScanStatus,
		"scan_summary":   skill.ScanSummary,
		"usage_count":    skill.UsageCount,
		"updated_at":     skill.UpdatedAt,
	}
}

func skillMatchesQuery(skill brokerSkillSummary, query string) bool {
	parts := []string{
		skill.ID,
		skill.Name,
		skill.Title,
		skill.Description,
		skill.Trigger,
		skill.Channel,
		skill.PluginID,
		skill.PluginKind,
		skill.HealthStatus,
		skill.HealthSummary,
		skill.SourceType,
		skill.SourceRef,
		skill.SourceHash,
		skill.ScanStatus,
		skill.ScanSummary,
	}
	parts = append(parts, skill.Tags...)
	parts = append(parts, skill.Capabilities...)
	haystack := strings.ToLower(strings.Join(parts, " "))
	return strings.Contains(haystack, query)
}

// skillPathSegment normalizes a skill name into the URL path segment the
// broker expects at /skills/<name>/invoke. Broker-side lookup is
// slug-insensitive but we still trim/lowercase here so the path is stable.
func skillPathSegment(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}
