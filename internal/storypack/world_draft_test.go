package storypack

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ta/internal/interactive"
)

// 自制-AI创建世界包与武侠规则复用 §6 and §8 ask for three things this file pins:
// a draft becomes a world that really loads, the rules behind it are the
// program's rather than the model's, and saving never overwrites an existing
// world.
//
// The draft tests build the pack and read it back through the same Library a
// player's page uses, so "the world appears in the directory" is checked the way
// the page checks it rather than by inspecting the struct that was written.

func draftTestWorld() WorldDraft {
	gameplay := RecommendedWorldGameplay()
	gameplay.Confirmed = true
	return WorldDraft{
		Name:        "寒山驿",
		Description: "北境一座被雪封住的驿镇，商队在此过冬。",
		Title:       "雪停之前",
		Origin:      "你是刚到寒山驿的旅人，驿丞说这场雪要下足七日。",
		Protagonist: "一个没有靠山的行脚人。",
		World: []DraftWorldEntry{{
			Name: "寒山驿", Type: "setting",
			BriefDescription: "北境驿镇，冬季封路。",
			Content:          "驿镇夹在两山之间，入冬后官道断绝，直到开春才通。",
		}},
		Characters: []DraftCharacter{{
			Name: "柳四", Age: 42, Gender: "female", SpeechStyle: "说话慢，先问来路再安排住处。",
			Role: "驿丞", Place: "寒山驿", Brief: "管着驿站的吃住与马匹。",
		}},
		Origins: []DraftOrigin{{
			Name: "行脚人", Basis: draftBasisWanderer, Description: "没有师门，也没有家业。",
		}},
		Affiliations: []DraftOption{{Name: "驿马帮"}},
		Locations:    []DraftOption{{Name: "寒山驿"}},
		Gameplay:     gameplay,
	}
}

func TestPackBindingDoesNotInventGameplayConfirmation(t *testing.T) {
	gameplay := RecommendedWorldGameplay()
	binding := (Pack{ID: "unconfirmed-world", Story: PackStory{Gameplay: gameplay}}).Binding()
	if binding.Gameplay != nil {
		t.Fatalf("unconfirmed world acquired gameplay capability: %+v", binding.Gameplay)
	}

	gameplay.Confirmed = true
	binding = (Pack{ID: "confirmed-world", Story: PackStory{Gameplay: gameplay}}).Binding()
	if binding.Gameplay == nil || binding.Gameplay.StatusTemplate != WorldStatusGeneralActions {
		t.Fatalf("confirmed world gameplay binding = %+v", binding.Gameplay)
	}
}

// TestADraftBuildsAWorldThatActuallyLoads is §8's "生成包真实出现在目录里并能从
// 正常入口开始" for the save half: write the draft, then read the world back
// through the list the page shows.
func TestRecommendedWorldUsesGenericChecksAndNeutralProtagonist(t *testing.T) {
	draft := draftTestWorld()
	if draft.Gameplay.StatusTemplate != "world-actions-v1" {
		t.Fatalf("recommended state scheme = %q, want the supported generic action checks", draft.Gameplay.StatusTemplate)
	}
	pack, err := draft.BuildPack()
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.ProtagonistTemplates) != 1 || pack.ProtagonistTemplates[0].ID != "world-protagonist" || pack.ProtagonistTemplates[0].Name == "侦探" {
		t.Fatalf("generated world still assigns a detective protagonist: %+v", pack.ProtagonistTemplates)
	}
	if _, _, err := pack.StoryRequestForTemplate("world-protagonist"); err != nil {
		t.Fatalf("the neutral protagonist template must start a story: %v", err)
	}
}

