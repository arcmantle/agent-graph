package extractor_test

import (
	"testing"

	"agent-graph/extractor"
	"agent-graph/graph"
)

func TestNewContributionAcceptsValidatedDatabaseNeutralData(t *testing.T) {
	vocabulary, err := graph.NewVocabulary(graph.VocabularyDefinition{
		NodeKinds: []graph.NodeKind{"file"},
	})
	if err != nil {
		t.Fatalf("new vocabulary: %v", err)
	}

	evidence := graph.FactEvidence{
		Span: graph.SourceSpan{
			Path:        "src/main.ts",
			StartLine:   1,
			StartColumn: 1,
			EndLine:     1,
			EndColumn:   1,
		},
		FileHash:   "sha256:fixture",
		Extractor:  "typescript",
		Provenance: "static",
		Confidence: graph.ConfidenceExtracted,
	}
	input := extractor.ContributionInput{
		SourcePath: "src/main.ts",
		Metadata: extractor.Metadata{
			Name:       "typescript",
			Version:    "v0",
			Extensions: []string{".ts"},
		},
		Facts: graph.Facts{
			Nodes: []graph.Node{{ID: "file:main", Kind: "file", Evidence: evidence}},
		},
		UnresolvedReferences: []extractor.UnresolvedReference{{
			SourceID: "file:main",
			Target:   "./other",
			Kind:     extractor.ModuleReferenceImport,
			Bindings: []extractor.ModuleBinding{{ImportedName: "helper"}},
		}},
		ExportedSurfaces: []extractor.ExportedSurface{{
			NodeID: "file:main",
			Name:   "main",
		}},
		Dependencies: []extractor.Dependency{{
			SourcePath: "src/main.ts",
			TargetPath: "src/other.ts",
		}},
		Diagnostics: []extractor.Diagnostic{{
			Severity: extractor.DiagnosticWarning,
			Message:  "fixture diagnostic",
		}},
	}

	contribution, err := extractor.NewContribution(vocabulary, input)
	if err != nil {
		t.Fatalf("new contribution: %v", err)
	}

	input.Facts.Nodes[0].ID = "changed"
	input.Metadata.Extensions[0] = ".changed"
	input.UnresolvedReferences[0].Target = "changed"
	input.UnresolvedReferences[0].Bindings[0].ImportedName = "changed"
	input.ExportedSurfaces[0].Name = "changed"
	input.Dependencies[0].TargetPath = "changed"
	input.Diagnostics[0].Message = "changed"

	if got := contribution.Facts().Nodes[0].ID; got != "file:main" {
		t.Errorf("fact node ID = %q, want %q", got, "file:main")
	}
	if got := contribution.Metadata().Extensions[0]; got != ".ts" {
		t.Errorf("metadata extension = %q, want %q", got, ".ts")
	}
	if got := contribution.SourcePath(); got != "src/main.ts" {
		t.Errorf("contribution source path = %q, want %q", got, "src/main.ts")
	}
	if got := contribution.UnresolvedReferences()[0].Target; got != "./other" {
		t.Errorf("unresolved target = %q, want %q", got, "./other")
	}
	if got := contribution.UnresolvedReferences()[0].Bindings[0].ImportedName; got != "helper" {
		t.Errorf("module binding imported name = %q, want %q", got, "helper")
	}
	if got := contribution.ExportedSurfaces()[0].Name; got != "main" {
		t.Errorf("exported surface name = %q, want %q", got, "main")
	}
	if got := contribution.Dependencies()[0].TargetPath; got != "src/other.ts" {
		t.Errorf("dependency target = %q, want %q", got, "src/other.ts")
	}
	if got := contribution.Diagnostics()[0].Message; got != "fixture diagnostic" {
		t.Errorf("diagnostic message = %q, want %q", got, "fixture diagnostic")
	}
}

