package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type DiscoverOptions struct {
	ConfiguredRoots []string
}

type Project struct {
	ID   string
	Root string
}

type Source struct {
	Path      string
	ProjectID string
}

type Discovery struct {
	Projects []Project
	Sources  []Source
}

type ignoreRule struct {
	pattern string
	include bool
}

func Discover(root string, options DiscoverOptions) (Discovery, error) {
	workspaceRoot, err := filepath.Abs(root)
	if err != nil {
		return Discovery{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	ignoreRules, err := loadIgnoreRules(workspaceRoot)
	if err != nil {
		return Discovery{}, err
	}

	projectRoots := make([]string, 0, len(options.ConfiguredRoots))
	for _, configuredRoot := range options.ConfiguredRoots {
		normalizedRoot, err := normalizeRoot(workspaceRoot, configuredRoot)
		if err != nil {
			return Discovery{}, err
		}
		projectRoots = append(projectRoots, normalizedRoot)
	}
	sourcePaths := make([]string, 0)
	if err := filepath.WalkDir(workspaceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(workspaceRoot, path)
		if err != nil {
			return fmt.Errorf("make path relative: %w", err)
		}
		relativePath = filepath.ToSlash(relativePath)
		if sourceExcluded(relativePath, ignoreRules) {
			return nil
		}
		if projectManifest(filepath.Base(path)) {
			projectRoots = append(projectRoots, projectRoot(relativePath))
		}
		if supportedSource(relativePath) {
			sourcePaths = append(sourcePaths, relativePath)
		}
		return nil
	}); err != nil {
		return Discovery{}, fmt.Errorf("walk workspace: %w", err)
	}

	projectRoots = uniqueSorted(projectRoots)
	sort.Strings(sourcePaths)

	projects := make([]Project, len(projectRoots))
	for index, root := range projectRoots {
		projects[index] = Project{ID: projectID(root), Root: root}
	}

	sources := make([]Source, 0, len(sourcePaths))
	for _, sourcePath := range sourcePaths {
		ownerRoot, found := mostSpecificRoot(sourcePath, projectRoots)
		if !found {
			continue
		}
		sources = append(sources, Source{Path: sourcePath, ProjectID: projectID(ownerRoot)})
	}

	return Discovery{Projects: projects, Sources: sources}, nil
}

func loadIgnoreRules(workspaceRoot string) ([]ignoreRule, error) {
	contents, err := os.ReadFile(filepath.Join(workspaceRoot, ".agraphignore"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read root .agraphignore: %w", err)
	}

	lines := strings.Split(string(contents), "\n")
	rules := make([]ignoreRule, 0, len(lines))
	for _, line := range lines {
		pattern := strings.TrimSpace(line)
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}
		include := strings.HasPrefix(pattern, "!")
		if include {
			pattern = strings.TrimPrefix(pattern, "!")
		}
		if pattern != "" {
			rules = append(rules, ignoreRule{pattern: pattern, include: include})
		}
	}
	return rules, nil
}

func sourceExcluded(sourcePath string, rules []ignoreRule) bool {
	if internalDirectory(sourcePath) {
		return true
	}

	excluded := false
	for _, rule := range rules {
		if ignorePatternMatches(sourcePath, rule.pattern) {
			excluded = !rule.include
		}
	}
	return excluded
}

func internalDirectory(sourcePath string) bool {
	for _, component := range strings.Split(sourcePath, "/") {
		switch component {
		case ".agent-graph", ".git", "node_modules":
			return true
		}
	}
	return false
}

func ignorePatternMatches(sourcePath, pattern string) bool {
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	directory := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return false
	}

	if directory {
		return directoryPatternMatches(sourcePath, pattern, anchored)
	}
	if !strings.Contains(pattern, "/") {
		matched, _ := path.Match(pattern, path.Base(sourcePath))
		return matched
	}
	if anchored {
		matched, _ := path.Match(pattern, sourcePath)
		return matched
	}
	for candidate := sourcePath; candidate != "."; candidate = path.Dir(candidate) {
		matched, _ := path.Match(pattern, candidate)
		if matched {
			return true
		}
	}
	return false
}

func directoryPatternMatches(sourcePath, pattern string, anchored bool) bool {
	components := strings.Split(sourcePath, "/")
	for index := range components[:len(components)-1] {
		candidate := strings.Join(components[:index+1], "/")
		if anchored && candidate != pattern {
			continue
		}
		if !anchored && !strings.Contains(pattern, "/") {
			matched, _ := path.Match(pattern, components[index])
			if matched {
				return true
			}
			continue
		}
		matched, _ := path.Match(pattern, candidate)
		if matched {
			return true
		}
	}
	return false
}

func projectManifest(name string) bool {
	switch name {
	case "go.mod", "package.json", "tsconfig.json", "jsconfig.json":
		return true
	default:
		return false
	}
}

func normalizeRoot(workspaceRoot, root string) (string, error) {
	candidate := root
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workspaceRoot, candidate)
	}
	relativeRoot, err := filepath.Rel(workspaceRoot, candidate)
	if err != nil {
		return "", fmt.Errorf("make configured root relative: %w", err)
	}
	if relativeRoot == ".." || strings.HasPrefix(relativeRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("configured root %q is outside workspace", root)
	}
	if relativeRoot == "." {
		return ".", nil
	}
	return filepath.ToSlash(filepath.Clean(relativeRoot)), nil
}

func uniqueSorted(values []string) []string {
	sort.Strings(values)
	if len(values) < 2 {
		return values
	}

	unique := values[:1]
	for _, value := range values[1:] {
		if value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}

func projectRoot(manifestPath string) string {
	directory := filepath.ToSlash(filepath.Dir(manifestPath))
	if directory == "." {
		return "."
	}
	return directory
}

func projectID(root string) string {
	return "project:" + root
}

func supportedSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".js", ".jsx", ".ts", ".tsx":
		return true
	default:
		return false
	}
}

func mostSpecificRoot(sourcePath string, roots []string) (string, bool) {
	bestRoot := ""
	for _, root := range roots {
		if root == "." || sourcePath == root || strings.HasPrefix(sourcePath, root+"/") {
			if len(root) > len(bestRoot) {
				bestRoot = root
			}
		}
	}
	return bestRoot, bestRoot != ""
}
