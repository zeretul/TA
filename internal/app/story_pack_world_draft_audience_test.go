package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/internal/agents/prompts"
	"ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/storypack"
)

// The supplied JSON is a deterministic model-response fixture. This verifies
// the real parser, merge, save, cold read and NPC projection, not AI judgment.
func TestGeneratedWorldAudienceSurvivesSaveAndNPCProjection(t *testing.T) {
	payload, err := parseWorldDraftPayload(`{"name":"寒山驿","origin":"旅人抵达驿站。","world":[{"name":"Archive","type":"rule","audience":"gm","content":"GM_SECRET_MARKER","load_mode":"auto"},{"name":"Market","type":"location","audience":"public","content":"PUBLIC_MARKER","load_mode":"auto"},{"name":"Legacy","type":"world","content":"LEGACY_PUBLIC_MARKER","load_mode":"auto"},{"name":"Unknown","type":"rule","audience":"conditional_unmapped","content":"UNKNOWN_SECRET_MARKER","load_mode":"auto"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	gameplay := storypack.RecommendedWorldGameplay()
	gameplay.Confirmed = true
	draft := mergeWorldDraft(storypack.WorldDraft{Gameplay: gameplay}, payload, false, worldDraftSeed{})
	wanted := map[string]string{"Archive": "gm", "Market": "public", "Legacy": "", "Unknown": "gm"}
	if len(draft.World) != len(wanted) {
		t.Fatalf("world entries lost: %+v", draft.World)
	}
	for _, entry := range draft.World {
		if audience, ok := wanted[entry.Name]; !ok || entry.Audience != audience {
			t.Fatalf("generated audience lost: %+v", entry)
		}
	}
	application := newExecutionProfileTestApp(t)
	saved, err := application.StoryPacks().SaveWorldDraft(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := storypack.NewLibrary(application.cfg.DataDir()).GetContent(saved.PackID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.World) != len(wanted) {
		t.Fatalf("saved world entries lost: %+v", pack.World)
	}
	for _, entry := range pack.World {
		if audience, ok := wanted[entry.Name]; !ok || entry.Audience != audience {
			t.Fatalf("cold world read changed audience: %+v", entry)
		}
	}
	workspace := t.TempDir()
	var ops []lore.Operation
	for _, input := range pack.WorldInputs() {
		ops = append(ops, lore.Operation{Op: "create", Item: input})
	}
	if _, err := lore.NewStore(workspace).ApplyOperations("Import generated world", ops); err != nil {
		t.Fatal(err)
	}
	items, err := lore.NewStore(workspace).ListAll()
	if err != nil || len(items) != len(wanted) {
		t.Fatalf("author editor lost generated lore: %d, %v", len(items), err)
	}
	projected := strings.Join(characterLoreFor(workspace, pack.WorldScope(), character.Profile{Name: "Archive Market Legacy Unknown"}, character.Scene{}, 0), "\n")
	for _, marker := range []string{"PUBLIC_MARKER", "LEGACY_PUBLIC_MARKER"} {
		if !strings.Contains(projected, marker) {
			t.Fatalf("public generated lore missing %s: %s", marker, projected)
		}
	}
	for _, marker := range []string{"GM_SECRET_MARKER", "UNKNOWN_SECRET_MARKER"} {
		if strings.Contains(projected, marker) {
			t.Fatalf("generated GM lore reached NPC: %s", projected)
		}
	}
}

func TestWorldDraftRewritePreservesAudienceUnlessExplicitlyChanged(t *testing.T) {
	for _, tc := range []struct {
		name, previous, input, want string
	}{
		{"omitted GM", "gm", "", "gm"},
		{"unknown preserves GM", "gm", "conditional_unmapped", "gm"},
		{"unknown is not a public rewrite", "public", "conditional_unmapped", "public"},
		{"explicit public", "gm", "public", "public"},
		{"explicit GM", "public", "gm", "gm"},
		{"legacy omission", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft := storypack.WorldDraft{World: []storypack.DraftWorldEntry{
				{Name: "Archive", Audience: tc.previous, Content: "Old archive"},
				{Name: "Market", Audience: "public", Content: "Old market"},
			}}
			entry := map[string]any{"name": " archive ", "content": "New archive"}
			if tc.input != "" {
				entry["audience"] = tc.input
			}
			raw, err := json.Marshal(map[string]any{"origin": "Rewritten premise", "world": []any{
				map[string]any{"name": "Market", "content": "New market"}, entry,
			}})
			if err != nil {
				t.Fatal(err)
			}
			payload, err := parseWorldDraftPayload(string(raw))
			if err != nil {
				t.Fatal(err)
			}
			before, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			merged := mergeWorldDraft(draft, payload, true, worldDraftSeed{})
			if len(merged.World) != 2 || merged.World[0].Audience != "public" || merged.World[1].Audience != tc.want || merged.World[1].Content != "New archive" {
				t.Fatalf("reordered rewrite changed audience incorrectly: %+v", merged.World)
			}
			if draft.World[0].Audience != tc.previous || draft.World[0].Content != "Old archive" {
				t.Fatal("rewrite mutated the original draft")
			}
			after, err := json.Marshal(payload)
			if err != nil || string(before) != string(after) {
				t.Fatalf("rewrite mutated the response fixture: %v", err)
			}
			kept := mergeWorldDraft(draft, payload, false, worldDraftSeed{})
			if !reflect.DeepEqual(kept.World, draft.World) {
				t.Fatal("non-rewrite replaced an already authored entry")
			}
		})
	}
}

func TestWorldDraftRewriteKeepsRenamedSecretPrivate(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"omitted audience", "", "gm"},
		{"unknown audience", "conditional_unmapped", "gm"},
		{"explicit public", "public", "public"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft := storypack.WorldDraft{World: []storypack.DraftWorldEntry{
				{Name: "Archive", Audience: "gm", Content: "Secret history"},
				{Name: "Market", Audience: "public", Content: "Public square"},
			}}
			payload := worldDraftPayload{World: []worldDraftEntryPayload{
				{Name: "Renamed archive", Audience: tc.input, Content: "Secret history rewritten"},
				{Name: "Market", Content: "Public square rewritten"},
			}}
			merged := mergeWorldDraft(draft, payload, true, worldDraftSeed{})
			if len(merged.World) != 2 || merged.World[0].Audience != tc.want || merged.World[0].Content != "Secret history rewritten" {
				t.Fatalf("renamed secret audience changed incorrectly: %+v", merged.World)
			}
			if merged.World[1].Audience != "public" {
				t.Fatalf("same-name public entry did not retain its audience: %+v", merged.World[1])
			}
			if payload.World[0].Audience != tc.input || draft.World[0].Name != "Archive" || draft.World[0].Audience != "gm" {
				t.Fatal("rewrite mutated its inputs")
			}
		})
	}
}

func TestWorldDraftWithoutGMPreservesLegacyPublicDefault(t *testing.T) {
	for _, tc := range []struct {
		name    string
		world   []storypack.DraftWorldEntry
		rewrite bool
	}{
		{name: "fresh draft"},
		{name: "fresh rewrite", rewrite: true},
		{name: "public-only rewrite", rewrite: true, world: []storypack.DraftWorldEntry{{Name: "Market", Audience: "public"}}},
		{name: "legacy-public rewrite", rewrite: true, world: []storypack.DraftWorldEntry{{Name: "Market"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			merged := mergeWorldDraft(storypack.WorldDraft{World: tc.world}, worldDraftPayload{
				World: []worldDraftEntryPayload{{Name: "New entry", Content: "Public by legacy default"}},
			}, tc.rewrite, worldDraftSeed{})
			if len(merged.World) != 1 || merged.World[0].Audience != "" || lore.NormalizeAudience(merged.World[0].Audience) == "gm" {
				t.Fatalf("draft without existing GM lost its legacy public default: %+v", merged.World)
			}
		})
	}
}

func TestWorldDraftAudienceReachesExistingPromptAndOutputContract(t *testing.T) {
	draft := storypack.WorldDraft{World: []storypack.DraftWorldEntry{
		{Name: "Archive", Type: "rule", BriefDescription: "Author-only source", Audience: "gm"},
		{Name: "Unknown", Type: "rule", BriefDescription: "Unmapped source", Audience: "conditional_unmapped"},
	}}
	instruction := prompts.BuildWorldCreationSystemInstruction(prompts.WorldCreationSystemInstructionInput{
		Existing: worldDraftPromptInput(draft, false),
	})
	for _, want := range []string{
		"Archive [rule]: Author-only source（audience=gm）",
		"Unknown [rule]: Unmapped source（audience=gm）",
		`"audience": "public|gm"`,
		"manual 不等于保密",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("draft prompt lost audience contract %q", want)
		}
	}
}
