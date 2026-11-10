package interactive

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// These tests hold the read-side floor that makes a later private declaration
// reach a story that already started.
//
// A story freezes its schema at creation, and that is what keeps a rules edit
// from rewriting a save. The cost is that a boundary declared afterwards never
// reaches a running story: the frozen schema is the only thing the projection
// reads, so an opponent's purse is withheld in a story that begins tomorrow and
// shown in one that began last week. T22 names the gap directly — 旧局冻结字段
// 也要验证，不能只改新预设 — so the preset's current declaration is applied on
// the read, without the save being rewritten.

const testPresenceLocationField = "出场与位置"

// laterPrivateXiuxianFields are the rows a story created before T22 froze as
// open. Each one is a fact the design withholds from a character the player is
// not playing, so a save that began earlier must stop showing them too.
var laterPrivateXiuxianFields = []string{
	xiuxianFieldBurden, xiuxianFieldBurdenFloor, xiuxianFieldRealmProgress,
	xiuxianFieldImplement, xiuxianFieldCurrency, xiuxianFieldArrangement,
	xiuxianFieldTrainingWindow,
	// 功法与专精 is the prose half of the method sheet. It is withheld for the
	// same reason 已学功法 is, and because its default is the literal "未定" —
	// a value nobody observed. Both templates carry it, so a reader opening an
	// NPC's sheet must not read a method out of it.
	xiuxianFieldMethod,
}

// xiuxianSchemaWithoutLaterMarks is the schema an older story froze: the
// private marks that existed then, and nothing for the fields declared after.
func xiuxianSchemaWithoutLaterMarks(t *testing.T) StoryDirectorActorStateSystem {
	t.Helper()
	module, ok := builtinActorStateModuleByID(ActorStateXiuxianID)
	if !ok {
		t.Fatalf("the xiuxian preset must be built in")
	}
	system := module.ActorState
	later := map[string]bool{}
	for _, name := range laterPrivateXiuxianFields {
		later[name] = true
	}
	for templateIndex := range system.Templates {
		template := &system.Templates[templateIndex]
		for fieldIndex := range template.Fields {
			field := &template.Fields[fieldIndex]
			if later[actorStateFieldID(*field)] {
				field.Visibility = ActorStateVisibilityOpen
			}
		}
	}
	return system
}

func xiuxianStoryWithFrozenSchema(t *testing.T, title string, system StoryDirectorActorStateSystem, actorStateID string) (*Store, string) {
	t.Helper()
	refs := DefaultStoryDirectorModuleRefs()
	refs.ActorStateID = actorStateID
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:      title,
		ModuleRefs: &refs,
		ActorState: &system,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

func schemaFieldVisibility(t *testing.T, schema *ActorStateSchemaSnapshot, templateID, fieldID string) string {
	t.Helper()
	if schema == nil {
		t.Fatalf("the read must carry a schema")
	}
	template := actorStateTemplateByID(schema.System, templateID)
	if template.ID == "" {
		t.Fatalf("the schema must carry template %q", templateID)
	}
	field, ok := actorStateFieldByID(template, fieldID)
	if !ok {
		t.Fatalf("template %q must carry field %q", templateID, fieldID)
	}
	return field.Visibility
}

func TestSnapshotWithholdsFieldsDeclaredPrivateAfterTheStoryStarted(t *testing.T) {
	system := xiuxianSchemaWithoutLaterMarks(t)
	store, storyID := xiuxianStoryWithFrozenSchema(t, "旧局", system, ActorStateXiuxianID)

	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range laterPrivateXiuxianFields {
		for _, templateID := range []string{ActorStateImportantCharacterTemplateID, ActorStateOpponentTemplateID} {
			if got := schemaFieldVisibility(t, snapshot.ActorStateSchema, templateID, field); got != ActorStateVisibilityPrivate {
				t.Fatalf("a story that started before %q was private must still withhold it from %q, got %q", field, templateID, got)
			}
		}
	}
	// The floor only ever adds a mark. A row the preset leaves open stays open,
	// so the boundary a reader gets is the preset's, not a blanket hide.
	if got := schemaFieldVisibility(t, snapshot.ActorStateSchema, ActorStateImportantCharacterTemplateID, testPresenceLocationField); got != ActorStateVisibilityOpen {
		t.Fatalf("a field the preset leaves open must stay readable, got %q", got)
	}
}

func TestSnapshotKeepsTheFrozenMarksThePresetNoLongerDeclares(t *testing.T) {
	// A story played under a stricter declaration keeps it: reopening the story
	// is not this read's business, and a field the reader never saw cannot
	// suddenly appear because the pack moved on.
	system := xiuxianSchemaWithoutLaterMarks(t)
	for templateIndex := range system.Templates {
		template := &system.Templates[templateIndex]
		if template.ID != ActorStateImportantCharacterTemplateID {
			continue
		}
		for fieldIndex := range template.Fields {
			if actorStateFieldID(template.Fields[fieldIndex]) == testPresenceLocationField {
				template.Fields[fieldIndex].Visibility = ActorStateVisibilityPrivate
			}
		}
	}
	store, storyID := xiuxianStoryWithFrozenSchema(t, "旧局更严", system, ActorStateXiuxianID)
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaFieldVisibility(t, snapshot.ActorStateSchema, ActorStateImportantCharacterTemplateID, testPresenceLocationField); got != ActorStateVisibilityPrivate {
		t.Fatalf("a mark the story froze must survive the floor, got %q", got)
	}
}

func TestSnapshotLeavesACustomPresetAlone(t *testing.T) {
	// A story on its own preset has no built-in declaration to borrow, and the
	// floor must not invent one for it.
	system := xiuxianSchemaWithoutLaterMarks(t)
	store, storyID := xiuxianStoryWithFrozenSchema(t, "自定义", system, "my-own-state")
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaFieldVisibility(t, snapshot.ActorStateSchema, ActorStateOpponentTemplateID, xiuxianFieldCurrency); got != ActorStateVisibilityOpen {
		t.Fatalf("a custom preset must not inherit a built-in boundary, got %q", got)
	}
}

func TestSnapshotDoesNotRewriteTheFrozenSchemaOnDisk(t *testing.T) {
	// The whole point of doing this on the read is that the save is untouched:
	// the journal keeps what it had, and the rules keep settling on the full
	// record. A story created from an explicit schema carries it in its own
	// journal rather than in the legacy side file, so that is what is compared.
	system := xiuxianSchemaWithoutLaterMarks(t)
	store, storyID := xiuxianStoryWithFrozenSchema(t, "不落盘", system, ActorStateXiuxianID)
	before, err := os.ReadFile(store.storyPath(storyID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Snapshot(storyID, "main"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(store.storyPath(storyID))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("reading the snapshot must not write the floor back to the journal")
	}
	var frozen *ActorStateSchemaSnapshot
	for _, line := range strings.Split(string(after), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "actor_state_schema") {
			continue
		}
		var record struct {
			ActorStateSchema *ActorStateSchemaSnapshot `json:"actor_state_schema"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil || record.ActorStateSchema == nil {
			continue
		}
		frozen = record.ActorStateSchema
		break
	}
	if frozen == nil {
		t.Fatalf("the journal must carry the schema it froze")
	}
	if got := schemaFieldVisibility(t, frozen, ActorStateOpponentTemplateID, xiuxianFieldCurrency); got != ActorStateVisibilityOpen {
		t.Fatalf("the journal must keep the schema it froze, got %q", got)
	}
}