func TestNewGraphUpdateGroupsImmutableContributions(t *testing.T) {
	vocabulary, err := graph.NewVocabulary(graph.VocabularyDefinition{NodeKinds: []graph.NodeKind{"file"}})
	if err != nil {
		t.Fatalf("new vocabulary: %v", err)
	}
	contribution, err := extractor.NewContribution(vocabulary, extractor.ContributionInput{
		SourcePath: "src/main.ts",
		Metadata:   extractor.Metadata{Name: "typescript", Version: "v0", Extensions: []string{".ts"}},
		Facts: graph.Facts{Nodes: []graph.Node{{
			ID:   "file:main",
			Kind: "file",
			Evidence: graph.FactEvidence{
				Span:       graph.SourceSpan{Path: "src/main.ts", StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 1},
				FileHash:   "sha256:fixture",
				Extractor:  "typescript",
				Provenance: "static",
				Confidence: graph.ConfidenceExtracted,
			},
		}}},
	})
	if err != nil {
		t.Fatalf("new contribution: %v", err)
	}

	update, err := extractor.NewGraphUpdate([]extractor.Contribution{contribution})
	if err != nil {
		t.Fatalf("new graph update: %v", err)
	}

	contributions := update.Contributions()
	contributions[0] = extractor.Contribution{}
	if got := len(update.Contributions()); got != 1 {
		t.Errorf("contribution count = %d, want 1", got)
	}
}

func TestNewContributionRejectsMalformedData(t *testing.T) {
	vocabulary, err := graph.NewVocabulary(graph.VocabularyDefinition{NodeKinds: []graph.NodeKind{"file"}})
	if err != nil {
		t.Fatalf("new vocabulary: %v", err)
	}

	testCases := []struct {
		name   string
		mutate func(*extractor.ContributionInput)
	}{
		{
			name: "empty source path",
			mutate: func(input *extractor.ContributionInput) {
				input.SourcePath = ""
			},
		},
		{
			name: "invalid metadata",
			mutate: func(input *extractor.ContributionInput) {
				input.Metadata.Name = ""
			},
		},
		{
			name: "unregistered fact kind",
			mutate: func(input *extractor.ContributionInput) {
				input.Facts.Nodes[0].Kind = "unknown"
			},
		},
		{
			name: "unresolved reference without local source",
			mutate: func(input *extractor.ContributionInput) {
				input.UnresolvedReferences = []extractor.UnresolvedReference{{SourceID: "missing", Target: "./other"}}
			},
		},
		{
			name: "exported surface without local node",
			mutate: func(input *extractor.ContributionInput) {
				input.ExportedSurfaces = []extractor.ExportedSurface{{NodeID: "missing", Name: "main"}}
			},
		},
		{
			name: "incomplete dependency",
			mutate: func(input *extractor.ContributionInput) {
				input.Dependencies = []extractor.Dependency{{SourcePath: "src/main.ts"}}
			},
		},
		{
			name: "unsupported diagnostic severity",
			mutate: func(input *extractor.ContributionInput) {
				input.Diagnostics = []extractor.Diagnostic{{Severity: "unknown", Message: "fixture diagnostic"}}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			input := validContributionInput()
			testCase.mutate(&input)
			if _, err := extractor.NewContribution(vocabulary, input); err == nil {
				t.Fatal("new contribution returned nil error")
			}
		})
	}
}

func TestNewGraphUpdateRejectsNoContributions(t *testing.T) {
	if _, err := extractor.NewGraphUpdate(nil); err == nil {
		t.Fatal("new graph update returned nil error")
	}
}

func TestResolverFileViewRequiresProjectRootAndCopiesFiles(t *testing.T) {
	if _, err := extractor.NewResolverFileView("", nil); err == nil {
		t.Fatal("new resolver file view returned nil error for an empty project root")
	}

	files := map[string][]byte{"tsconfig.json": []byte("original")}
	view, err := extractor.NewResolverFileView(".", files)
	if err != nil {
		t.Fatalf("new resolver file view: %v", err)
	}
	files["tsconfig.json"][0] = 'c'

	contents, found := view.File("tsconfig.json")
	if !found || string(contents) != "original" {
		t.Fatalf("view contents = %q, want %q", contents, "original")
	}
	contents[0] = 'c'

	contents, found = view.File("tsconfig.json")
	if !found || string(contents) != "original" {
		t.Errorf("view contents after caller mutation = %q, want %q", contents, "original")
	}
}

func validContributionInput() extractor.ContributionInput {
	evidence := graph.FactEvidence{
		Span:       graph.SourceSpan{Path: "src/main.ts", StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 1},
		FileHash:   "sha256:fixture",
		Extractor:  "typescript",
		Provenance: "static",
		Confidence: graph.ConfidenceExtracted,
	}
	return extractor.ContributionInput{
		SourcePath: "src/main.ts",
		Metadata:   extractor.Metadata{Name: "typescript", Version: "v0", Extensions: []string{".ts"}},
		Facts:      graph.Facts{Nodes: []graph.Node{{ID: "file:main", Kind: "file", Evidence: evidence}}},
	}
}
