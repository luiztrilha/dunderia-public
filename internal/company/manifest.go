package company

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/atomicfile"
	"github.com/nex-crm/wuphf/internal/config"
	"github.com/nex-crm/wuphf/internal/lightstate"
	"github.com/nex-crm/wuphf/internal/provider"
)

var (
	atomicReplaceManifestFile = atomicfile.Replace
	syncManifestTempFile      = func(file *os.File) error { return file.Sync() }
	syncManifestDir           = atomicfile.SyncDir
)

type MemberSpec struct {
	Slug           string                   `json:"slug"`
	Name           string                   `json:"name"`
	Role           string                   `json:"role,omitempty"`
	Expertise      []string                 `json:"expertise,omitempty"`
	Personality    string                   `json:"personality,omitempty"`
	PermissionMode string                   `json:"permission_mode,omitempty"`
	AllowedTools   []string                 `json:"allowed_tools,omitempty"`
	System         bool                     `json:"system,omitempty"`
	Provider       provider.ProviderBinding `json:"provider,omitempty"`
}

type ChannelSurfaceSpec struct {
	Provider    string `json:"provider,omitempty"`
	RemoteID    string `json:"remote_id,omitempty"`
	RemoteTitle string `json:"remote_title,omitempty"`
	Mode        string `json:"mode,omitempty"`
	BotTokenEnv string `json:"bot_token_env,omitempty"`
}

type BlueprintRef struct {
	Kind   string `json:"kind,omitempty"`
	ID     string `json:"id,omitempty"`
	Source string `json:"source,omitempty"`
}

type ChannelSpec struct {
	Slug        string              `json:"slug"`
	Name        string              `json:"name,omitempty"`
	Description string              `json:"description,omitempty"`
	Members     []string            `json:"members,omitempty"`
	Disabled    []string            `json:"disabled,omitempty"`
	Surface     *ChannelSurfaceSpec `json:"surface,omitempty"`
}

type Manifest struct {
	Name          string         `json:"name,omitempty"`
	Description   string         `json:"description,omitempty"`
	Lead          string         `json:"lead,omitempty"`
	Members       []MemberSpec   `json:"members,omitempty"`
	Channels      []ChannelSpec  `json:"channels,omitempty"`
	BlueprintRefs []BlueprintRef `json:"blueprint_refs,omitempty"`
	UpdatedAt     string         `json:"updated_at,omitempty"`
}

func (m Manifest) ActiveBlueprintRefs() []BlueprintRef {
	return normalizeBlueprintRefs(m.BlueprintRefs)
}

func (m Manifest) PrimaryBlueprintRef() (BlueprintRef, bool) {
	refs := m.ActiveBlueprintRefs()
	if len(refs) == 0 {
		return BlueprintRef{}, false
	}
	return refs[0], true
}

func (m Manifest) BlueprintRefsByKind(kind string) []BlueprintRef {
	kind = normalizeBlueprintKind(kind)
	refs := m.ActiveBlueprintRefs()
	if kind == "" {
		return refs
	}
	out := make([]BlueprintRef, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind == kind {
			out = append(out, ref)
		}
	}
	return out
}

func ManifestPath() string {
	if path := strings.TrimSpace(os.Getenv("WUPHF_COMPANY_FILE")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("NEX_COMPANY_FILE")); path != "" {
		return path
	}

	if strings.TrimSpace(os.Getenv("WUPHF_RUNTIME_HOME")) == "" {
		if cwd, err := os.Getwd(); err == nil {
			local := filepath.Join(cwd, "wuphf.company.json")
			if _, err := os.Stat(local); err == nil {
				return local
			}
		}
	}

	home := config.RuntimeHomeDir()
	if home == "" {
		return filepath.Join(".wuphf", "company.json")
	}
	return filepath.Join(home, ".wuphf", "company.json")
}

func LoadManifest() (Manifest, error) {
	path := ManifestPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Saved topology is only restored through explicit restore flows; plain
			// reads must not materialize prior office state on a clean install.
			manifest := DefaultManifest()
			return manifest, nil
		} else {
			return Manifest{}, err
		}
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	manifest = backfillFromConfig(manifest)
	manifest = normalizeManifest(manifest)
	return manifest, nil
}

