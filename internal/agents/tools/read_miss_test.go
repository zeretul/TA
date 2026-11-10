package tools

import (
	"context"
	"strings"
	"testing"

	agent "ta/agent"

	"ta/internal/interactive"
)

// 自制-叙事演绎 §8.4 draws a line the product did not draw: 没有相关资料、NPC 本人
// 不知道、世界中渠道不可达可以自然表达；资料库故障、模型失败和提交失败是技术问题，
// 明确提示并保留恢复能力。不能编造情节补系统失败。 §13 NAR-13 restates the same
// requirement as 分开正常未命中与真实系统故障.
//
// The two were the same thing in the code. A read that matched nothing returned an
// error, the tool runtime turned that error into ToolResultError, and the SSE
// projection reported status "error" — so asking for a lore entry the story does
// not have, or a card id that was never in the pack, was indistinguishable from
// the workspace being unreadable. A player-facing notice on that signal would have
// cried wolf on every honest miss.
//
// A miss is an answer. It says the material is not there, which is something the
// narrator can work with; a failure says nothing was learned at all.

// findTool returns one definition by name from a tool set.
func findTool(t *testing.T, definitions []agent.ToolDefinition, name string) agent.ToolDefinition {
	t.Helper()
	for _, definition := range definitions {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatalf("read tool info: %v", err)
		}
		if info.Name == name {
			return definition
		}
	}
	t.Fatalf("tool %q is not registered; the read path cannot be tested", name)
	return agent.ToolDefinition{}
}

func runTool(t *testing.T, definition agent.ToolDefinition, input string) agent.ToolResult {
	t.Helper()
	result, err := definition.Tool.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("tool returned a transport error instead of a result: %v", err)
	}
	return result
}

// TestAMissedLoreReadIsNotASystemFailure is the first half. An id that is not in
// the book, and a name that matches nothing, are both ordinary answers: the model
// asked, the answer is no. Neither may carry the error bit, because the error bit
// is what the player is shown as a technical failure.
func TestAMissedLoreReadIsNotASystemFailure(t *testing.T) {
	workspace := t.TempDir()
	definitions, err := newLoreTools(workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	read := findTool(t, definitions, "read_lore_items")

	// An empty book: every lookup misses.
	for _, input := range []string{
		`{"ids":["lore_missing"]}`,
		`{"names":["不存在的资料"]}`,
	} {
		result := runTool(t, read, input)
		if result.Status != agent.ToolResultSuccess {
			t.Fatalf("a miss was reported as %q, so a technical failure is the only "+
				"reading the UI can take from it; input=%s content=%s", result.Status, input, result.ModelContent)
		}
		if !strings.Contains(result.ModelContent, "not") && !strings.Contains(result.ModelContent, "没有") {
			t.Fatalf("a miss does not say plainly that the material is absent, so the model "+
				"cannot tell it apart from an empty result: %s", result.ModelContent)
		}
	}
}

// TestAMissedLoreReadStillNamesWhatWasMissing keeps the miss useful. Telling the
// model only "nothing matched" loses which of the requested entries were absent,
// and the narrator then has no way to say what it does and does not know.
func TestAMissedLoreReadStillNamesWhatWasMissing(t *testing.T) {
	workspace := t.TempDir()
	definitions, err := newLoreTools(workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	read := findTool(t, definitions, "read_lore_items")

	result := runTool(t, read, `{"ids":["lore_absent"]}`)
	if !strings.Contains(result.ModelContent, "lore_absent") {
		t.Fatalf("the miss does not name the id that was absent: %s", result.ModelContent)
	}
}

// TestAnUnreadableLoreStoreIsStillAFailure is the other half, and the one that
// keeps the fix from swallowing real breakage. When the book cannot be read at
// all, the tool must still report failure — the tool runtime turns that into the
// error status the UI shows. §8.4 says a failed read is a technical problem, and
// §8.3 forbids pretending material was read when it was not.
func TestAnUnreadableLoreStoreIsStillAFailure(t *testing.T) {
	// An unscoped tool set with no workspace cannot reach any book. This is the
	// same condition a broken workspace produces, and it is not a miss.
	definitions, err := newLoreTools("", true)
	if err != nil {
		t.Fatal(err)
	}
	read := findTool(t, definitions, "read_lore_items")
	if _, err := read.Tool.Run(context.Background(), `{"ids":["lore_any"]}`); err == nil {
		t.Fatal("an unreachable store was answered as though the read had succeeded; " +
			"a failure that looks like a successful miss is exactly what §8.4 rules out")
	}
}

// TestAMissedEventCardIsNotASystemFailure is the same line drawn for event
// material. A card id that the story's packs do not contain is a miss, and an id
// that exists in two packs is an ambiguity the model can resolve by qualifying it
// — neither is a broken library.
func TestAMissedEventCardIsNotASystemFailure(t *testing.T) {
	packages := []interactive.EventPackage{{
		ID: "pack", Name: "pack", Enabled: true,
		Events: []interactive.EventCard{{ID: "ambush", TypeName: "伏击", Enabled: true}},
	}}
	definitions, err := newInteractiveEventTools(InteractiveContext{EventPackages: packages})
	if err != nil {
		t.Fatal(err)
	}
	read := findTool(t, definitions, "read_event_card")

	result := runTool(t, read, `{"card_id":"never_written"}`)
	if result.Status != agent.ToolResultSuccess {
		t.Fatalf("an absent card was reported as %q, which the UI shows as a system "+
			"failure; content=%s", result.Status, result.ModelContent)
	}
	if !strings.Contains(result.ModelContent, "normal empty result") {
		t.Fatalf("the miss does not say it is a normal answer rather than a fault: %s", result.ModelContent)
	}
	if !strings.Contains(result.ModelContent, "never_written") {
		t.Fatalf("the miss does not name the card that was absent: %s", result.ModelContent)
	}
}

// TestAnAmbiguousEventCardIsNotASystemFailure covers the other outcome
// FindEventCard reports. Two packs carrying the same id is a question the model
// answers by qualifying the id, not a library failure.
func TestAnAmbiguousEventCardIsNotASystemFailure(t *testing.T) {
	card := interactive.EventCard{ID: "ambush", TypeName: "伏击", Enabled: true}
	packages := []interactive.EventPackage{
		{ID: "north", Name: "north", Enabled: true, Events: []interactive.EventCard{card}},
		{ID: "south", Name: "south", Enabled: true, Events: []interactive.EventCard{card}},
	}
	definitions, err := newInteractiveEventTools(InteractiveContext{EventPackages: packages})
	if err != nil {
		t.Fatal(err)
	}
	read := findTool(t, definitions, "read_event_card")

	result := runTool(t, read, `{"card_id":"ambush"}`)
	if result.Status != agent.ToolResultSuccess {
		t.Fatalf("an ambiguous card was reported as %q: %s", result.Status, result.ModelContent)
	}
	if !strings.Contains(result.ModelContent, "north:ambush") || !strings.Contains(result.ModelContent, "south:ambush") {
		t.Fatalf("the answer does not say which qualified ids exist: %s", result.ModelContent)
	}
}
