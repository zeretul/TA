package storypack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/presetlayout"
	"ta/internal/style"
)

// writePackFile writes one raw pack document into a data root's pack directory
// and returns the data root. Tests write the file themselves rather than
// marshalling a Pack so the on-disk shape stays the documented shape: a test
// that round-trips its own struct would not catch a field the reader expects
// under a different JSON name than the format documents.
func writePackFile(t *testing.T, dataRoot, fileName, document string) {
	t.Helper()
	dir := presetlayout.StoryPacks(dataRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
}

// externalPackDocument is a minimal but complete pack: the smallest thing that
// can actually start a story, using the original default modules.
//
// It deliberately still carries the legacy "narrative_style_id" key. Packs in
// the wild were written when a pack could bind a global style, and the format
// must keep loading them; the key is simply no longer read.
func externalPackDocument(id, name string) string {
	return `{
  "version": 1,
  "id": "` + id + `",
  "name": "` + name + `",
  "story": {
    "title": "` + name + `",
    "origin": "外置包的开场。",
    "protagonist_name": "测试者",
    "module_refs": {
      "narrative_style_id": "rhythm",
      "event_package_ids": ["default"],
      "rule_system_id": "default",
      "actor_state_id": "default",
      "image_preset_id": "realistic"
    }
  },
  "world": [
    {
      "id": "` + id + `-place",
      "name": "外置地点",
      "type": "location",
      "load_mode": "resident",
      "content": "只属于这个外置包的地点。"
    }
  ]
}`
}

// TestExternalPackLoadsFromTheDataRoot is the acceptance operation for the pack
// directory: a pack the user drops into the data root is listed, readable in
// full, and startable, with no build step and no change to the binary.
func TestExternalPackLoadsFromTheDataRoot(t *testing.T) {
	dataRoot := t.TempDir()
	writePackFile(t, dataRoot, "my-world.json", externalPackDocument("my-world", "我的世界"))

	library := NewLibrary(dataRoot)

	summaries, err := library.List()
	if err != nil {
		t.Fatalf("listing packs: %v", err)
	}
	var found *Summary
	for i := range summaries {
		if summaries[i].ID == "my-world" {
			found = &summaries[i]
		}
	}
	if found == nil {
		t.Fatalf("the external pack is missing from the catalog: %+v", summaries)
	}
	if found.Invalid {
		t.Fatalf("the external pack was listed as invalid: %s", found.Error)
	}
	if found.WorldCount != 1 {
		t.Fatalf("external pack reported %d world entries, want 1", found.WorldCount)
	}
	// A user pack sorts ahead of the builtins, so the user's own worlds are what
	// they see first.
	if summaries[0].ID != "my-world" {
		t.Fatalf("user packs must sort first, got %s", summaries[0].ID)
	}

	// The same ID must be readable in full, otherwise the pack is visible and
	// unselectable.
	pack, err := library.Get("my-world")
	if err != nil {
		t.Fatalf("reading the external pack: %v", err)
	}
	if pack.Name != "我的世界" {
		t.Fatalf("external pack name = %q", pack.Name)
	}
	if pack.Builtin {
		t.Fatal("a pack read from the data root was marked builtin")
	}

	// And it must produce a real story request bound to its own world.
	request := pack.StoryRequest()
	if request.PackBinding == nil || request.PackBinding.Scope != pack.WorldScope() {
		t.Fatalf("external pack produced no usable binding: %+v", request.PackBinding)
	}
	if request.PackBinding.Scope == interactive.PackScopeFromID("my-world") {
		t.Fatalf("binding scope %q ignored the pack content revision", request.PackBinding.Scope)
	}
	// The legacy style key is still in the document above. It must load and be
	// ignored: a pack that named "rhythm" back when packs could bind a style must
	// not now override anything through the module graph, and must not fail to
	// load either. The request's style comes from the settings block instead.
	if request.ModuleRefs.NarrativeStyleID != "" {
		t.Fatalf("the legacy narrative_style_id key still reached the module graph: %q", request.ModuleRefs.NarrativeStyleID)
	}
	if request.StoryTellerID != style.DefaultID {
		t.Fatalf("a settings-less pack should start on the default style, got %q", request.StoryTellerID)
	}
}

// TestExternalPackOverridesABuiltinOfTheSameID pins the precedence rule: a file
// in the data root wins over the compiled-in pack, so a user can repair or
// replace a shipped world without rebuilding.
func TestExternalPackOverridesABuiltinOfTheSameID(t *testing.T) {
	dataRoot := t.TempDir()
	builtin := builtinPacks()[0]
	writePackFile(t, dataRoot, builtin.ID+".json", externalPackDocument(builtin.ID, "覆盖后的世界"))

	library := NewLibrary(dataRoot)
	pack, err := library.Get(builtin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Builtin {
		t.Fatal("the data-root file did not override the builtin pack")
	}
	if pack.Name != "覆盖后的世界" {
		t.Fatalf("override was not read: name = %q", pack.Name)
	}

	// The list must agree with Get, and must not show the builtin twice.
	summaries, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, summary := range summaries {
		if summary.ID == builtin.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("pack %s appears %d times in the catalog, want 1", builtin.ID, count)
	}
}

// TestPackFileNameMustMatchItsID covers the mapping between the two ways a pack
// is addressed. List finds packs by filename and Get looks them up by declared
// ID, so a mismatch produces a pack that is visible and impossible to select.
// The mismatch is rejected instead, and the broken file stays listed so the user
// can see and fix it.
func TestPackFileNameMustMatchItsID(t *testing.T) {
	dataRoot := t.TempDir()
	writePackFile(t, dataRoot, "on-disk-name.json", externalPackDocument("declared-id", "名字对不上"))

	library := NewLibrary(dataRoot)

	// Get addresses the pack by its declared ID, which is what List shows, so
	// that lookup is the one that has to work.
	if _, err := library.Get("declared-id"); err == nil {
		t.Fatal("a pack whose filename contradicts its id was readable by declared id")
	}
	if _, err := library.Get("on-disk-name"); err == nil {
		t.Fatal("a pack whose filename contradicts its id was readable by filename")
	}

	summaries, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	var broken *Summary
	for i := range summaries {
		if summaries[i].ID == "on-disk-name" {
			broken = &summaries[i]
		}
	}
	if broken == nil {
		t.Fatal("the mismatched pack vanished from the catalog instead of being reported")
	}
	if !broken.Invalid || !strings.Contains(broken.Error, "不一致") {
		t.Fatalf("the mismatch was not explained to the user: %+v", broken)
	}
}

// TestPackWithAnUninstalledModuleIsReportedNotDowngraded is the acceptance
// operation for module validation. The pack names a rule system that is not in
// the catalog, which is the realistic failure: the author ships a world whose
// mechanics depend on a module the user never installed. Loading it silently
// would start the story on the original default rule system and present a
// different game than the author wrote.
func TestPackWithAnUninstalledModuleIsReportedNotDowngraded(t *testing.T) {
	dataRoot := t.TempDir()
	document := strings.Replace(
		externalPackDocument("needs-a-module", "缺模块的世界"),
		`"rule_system_id": "default"`,
		`"rule_system_id": "dm-not-installed"`,
		1,
	)
	writePackFile(t, dataRoot, "needs-a-module.json", document)

	library := NewLibrary(dataRoot)

	if _, err := library.Get("needs-a-module"); err == nil {
		t.Fatal("a pack requiring a missing module loaded successfully")
	} else if !strings.Contains(err.Error(), "dm-not-installed") {
		t.Fatalf("the error does not name the missing module: %v", err)
	}

	summaries, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	var broken *Summary
	for i := range summaries {
		if summaries[i].ID == "needs-a-module" {
			broken = &summaries[i]
		}
	}
	if broken == nil || !broken.Invalid {
		t.Fatalf("a pack with a missing module was offered as playable: %+v", broken)
	}
	if !strings.Contains(broken.Error, "dm-not-installed") {
		t.Fatalf("the list does not name the missing module: %s", broken.Error)
	}

	// Installing the module must be the only thing needed to make it playable,
	// so the check reads the live catalog rather than a cached verdict.
	writeModuleFile(t, presetlayout.RuleSystems(dataRoot), "dm-not-installed.json")
	pack, err := NewLibrary(dataRoot).Get("needs-a-module")
	if err != nil {
		t.Fatalf("the pack stayed unplayable after its module was installed: %v", err)
	}
	if pack.Story.ModuleRefs.RuleSystemID != "dm-not-installed" {
		t.Fatalf("the pack lost its module reference: %+v", pack.Story.ModuleRefs)
	}
}

// TestBuiltinPacksResolveAgainstAnEmptyDataRoot is the shipped-build check: on a
// machine where the user has installed nothing, builtin packs must still
// load. They bind only modules this build ships, so if this fails the delivered
// version would show its own packs as broken.
func TestBuiltinPacksResolveAgainstAnEmptyDataRoot(t *testing.T) {
	dataRoot := t.TempDir()
	library := NewLibrary(dataRoot)

	for _, pack := range builtinPacks() {
		if err := pack.ValidateModules(dataRoot); err != nil {
			t.Fatalf("builtin pack %s does not resolve: %v", pack.ID, err)
		}
	}
	summaries, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	if want := len(builtinPacks()) - 1; len(summaries) != want {
		t.Fatalf("catalog holds %d packs, want %d visible builtins", len(summaries), want)
	}
	for _, summary := range summaries {
		if summary.ID == HistoricalSanguoPackID {
			t.Fatal("the archived historical pack appeared in the adventure catalog")
		}
		if summary.Invalid {
			t.Fatalf("builtin pack %s is offered as invalid: %s", summary.ID, summary.Error)
		}
	}
	if _, err := library.GetContent(HistoricalSanguoPackID); err != nil {
		t.Fatalf("the historical pack must still resolve for old saves: %v", err)
	}
}

// writeModuleFile writes a director module document that the libraries will
// accept, so a test can install a module the way a user would.
func writeModuleFile(t *testing.T, dir, fileName string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	document := map[string]any{
		"version": 1,
		"id":      strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		"name":    "测试检定",
		"trpg_system": map[string]any{
			"rule_templates": []any{},
		},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