// backfillFromConfig fills empty manifest Name/Description from config
// so onboarding answers flow into the company manifest.
func backfillFromConfig(manifest Manifest) Manifest {
	cfg, _ := config.Load()
	if strings.TrimSpace(manifest.Name) == "" || manifest.Name == "The WUPHF Office" || manifest.Name == "The DunderIA Office" {
		if name := strings.TrimSpace(cfg.CompanyName); name != "" {
			manifest.Name = name
		}
	}
	if strings.TrimSpace(manifest.Description) == "" || strings.Contains(strings.ToLower(manifest.Description), "founding team") {
		if desc := strings.TrimSpace(cfg.CompanyDescription); desc != "" {
			manifest.Description = desc
		} else {
			manifest.Description = "Autonomous office runtime."
		}
	}
	if len(normalizeBlueprintRefs(manifest.BlueprintRefs)) == 0 {
		if blueprint := strings.TrimSpace(cfg.ActiveBlueprint()); blueprint != "" {
			manifest.BlueprintRefs = []BlueprintRef{{
				Kind:   "operation",
				ID:     normalizeSlug(blueprint),
				Source: "config",
			}}
		}
	}
	return manifest
}

func SaveManifest(manifest Manifest) error {
	manifest = normalizeManifest(manifest)
	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	path := ManifestPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := syncManifestTempFile(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := atomicReplaceManifestFile(tmpPath, path); err != nil {
		return err
	}
	if err := syncManifestDir(dir); err != nil {
		return err
	}
	// Cloud mirror is best-effort; local manifest persistence stays authoritative.
	if err := lightstate.MirrorCompany(config.ResolveCloudBackupSettings(), path); err != nil {
		log.Printf("company manifest: cloud mirror failed after local write: %v", err)
	}
	return nil
}

func DefaultManifest() Manifest {
	now := time.Now().UTC().Format(time.RFC3339)
	cfg, _ := config.Load()
	if launchFromScratchRequested() {
		return normalizeManifest(fromScratchDefaultManifest(now))
	}
	blueprintID := normalizeSlug(cfg.ActiveBlueprint())
	manifest := Manifest{
		Name:        "The DunderIA Office",
		Description: "Autonomous office runtime.",
		Lead:        "ceo",
		UpdatedAt:   now,
	}
	if blueprintID != "" {
		manifest.BlueprintRefs = []BlueprintRef{{
			Kind:   "operation",
			ID:     blueprintID,
			Source: "config",
		}}
		if resolved, ok := MaterializeManifest(manifest, repoRootFromCWD()); ok {
			resolved.UpdatedAt = now
			return normalizeManifest(resolved)
		}
	}
	manifest.Members = []MemberSpec{
		{
			Slug:           "ceo",
			Name:           "CEO",
			Role:           "Owns priorities, approvals, and escalation decisions",
			PermissionMode: "plan",
			System:         true,
		},
		{
			Slug:           "pm",
			Name:           "PM",
			Role:           "Turns requests into scope, acceptance criteria, and sequenced work",
			PermissionMode: "plan",
			Expertise:      []string{"Scoping", "Backlog design", "Acceptance criteria"},
		},
		{
			Slug:           "research-lead",
			Name:           "Research Lead",
			Role:           "Frames ambiguous problems, compares approaches, and shapes the first attack plan",
			PermissionMode: "plan",
			Expertise:      []string{"Discovery", "Problem framing", "Technical investigation"},
		},
		{
			Slug:           "estagiario",
			Name:           "Estagiario",
			Role:           "Handles low-risk triage, context gathering, and operational prep work",
			PermissionMode: "auto",
			Expertise:      []string{"Triage", "Reproduction", "Context gathering"},
		},
		{
			Slug:           "backend",
			Name:           "Backend",
			Role:           "Owns APIs, data, integrations, jobs, and server-side architecture",
			PermissionMode: "auto",
			Expertise:      []string{"APIs", "Databases", "Integrations"},
		},
		{
			Slug:           "frontend",
			Name:           "Frontend",
			Role:           "Owns UI, UX, accessibility, and client-side implementation quality",
			PermissionMode: "auto",
			Expertise:      []string{"UI", "UX", "Accessibility"},
		},
		{
			Slug:           "builder",
			Name:           "Builder",
			Role:           "Closes cross-stack delivery, wiring, and end-to-end integration work",
			PermissionMode: "auto",
			Expertise:      []string{"Full stack delivery", "Integration", "Execution"},
		},
		{
			Slug:           "reviewer",
			Name:           "Reviewer",
			Role:           "Owns review findings, regression risk, and acceptance validation",
			PermissionMode: "plan",
			Expertise:      []string{"Code review", "Regression analysis", "Verification"},
		},
		{
			Slug:           "game-master",
			Name:           "Game Master",
			Role:           "Owner-invoked escalation operator for critical takeovers and direct intervention",
			PermissionMode: "all",
			Expertise:      []string{"Escalation handling", "Cross-stack recovery", "Direct execution"},
		},
	}
	generalMembers := make([]string, 0, len(manifest.Members))
	for _, member := range manifest.Members {
		generalMembers = append(generalMembers, member.Slug)
	}
	manifest.Channels = []ChannelSpec{{
		Slug:        "general",
		Name:        "general",
		Description: "Single default room for coordination, triage, planning, and execution.",
		Members:     generalMembers,
	}}
	return normalizeManifest(manifest)
}

func launchFromScratchRequested() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WUPHF_START_FROM_SCRATCH"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func fromScratchDefaultManifest(now string) Manifest {
	members := []MemberSpec{
		{Slug: "founder", Name: "Founder", Role: "Founder", System: true},
		{Slug: "operator", Name: "Operator", Role: "Operator", System: true},
		{Slug: "builder", Name: "Builder", Role: "Builder"},
		{Slug: "reviewer", Name: "Reviewer", Role: "Reviewer"},
	}
	channelMembers := make([]string, 0, len(members))
	for _, member := range members {
		channelMembers = append(channelMembers, member.Slug)
	}
	return Manifest{
		Name:        "DunderIA Office",
		Description: "Autonomous office runtime that starts from a directive instead of a saved blueprint.",
		Lead:        "founder",
		Members:     members,
		Channels: []ChannelSpec{{
			Slug:        "general",
			Name:        "general",
			Description: "Primary room for inventing and operating the business from scratch.",
			Members:     channelMembers,
		}},
		UpdatedAt: now,
	}
}