func TestADraftBuildsAWorldThatActuallyLoads(t *testing.T) {
	dataRoot := t.TempDir()
	library := NewLibrary(dataRoot)

	result, err := library.SaveWorld(context.Background(), draftTestWorld())
	if err != nil {
		t.Fatalf("saving a complete draft must succeed: %v", err)
	}
	if result.PackID == "" {
		t.Fatal("the save did not report the id it wrote")
	}

	// It is on disk under the id it reported.
	path := filepath.Join(presetStoryPacksDir(dataRoot), result.PackID+fileExtension)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the saved world is not at %s: %v", path, err)
	}

	// And it reads back through the ordinary entry point, with the prose intact.
	pack, err := library.Get(result.PackID)
	if err != nil {
		t.Fatalf("the saved world does not load: %v", err)
	}
	if pack.Name != "寒山驿" {
		t.Fatalf("the world lost its name: %q", pack.Name)
	}
	if pack.Story.Origin == "" {
		t.Fatal("the world lost its premise")
	}
	if len(pack.World) != 1 || pack.World[0].Name != "寒山驿" {
		t.Fatalf("the world book did not survive the save: %#v", pack.World)
	}
	if len(pack.Characters) != 1 || pack.Characters[0].Name != "柳四" {
		t.Fatalf("the people did not survive the save: %#v", pack.Characters)
	}
	person := pack.Characters[0]
	if person.Age != 42 || person.Gender != "female" || person.SpeechStyle != "说话慢，先问来路再安排住处。" {
		t.Fatalf("authored character facts did not survive the catalog: %+v", person)
	}
	frozen := pack.FreezeCatalog().CatalogBySourceKey()[person.SourceKey]
	if frozen.Age != person.Age || frozen.Gender != person.Gender || frozen.SpeechStyle != person.SpeechStyle {
		t.Fatalf("character facts changed in the frozen catalog: %+v", frozen)
	}

	// The list shows it as a startable user world, not as a broken file.
	summaries, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, summary := range summaries {
		if summary.ID != result.PackID {
			continue
		}
		found = true
		if summary.Invalid {
			t.Fatalf("the saved world is listed as invalid: %s", summary.Error)
		}
		if summary.Missing > 0 {
			t.Fatalf("the saved world names %d modules this machine lacks", summary.Missing)
		}
		if summary.Creation == nil || len(summary.Creation.Origins) == 0 {
			t.Fatal("the saved world cannot create a character")
		}
	}
	if !found {
		t.Fatalf("the saved world %s is not in the list", result.PackID)
	}
}

// TestAGeneratedWorldCanResolveACharacter is the real acceptance: the world must
// not merely load, it must be able to start somebody. ResolveIdentity is where a
// thin world fails — no aptitude, no place, or no unaffiliated option each
// refuses every character — so the draft is resolved here rather than trusted.
func TestAGeneratedWorldCanResolveACharacter(t *testing.T) {
	dataRoot := t.TempDir()
	library := NewLibrary(dataRoot)
	draft := draftTestWorld()
	// This acceptance retains the older cultivation creation assertions. The
	// ordinary recommendation uses general action checks with no numeric growth.
	draft.Gameplay.StatusTemplate = WorldStatusCultivation
	draft.Gameplay.NumericScale = WorldScaleCultivation
	draft.Gameplay.Growth = WorldGrowthCultivation
	draft.Gameplay.Combat = WorldCombatCultivation
	result, err := library.SaveWorld(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := library.Get(result.PackID)
	if err != nil {
		t.Fatal(err)
	}
	if !pack.CreationReady() {
		t.Fatal("a saved generated world must be ready for creation")
	}

	// The creation page always sends an origin; the world must accept the one it
	// offers. Everything else is left to the world.
	if len(pack.Creation.Origins) == 0 {
		t.Fatal("the world offers no origin")
	}
	first := pack.Creation.Origins[0]
	identity, err := pack.ResolveIdentity(IdentityChoice{
		Name: "沈砚", Age: 22, OriginID: first.ID,
		RootCategoryID: "waste",
	})
	if err != nil {
		t.Fatalf("a generated world must be able to start a character who chooses nothing: %v", err)
	}
	if identity.LocationID == "" {
		t.Fatal("the resolved character has no starting place")
	}
	if identity.InitialState == nil {
		t.Fatal("the resolved character has no initial state")
	}
	if identity.InitialState.Realm == "" {
		t.Fatal("the resolved character starts at no realm")
	}

	// And one who also names an affiliation and a place.
	affiliation := pack.Creation.Affiliations[0]
	location := pack.Creation.Locations[0]
	chosen, err := pack.ResolveIdentity(IdentityChoice{
		Name: "沈砚", Age: 22, OriginID: first.ID,
		AffiliationID: affiliation.ID, LocationID: location.ID,
		RootCategoryID: "waste",
	})
	if err != nil {
		t.Fatalf("a generated world must start the origin it offers: %v", err)
	}
	if chosen.OriginID != first.ID {
		t.Fatalf("the player's origin was replaced: %q", chosen.OriginID)
	}
}

// TestTheMechanicsOfAGeneratedWorldAreThePrograms is §6's division of labour.
//
// A draft may say anything in prose. The aptitude tiers, the starting purse and
// the technique definition are the program's, and a client that sends its own
// numbers back on save must not be able to change them.
func TestTheMechanicsOfAGeneratedWorldAreThePrograms(t *testing.T) {
	draft := draftTestWorld()
	// A hostile edit: the draft claims a starting purse and a template state.
	// Neither field exists on a draft, so the only way to attempt this is through
	// the pack the draft builds — which must still produce the program's values.
	pack, err := draft.BuildPack()
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Creation.Aptitudes) != 0 {
		t.Fatal("a non-cultivation world must not offer cultivation aptitude")
	}
	if len(pack.ProtagonistTemplates) == 0 {
		t.Fatal("a generated world must ship at least one ready-made start")
	}
	for _, template := range pack.ProtagonistTemplates {
		for _, forbidden := range []string{"当前境界", "灵根与特殊体质", "灵石"} {
			if _, exists := template.State[forbidden]; exists {
				t.Fatalf("non-cultivation template %s contains %q", template.ID, forbidden)
			}
		}
		for _, field := range draft.Gameplay.StateFields {
			if got := template.State[field.Name]; got != float64(field.Initial) && got != field.Initial {
				t.Fatalf("template %s starts with %s=%v, want the configured value %d", template.ID, field.Name, got, field.Initial)
			}
		}
	}
}

