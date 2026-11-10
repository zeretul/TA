package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathUsesLegacyTargetWhenCurrentOnlyHasEmptyGeneratedLore(t *testing.T) {
	workspace := t.TempDir()
	currentLore := filepath.Join(workspace, DataDirName, "lore", "items.json")
	legacyLore := filepath.Join(workspace, LegacyDataDirName, "lore", "items.json")
	writeFile(t, currentLore, `{"version":1,"items":[]}`)
	writeFile(t, legacyLore, `{"version":1,"items":[{"id":"hero"}]}`)

	if got := Path(workspace, "lore", "items.json"); got != legacyLore {
		t.Fatalf("Path should keep using legacy lore with data: want=%s got=%s", legacyLore, got)
	}
	if got := Rel(workspace, "lore", "items.json"); got != LegacyDataDirName+"/lore/items.json" {
		t.Fatalf("Rel should match the selected legacy lore path: %s", got)
	}
}

func TestPathKeepsCurrentTargetWhenBothTargetsHaveData(t *testing.T) {
	workspace := t.TempDir()
	currentLore := filepath.Join(workspace, DataDirName, "lore", "items.json")
	legacyLore := filepath.Join(workspace, LegacyDataDirName, "lore", "items.json")
	writeFile(t, currentLore, `{"version":1,"items":[{"id":"current"}]}`)
	writeFile(t, legacyLore, `{"version":1,"items":[{"id":"legacy"}]}`)

	if got := Path(workspace, "lore", "items.json"); got != currentLore {
		t.Fatalf("Path should keep current target when it has data: want=%s got=%s", currentLore, got)
	}
}

func TestDirNameFallsBackToLegacyWhenCurrentOnlyHasEphemeralData(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, DataDirName, "runs", "run.jsonl"), `{"type":"run_created"}`)
	writeFile(t, filepath.Join(workspace, LegacyDataDirName, "lore", "items.json"), `{"version":1,"items":[{"id":"hero"}]}`)

	if got := DirName(workspace); got != LegacyDataDirName {
		t.Fatalf("DirName should keep legacy active when current only has ephemeral data: %s", got)
	}
}

func TestDirNameUsesCurrentNameForFreshWorkspace(t *testing.T) {
	workspace := t.TempDir()

	if got := DirName(workspace); got != DataDirName {
		t.Fatalf("fresh workspace should use %s: got=%s", DataDirName, got)
	}
	if got := Dir(workspace); got != filepath.Join(workspace, DataDirName) {
		t.Fatalf("Dir should point at the current data directory: %s", got)
	}
}

func TestDirNameKeepsPreviousGenerationWhenCurrentIsAbsent(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, LegacyDataDirName, "lore", "items.json"), `{"version":1,"items":[{"id":"hero"}]}`)

	if got := DirName(workspace); got != LegacyDataDirName {
		t.Fatalf("workspace without the current directory must keep reading its existing state: got=%s", got)
	}
}

func TestDirNamePrefersNewestGenerationHoldingData(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, DataDirName, "lore", "items.json"), `{"version":1,"items":[{"id":"current"}]}`)
	writeFile(t, filepath.Join(workspace, LegacyDataDirName, "lore", "items.json"), `{"version":1,"items":[{"id":"legacy"}]}`)
	writeFile(t, filepath.Join(workspace, OldestDataDirName, "lore", "items.json"), `{"version":1,"items":[{"id":"oldest"}]}`)

	if got := DirName(workspace); got != DataDirName {
		t.Fatalf("newest generation with data should win: got=%s", got)
	}
}

func TestDataDirRelsCoversEveryGeneration(t *testing.T) {
	rels := DataDirRels("runs")

	if len(rels) != len(DataDirNames) {
		t.Fatalf("DataDirRels should cover every generation: %#v", rels)
	}
	for i, name := range DataDirNames {
		if want := name + "/runs"; rels[i] != want {
			t.Fatalf("DataDirRels[%d] = %q, want %q", i, rels[i], want)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
