package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/agents/prompts"
)

// The configuration Skill tells the model to open a resource's field contract
// with read({"path":"skill://<skill>/references/<file>.md"}). A purpose-scoped
// assistant is exactly the surface that needs that instruction: its resource
// descriptors name a reference, and the reference is the only place the write
// shape is written down.
//
// The skill:// route is registered by skillassembly and authorized by the same
// AgentToolSkills capability that authorizes the skill tool, so a scope that
// keeps config_read/config_apply but drops that capability leaves read unable
// to resolve a skill:// path. The model then walks the filesystem looking for
// the reference and stops at the repository root on a write approval.
func TestScopedAssemblyResolvesSkillReferencePaths(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "builtin-skills")
	writeScopedReferenceSkill(t, skillsDir, "configuration", "world-book", "# World book\n\nField contract.\n")

	cfg := &config.Config{
		Workspace: filepath.Join(root, "workspace"),
		DenovaDir: filepath.Join(root, "data"),
		SkillsDir: skillsDir,
	}
	if err := os.MkdirAll(cfg.Workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	host := scopedHost("world-a")
	assembly, err := BuildExternalConversationAssembly(
		context.Background(), cfg, nil, prompts.IDEStoryTeller{}, config.AgentKindIDE, host,
	)
	if err != nil {
		t.Fatal(err)
	}

	readTool := scopedToolNamed(t, assembly, "read")
	result, err := readTool.Tool.Run(context.Background(), `{"path":"skill://configuration/references/world-book.md"}`)
	if err != nil {
		t.Fatalf("read could not resolve a skill:// reference: %v", err)
	}
	if !strings.Contains(result.ModelContent, "Field contract.") {
		t.Fatalf("read did not return the reference body:\n%s", result.ModelContent)
	}
}

// A scope still must not gain the Skill catalog or the skill tool: the purpose
// is narrower than the editor, and the reference route is the only Skill
// surface it keeps.
func TestScopedAssemblyKeepsNoSkillCatalogButKeepsTheReferenceRoute(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "builtin-skills")
	writeScopedReferenceSkill(t, skillsDir, "configuration", "world-book", "# World book\n")

	cfg := &config.Config{
		Workspace: filepath.Join(root, "workspace"),
		DenovaDir: filepath.Join(root, "data"),
		SkillsDir: skillsDir,
	}
	if err := os.MkdirAll(cfg.Workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	assembly, err := BuildExternalConversationAssembly(
		context.Background(), cfg, nil, prompts.IDEStoryTeller{}, config.AgentKindIDE, scopedHost("world-a"),
	)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]struct{}, len(assembly.Tools))
	for _, definition := range assembly.Tools {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = struct{}{}
	}
	if _, ok := names["skill"]; ok {
		t.Error("a scoped assembly registered the skill tool")
	}
	if _, ok := names["read"]; !ok {
		t.Error("a scoped assembly lost the read tool")
	}
}

func writeScopedReferenceSkill(t *testing.T, skillsDir, name, reference, body string) {
	t.Helper()
	dir := filepath.Join(skillsDir, name, "references")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: " + name + "\ndescription: scoped reference test\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(skillsDir, name, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, reference+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scopedToolNamed(t *testing.T, assembly ExternalAssembly, name string) ToolDefinition {
	t.Helper()
	for _, definition := range assembly.Tools {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == name {
			return definition
		}
	}
	t.Fatalf("assembly has no %q tool", name)
	return ToolDefinition{}
}