func normalizeManifest(manifest Manifest) Manifest {
	if strings.TrimSpace(manifest.Name) == "" {
		manifest.Name = "The DunderIA Office"
	}
	if strings.TrimSpace(manifest.Lead) == "" {
		manifest.Lead = "ceo"
	}
	manifest.BlueprintRefs = normalizeBlueprintRefs(manifest.BlueprintRefs)

	seenMembers := make(map[string]struct{}, len(manifest.Members))
	members := make([]MemberSpec, 0, len(manifest.Members))
	for _, member := range manifest.Members {
		member.Slug = normalizeSlug(member.Slug)
		if member.Slug == "" {
			continue
		}
		if _, ok := seenMembers[member.Slug]; ok {
			continue
		}
		seenMembers[member.Slug] = struct{}{}
		member.Name = strings.TrimSpace(member.Name)
		if member.Name == "" {
			member.Name = humanizeSlug(member.Slug)
		}
		member.Role = strings.TrimSpace(member.Role)
		if member.Role == "" {
			member.Role = member.Name
		}
		member.Expertise = normalizeStrings(member.Expertise)
		member.AllowedTools = normalizeStrings(member.AllowedTools)
		member.System = member.Slug == manifest.Lead || member.Slug == "ceo" || member.System
		members = append(members, member)
	}
	if len(members) == 0 {
		if resolved, ok := MaterializeManifest(manifest, repoRootFromCWD()); ok {
			return resolved
		}
		return DefaultManifest()
	}
	manifest.Members = members

	seenChannels := make(map[string]struct{}, len(manifest.Channels))
	channels := make([]ChannelSpec, 0, len(manifest.Channels))
	for _, channel := range manifest.Channels {
		channel.Slug = normalizeSlug(channel.Slug)
		if channel.Slug == "" {
			continue
		}
		if _, ok := seenChannels[channel.Slug]; ok {
			continue
		}
		seenChannels[channel.Slug] = struct{}{}
		channel.Name = strings.TrimSpace(channel.Name)
		if channel.Name == "" {
			channel.Name = channel.Slug
		}
		channel.Description = strings.TrimSpace(channel.Description)
		if channel.Description == "" {
			channel.Description = defaultChannelDescription(channel.Slug, channel.Name)
		}
		channel.Members = normalizeSlugs(channel.Members)
		channel.Disabled = normalizeSlugs(channel.Disabled)
		channel.Members = ensureLeadMember(channel.Members, manifest.Lead)
		channel.Disabled = removeSlug(channel.Disabled, manifest.Lead)
		channels = append(channels, channel)
	}
	if !containsChannel(channels, "general") {
		members := make([]string, 0, len(manifest.Members))
		for _, member := range manifest.Members {
			members = append(members, member.Slug)
		}
		channels = append([]ChannelSpec{{
			Slug:        "general",
			Name:        "general",
			Description: defaultChannelDescription("general", "general"),
			Members:     ensureLeadMember(members, manifest.Lead),
		}}, channels...)
	}
	manifest.Channels = channels
	return manifest
}