// TestTheWorldBindsOnlySupportedModules is §6's "不得用无关规则无声兜底". A
// generated world must bind the rule system it can actually run, and the state
// template whose fields its protagonist state is written in.
func TestTheWorldBindsOnlySupportedModules(t *testing.T) {
	pack, err := draftTestWorld().BuildPack()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := pack.ValidateModules(root); err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(root).Get(pack.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	if module.ActorState.WorldContent == nil || module.ActorState.WorldContent.PackID != pack.ID || interactive.ActorStateSystemIsXiuxian(module.ActorState) || module.ActorState.AdventureRules != nil {
		t.Fatal("generic world inherited an unrelated rule family")
	}
	if pack.Story.ModuleRefs.ActorStateID == interactive.ActorStateXiuxianID {
		t.Fatal("generic world bound cultivation")
	}

}

func TestGameplayModulesCanBeRebuiltFromAnImportedPack(t *testing.T) {
	firstRoot := t.TempDir()
	draft := draftTestWorld()
	draft.Gameplay.StateFields[0].Name = "洞察"
	draft.Gameplay.StateFields[0].Description = "留意现场线索。"
	draft.Gameplay.StateFields[0].Minimum = 2
	draft.Gameplay.StateFields[0].Maximum = 12
	draft.Gameplay.StateFields[0].Initial = 8
	draft.Gameplay.CheckBindings[0].Field = "洞察"
	draft.Gameplay.CheckBindings[0].Scale = 1.5
	draft.Gameplay.CheckBindings[0].Offset = -7

	saved, err := NewLibrary(firstRoot).SaveWorld(context.Background(), draft)
	if err != nil {
		t.Fatalf("save configured world: %v", err)
	}
	first, err := NewLibrary(firstRoot).Get(saved.PackID)
	if err != nil {
		t.Fatalf("load configured world: %v", err)
	}
	if first.Story.ModuleRefs.ActorStateID == interactive.ActorStateDetectiveID || first.Story.ModuleRefs.RuleSystemID == interactive.RuleSystemDetectiveStatsID {
		t.Fatalf("saved world still points at shared defaults: %+v", first.Story.ModuleRefs)
	}
	stateModule, err := interactive.NewActorStateLibrary(firstRoot).Get(first.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatalf("read world state module: %v", err)
	}
	field, ok := findWorldStateField(stateModule.ActorState, "洞察")
	if !ok || field.Min == nil || *field.Min != 2 || field.Max == nil || *field.Max != 12 || field.Default != float64(8) {
		t.Fatalf("world field definition was not materialized: %#v", field)
	}
	ruleModule, err := interactive.NewRuleSystemLibrary(firstRoot).Get(first.Story.ModuleRefs.RuleSystemID)
	if err != nil {
		t.Fatalf("read world rule module: %v", err)
	}
	binding := findWorldBinding(ruleModule.TRPGSystem.RuleTemplates, "search")
	if binding.ID != "search" || binding.Modifiers[0].FieldID != "洞察" || binding.Modifiers[0].Scale != 1.5 || binding.Modifiers[0].Offset != -7 {
		t.Fatalf("world check binding was not materialized: %#v", binding)
	}

	// Only the pack travels. The receiver reconstructs the rule files from the
	// pack's typed mechanics before it validates or starts the world.
	packBytes, err := os.ReadFile(filepath.Join(NewLibrary(firstRoot).dir(), saved.PackID+fileExtension))
	if err != nil {
		t.Fatal(err)
	}
	secondRoot := t.TempDir()
	secondPackDir := NewLibrary(secondRoot).dir()
	if err := os.MkdirAll(secondPackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondPackDir, saved.PackID+fileExtension), packBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	imported, err := NewLibrary(secondRoot).Get(saved.PackID)
	if err != nil {
		t.Fatalf("rebuild an imported world: %v", err)
	}
	rebuiltState, err := interactive.NewActorStateLibrary(secondRoot).Get(imported.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatalf("read rebuilt world state module: %v", err)
	}
	rebuiltRule, err := interactive.NewRuleSystemLibrary(secondRoot).Get(imported.Story.ModuleRefs.RuleSystemID)
	if err != nil {
		t.Fatalf("read rebuilt world rule module: %v", err)
	}
	if _, ok := findWorldStateField(rebuiltState.ActorState, "洞察"); !ok || findWorldBinding(rebuiltRule.TRPGSystem.RuleTemplates, "search").Modifiers[0].FieldID != "洞察" {
		t.Fatal("the imported pack did not recreate its saved fields and binding")
	}
	request, initialState, err := imported.StoryRequestForTemplate("world-protagonist")
	if err != nil {
		t.Fatalf("prepare a story from the imported pack: %v", err)
	}
	initialInsight := initialState["洞察"]
	if request.ModuleRefs.ActorStateID != imported.Story.ModuleRefs.ActorStateID || (initialInsight != 8 && initialInsight != float64(8)) {
		t.Fatalf("story creation did not receive the imported mechanics: refs=%+v state=%#v", request.ModuleRefs, initialState)
	}
}

func TestWorldGameplayRejectsReservedFieldsAndUnboundAbilities(t *testing.T) {
	for _, name := range []string{"姓名", "基本身份", "重要物品", "技能与能力"} {
		t.Run("reserved_"+name, func(t *testing.T) {
			gameplay := draftTestWorld().Gameplay
			gameplay.StateFields[0].Name = name
			gameplay.CheckBindings[0].Field = name
			if err := ValidateWorldGameplay(gameplay); err == nil {
				t.Fatalf("reserved actor field %q was accepted as a numeric ability", name)
			}
		})
	}
	t.Run("unbound_ability", func(t *testing.T) {
		gameplay := draftTestWorld().Gameplay
		gameplay.StateFields = append(gameplay.StateFields, WorldStateField{
			Name: "直觉", Minimum: 1, Maximum: 10, Initial: 5,
		})
		if err := ValidateWorldGameplay(gameplay); err == nil {
			t.Fatal("an ability with no registered check binding was accepted")
		}
	})
}

func TestCorruptWorldGameplayModuleIsNotOverwritten(t *testing.T) {
	root := t.TempDir()
	library := NewLibrary(root)
	saved, err := library.SaveWorld(context.Background(), draftTestWorld())
	if err != nil {
		t.Fatal(err)
	}
	pack, err := library.Get(saved.PackID)
	if err != nil {
		t.Fatal(err)
	}
	stateModule, err := interactive.NewActorStateLibrary(root).Get(pack.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("{not-json")
	if err := os.WriteFile(stateModule.Path, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Get(saved.PackID); err == nil {
		t.Fatal("loading a world silently replaced a corrupt module")
	}
	got, err := os.ReadFile(stateModule.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("corrupt module bytes changed during failed load: %q", got)
	}
}

func findWorldStateField(system interactive.StoryDirectorActorStateSystem, name string) (interactive.ActorStateField, bool) {
	for _, template := range system.Templates {
		if template.ID != interactive.DefaultActorID {
			continue
		}
		for _, field := range template.Fields {
			if field.Name == name {
				return field, true
			}
		}
	}
	return interactive.ActorStateField{}, false
}

func findWorldBinding(templates []interactive.RuleCheck, id string) interactive.RuleStateBinding {
	for _, template := range templates {
		for _, binding := range template.StateBindings {
			if binding.ID == id {
				return binding
			}
		}
	}
	return interactive.RuleStateBinding{}
}

// TestSavingAWorldNeverOverwritesAnother is §7's "不能覆盖内置包" and the ordinary
// case behind it: pressing save twice, or naming a second world the same thing,
// must produce a second world rather than destroy the first.
func TestSavingAWorldNeverOverwritesAnother(t *testing.T) {
	dataRoot := t.TempDir()
	library := NewLibrary(dataRoot)
	ctx := context.Background()

	first, err := library.SaveWorld(ctx, draftTestWorld())
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(presetStoryPacksDir(dataRoot), first.PackID+fileExtension))
	if err != nil {
		t.Fatal(err)
	}

	second, err := library.SaveWorld(ctx, draftTestWorld())
	if err != nil {
		t.Fatalf("saving a second world with the same name must succeed: %v", err)
	}
	if second.PackID == first.PackID {
		t.Fatalf("the second save reused the first world's id: %s", second.PackID)
	}
	after, err := os.ReadFile(filepath.Join(presetStoryPacksDir(dataRoot), first.PackID+fileExtension))
	if err != nil {
		t.Fatalf("the first world was removed by the second save: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("the second save rewrote the first world's file")
	}
}

// TestABuiltinWorldCannotBeShadowedByAGeneratedOne is the other half of "不能覆盖
// 内置包": a draft named after a shipped world must not take its id.
func TestABuiltinWorldCannotBeShadowedByAGeneratedOne(t *testing.T) {
	dataRoot := t.TempDir()
	library := NewLibrary(dataRoot)

	draft := draftTestWorld()
	// The builtin's id is derived from its name; naming a world exactly that is
	// the attempt this guards against.
	for _, builtin := range builtinPacks() {
		if builtin.ID != "fanren-jie" {
			continue
		}
		draft.Name = "凡人界"
	}
	result, err := library.SaveWorld(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if result.PackID == "fanren-jie" {
		t.Fatal("a generated world took a builtin world's id")
	}

	// The builtin still reads as itself, with its own people.
	builtin, err := library.Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	if !builtin.Builtin {
		t.Fatal("the builtin world was shadowed by the generated one")
	}
	if len(builtin.Characters) < 100 {
		t.Fatalf("the builtin world lost its catalog: %d people", len(builtin.Characters))
	}
}

// TestADraftWithNoWorldBookIsRefused covers the honest failure: a world with no
// entries cannot be read at all, so it is refused rather than saved empty.
func TestADraftWithNoWorldBookIsRefused(t *testing.T) {
	draft := draftTestWorld()
	draft.World = nil
	if _, err := draft.BuildPack(); err == nil {
		t.Fatal("a draft with no world book must be refused")
	}
}

// TestADraftWithNoPremiseIsRefused covers the other required half.
func TestADraftWithNoPremiseIsRefused(t *testing.T) {
	draft := draftTestWorld()
	draft.Origin = ""
	if _, err := draft.BuildPack(); err == nil {
		t.Fatal("a draft with no premise must be refused")
	}
}

// TestADraftPreviewMatchesWhatASaveWouldDo keeps the page honest: the preview
// the player sees before saving must report the same readiness a save would.
func TestADraftPreviewMatchesWhatASaveWouldDo(t *testing.T) {
	dataRoot := t.TempDir()
	preview := PreviewWorldDraft(dataRoot, draftTestWorld())
	if !preview.Startable {
		t.Fatalf("a complete draft previews as unstartable: %v", preview.Reasons)
	}
	if preview.WorldCount != 1 || preview.CharacterCount != 1 {
		t.Fatalf("the preview counts the wrong content: %+v", preview)
	}
	if preview.Notice == "" {
		t.Fatal("the preview must say which rules actually run")
	}

	// A thin draft previews too, but says why it cannot start.
	thin := draftTestWorld()
	thin.Origin = ""
	thin.World = nil
	broken := PreviewWorldDraft(dataRoot, thin)
	if broken.Startable {
		t.Fatal("a draft with no premise and no world book previews as startable")
	}
	if len(broken.Reasons) == 0 {
		t.Fatal("an unstartable draft must say why")
	}
}

// TestTheNoticeNamesTheSupportedSystemAndNothingElse pins the sentence §6
// requires: a generated world says which rules run, and does not claim a system
// this build does not have.
func TestTheNoticeNamesTheSelectedSupportedSystem(t *testing.T) {
	general := DraftNoticeForGameplay(RecommendedWorldGameplay())
	if general != "worldCreation.notice.generalActionLocal" {
		t.Fatalf("the notice does not select the localized general-action message: %s", general)
	}
	letter := RecommendedWorldGameplay()
	letter.Communication = WorldCommunicationLetter
	if got := DraftNoticeForGameplay(letter); got != "worldCreation.notice.generalActionLetter" {
		t.Fatalf("letter world notice key = %s", got)
	}
	cultivation := RecommendedWorldGameplay()
	cultivation.StatusTemplate = WorldStatusCultivation
	cultivation.NumericScale, cultivation.Growth, cultivation.Combat = WorldScaleCultivation, WorldGrowthCultivation, WorldCombatCultivation
	if got := DraftNoticeForGameplay(cultivation); got != "worldCreation.notice.cultivation" {
		t.Fatalf("cultivation plan has the wrong localized notice key: %s", got)
	}
}

// presetStoryPacksDir is where the library writes, named here so the test reads
// the same path the save wrote without re-deriving the layout.
func presetStoryPacksDir(dataRoot string) string {
	return NewLibrary(dataRoot).dir()
}
