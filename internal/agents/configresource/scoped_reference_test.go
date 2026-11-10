package configresource

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The three scoped resources exist only for a conversation bound to one story or
// one world, so they are absent from the shared preset libraries and from the
// Skill's own routing table unless they are declared here. A model that can
// describe a resource but cannot find its field contract goes looking on the
// filesystem instead, and that walk ends at the repository root where a write
// approval stops the whole collection. These tests pin both halves of the route:
// the descriptor names a reference, and the reference is a file that ships.

func scopedResources() []Adapter {
	target := &ScopedTarget{StoryID: "story-a", WorldID: "world-a"}
	return []Adapter{
		newStorySettingsResource(target),
		newWorldDefaultsResource(target),
		newWorldBookResource(target),
	}
}

func TestScopedResourcesNameAReference(t *testing.T) {
	for _, adapter := range scopedResources() {
		descriptor := adapter.Descriptor()
		if strings.TrimSpace(descriptor.Reference) == "" {
			t.Fatalf("%s has no reference, so a model cannot find its field contract", descriptor.Name)
		}
	}
}

func TestScopedResourceReferencesAreFilesThatShip(t *testing.T) {
	root := repositoryRoot(t)
	for _, adapter := range scopedResources() {
		descriptor := adapter.Descriptor()
		path := filepath.Join(root, "skills", "configuration", filepath.FromSlash(descriptor.Reference))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s routes to %s, which does not exist: %v", descriptor.Name, descriptor.Reference, err)
		}
		if info.IsDir() {
			t.Fatalf("%s routes to %s, which is a directory", descriptor.Name, descriptor.Reference)
		}
	}
}

// The Skill's routing table is what the model reads before it opens a
// reference. A resource missing from it is a resource the model has to guess
// about, which is exactly the walk this fix removed.
func TestTheConfigurationSkillRoutesEveryScopedResource(t *testing.T) {
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "skills", "configuration", "SKILL.md"))
	if err != nil {
		t.Fatalf("reading the configuration Skill: %v", err)
	}
	skill := string(raw)
	for _, adapter := range scopedResources() {
		descriptor := adapter.Descriptor()
		line := "- `" + descriptor.Name + "` → `skill://configuration/" + descriptor.Reference + "`"
		if !strings.Contains(skill, line) {
			t.Fatalf("the configuration Skill does not route %s to %s", descriptor.Name, descriptor.Reference)
		}
	}
}

// repositoryRoot walks up from this test file to the module root. The reference
// files ship beside the Skill rather than in the Go tree, so the only honest
// check that they exist is against the checkout itself.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("this test cannot locate its own source file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", file)
		}
		dir = parent
	}
}
