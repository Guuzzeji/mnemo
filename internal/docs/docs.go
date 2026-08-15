package docs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Doc struct {
	ID     string   `yaml:"id"`
	Tags   []string `yaml:"tags"`
	Status string   `yaml:"status"`
	Path   string
	Body   string
}

type frontmatter struct {
	ID     string   `yaml:"id"`
	Tags   []string `yaml:"tags"`
	Status string   `yaml:"status"`
}

func Parse(content []byte) (*Doc, error) {
	fm, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(fm.ID) == "" {
		return nil, errors.New("frontmatter: id must be non-empty")
	}
	if !validID(fm.ID) {
		return nil, fmt.Errorf("frontmatter: invalid id %q (lowercase alphanumeric and dashes only)", fm.ID)
	}
	if len(fm.Tags) == 0 {
		return nil, errors.New("frontmatter: tags must be non-empty")
	}
	if fm.Status != "active" && fm.Status != "deprecated" {
		return nil, fmt.Errorf("frontmatter: status must be 'active' or 'deprecated', got %q", fm.Status)
	}
	if !strings.Contains(body, "## Summary") {
		return nil, errors.New("body: missing required section '## Summary'")
	}
	if !strings.Contains(body, "## Key Decisions") {
		return nil, errors.New("body: missing required section '## Key Decisions'")
	}

	return &Doc{
		ID:     fm.ID,
		Tags:   fm.Tags,
		Status: fm.Status,
		Body:   body,
	}, nil
}

func splitFrontmatter(content []byte) (frontmatter, string, error) {
	var fm frontmatter

	delim := []byte("---\n")
	parts := bytes.SplitN(content, delim, 3)
	if len(parts) < 3 {
		return fm, "", errors.New("invalid frontmatter: expected --- delimiters")
	}

	if err := yaml.Unmarshal(parts[1], &fm); err != nil {
		return fm, "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	body := string(parts[2])
	return fm, body, nil
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func validID(id string) bool { return idRe.MatchString(id) }

func ValidateContent(content []byte, allowedCategories []string) error {
	doc, err := Parse(content)
	if err != nil {
		return err
	}

	allowed := make(map[string]struct{}, len(allowedCategories))
	for _, c := range allowedCategories {
		allowed[strings.ToLower(c)] = struct{}{}
	}

	for _, tag := range doc.Tags {
		if _, ok := allowed[strings.ToLower(tag)]; !ok {
			return fmt.Errorf("tag %q is not in allowed categories %v", tag, allowedCategories)
		}
	}
	return nil
}

type Store struct {
	docsDir           string
	allowedCategories []string
}

func New(docsDir string, allowedCategories []string) (*Store, error) {
	info, err := os.Stat(docsDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("docs directory does not exist: %s", docsDir)
	}
	return &Store{
		docsDir:           docsDir,
		allowedCategories: allowedCategories,
	}, nil
}

func (s *Store) Create(doc Doc, content []byte) (string, error) {
	parsed, err := Parse(content)
	if err != nil {
		return "", err
	}
	if parsed.ID != doc.ID {
		return "", fmt.Errorf("Doc.ID %q does not match frontmatter id %q", doc.ID, parsed.ID)
	}
	if err := ValidateContent(content, s.allowedCategories); err != nil {
		return "", err
	}
	if err := s.checkUnique(doc.ID); err != nil {
		return "", err
	}

	path := filepath.Join(s.docsDir, doc.ID+".md")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func (s *Store) checkUnique(id string) error {
	existing, err := s.listIDs()
	if err != nil {
		return err
	}
	if _, dup := existing[id]; dup {
		return fmt.Errorf("duplicate document id: %q already exists in %s", id, s.docsDir)
	}
	return nil
}

func (s *Store) listIDs() (map[string]struct{}, error) {
	ids := make(map[string]struct{})
	entries, err := os.ReadDir(s.docsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(s.docsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fm, _, err := splitFrontmatter(data)
		if err != nil {
			continue
		}
		if fm.ID != "" {
			ids[fm.ID] = struct{}{}
		}
	}
	return ids, nil
}

func (s *Store) AppendSection(id, heading, content string) error {
	if !validID(id) {
		return fmt.Errorf("invalid doc id %q", id)
	}
	path := filepath.Join(s.docsDir, id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("doc %q not found: %w", id, err)
	}

	section := fmt.Sprintf("\n## %s\n%s", heading, content)
	updated := append(data, []byte(section)...)

	if err := ValidateContent(updated, s.allowedCategories); err != nil {
		return fmt.Errorf("append would produce invalid doc: %w", err)
	}

	return os.WriteFile(path, updated, 0o644)
}

func (s *Store) SetStatus(id, status string) error {
	if !validID(id) {
		return fmt.Errorf("invalid doc id %q", id)
	}
	if status != "active" && status != "deprecated" {
		return fmt.Errorf("status must be 'active' or 'deprecated', got %q", status)
	}
	path := filepath.Join(s.docsDir, id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("doc %q not found: %w", id, err)
	}
	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return err
	}
	fm.Status = status
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return err
	}
	updated := append(append([]byte("---\n"), fmBytes...), []byte("---\n"+body)...)
	return os.WriteFile(path, updated, 0o644)
}

func (s *Store) Get(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("invalid doc id %q", id)
	}
	path := filepath.Join(s.docsDir, id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("doc %q not found: %w", id, err)
	}
	return string(data), nil
}

func (s *Store) List() ([]Doc, error) {
	entries, err := os.ReadDir(s.docsDir)
	if err != nil {
		return nil, err
	}

	var docs []Doc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(s.docsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, err := Parse(data)
		if err != nil {
			continue
		}
		parsed.Path = path
		docs = append(docs, *parsed)
	}
	return docs, nil
}

func (s *Store) Exists(id string) bool {
	path := filepath.Join(s.docsDir, id+".md")
	_, err := os.Stat(path)
	return err == nil
}
