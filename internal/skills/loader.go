package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

var headingPattern = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)

// LocalLoader scans one root directory and loads local skills.
type LocalLoader struct {
	root string
}

// NewLocalLoader creates a loader for one local skills root.
func NewLocalLoader(root string) *LocalLoader {
	return &LocalLoader{root: strings.TrimSpace(root)}
}

// Load scans local skill directories and returns parsed skills + non-fatal issues.
func (l *LocalLoader) Load(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	root := strings.TrimSpace(l.root)
	if root == "" {
		return Snapshot{}, fmt.Errorf("%w: empty root", ErrSkillRootNotFound)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("skills: resolve root %q: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, fmt.Errorf("%w: %s", ErrSkillRootNotFound, absRoot)
		}
		return Snapshot{}, fmt.Errorf("skills: stat root %q: %w", absRoot, err)
	}
	if !info.IsDir() {
		return Snapshot{}, fmt.Errorf("skills: root %q is not directory", absRoot)
	}

	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("skills: read root %q: %w", absRoot, err)
	}

	candidates := make([]string, 0, len(entries)+1)
	rootSkillFile := filepath.Join(absRoot, skillFileName)
	if _, err := os.Stat(rootSkillFile); err == nil {
		candidates = append(candidates, absRoot)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		candidates = append(candidates, filepath.Join(absRoot, name))
	}
	sort.Strings(candidates)

	snapshot := Snapshot{
		Skills: make([]Skill, 0, len(candidates)),
		Issues: make([]LoadIssue, 0, len(candidates)),
	}
	for _, skillDir := range candidates {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}

		skillPath := filepath.Join(skillDir, skillFileName)
		data, readErr := os.ReadFile(skillPath)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				snapshot.Issues = append(snapshot.Issues, LoadIssue{
					Code:    IssueSkillFileMissing,
					Path:    skillPath,
					Message: "missing SKILL.md",
					Err:     readErr,
				})
				continue
			}
			snapshot.Issues = append(snapshot.Issues, LoadIssue{
				Code:    IssueReadFailed,
				Path:    skillPath,
				Message: "read skill file failed",
				Err:     readErr,
			})
			continue
		}

		skill, parseIssues, parseErr := parseLocalSkill(absRoot, skillDir, skillPath, string(data))
		if len(parseIssues) > 0 {
			snapshot.Issues = append(snapshot.Issues, parseIssues...)
		}
		if parseErr != nil {
			continue
		}
		snapshot.Skills = append(snapshot.Skills, skill)
	}

	return snapshot, nil
}

type skillFrontMatter struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Version     string   `yaml:"version"`
	Source      string   `yaml:"source"`
	Scope       string   `yaml:"scope"`
	ToolHints   []string `yaml:"tool_hints"`
}

func parseLocalSkill(root, skillDir, skillPath, raw string) (Skill, []LoadIssue, error) {
	metaText, body, hasMeta, splitErr := splitFrontMatter(raw)
	if splitErr != nil {
		issue := LoadIssue{
			Code:    IssueInvalidMetadata,
			Path:    skillPath,
			Message: "invalid frontmatter",
			Err:     splitErr,
		}
		return Skill{}, []LoadIssue{issue}, splitErr
	}

	meta := skillFrontMatter{}
	if hasMeta {
		if err := yaml.Unmarshal([]byte(metaText), &meta); err != nil {
			issue := LoadIssue{
				Code:    IssueInvalidMetadata,
				Path:    skillPath,
				Message: "frontmatter parse failed",
				Err:     err,
			}
			return Skill{}, []LoadIssue{issue}, err
		}
	}

	scope := ScopeExplicit
	if strings.TrimSpace(meta.Scope) != "" {
		scope = ActivationScope(strings.TrimSpace(meta.Scope))
		if err := scope.Validate(); err != nil {
			issue := LoadIssue{
				Code:    IssueInvalidMetadata,
				Path:    skillPath,
				Message: "invalid scope",
				Err:     err,
			}
			return Skill{}, []LoadIssue{issue}, err
		}
	}

	if src := strings.TrimSpace(meta.Source); src != "" {
		sourceKind := SourceKind(src)
		if err := sourceKind.Validate(); err != nil {
			issue := LoadIssue{
				Code:    IssueInvalidMetadata,
				Path:    skillPath,
				Message: "invalid source kind",
				Err:     err,
			}
			return Skill{}, []LoadIssue{issue}, err
		}
		if sourceKind != SourceKindLocal {
			issue := LoadIssue{
				Code:    IssueUnsupportedSource,
				Path:    skillPath,
				Message: "local loader only supports source=local",
			}
			return Skill{}, []LoadIssue{issue}, errors.New(issue.Message)
		}
	}

	sections := parseSections(body)
	instruction := strings.TrimSpace(sections["instruction"])
	if instruction == "" {
		instruction = strings.TrimSpace(body)
	}
	content := Content{
		Instruction: instruction,
		References:  parseReferences(sections["references"]),
		Examples:    parseListItems(sections["examples"]),
		ToolHints:   mergeToolHints(parseListItems(sections["toolhints"]), meta.ToolHints),
	}
	if err := content.Validate(); err != nil {
		issue := LoadIssue{
			Code:    IssueEmptyContent,
			Path:    skillPath,
			Message: "skill content is empty",
			Err:     err,
		}
		return Skill{}, []LoadIssue{issue}, err
	}

	id := strings.TrimSpace(meta.ID)
	if id == "" {
		id = deriveSkillID(skillDir)
	}
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = firstHeading(body)
	}
	if strings.TrimSpace(name) == "" {
		name = id
	}
	version := strings.TrimSpace(meta.Version)
	if version == "" {
		version = "v1"
	}

	descriptor := Descriptor{
		ID:          id,
		Name:        name,
		Description: strings.TrimSpace(meta.Description),
		Version:     version,
		Source: Source{
			Kind:     SourceKindLocal,
			RootDir:  root,
			SkillDir: skillDir,
			FilePath: skillPath,
		},
		Scope: scope,
	}
	if err := descriptor.Validate(); err != nil {
		issue := LoadIssue{
			Code:    IssueInvalidMetadata,
			Path:    skillPath,
			SkillID: id,
			Message: "descriptor validation failed",
			Err:     err,
		}
		return Skill{}, []LoadIssue{issue}, err
	}

	return Skill{
		Descriptor: descriptor,
		Content:    content,
	}, nil, nil
}