func normalizeBlueprintRefs(refs []BlueprintRef) []BlueprintRef {
	seen := make(map[string]struct{}, len(refs))
	out := make([]BlueprintRef, 0, len(refs))
	for _, ref := range refs {
		ref.Kind = normalizeBlueprintKind(ref.Kind)
		ref.ID = normalizeSlug(ref.ID)
		ref.Source = strings.ToLower(strings.TrimSpace(ref.Source))
		if ref.Source == "" {
			ref.Source = "manifest"
		}
		if ref.ID == "" {
			continue
		}
		key := ref.Kind + "|" + ref.ID + "|" + ref.Source
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}
	return out
}

func normalizeBlueprintKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "operation", "business", "template":
		return "operation"
	case "employee", "role":
		return "employee"
	default:
		return normalizeSlug(kind)
	}
}

func containsChannel(channels []ChannelSpec, slug string) bool {
	for _, channel := range channels {
		if channel.Slug == slug {
			return true
		}
	}
	return false
}

func defaultChannelDescription(slug, name string) string {
	if strings.TrimSpace(slug) == "" {
		slug = strings.TrimSpace(name)
	}
	switch normalizeSlug(slug) {
	case "general":
		return "The default company-wide room for top-level coordination, announcements, and cross-functional discussion."
	default:
		label := strings.TrimSpace(name)
		if label == "" {
			label = humanizeSlug(slug)
		}
		return label + " focused work. Use this channel for discussion, decisions, and execution specific to that stream."
	}
}

func ensureLeadMember(members []string, lead string) []string {
	lead = normalizeSlug(lead)
	if lead == "" {
		lead = "ceo"
	}
	if containsSlug(members, lead) {
		return normalizeSlugs(members)
	}
	return append([]string{lead}, normalizeSlugs(members)...)
}

func removeSlug(items []string, slug string) []string {
	slug = normalizeSlug(slug)
	var out []string
	for _, item := range normalizeSlugs(items) {
		if item != slug {
			out = append(out, item)
		}
	}
	return out
}

func containsSlug(items []string, want string) bool {
	want = normalizeSlug(want)
	for _, item := range normalizeSlugs(items) {
		if item == want {
			return true
		}
	}
	return false
}

func normalizeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeSlugs(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = normalizeSlug(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeSlug(input string) string {
	slug := strings.ToLower(strings.TrimSpace(input))
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	return slug
}

func humanizeSlug(slug string) string {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(slug), "-", " "), " ")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
