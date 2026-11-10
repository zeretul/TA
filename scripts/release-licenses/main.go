// Command release-licenses collects the license files of every third-party
// component shipped inside a Text Adventure release package.
//
// The release archive contains a compiled binary and a built frontend, so the
// dependency license files that normally sit next to the sources are not
// present in it. Apache-2.0, MIT, BSD and OFL all require the notices to
// travel with a binary distribution, which is why this runs as part of
// packaging rather than as an afterthought.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type component struct {
	Name    string
	Version string
	License string
	Files   []licenseFile
}

// licenseFile is either a file copied from the dependency directory or, when a
// package ships no license file at all, a generated note recording what its
// manifest declares. Body is set only in the second case.
type licenseFile struct {
	Name string
	Path string
	Body []byte
}

type moduleEntry struct {
	Path    string
	Version string
	Dir     string
}

// ThirdPartyNotices.txt (monaco-editor) and NOTICE.md (@pierre/trees) carry
// notices for code derived from other projects; they must be forwarded
// alongside the package's own license, not instead of it.
var licenseFilePattern = regexp.MustCompile(`(?i)^(LICEN[CS]E|COPYING|NOTICE|UNLICEN[CS]E|THIRD.?PARTY)`)

var maxLicenseSize = 2 * 1024 * 1024

func main() {
	modulesFile := flag.String("modules", "", "file of go list output, one path|version|dir per line")
	webRoot := flag.String("web", "", "frontend root containing package.json and node_modules")
	repoRoot := flag.String("repo", "", "repository root; modules inside it are first-party and skipped")
	destination := flag.String("destination", "", "release package root to write licenses/third-party into")
	version := flag.String("version", "", "release version recorded in the index")
	flag.Parse()

	if *destination == "" {
		fail("missing -destination")
	}
	var components []component
	if *modulesFile != "" {
		got, err := collectGo(*modulesFile, *repoRoot)
		if err != nil {
			fail(err.Error())
		}
		components = append(components, got...)
	}
	if *webRoot != "" {
		got, err := collectNPM(*webRoot)
		if err != nil {
			fail(err.Error())
		}
		components = append(components, got...)
	}
	if len(components) == 0 {
		fail("no third-party components found")
	}
	if err := write(*destination, *version, components); err != nil {
		fail(err.Error())
	}
	fmt.Printf("collected %d third-party components\n", len(components))
}

func fail(message string) {
	fmt.Fprintf(os.Stderr, "release-licenses: %s\n", message)
	os.Exit(1)
}

func collectGo(listPath, repoRoot string) ([]component, error) {
	file, err := os.Open(listPath)
	if err != nil {
		return nil, fmt.Errorf("open go module list: %w", err)
	}
	defer file.Close()

	var entries []moduleEntry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected go module line: %q", line)
		}
		entries = append(entries, moduleEntry{Path: parts[0], Version: parts[1], Dir: parts[2]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read go module list: %w", err)
	}

	var components []component
	for _, entry := range entries {
		if insideDir(entry.Dir, repoRoot) {
			continue
		}
		files, err := licenseFiles(entry.Dir)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", entry.Path, entry.Version, err)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("%s %s: no license file in module cache", entry.Path, entry.Version)
		}
		components = append(components, component{
			Name:    entry.Path,
			Version: entry.Version,
			License: classify(files),
			Files:   files,
		})
	}
	return components, nil
}

