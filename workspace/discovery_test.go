package workspace_test

import (
	"reflect"
	"testing"

	"agent-graph/testkit"
	"agent-graph/workspace"
)

func TestDiscoverFindsNestedManifestProjectsAndAssignsSourcesToMostSpecificRoot(t *testing.T) {
	fixture := testkit.NewWorkspace(t, map[string]string{
		"package.json":                     `{"name":"root"}`,
		"src/root.ts":                      "export const root = 1;\n",
		"packages/app/package.json":        `{"name":"app"}`,
		"packages/app/src/main.ts":         "export const main = 1;\n",
		"packages/app/src/ignored.txt":     "not source\n",
		"packages/app/nested/secondary.ts": "export const secondary = 1;\n",
		"packages/utility/src/utility.js":  "export const utility = 1;\n",
		"packages/utility/README.md":       "not source\n",
	})

	first, err := workspace.Discover(fixture.Root, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatalf("discover workspace: %v", err)
	}
	second, err := workspace.Discover(fixture.Root, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatalf("discover workspace again: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("discover result is not deterministic\nfirst: %#v\nsecond: %#v", first, second)
	}

	want := workspace.Discovery{
		Projects: []workspace.Project{
			{ID: "project:.", Root: "."},
			{ID: "project:packages/app", Root: "packages/app"},
		},
		Sources: []workspace.Source{
			{Path: "packages/app/nested/secondary.ts", ProjectID: "project:packages/app"},
			{Path: "packages/app/src/main.ts", ProjectID: "project:packages/app"},
			{Path: "packages/utility/src/utility.js", ProjectID: "project:."},
			{Path: "src/root.ts", ProjectID: "project:."},
		},
	}
	if !reflect.DeepEqual(first, want) {
		t.Errorf("discovery = %#v, want %#v", first, want)
	}
}

func TestDiscoverAddsConfiguredRootsAndPrefersThemOverAncestorProjects(t *testing.T) {
	fixture := testkit.NewWorkspace(t, map[string]string{
		"package.json":                    `{"name":"root"}`,
		"src/root.ts":                     "export const root = 1;\n",
		"packages/utility/src/utility.js": "export const utility = 1;\n",
	})

	discovery, err := workspace.Discover(fixture.Root, workspace.DiscoverOptions{
		ConfiguredRoots: []string{"packages/utility"},
	})
	if err != nil {
		t.Fatalf("discover workspace: %v", err)
	}

	want := workspace.Discovery{
		Projects: []workspace.Project{
			{ID: "project:.", Root: "."},
			{ID: "project:packages/utility", Root: "packages/utility"},
		},
		Sources: []workspace.Source{
			{Path: "packages/utility/src/utility.js", ProjectID: "project:packages/utility"},
			{Path: "src/root.ts", ProjectID: "project:."},
		},
	}
	if !reflect.DeepEqual(discovery, want) {
		t.Errorf("discovery = %#v, want %#v", discovery, want)
	}
}

func TestDiscoverUsesTypeScriptAndJavaScriptManifestsWithOneIdentityPerRoot(t *testing.T) {
	fixture := testkit.NewWorkspace(t, map[string]string{
		"packages/app/package.json":    `{"name":"app"}`,
		"packages/app/tsconfig.json":   `{}`,
		"packages/app/src/main.ts":     "export const main = 1;\n",
		"packages/types/tsconfig.json": `{}`,
		"packages/types/src/types.ts":  "export type Item = string;\n",
		"packages/web/jsconfig.json":   `{}`,
		"packages/web/src/client.jsx":  "export const Client = () => null;\n",
	})

	discovery, err := workspace.Discover(fixture.Root, workspace.DiscoverOptions{
		ConfiguredRoots: []string{"packages/app"},
	})
	if err != nil {
		t.Fatalf("discover workspace: %v", err)
	}

	want := workspace.Discovery{
		Projects: []workspace.Project{
			{ID: "project:packages/app", Root: "packages/app"},
			{ID: "project:packages/types", Root: "packages/types"},
			{ID: "project:packages/web", Root: "packages/web"},
		},
		Sources: []workspace.Source{
			{Path: "packages/app/src/main.ts", ProjectID: "project:packages/app"},
			{Path: "packages/types/src/types.ts", ProjectID: "project:packages/types"},
			{Path: "packages/web/src/client.jsx", ProjectID: "project:packages/web"},
		},
	}
	if !reflect.DeepEqual(discovery, want) {
		t.Errorf("discovery = %#v, want %#v", discovery, want)
	}
}

func TestDiscoverAppliesRootAgraphignorePatterns(t *testing.T) {
	fixture := testkit.NewWorkspace(t, map[string]string{
		"package.json":                  `{"name":"root"}`,
		".agraphignore":                 "# generated files\ngenerated/\n*.test.ts\n/root-only.ts\n!generated/keep.ts\n",
		"root-only.ts":                  "export const rootOnly = 1;\n",
		"src/main.ts":                   "export const main = 1;\n",
		"src/main.test.ts":              "export const test = 1;\n",
		"generated/build.ts":            "export const build = 1;\n",
		"generated/keep.ts":             "export const keep = 1;\n",
		"nested/.agraphignore":          "*.ts\n",
		"nested/still-included.ts":      "export const nested = 1;\n",
		"node_modules/package/index.js": "module.exports = {};\n",
		".agent-graph/cache/index.ts":   "export const cache = 1;\n",
		".git/hooks/ignored.ts":         "export const hook = 1;\n",
	})

	discovery, err := workspace.Discover(fixture.Root, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatalf("discover workspace: %v", err)
	}

	want := workspace.Discovery{
		Projects: []workspace.Project{{ID: "project:.", Root: "."}},
		Sources: []workspace.Source{
			{Path: "generated/keep.ts", ProjectID: "project:."},
			{Path: "nested/still-included.ts", ProjectID: "project:."},
			{Path: "src/main.ts", ProjectID: "project:."},
		},
	}
	if !reflect.DeepEqual(discovery, want) {
		t.Errorf("discovery = %#v, want %#v", discovery, want)
	}
}

func TestDiscoverAppliesRootRelativeDirectoryPatterns(t *testing.T) {
	fixture := testkit.NewWorkspace(t, map[string]string{
		"package.json":                       `{"name":"root"}`,
		".agraphignore":                      "/generated/\n*.generated.ts\n!src/keep.generated.ts\n",
		"generated/root.ts":                  "export const generated = 1;\n",
		"packages/app/generated/retained.ts": "export const retained = 1;\n",
		"src/drop.generated.ts":              "export const drop = 1;\n",
		"src/keep.generated.ts":              "export const keep = 1;\n",
		"src/main.ts":                        "export const main = 1;\n",
	})

	discovery, err := workspace.Discover(fixture.Root, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatalf("discover workspace: %v", err)
	}

	want := []workspace.Source{
		{Path: "packages/app/generated/retained.ts", ProjectID: "project:."},
		{Path: "src/keep.generated.ts", ProjectID: "project:."},
		{Path: "src/main.ts", ProjectID: "project:."},
	}
	if !reflect.DeepEqual(discovery.Sources, want) {
		t.Errorf("sources = %#v, want %#v", discovery.Sources, want)
	}
}
