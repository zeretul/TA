package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ta/internal/exploration"
)

const defaultInput = "tools/exploration-check/instance-replay/samples/synthetic-archive-cave.json"

func main() {
	inputFlag := flag.String("input", defaultInput, "synthetic definition JSON; overrides are limited to the Q2 output directory")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not determine repository root")
		os.Exit(2)
	}
	inputPath, relativePath, err := resolveInput(repoRoot, *inputFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "input path rejected:", err)
		os.Exit(2)
	}
	report := newReplayReport(relativePath, []byte{})
	data, err := readBounded(inputPath, exploration.MaxDefinitionBytes)
	if err != nil {
		report.Valid = false
		report.Diagnostics = append(report.Diagnostics, exploration.Diagnostic{Code: "input_read_failed", Path: relativePath, Message: stableError(err)})
	} else {
		report = parseAndExecute(data, relativePath)
	}
	outputDir := filepath.Join(repoRoot, "work", "luna6-exploration", "q2")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "could not create Q2 output directory:", err)
		os.Exit(2)
	}
	outputs := []struct {
		name  string
		value any
	}{{"report.json", report}, {"diagnostics.json", report.Diagnostics}}
	for _, output := range outputs {
		if err := writeJSON(filepath.Join(outputDir, output.name), output.value); err != nil {
			fmt.Fprintln(os.Stderr, "could not write Q2 output:", err)
			os.Exit(2)
		}
	}
	if !report.Valid {
		fmt.Fprintf(os.Stderr, "synthetic instance replay rejected input with %d diagnostic(s); report: work/luna6-exploration/q2/report.json\n", len(report.Diagnostics))
		os.Exit(2)
	}
	fmt.Println("synthetic dungeon instance replay completed; proposal only")
}

func parseAndExecute(data []byte, relativePath string) replayReport {
	report := newReplayReport(relativePath, data)
	definition, diagnostics, err := exploration.ParseDungeonDefinition(data)
	if err != nil {
		report.Valid = false
		report.Diagnostics = append(report.Diagnostics, exploration.Diagnostic{Code: "definition_parse_failed", Path: "definition", Message: stableError(err)})
		return report
	}
	if len(diagnostics) != 0 {
		report.Valid = false
		report.Diagnostics = append(report.Diagnostics, diagnostics...)
		return report
	}
	return executeReplay(definition, relativePath, data)
}

func resolveInput(repoRoot, requested string) (string, string, error) {
	requestedPath := filepath.Clean(filepath.FromSlash(requested))
	if !filepath.IsAbs(requestedPath) {
		requestedPath = filepath.Join(repoRoot, requestedPath)
	}
	absolute, err := filepath.Abs(requestedPath)
	if err != nil {
		return "", "", err
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return "", "", err
	}
	relative = filepath.ToSlash(relative)
	if relative == defaultInput {
		return absolute, relative, nil
	}
	const q2Prefix = "work/luna6-exploration/q2/"
	if filepath.IsLocal(filepath.FromSlash(relative)) && strings.HasPrefix(relative, q2Prefix) {
		return absolute, relative, nil
	}
	return "", "", fmt.Errorf("input must be the synthetic sample or a Q2-local test file")
}

func readBounded(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, exploration.ErrInputTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, exploration.ErrInputTooLarge
	}
	return data, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
