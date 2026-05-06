package commands

import (
	"net/http"
	"sort"
	"strings"
)

const (
	SurfaceAll = "all"
	SurfaceTUI = "tui"
	SurfaceWeb = "web"
)

// ManifestEntry describes slash command behavior for generated help,
// autocomplete, and drift checks. Names include the leading slash.
type ManifestEntry struct {
	Name                 string   `json:"name"`
	Category             string   `json:"category"`
	Description          string   `json:"description"`
	Surface              string   `json:"surface"`
	Route                string   `json:"route,omitempty"`
	Method               string   `json:"method,omitempty"`
	Args                 string   `json:"args,omitempty"`
	Mutating             bool     `json:"mutating"`
	RequiresConfirmation bool     `json:"requires_confirmation,omitempty"`
	TopologySensitive    bool     `json:"topology_sensitive,omitempty"`
	Signals              []string `json:"signals,omitempty"`
}

func BuildCommandManifest() []ManifestEntry {
	entries := []ManifestEntry{
		{Name: "/help", Category: "navigation", Description: "Show available commands.", Surface: SurfaceWeb, Mutating: false},
		{Name: "/search", Category: "navigation", Description: "Open search.", Surface: "web,tui", Mutating: false},
		{Name: "/threads", Category: "navigation", Description: "Open thread view.", Surface: "web,tui", Mutating: false},
		{Name: "/requests", Category: "navigation", Description: "Open human requests.", Surface: "web,tui", Mutating: false},
		{Name: "/policies", Category: "navigation", Description: "Open active policies.", Surface: "web,tui", Mutating: false},
		{Name: "/skills", Category: "navigation", Description: "Open skills.", Surface: "web,tui", Mutating: false},
		{Name: "/calendar", Category: "navigation", Description: "Open calendar.", Surface: "web,tui", Mutating: false},
		{Name: "/tasks", Category: "navigation", Description: "Open tasks.", Surface: "web,tui", Mutating: false},
		{Name: "/recover", Category: "diagnostics", Description: "Open recovery and health checks.", Surface: "web,tui", Route: "/runtime/doctor", Method: http.MethodGet, Mutating: false},
		{Name: "/doctor", Category: "diagnostics", Description: "Open recovery and health checks.", Surface: "web,tui", Route: "/runtime/doctor", Method: http.MethodGet, Mutating: false},
		{Name: "/provider", Category: "runtime", Description: "Open provider switcher.", Surface: "web,tui", Mutating: false},
		{Name: "/clear", Category: "channel", Description: "Clear the current channel timeline.", Surface: SurfaceWeb, Route: "/channels/clear", Method: http.MethodPost, Mutating: true, RequiresConfirmation: false, Signals: []string{"channel_messages"}},
		{Name: "/focus", Category: "runtime", Description: "Enable focus mode.", Surface: "web,tui", Route: "/focus-mode", Method: http.MethodPost, Mutating: true, Signals: []string{"runtime_mode"}},
		{Name: "/collab", Category: "runtime", Description: "Disable focus mode and use collaborative mode.", Surface: "web,tui", Route: "/focus-mode", Method: http.MethodPost, Mutating: true, Signals: []string{"runtime_mode"}},
		{Name: "/pause", Category: "runtime", Description: "Record a pause signal for agents.", Surface: SurfaceWeb, Route: "/signals", Method: http.MethodPost, Mutating: true, Signals: []string{"agent_signal"}},
		{Name: "/resume", Category: "runtime", Description: "Record a resume signal for agents.", Surface: "web,tui", Route: "/signals", Method: http.MethodPost, Mutating: true, Signals: []string{"agent_signal"}},
		{Name: "/reset", Category: "runtime", Description: "Reset runtime messages, tasks, and requests while preserving protected topology and policies.", Surface: "web,tui", Route: "/reset", Method: http.MethodPost, Mutating: true, RequiresConfirmation: true, TopologySensitive: true, Signals: []string{"runtime_reset", "protected_topology"}},
		{Name: "/1o1", Category: "channel", Description: "Open or create a direct channel with an agent.", Surface: "web,tui", Route: "/channels/dm", Method: http.MethodPost, Args: "<agent>", Mutating: true, TopologySensitive: true, Signals: []string{"dm_channel"}},
		{Name: "/task", Category: "task", Description: "Apply an action to a task.", Surface: "web,tui", Route: "/tasks", Method: http.MethodPost, Args: "<action> <id> [details]", Mutating: true, Signals: []string{"task_mutation"}},
		{Name: "/cancel", Category: "task", Description: "Release or cancel task follow-up.", Surface: "web,tui", Route: "/tasks", Method: http.MethodPost, Args: "<id>", Mutating: true, Signals: []string{"task_mutation"}},

		{Name: "/init", Category: "setup", Description: "Run setup and save your default runtime choices.", Surface: SurfaceTUI, Mutating: true, Signals: []string{"runtime_setup"}},
		{Name: "/integrate", Category: "setup", Description: "Connect a managed integration.", Surface: SurfaceTUI, Mutating: true, Signals: []string{"integration_setup"}},
		{Name: "/connect", Category: "setup", Description: "Bring Telegram or other integrations into the office.", Surface: SurfaceTUI, Mutating: true, Signals: []string{"integration_setup"}},
		{Name: "/messages", Category: "navigation", Description: "Show the main office feed.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/general", Category: "navigation", Description: "Return to the main office feed.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/inbox", Category: "navigation", Description: "Show the selected agent inbox lane in direct mode.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/outbox", Category: "navigation", Description: "Show the selected agent outbox lane in direct mode.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/rewind", Category: "navigation", Description: "Catch up from a recovery point.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/insert", Category: "navigation", Description: "Insert a channel, task, request, or message reference.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/switcher", Category: "navigation", Description: "Switch office or direct session views.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/switch", Category: "navigation", Description: "Switch to another channel.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/channels", Category: "navigation", Description: "Browse and manage channels.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/channel", Category: "channels", Description: "Create or remove a channel.", Surface: SurfaceTUI, Args: "<add|remove> <slug>", Mutating: true, TopologySensitive: true, Signals: []string{"protected_topology", "channel_mutation"}},
		{Name: "/agents", Category: "people", Description: "Manage your team roster.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/agent", Category: "people", Description: "Add, remove, enable, disable, or edit a teammate.", Surface: SurfaceTUI, Mutating: true, TopologySensitive: true, Signals: []string{"protected_topology", "agent_mutation"}},
		{Name: "/agent prompt", Category: "people", Description: "Generate a new teammate from a prompt.", Surface: SurfaceTUI, Mutating: true, TopologySensitive: true, Signals: []string{"protected_topology", "agent_mutation"}},
		{Name: "/request", Category: "work", Description: "Focus, answer, or snooze a human request.", Surface: SurfaceTUI, Args: "<focus|answer|snooze> <id>", Mutating: true, Signals: []string{"human_request"}},
		{Name: "/queue", Category: "navigation", Description: "Alias for /calendar.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/artifacts", Category: "navigation", Description: "Show task logs, approvals, and workflow artifacts.", Surface: SurfaceTUI, Mutating: false},
		{Name: "/skill", Category: "work", Description: "Create, invoke, or manage a skill.", Surface: SurfaceTUI, Mutating: true, Signals: []string{"skill_mutation"}},
		{Name: "/reply", Category: "conversation", Description: "Reply in a thread.", Surface: SurfaceTUI, Args: "<message-id>", Mutating: false},
		{Name: "/expand", Category: "conversation", Description: "Expand a collapsed thread.", Surface: SurfaceTUI, Args: "<message-id|all>", Mutating: false},
		{Name: "/collapse", Category: "conversation", Description: "Collapse a thread.", Surface: SurfaceTUI, Args: "<message-id|all>", Mutating: false},
		{Name: "/reset-dm", Category: "runtime", Description: "Clear direct messages with an agent.", Surface: SurfaceTUI, Args: "<agent>", Mutating: true, RequiresConfirmation: true, TopologySensitive: true, Signals: []string{"runtime_reset", "dm_channel"}},
		{Name: "/dm", Category: "channel", Description: "Open a direct message channel with an agent.", Surface: SurfaceTUI, Args: "<agent>", Mutating: true, TopologySensitive: true, Signals: []string{"dm_channel"}},
		{Name: "/quit", Category: "session", Description: "Exit MaestrIA.", Surface: SurfaceTUI, Mutating: false},
	}
	sortCommandManifest(entries)
	return entries
}

func FilterCommandManifest(entries []ManifestEntry, surface string) []ManifestEntry {
	surface = NormalizeManifestSurface(surface)
	if surface == "" {
		return nil
	}
	if surface == SurfaceAll {
		return append([]ManifestEntry(nil), entries...)
	}
	filtered := make([]ManifestEntry, 0, len(entries))
	for _, entry := range entries {
		if ManifestEntryHasSurface(entry, surface) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func ManifestEntryHasSurface(entry ManifestEntry, surface string) bool {
	surface = NormalizeManifestSurface(surface)
	if surface == "" {
		return false
	}
	if surface == SurfaceAll {
		return true
	}
	for _, part := range strings.Split(entry.Surface, ",") {
		if strings.EqualFold(strings.TrimSpace(part), surface) {
			return true
		}
	}
	return false
}

func NormalizeManifestSurface(surface string) string {
	switch strings.ToLower(strings.TrimSpace(surface)) {
	case "", SurfaceWeb:
		return SurfaceWeb
	case SurfaceTUI:
		return SurfaceTUI
	case SurfaceAll:
		return SurfaceAll
	default:
		return ""
	}
}

func sortCommandManifest(entries []ManifestEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Name < entries[j].Name
	})
}
