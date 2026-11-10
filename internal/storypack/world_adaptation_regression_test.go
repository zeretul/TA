package storypack

import (
	"context"
	booklore "ta/internal/book/lore"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"testing"
)

func TestWorldAdaptationTrimmedCultivationModuleRoundTrip(t *testing.T) {
	for _, kind := range []string{"items", "skill_templates", "stage_order"} {
		t.Run(kind, func(t *testing.T) {
			p := fanrenPack(t)
			r := cultivation.CloneRules(p.Story.Gameplay.CultivationRules)
			switch kind {
			case "items":
				r.Items = nil
				r.CraftMethods = nil
			case "skill_templates":
				r.SkillTemplates = nil
			case "stage_order":
				r.StageOrder[1], r.StageOrder[2] = r.StageOrder[2], r.StageOrder[1]
			}
			p.Story.Gameplay.CultivationRules = r
			if err := r.Validate(); err != nil {
				if kind != "stage_order" {
					t.Fatal(err)
				}
				return
			}
			if kind == "stage_order" {
				t.Fatal("invalid stage order accepted")
			}
			lib := NewLibrary(t.TempDir())
			if _, err := lib.materializeGameplayModules(&p); err != nil {
				t.Fatal("first materialization:", err)
			}
			if _, err := lib.materializeGameplayModules(&p); err != nil {
				t.Fatal("unchanged package cannot reload:", err)
			}
		})
	}
}
func TestWorldAdaptationCultivationRejectsUnknownCapabilities(t *testing.T) {
	p := fanrenPack(t)
	for _, field := range []string{"communication", "opening"} {
		t.Run(field, func(t *testing.T) {
			g := p.Story.Gameplay
			if field == "communication" {
				g.Communication = "unimplemented-telepathy"
			} else {
				g.Opening = "unimplemented-mode"
			}
			if err := ValidateWorldGameplay(g); err == nil {
				t.Fatal("unsupported declaration silently accepted", field)
			}
		})
	}
}
func TestWorldAdaptationRejectsDanglingLoreKnowledgeGate(t *testing.T) {
	p := fanrenPack(t)
	p.Story.Gameplay.WorldContent.LoreGates["missing-world-entry"] = p.Story.Gameplay.WorldContent.LoreGates["fanren-spirit-journey"]
	if err := p.Validate(); err == nil {
		t.Fatal("gate references absent lore but package accepted")
	}
}
func TestWorldAdaptationNewMechanismCanRequestOpeningStateAdaptation(t *testing.T) {
	d := draftTestWorld()
	d.World = []DraftWorldEntry{{Name: "灯印规则", Type: "rule", Content: "已成立约定要分别记录承诺内容、见证人、成立时间、约定期限和履行状态。"}}
	p, err := d.BuildPack()
	if err != nil {
		t.Fatal(err)
	}
	req := p.StoryRequest()
	if !interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(req.StateSchemaPolicy) {
		t.Fatalf("custom mechanism cannot adapt opening schema: mode=%s", req.StateSchemaPolicy.Mode)
	}
}
func TestWorldAdaptationPublicSaveWithEmptySkillTemplates(t *testing.T) {
	d := draftTestWorld()
	d.Gameplay = fanrenPack(t).Story.Gameplay
	d.Gameplay.WorldContent = nil
	d.Gameplay.CultivationRules = cultivation.CloneRules(d.Gameplay.CultivationRules)
	d.Gameplay.CultivationRules.SkillTemplates = nil
	_, err := NewLibrary(t.TempDir()).SaveWorld(context.Background(), d)
	if err != nil {
		t.Fatal("public world save rejected valid empty generation catalog:", err)
	}
}
func TestWorldAdaptationCultivationSnapshotCannotOmitRules(t *testing.T) {
	p := fanrenPack(t)
	p.Story.Gameplay.CultivationRules = nil
	if err := p.Validate(); err != nil {
		t.Log("correctly rejected:", err)
		return
	}
	lib := NewLibrary(t.TempDir())
	if _, err := lib.materializeGameplayModules(&p); err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(lib.dataRoot).Get(p.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	if interactive.ActorStateSystemIsXiuxian(module.ActorState) && module.ActorState.CultivationRules == nil {
		t.Fatal("cultivation accepted without frozen rules: calculators and catalogs fall back to program defaults")
	}
}
func TestWorldAdaptationBuiltinPackagesStillReload(t *testing.T) {
	root := t.TempDir()
	lib := NewLibrary(root)
	for _, id := range []string{"fanren-jie", "lanchuan-qunxia", "xinghai-689", "sui-guan-realms"} {
		t.Run(id, func(t *testing.T) {
			for n := 0; n < 2; n++ {
				p, err := lib.Get(id)
				if err != nil {
					t.Fatal(err)
				}
				if p.Story.Gameplay.WorldContent == nil || p.Story.Gameplay.WorldContent.PackID != id {
					t.Fatal("missing own catalog")
				}
			}
		})
	}
}
func TestWorldAdaptationRenamedLoreCannotBypassKnowledgeGate(t *testing.T) {
	p := fanrenPack(t)
	const renamed = "renamed-spirit-journey"
	for i := range p.World {
		if p.World[i].ID == "fanren-spirit-journey" {
			p.World[i].ID = renamed
		}
	}
	if err := p.Validate(); err != nil {
		t.Log("correctly rejected stale gate:", err)
		return
	}
	meta := interactive.StoryMeta{PackBinding: p.Binding(), ActorStateSchema: &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{WorldContent: p.Story.Gameplay.WorldContent, CultivationRules: p.Story.Gameplay.CultivationRules}}}
	visible := interactive.StoryLoreVisibility(meta, map[string]any{})
	for _, entry := range p.WorldInputs() {
		if entry.Name == "通往灵界的远行" && visible(booklore.Item{ID: entry.ID, Scope: entry.Scope, Content: entry.Content}) {
			t.Fatal("renamed gated lore is now visible without stage or location qualification")
		}
	}
}
