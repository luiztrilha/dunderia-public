package team

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStarterKitPoliciesParsesValidatedRules(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	rules, err := loadStarterKitPolicies(repoRoot)
	if err != nil {
		t.Fatalf("loadStarterKitPolicies: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("expected starter-kit policies")
	}
	want := "DunderIA Office Topology: Keep `game-master` manual-only and owner-invoked; do not route work to it automatically."
	if !containsExactString(rules, want) {
		t.Fatalf("expected policy %q in %+v", want, rules)
	}
}

func TestLoadStarterKitSkillsBuildsVisibleCatalog(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	skills, err := loadStarterKitSkills(repoRoot)
	if err != nil {
		t.Fatalf("loadStarterKitSkills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("expected starter-kit skills")
	}
	assertSkill := func(name, title, source string) {
		t.Helper()
		for _, skill := range skills {
			if skill.Name != name {
				continue
			}
			if skill.Title != title {
				t.Fatalf("skill %s title = %q, want %q", name, skill.Title, title)
			}
			if skill.Source != source {
				t.Fatalf("skill %s source = %q, want %q", name, skill.Source, source)
			}
			if !strings.Contains(skill.Content, "templates/starter-kit/") {
				t.Fatalf("skill %s content should point to starter-kit file, got %q", name, skill.Content)
			}
			return
		}
		t.Fatalf("expected skill %q in catalog", name)
	}

	assertSkill("codex.code-review-findings", "code-review-findings", "codex")
	assertSkill("superpowers.using-superpowers", "using-superpowers", "superpowers")
	assertSkill("agents.frontend-design", "frontend-design", "agents")
}

func containsExactString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