// insideDir reports whether path sits under root, so first-party modules that
// the root LICENSE already covers are not listed twice.
func insideDir(path, root string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

var (
	apacheText    = regexp.MustCompile(`(?i)apache license[ ,]*\s*(?:version\s+)?2\.0`)
	apacheShort   = regexp.MustCompile(`(?i)licensed under the apache license, version 2\.0`)
	mitText       = regexp.MustCompile(`(?i)permission is hereby granted, free of charge`)
	bsdText       = regexp.MustCompile(`(?i)redistribution and use in source and binary forms`)
	bsdEndorse    = regexp.MustCompile(`(?i)neither the name`)
	iscText       = regexp.MustCompile(`(?i)permission to use, copy, modify`)
	mplText       = regexp.MustCompile(`(?i)mozilla public license`)
	unlicenseText = regexp.MustCompile(`(?i)this is free and unencumbered software`)
	oflText       = regexp.MustCompile(`(?i)sil open font license`)
	ccZeroText    = regexp.MustCompile(`(?i)creative commons.*cc0`)
)

func licenseFiles(dir string) ([]licenseFile, error) {
	if dir == "" {
		return nil, fmt.Errorf("no module directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read module directory: %w", err)
	}
	var files []licenseFile
	for _, entry := range entries {
		if entry.IsDir() || !licenseFilePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", entry.Name(), err)
		}
		if info.Size() > int64(maxLicenseSize) {
			return nil, fmt.Errorf("%s exceeds %d bytes", entry.Name(), maxLicenseSize)
		}
		files = append(files, licenseFile{Name: entry.Name(), Path: filepath.Join(dir, entry.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

// declaredLicenseNote stands in for a package that declares its license in
// package.json but ships no license file — @ai-sdk/provider-utils@5.0.5 is one.
// Recording the declaration verbatim is all the package itself provides; the
// standard text of every identifier used here is reproduced in the release's
// own LICENSE (Apache-2.0) and under notices/apache-2.0/.
func declaredLicenseNote(name, version, declared string) licenseFile {
	body := fmt.Sprintf(
		"%s %s\n\n"+
			"This package declares its license in package.json as:\n\n"+
			"    %s\n\n"+
			"The package ships no license file of its own, so there is no notice to\n"+
			"reproduce here. The full text of the license is part of the release's\n"+
			"root LICENSE file, which is Apache-2.0.\n",
		name, version, declared)
	return licenseFile{Name: "DECLARED-LICENSE.txt", Body: []byte(body)}
}

func declaredLicense(manifest map[string]any) string {
	if value, ok := manifest["license"].(string); ok && value != "" {
		return value
	}
	if group, ok := manifest["licenses"].([]any); ok {
		var ids []string
		for _, item := range group {
			if entry, ok := item.(map[string]any); ok {
				if id, ok := entry["type"].(string); ok && id != "" {
					ids = append(ids, id)
				}
			}
		}
		if len(ids) > 0 {
			return strings.Join(ids, " OR ")
		}
	}
	return ""
}

func classify(files []licenseFile) string {
	var seen []string
	add := func(id string) {
		for _, existing := range seen {
			if existing == id {
				return
			}
		}
		seen = append(seen, id)
	}
	for _, file := range files {
		// A NOTICE or THIRD-PARTY file forwards other projects' notices and
		// often quotes their license text; classifying it would attribute those
		// licenses to this component.
		if !isPrimaryLicense(file.Name) {
			continue
		}
		var body []byte
		if file.Body != nil {
			body = file.Body
		} else if read, err := os.ReadFile(file.Path); err == nil {
			body = read
		} else {
			continue
		}
		text := string(body)
		switch {
		case apacheText.MatchString(text), apacheShort.MatchString(text):
			add("Apache-2.0")
		// OFL text ends with an MIT-style grant for the font software, so it has
		// to be tested before the MIT pattern.
		case oflText.MatchString(text):
			add("OFL-1.1")
		case unlicenseText.MatchString(text):
			add("Unlicense")
		case mplText.MatchString(text):
			add("MPL-2.0")
		case ccZeroText.MatchString(text):
			add("CC0-1.0")
		case mitText.MatchString(text):
			add("MIT")
		case bsdText.MatchString(text):
			if bsdEndorse.MatchString(text) {
				add("BSD-3-Clause")
			} else {
				add("BSD-2-Clause")
			}
		case iscText.MatchString(text):
			add("ISC")
		}
	}
	if len(seen) == 0 {
		return "see notice"
	}
	return strings.Join(seen, ", ")
}

func isPrimaryLicense(name string) bool {
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "NOTICE") || strings.HasPrefix(upper, "THIRD") {
		return false
	}
	return true
}

func collectNPM(webRoot string) ([]component, error) {
	modules := filepath.Join(webRoot, "node_modules")
	rootManifest, err := readManifest(filepath.Join(webRoot, "package.json"))
	if err != nil {
		return nil, err
	}
	dependencies, ok := rootManifest["dependencies"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s declares no dependencies", filepath.Join(webRoot, "package.json"))
	}

	// Resolve the runtime dependency closure the way the bundler does: follow
	// every dependency, peer and optional edge from the direct set. An unmet
	// peer or optional dependency ships nothing, so it is skipped rather than
	// failing the release; an unmet runtime dependency is a broken install.
	seen := map[string]string{}
	type pending struct {
		name    string
		root    string
		require bool
	}
	queue := make([]pending, 0, len(dependencies))
	for name := range dependencies {
		queue = append(queue, pending{name: name, root: modules, require: true})
	}
	sort.Slice(queue, func(i, j int) bool { return queue[i].name < queue[j].name })
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if _, done := seen[item.name]; done {
			continue
		}
		dir, manifest, err := locatePackage(item.root, modules, item.name)
		if err != nil {
			if !item.require {
				continue
			}
			return nil, err
		}
		seen[item.name] = dir
		// Under pnpm a package's own dependencies are linked next to it, not at
		// the top of node_modules, so its dependency root is the nearest
		// node_modules directory above it. Nested installs use
		// <package>/node_modules instead.
		roots := []string{packageModules(dir), filepath.Join(dir, "node_modules"), modules}
		for _, field := range []string{"dependencies", "peerDependencies", "optionalDependencies"} {
			group, _ := manifest[field].(map[string]any)
			for sub := range group {
				if _, done := seen[sub]; done {
					continue
				}
				for _, root := range roots {
					queue = append(queue, pending{name: sub, root: root, require: field == "dependencies"})
				}
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	var components []component
	for _, name := range names {
		dir := seen[name]
		manifest, err := readManifest(filepath.Join(dir, "package.json"))
		if err != nil {
			return nil, err
		}
		version, _ := manifest["version"].(string)
		files, err := licenseFiles(dir)
		if err != nil {
			return nil, fmt.Errorf("%s@%s: %w", name, version, err)
		}
		license := classify(files)
		if len(files) == 0 {
			declared := declaredLicense(manifest)
			if declared == "" {
				return nil, fmt.Errorf("%s@%s: ships no license file and declares no license", name, version)
			}
			files = []licenseFile{declaredLicenseNote(name, version, declared)}
			license = declared
		}
		components = append(components, component{
			Name:    name,
			Version: version,
			License: license,
			Files:   files,
		})
	}
	return components, nil
}

// packageModules returns the node_modules directory that holds a package's
// siblings: for <root>/node_modules/@scope/name it is <root>/node_modules, and
// for a nested install it is the package's own node_modules.
func packageModules(dir string) string {
	for current := dir; ; current = filepath.Dir(current) {
		if filepath.Base(current) == "node_modules" {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return dir
		}
	}
}

func locatePackage(root, modules, name string) (string, map[string]any, error) {
	direct := filepath.Join(root, filepath.FromSlash(name))
	if manifest, err := readManifest(filepath.Join(direct, "package.json")); err == nil {
		return direct, manifest, nil
	}
	if root != modules {
		return "", nil, fmt.Errorf("dependency %s is not installed under %s", name, root)
	}
	// A nested copy under another package still ships the same code.
	entries, err := os.ReadDir(modules)
	if err != nil {
		return "", nil, fmt.Errorf("read node_modules: %w", err)
	}
	for _, entry := range entries {
		nested := filepath.Join(modules, entry.Name(), "node_modules", filepath.FromSlash(name))
		if manifest, err := readManifest(filepath.Join(nested, "package.json")); err == nil {
			return nested, manifest, nil
		}
	}
	return "", nil, fmt.Errorf("dependency %s is not installed under %s", name, modules)
}

func readManifest(path string) (map[string]any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return manifest, nil
}

func write(destination, version string, components []component) error {
	root := filepath.Join(destination, "licenses", "third-party")
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("clear previous third-party licenses: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create third-party license directory: %w", err)
	}

	sort.Slice(components, func(i, j int) bool { return components[i].Name < components[j].Name })

	var index strings.Builder
	index.WriteString("# Third-party licenses\n\n")
	if version != "" {
		fmt.Fprintf(&index, "Text Adventure %s includes the components below. Their license and\n", version)
	} else {
		index.WriteString("Text Adventure includes the components below. Their license and\n")
	}
	index.WriteString("notice files are reproduced verbatim under `notices/`.\n\n")
	index.WriteString("Text Adventure itself is licensed under Apache-2.0; see the `LICENSE` file\n")
	index.WriteString("at the root of this package, which also carries the upstream copyright\n")
	index.WriteString("notice this work derives from.\n\n")
	index.WriteString("The font files served with the interface are Inter, licensed under the SIL\n")
	index.WriteString("Open Font License 1.1 (`@fontsource-variable/inter` below). The KaTeX fonts\n")
	index.WriteString("are MIT, like KaTeX itself.\n\n")
	index.WriteString("Story content — the world, characters, text, images and any video shipped\n")
	index.WriteString("with a story pack — is separate from the software licenses above. Nothing\n")
	index.WriteString("in this file grants rights to that content.\n\n")
	index.WriteString("| Component | Version | License | Notices |\n")
	index.WriteString("| --- | --- | --- | --- |\n")

	for _, item := range components {
		relative, err := noticeDir(item.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(root, relative)
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("create license directory for %s: %w", item.Name, err)
		}
		var names []string
		for _, file := range item.Files {
			body := file.Body
			if body == nil {
				read, err := os.ReadFile(file.Path)
				if err != nil {
					return fmt.Errorf("read license for %s: %w", item.Name, err)
				}
				body = read
			}
			if err := os.WriteFile(filepath.Join(target, file.Name), body, 0o644); err != nil {
				return fmt.Errorf("write license for %s: %w", item.Name, err)
			}
			names = append(names, file.Name)
		}
		fmt.Fprintf(&index, "| %s | %s | %s | %s |\n",
			escapeCell(item.Name), escapeCell(item.Version), escapeCell(item.License),
			escapeCell(filepath.ToSlash(filepath.Join(relative, names[0])))+noticeSuffix(names))
	}
	return os.WriteFile(filepath.Join(root, "README.md"), []byte(index.String()), 0o644)
}

func noticeSuffix(names []string) string {
	if len(names) <= 1 {
		return ""
	}
	return " (+" + strings.Join(names[1:], ", ") + ")"
}

// noticeDir keeps the module scope in the directory name so two packages that
// share a short name never overwrite each other's notice.
func noticeDir(name string) (string, error) {
	clean := strings.NewReplacer("/", "__", "\\", "__", ":", "_", "@", "at-").Replace(name)
	if clean == "" || strings.Contains(clean, "..") {
		return "", fmt.Errorf("refusing unsafe component name %q", name)
	}
	return filepath.Join("notices", clean), nil
}

func escapeCell(value string) string {
	if value == "" {
		return "-"
	}
	return strings.ReplaceAll(value, "|", "\\|")
}
