package skills

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Metadata represents the nanobot metadata in the skill frontmatter.
type Metadata struct {
	Description string `yaml:"description"`
	Nanobot     struct {
		Always   bool     `yaml:"always"`
		Requires struct {
			Bins []string `yaml:"bins"`
			Env  []string `yaml:"env"`
		} `yaml:"requires"`
	} `yaml:"nanobot"`
}

// Skill represents a loaded skill.
type Skill struct {
	Name        string
	Path        string
	Source      string // "workspace" or "builtin"
	Description string
	Available   bool
	Missing     []string
	Content     string
	Always      bool
}

// Loader manages skill loading.
type Loader struct {
	Workspace string
	SkillsDir string
}

// NewLoader creates a new skills loader.
func NewLoader(workspace string) *Loader {
	return &Loader{
		Workspace: workspace,
		SkillsDir: filepath.Join(workspace, "skills"),
	}
}

// ListSkills lists all available skills.
func (l *Loader) ListSkills() ([]Skill, error) {
	var skills []Skill
	seen := make(map[string]bool)

	// Scan workspace skills
	if err := l.scanDir(l.SkillsDir, "workspace", &skills, seen); err != nil {
		// It's okay if workspace skills dir doesn't exist yet
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	// In a real implementation, we might have a separate built-in skills directory.
	// For now, we assume skills are copied to workspace or managed there.
	// If we were distributing this as a binary, built-in skills might be embedded.

	return skills, nil
}

func (l *Loader) scanDir(dir, source string, skills *[]Skill, seen map[string]bool) error {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if seen[name] {
			continue
		}

		skillPath := filepath.Join(dir, name, "SKILL.md")
		if _, err := os.Stat(skillPath); err == nil {
			skill, err := l.loadSkill(name, skillPath, source)
			if err == nil {
				*skills = append(*skills, skill)
				seen[name] = true
			}
		}
	}
	return nil
}

func (l *Loader) loadSkill(name, path, source string) (Skill, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}

	meta, _ := parseFrontmatter(content)
	
	missing := checkRequirements(meta.Nanobot.Requires.Bins, meta.Nanobot.Requires.Env)
	available := len(missing) == 0

	desc := meta.Description
	if desc == "" {
		desc = name
	}

	return Skill{
		Name:        name,
		Path:        path,
		Source:      source,
		Description: desc,
		Available:   available,
		Missing:     missing,
		Content:     string(content),
		Always:      meta.Nanobot.Always,
	}, nil
}

// LoadSkillsForContext loads skills content for the context.
func (l *Loader) LoadSkillsForContext(names []string) string {
	var parts []string
	for _, name := range names {
		path := filepath.Join(l.SkillsDir, name, "SKILL.md")
		skillDir := filepath.Join(l.SkillsDir, name)
		if content, err := ioutil.ReadFile(path); err == nil {
			cleanContent := stripFrontmatter(content)
			
			// Replace {baseDir} with actual path
			absDir, _ := filepath.Abs(skillDir)
			cleanContent = strings.ReplaceAll(cleanContent, "{baseDir}", absDir)
			
			parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", name, cleanContent))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSkillsSummary builds the summary for progressive loading.
func (l *Loader) BuildSkillsSummary() string {
	skills, err := l.ListSkills()
	if err != nil {
		return ""
	}

	var sb strings.Builder

	for _, s := range skills {
		status := "Available"
		if !s.Available {
			status = fmt.Sprintf("Unavailable (Missing: %s)", strings.Join(s.Missing, ", "))
		}
		
		sb.WriteString(fmt.Sprintf("- **%s** (%s)\n", s.Name, status))
		sb.WriteString(fmt.Sprintf("  Description: %s\n", s.Description))
		sb.WriteString(fmt.Sprintf("  Instruction File: %s\n", s.Path))
		sb.WriteString("\n")
	}
	
	return sb.String()
}

// GetAlwaysSkills returns names of skills that should always be loaded.
func (l *Loader) GetAlwaysSkills() []string {
	skills, _ := l.ListSkills()
	var names []string
	for _, s := range skills {
		if s.Always && s.Available {
			names = append(names, s.Name)
		}
	}
	return names
}

// Helper functions

func parseFrontmatter(content []byte) (Metadata, error) {
	var meta Metadata
	s := string(content)
	if strings.HasPrefix(s, "---") {
		parts := strings.SplitN(s, "---", 3)
		if len(parts) >= 3 {
			err := yaml.Unmarshal([]byte(parts[1]), &meta)
			return meta, err
		}
	}
	return meta, nil
}

func stripFrontmatter(content []byte) string {
	s := string(content)
	if strings.HasPrefix(s, "---") {
		parts := strings.SplitN(s, "---", 3)
		if len(parts) >= 3 {
			return strings.TrimSpace(parts[2])
		}
	}
	return s
}

func checkRequirements(bins, envs []string) []string {
	var missing []string
	for _, bin := range bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, fmt.Sprintf("CLI: %s", bin))
		}
	}
	for _, env := range envs {
		if os.Getenv(env) == "" {
			missing = append(missing, fmt.Sprintf("ENV: %s", env))
		}
	}
	return missing
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
