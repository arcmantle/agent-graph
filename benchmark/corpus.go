package benchmark

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type CorpusSpec struct {
	SourceFiles           int
	FunctionsPerFile      int
	AdditionalFunctions   int
	UncalledFunctions     int
	ExtraSideEffectImport bool
}

var DefaultCorpusSpec = CorpusSpec{
	SourceFiles:           10000,
	FunctionsPerFile:      98,
	AdditionalFunctions:   9999,
	UncalledFunctions:     9996,
	ExtraSideEffectImport: true,
}

func ExactScaleCorpusSpec(sourceFiles int) (CorpusSpec, error) {
	if sourceFiles < 4 {
		return CorpusSpec{}, fmt.Errorf("exact scale corpus requires at least 4 source files")
	}
	return CorpusSpec{
		SourceFiles:           sourceFiles,
		FunctionsPerFile:      98,
		AdditionalFunctions:   sourceFiles - 1,
		UncalledFunctions:     sourceFiles - 4,
		ExtraSideEffectImport: true,
	}, nil
}

type Corpus struct {
	SourceFiles      int
	FunctionsPerFile int
	FunctionCount    int
	ExpectedNodes    int
	ExpectedEdges    int
	UpdatePath       string
	QueryTerm        string
	PathSource       string
	PathTarget       string
	ExplainTerm      string
}

func GenerateCorpus(root string, specification CorpusSpec) (Corpus, error) {
	return GenerateCorpusWithProgress(root, specification, nil)
}

func GenerateCorpusWithProgress(root string, specification CorpusSpec, progress func(created, total int)) (Corpus, error) {
	if root == "" || specification.SourceFiles <= 0 || specification.FunctionsPerFile <= 0 || specification.AdditionalFunctions < 0 || specification.UncalledFunctions < 0 || specification.UncalledFunctions > specification.FunctionsPerFile+specification.AdditionalFunctions-1 {
		return Corpus{}, fmt.Errorf("generate benchmark corpus: root, positive source file count, positive function count, and valid additional function settings are required")
	}

	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"agent-graph-benchmark","type":"module"}`), 0o644); err != nil {
		return Corpus{}, fmt.Errorf("generate benchmark corpus: write package manifest: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		return Corpus{}, fmt.Errorf("generate benchmark corpus: create source directory: %w", err)
	}

	width := len(strconv.Itoa(specification.SourceFiles - 1))
	for sourceIndex := 0; sourceIndex < specification.SourceFiles; sourceIndex++ {
		path := modulePath(sourceIndex, width)
		contents := moduleContents(sourceIndex, specification, width)
		if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0o644); err != nil {
			return Corpus{}, fmt.Errorf("generate benchmark corpus: write source %q: %w", path, err)
		}
		if progress != nil {
			progress(sourceIndex+1, specification.SourceFiles)
		}
	}

	firstModule := moduleName(0, width)
	secondModule := moduleName(min(1, specification.SourceFiles-1), width)
	entry := functionName(0, 0, width)
	target := functionName(min(1, specification.SourceFiles-1), 0, width)
	functionCount := specification.SourceFiles*specification.FunctionsPerFile + specification.AdditionalFunctions
	expectedEdges := 2*functionCount - specification.UncalledFunctions + 3*specification.SourceFiles - 3
	if specification.ExtraSideEffectImport && specification.SourceFiles >= 3 {
		expectedEdges++
	}
	return Corpus{
		SourceFiles:      specification.SourceFiles,
		FunctionsPerFile: specification.FunctionsPerFile,
		FunctionCount:    functionCount,
		ExpectedNodes:    functionCount + specification.SourceFiles + 1,
		ExpectedEdges:    expectedEdges,
		UpdatePath:       modulePath(0, width),
		QueryTerm:        entry,
		PathSource:       "src/" + firstModule + ".ts::" + entry,
		PathTarget:       "src/" + secondModule + ".ts::" + target,
		ExplainTerm:      entry,
	}, nil
}

func moduleContents(sourceIndex int, specification CorpusSpec, width int) string {
	var contents strings.Builder
	if sourceIndex+1 < specification.SourceFiles {
		nextFunction := functionName(sourceIndex+1, 0, width)
		fmt.Fprintf(&contents, "import { %s } from './%s';\n", nextFunction, moduleName(sourceIndex+1, width))
	}
	if sourceIndex == 0 && specification.ExtraSideEffectImport && specification.SourceFiles >= 3 {
		fmt.Fprintf(&contents, "import './%s';\n", moduleName(2, width))
	}
	functionCount := specification.FunctionsPerFile + distributedCount(specification.AdditionalFunctions, specification.SourceFiles, sourceIndex)
	uncalledFunctions := distributedCount(specification.UncalledFunctions, specification.SourceFiles, sourceIndex)
	for functionIndex := 0; functionIndex < functionCount; functionIndex++ {
		name := functionName(sourceIndex, functionIndex, width)
		call := "0"
		if functionIndex >= functionCount-uncalledFunctions {
			call = "0"
		} else if functionIndex > 0 {
			call = functionName(sourceIndex, functionIndex-1, width) + "()"
		} else if sourceIndex+1 < specification.SourceFiles {
			call = functionName(sourceIndex+1, 0, width) + "()"
		}
		fmt.Fprintf(&contents, "export function %s(): number { return %s; }\n", name, call)
	}
	return contents.String()
}

func distributedCount(total, buckets, bucket int) int {
	count := total / buckets
	if bucket < total%buckets {
		count++
	}
	return count
}

func modulePath(sourceIndex, width int) string {
	return filepath.Join("src", moduleName(sourceIndex, width)+".ts")
}

func moduleName(sourceIndex, width int) string {
	return "module-" + fmt.Sprintf("%0*d", width, sourceIndex)
}

func functionName(sourceIndex, functionIndex, width int) string {
	return fmt.Sprintf("function_%0*d_%03d", width, sourceIndex, functionIndex)
}