func splitFrontMatter(raw string) (meta string, body string, has bool, err error) {
	trimmed := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(trimmed, "---\n") {
		return "", trimmed, false, nil
	}

	rest := strings.TrimPrefix(trimmed, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return "", "", false, errors.New("frontmatter end marker not found")
	}

	meta = strings.TrimSpace(rest[:end])
	body = strings.TrimSpace(rest[end+len("\n---\n"):])
	return meta, body, true, nil
}

func deriveSkillID(skillDir string) string {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(skillDir)))
	base = strings.ReplaceAll(base, "_", "-")
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.Trim(base, "-")
	if base == "" {
		return "skill"
	}
	return base
}

func firstHeading(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for _, line := range lines {
		m := headingPattern.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		return strings.TrimSpace(strings.Trim(m[1], "#"))
	}
	return ""
}

func parseSections(body string) map[string]string {
	sections := map[string]string{
		"instruction": "",
		"references":  "",
		"examples":    "",
		"toolhints":   "",
	}
	current := "instruction"
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")

	appendLine := func(key string, line string) {
		if _, ok := sections[key]; !ok {
			return
		}
		if sections[key] == "" {
			sections[key] = line
			return
		}
		sections[key] += "\n" + line
	}

	for _, line := range lines {
		heading := normalizedHeading(line)
		if heading != "" {
			switch heading {
			case "instruction":
				current = "instruction"
				continue
			case "references", "reference":
				current = "references"
				continue
			case "examples", "example":
				current = "examples"
				continue
			case "toolhints", "tool-hints", "tool_hints":
				current = "toolhints"
				continue
			default:
				current = "instruction"
			}
		}
		appendLine(current, line)
	}

	for key := range sections {
		sections[key] = strings.TrimSpace(sections[key])
	}
	return sections
}

func normalizedHeading(line string) string {
	m := headingPattern.FindStringSubmatch(line)
	if len(m) != 2 {
		return ""
	}
	heading := strings.ToLower(strings.TrimSpace(m[1]))
	heading = strings.ReplaceAll(heading, " ", "")
	return heading
}

var markdownLinkPattern = regexp.MustCompile(`\[(?P<title>[^\]]+)\]\((?P<path>[^)]+)\)`)

func parseReferences(text string) []Reference {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	refs := make([]Reference, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimLeft(line, "-*"))
		if trimmed == "" {
			continue
		}

		m := markdownLinkPattern.FindStringSubmatch(trimmed)
		if len(m) == 3 {
			refs = append(refs, Reference{
				Title: strings.TrimSpace(m[1]),
				Path:  strings.TrimSpace(m[2]),
			})
			continue
		}
		refs = append(refs, Reference{
			Path: trimmed,
		})
	}
	return refs
}

func parseListItems(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimLeft(line, "-*"))
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}
	return items
}

func mergeToolHints(sectionHints []string, metadataHints []string) []string {
	out := make([]string, 0, len(sectionHints)+len(metadataHints))
	seen := make(map[string]struct{}, len(sectionHints)+len(metadataHints))
	appendHint := func(raw string) {
		h := strings.TrimSpace(raw)
		if h == "" {
			return
		}
		key := strings.ToLower(h)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, h)
	}
	for _, hint := range sectionHints {
		appendHint(hint)
	}
	for _, hint := range metadataHints {
		appendHint(hint)
	}
	return out
}
