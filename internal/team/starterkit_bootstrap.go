package team

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type seededSkillSpec struct {
	Name        string
	Title       string
	Description string
	Source      string
	Content     string
}

func (l *Launcher) seedStarterKitBootstrap() {
	if l == nil || l.broker == nil {
		return
	}
	repoRoot := resolveRepoRoot(l.cwd)

	rules, err := loadStarterKitPolicies(repoRoot)
	if err != nil {
		log.Printf("starter-kit: load policies: %v", err)
	} else {
		l.broker.SeedPolicies("human_directed", rules)
	}

	skills, err := loadStarterKitSkills(repoRoot)
	if err != nil {
		log.Printf("starter-kit: load skills: %v", err)
	} else {
		l.broker.SeedSkills(skills)
	}
}

func starterKitRoot(repoRoot string) string {
	return filepath.Join(repoRoot, "templates", "starter-kit")
}

func loadStarterKitPolicies(repoRoot string) ([]string, error) {
	path := filepath.Join(starterKitRoot(repoRoot), "policies.validated.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	rules := make([]string, 0, 16)
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "## "):
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		case strings.HasPrefix(line, "- "):
			rule := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if rule == "" {
				continue
			}
			if section != "" {
				rule = section + ": " + rule
			}
			key := strings.ToLower(rule)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			rules = append(rules, rule)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

func loadStarterKitSkills(repoRoot string) ([]seededSkillSpec, error) {
	type skillRoot struct {
		dir    string
		source string
		prefix string
	}

	roots := []skillRoot{
		{dir: filepath.Join(starterKitRoot(repoRoot), "codex", "skills"), source: "codex", prefix: "codex"},
		{dir: filepath.Join(starterKitRoot(repoRoot), "codex", "superpowers", "skills"), source: "superpowers", prefix: "superpowers"},
		{dir: filepath.Join(starterKitRoot(repoRoot), "agents", "skills"), source: "agents", prefix: "agents"},
	}

	var out []seededSkillSpec
	seen := make(map[string]struct{})
	for _, root := range roots {
		if !dirExists(root.dir) {
			continue
		}
		err := filepath.WalkDir(root.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
				return nil
			}

			relDir, err := filepath.Rel(root.dir, filepath.Dir(path))
			if err != nil {
				return err
			}
			relDir = filepath.ToSlash(relDir)
			title, description := parseSkillFrontMatter(path)

			baseTitle := title
			if strings.TrimSpace(baseTitle) == "" {
				baseTitle = filepath.Base(filepath.Dir(path))
			}
			name := root.prefix + "." + strings.ReplaceAll(relDir, "/", "-")
			if _, ok := seen[name]; ok {
				return nil
			}
			seen[name] = struct{}{}

			displaySource := root.source
			if strings.Contains(relDir, "/") {
				displaySource = root.source + "/" + strings.Split(relDir, "/")[0]
			}

			out = append(out, seededSkillSpec{
				Name:        name,
				Title:       strings.TrimSpace(baseTitle),
				Description: strings.TrimSpace(description),
				Source:      displaySource,
				Content: fmt.Sprintf(
					"Starter-kit skill metadata.\n\nSource file: %s\nUse the packaged SKILL.md for full instructions.",
					filepath.ToSlash(filepath.Join("templates", "starter-kit", relativeStarterKitPath(path))),
				),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func relativeStarterKitPath(fullPath string) string {
	fullPath = filepath.ToSlash(fullPath)
	idx := strings.Index(fullPath, "/templates/starter-kit/")
	if idx >= 0 {
		return fullPath[idx+len("/templates/starter-kit/"):]
	}
	if strings.HasPrefix(fullPath, "templates/starter-kit/") {
		return strings.TrimPrefix(fullPath, "templates/starter-kit/")
	}
	return filepath.Base(fullPath)
}

func parseSkillFrontMatter(path string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return "", ""
	}

	values := map[string]string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		values[key] = value
	}
	return values["name"], values["description"]
}

func (b *Broker) SeedPolicies(source string, rules []string) {
	if b == nil || len(rules) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	changed := false
	for _, rule := range rules {
		if _, alreadyActive, err := b.recordPolicyLocked(source, rule); err == nil && !alreadyActive {
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := b.saveLocked(); err != nil {
		log.Printf("broker: saveLocked after seeding policies: %v", err)
	}
}

func (b *Broker) SeedSkills(specs []seededSkillSpec) {
	if b == nil || len(specs) == 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	b.mu.Lock()
	defer b.mu.Unlock()

	changed := false
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" || b.findSkillByNameLocked(name) != nil {
			continue
		}
		title := strings.TrimSpace(spec.Title)
		if title == "" {
			title = name
		}
		b.counter++
		b.skills = append(b.skills, teamSkill{
			ID:          fmt.Sprintf("skill-%s", skillSlug(name)),
			Name:        name,
			Title:       title,
			Description: strings.TrimSpace(spec.Description),
			Source:      strings.TrimSpace(spec.Source),
			Content:     strings.TrimSpace(spec.Content),
			CreatedBy:   "system",
			Channel:     globalSkillChannel,
			Status:      "active",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		changed = true
	}
	if !changed {
		return
	}
	if err := b.saveLocked(); err != nil {
		log.Printf("broker: saveLocked after seeding starter-kit skills: %v", err)
	}
}
