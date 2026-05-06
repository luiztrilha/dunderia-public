package commands

import "testing"

func TestCommandManifestSurfaceFilters(t *testing.T) {
	entries := BuildCommandManifest()
	web := FilterCommandManifest(entries, SurfaceWeb)
	tui := FilterCommandManifest(entries, SurfaceTUI)

	if len(web) == 0 || len(tui) == 0 {
		t.Fatalf("expected web and TUI command surfaces, got web=%d tui=%d", len(web), len(tui))
	}
	assertHasCommand(t, web, "/help")
	assertMissingCommand(t, web, "/messages")
	assertHasCommand(t, tui, "/messages")
	assertHasCommand(t, tui, "/requests")
	assertHasCommand(t, tui, "/request")
	assertMissingCommand(t, tui, "/help")
	if got := FilterCommandManifest(entries, "bogus"); len(got) != 0 {
		t.Fatalf("expected unknown surface to return no commands, got %+v", got)
	}
}

func TestCommandManifestHasUniqueNames(t *testing.T) {
	seen := map[string]struct{}{}
	for _, entry := range BuildCommandManifest() {
		if entry.Name == "" {
			t.Fatal("manifest command name is empty")
		}
		if _, ok := seen[entry.Name]; ok {
			t.Fatalf("duplicate manifest command %q", entry.Name)
		}
		seen[entry.Name] = struct{}{}
	}
}

func assertHasCommand(t *testing.T, entries []ManifestEntry, name string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			return
		}
	}
	t.Fatalf("expected command %s", name)
}

func assertMissingCommand(t *testing.T, entries []ManifestEntry, name string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			t.Fatalf("did not expect command %s", name)
		}
	}
}
