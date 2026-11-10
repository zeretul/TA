package interactive

import (
	"math"
	"sort"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// This file closes 第16项's remaining gap. F17 already proves the pieces: a
// meeting is not a fight, the creature grows on its own stage, a wound survives
// recall, an archived creature is neither companion nor prey, and a retry does
// not settle the fight twice. Each of those tests stops at the one write it was
// written for.
//
// What a player actually meets is one creature over many turns: it is found, a
// working bond is built out of real interactions, ordinary care is given, it
// really takes part in what the protagonist does, and the save holds all of it.
// So this is one chain through the production paths — no taming system, no
// feeding table, no creature panel. The bond is the same relationship record a
// person gets, which is exactly why the rules also accept it as a breakthrough
// guardian; the care is an ordinary interval of real time in which the creature
// keeps growing on its own arrangement and nothing new is measured about it; and
// the creature's part in the protagonist's action is read back as the rules'
// own number rather than as a sentence in the prose.

const f17FlowBeast = "青焰狐"
const f17FlowBeastSourceKey = "test_species_qingyan_fox"

// f17FlowStory builds a story whose protagonist stands at the last refining
// layer with the threshold met and a whole body, so the guardian the creature
// becomes is worth something the test can actually measure.
func f17FlowStory(t *testing.T, lifeform ...string) (*Store, string) {
	t.Helper()
	registeredLifeform := PackCatalogLifeformSpiritBeast
	if len(lifeform) > 0 {
		registeredLifeform = lifeform[0]
	}
	methodID := "method.spirit_beast_breathing"
	if registeredLifeform != PackCatalogLifeformSpiritBeast {
		methodID = ""
	}
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(cultivation.ReferenceRules(),
				spec),
			storyContextStateTemplate(spec),
			importantCharacterStateTemplate(cultivation.ReferenceRules(),
				spec),
			opponentStateTemplate(cultivation.ReferenceRules(),
				spec),
			worldEntitiesStateTemplate(spec),
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: b1BreakthroughFixture(t)},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
			{ID: DefaultWorldEntitiesActorID, Name: "世界实体", TemplateID: ActorStateWorldEntitiesTemplateID},
		},
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "灵兽", ActorState: &system,
		PackCatalog: &PackCatalogSnapshot{Characters: []PackCatalogPerson{{
			SourceKey: f17FlowBeastSourceKey, Name: f17FlowBeast,
			Lifeform: registeredLifeform, CultivationMethodID: methodID,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入林", Narrative: "主角沿溪入林。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

func TestOrdinaryAnimalDoesNotReceiveCultivationFromSharedCharacterTemplate(t *testing.T) {
	store, storyID := f17FlowStory(t, PackCatalogLifeformAnimal)
	f17FlowMeetAs(t, store, storyID, PackCatalogLifeformAnimal, "")
	progressBefore := f17BeastProgress(t, store, storyID, f17FlowBeast)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-f17-flow-ordinary-animal")
	for _, record := range npcGrowthTestRecords(t, store, storyID) {
		if record.Input.ActorID == f17FlowBeast {
			t.Fatalf("ordinary animal received cultivation settlement: %+v", record.Input)
		}
	}
	if progressAfter := f17BeastProgress(t, store, storyID, f17FlowBeast); progressAfter != progressBefore {
		t.Fatalf("ordinary animal cultivation changed: %v -> %v", progressBefore, progressAfter)
	}
}

// f17FlowMeet commits the turn in which the creature first appears. It becomes
// an Actor the rules track, a person the story knows, and one of the people in
// the scene — three facts a real meeting establishes in the same turn.
func f17FlowMeet(t *testing.T, store *Store, storyID string) {
	f17FlowMeetAs(t, store, storyID, PackCatalogLifeformSpiritBeast, "method.spirit_beast_breathing")
}

func f17FlowMeetAs(t *testing.T, store *Store, storyID, lifeform, methodID string) {
	t.Helper()
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "在溪石后停下",
		Narrative: "溪石后伏着一只青焰狐，前爪上扎着一截碎骨。它没有立刻扑上来，只是盯着你。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Create, Path: "/" + f17FlowBeast, Value: map[string]any{
					"template_id": ActorStateImportantCharacterTemplateID,
					"name":        f17FlowBeast,
					"state":       map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(2))},
				}},
				{Op: interactivestate.Replace, Path: "/" + DefaultStoryContextActorID + "/" + storyScenePresentField,
					Value: []any{DefaultActorID, f17FlowBeast}},
			},
			CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{
				CharacterID: f17FlowBeast, Name: f17FlowBeast, Role: "溪谷里的灵狐",
				Lifeform: lifeform, CultivationMethodID: methodID,
				Description: "一只前爪受了伤的青焰狐，眼睛是琥珀色，不肯退，也不肯靠近。",
			}}},
		},
	}); err != nil {
		t.Fatalf("meet the creature: %v", err)
	}
}

// f17FlowTurn commits one turn in which something real passed between the
// protagonist and the creature. It is the ordinary turn path, so the exchanges,
// the feeling and any formal bond are the same records a person's would be.
func f17FlowTurn(t *testing.T, store *Store, storyID, user, narrative string, facts TurnCharacterFactsInput) {
	t.Helper()
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: user, Narrative: narrative,
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &facts},
	}); err != nil {
		t.Fatalf("commit %q: %v", user, err)
	}
}

// f17FlowPair is the two lines one interaction leaves: the protagonist's own
// words and the creature's answer. Both are filed under the creature, because a
// bond cites the line that offered it and the line that answered it.
func f17FlowPair(playerLine, beastLine string) []TurnCharacterExchangeInput {
	return []TurnCharacterExchangeInput{
		{CharacterID: f17FlowBeast, Speaker: "player", Channel: "scene", AddressedTo: f17FlowBeast, Text: playerLine},
		{CharacterID: f17FlowBeast, Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: beastLine},
	}
}

// f17FlowHusbandryNames are the numbers a separate 御兽 system would have
// invented: how hungry it is, how loyal, what mood it is in. §2 keeps ordinary
// 照料 out of the rules, so their absence from the creature's own record is the
// assertion.
var f17FlowHusbandryNames = []string{"饥饿", "饱食", "忠诚", "心情", "亲密度", "喂养", "投喂"}

// f17FlowAssertNoHusbandryFields states that caring for the creature did not
// grow it a second, parallel set of needs.
func f17FlowAssertNoHusbandryFields(t *testing.T, store *Store, storyID, when string) {
	t.Helper()
	state := combatTestState(t, store, storyID)
	actors, _ := state[actorStateRoot].(map[string]any)
	actor, _ := actors[f17FlowBeast].(map[string]any)
	values, _ := actor["state"].(map[string]any)
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, husbandry := range f17FlowHusbandryNames {
			if strings.Contains(name, husbandry) {
				t.Fatalf("%s: 照料 must not become a %q number: %v", when, husbandry, names)
			}
		}
	}
}

// f17FlowGuardianChance is what the rules would compare a foundation attempt's
// roll against, read from the committed facts. It settles nothing.
func f17FlowGuardianChance(t *testing.T, store *Store, storyID, branchID string) float64 {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := store.CharacterState(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	guardian := xiuxianBreakthroughGuardian(snapshot.State, registry.Relationships)
	chance, ok := cultivation.BreakthroughChance(cultivation.BreakthroughFoundation, cultivation.BreakthroughModifiers{
		QualifiedGuardian: guardian,
	})
	if !ok {
		t.Fatal("the rules must price a foundation attempt")
	}
	return chance
}

// TestOneSpiritBeastRunsFromMeetingToReopen is 第16项's chain.
func TestOneSpiritBeastRunsFromMeetingToReopen(t *testing.T) {
	store, storyID := f17FlowStory(t)

	// --- 遇见：it is a creature in the world, not a prize ---------------------
	f17FlowMeet(t, store, storyID)

	if stage := f17BeastStage(t, store, storyID, f17FlowBeast); stage != cultivation.StageKey(cultivation.Qi(2)) {
		t.Fatalf("the creature's stage is its own, not the protagonist's: %q", stage)
	}
	encounter, err := store.CombatEncounter(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(encounter.Bystanders) != 1 || encounter.Bystanders[0] != f17FlowBeast {
		t.Fatalf("a creature merely met is not a combatant: %#v", encounter)
	}
	met, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	// Meeting it built no bond. A bond has to be made; a creature that was only
	// seen has not made one.
	if relationship, ok := character.CurrentRelationship(f17FlowBeast, met.Relationships); ok {
		t.Fatalf("a meeting must not create a bond: %+v", relationship)
	}
	// It is a person the story knows, though: the window can list it and the
	// projector can file what it says. Without this a bond could never be
	// recorded at all, because a bond is between two known people.
	if _, ok := met.Profile(f17FlowBeast); !ok {
		t.Fatalf("the creature the story met is not registered: %+v", met.Profiles)
	}
	profile, _ := met.Profile(f17FlowBeast)
	if profile.SourceKey != "" {
		t.Fatalf("a newborn creature borrowed an author-catalog identity: %+v", profile)
	}
	if profile.Lifeform != PackCatalogLifeformSpiritBeast || profile.CultivationMethodID != "method.spirit_beast_breathing" {
		t.Fatalf("the creature's submitted cultivation qualification was not stored: %+v", profile)
	}

	beforeCooperation, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	meetingTurn := beforeCooperation.CurrentTurn.ID

	// --- 建立合作：a bond made of what actually happened ----------------------
	//
	// A bond is not a number crossing a threshold. Four real interactions settle
	// four distinct facts, and only then does the declaration cite the two lines
	// that offered and accepted it.
	f17FlowTurn(t, store, storyID, "替它挑出碎骨", "你按住它的前爪，把碎骨一点点挑出来。它疼得发抖，却没有咬你。",
		TurnCharacterFactsInput{
			Exchanges:  f17FlowPair("别动，挑出来就不疼了。", "（它把爪子往你手心里送了一点。）"),
			Affinities: []TurnCharacterAffinityInput{{CharacterID: f17FlowBeast, Kind: "meaningful", Delta: 8, Basis: "主角替它挑出扎进前爪的碎骨"}},
		})
	f17FlowTurn(t, store, storyID, "赶走偷猎者", "设陷阱的偷猎者摸进溪谷，你和青焰狐一前一后把他们逼退。",
		TurnCharacterFactsInput{
			Exchanges:  f17FlowPair("往我这边靠，别走散。", "（它绕到偷猎者侧后，火焰沿着脊背烧起来。）"),
			Affinities: []TurnCharacterAffinityInput{{CharacterID: f17FlowBeast, Kind: "major", Delta: 15, Basis: "主角与青焰狐一同赶走设陷阱的偷猎者"}},
		})
	f17FlowTurn(t, store, storyID, "分它半只烤兔", "你把烤好的兔子撕成两半，一半放在它面前。",
		TurnCharacterFactsInput{
			Exchanges:  f17FlowPair("吃吧，明天还有。", "（它先看你一眼，才低头叼走。）"),
			Affinities: []TurnCharacterAffinityInput{{CharacterID: f17FlowBeast, Kind: "meaningful", Delta: 8, Basis: "主角把烤好的兔子分了一半给它"}},
		})
	f17FlowTurn(t, store, storyID, "守夜", "后半夜，青焰狐挨着你腿边趴下，尾巴搭在你靴上。",
		TurnCharacterFactsInput{
			Exchanges:  f17FlowPair("睡吧，我守着。", "（它闭上眼，耳朵还朝着溪口。）"),
			Affinities: []TurnCharacterAffinityInput{{CharacterID: f17FlowBeast, Kind: "major", Delta: 15, Basis: "守夜时青焰狐第一次睡在主角身边"}},
		})

	// The declaration. Both halves are lines this turn really carries, which is
	// what a formal bond requires and what a favourable number cannot supply.
	f17FlowTurn(t, store, storyID, "结伴", "你说往后一起走，青焰狐把尾巴绕上你的手腕。",
		TurnCharacterFactsInput{
			Exchanges: f17FlowPair("往后我走到哪，你跟到哪。", "（它把尾巴绕上你的手腕，没有松开。）"),
			Relationships: []TurnCharacterRelationshipInput{{
				CharacterID: f17FlowBeast, Kind: "close_friend",
				Statement:        "主角与青焰狐结为同行的伙伴",
				ExpressedByIndex: 1, AnsweredByIndex: 2,
			}},
		})

	cooperated, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	bond, ok := character.CurrentRelationship(f17FlowBeast, cooperated.Relationships)
	if !ok {
		t.Fatalf("the declaration committed no bond: %+v", cooperated.Relationships)
	}
	if bond.Kind != character.RelationshipCloseFriend {
		t.Fatalf("bond kind = %q, want close_friend", bond.Kind)
	}
	// The bond names the words it was made of, so it is an event on file rather
	// than a status that silently changed.
	if bond.ExpressedBy == "" || bond.AnsweredBy == "" {
		t.Fatalf("the bond does not cite the lines it was made of: %+v", bond)
	}

	// --- 实际参与行动：it really takes part -----------------------------------
	//
	// First the fight it is drawn into. The scene declares it is attacking, the
	// exchange lands on its body, and the wound is its own record.
	f17DeclareEngagement(t, store, storyID, f17FlowBeast, "被逼到溪石后扑上来")
	whole := f17BeastHealth(t, store, storyID, f17FlowBeast)
	state := combatTestState(t, store, storyID)
	system := f17BeastSystem(t, store, storyID)
	fight := CombatEncounterFromState(state, system)
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-f17-flow", "cmd-f17-flow-fight", fight.Combatants, nil, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: f17FlowBeast},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第2日",
		Session: session, Outcome: outcome,
		CommandID: "cmd-f17-flow-fight", CommandHash: "hash-f17-flow",
	}); err != nil {
		t.Fatal(err)
	}
	wounded := f17BeastHealth(t, store, storyID, f17FlowBeast)
	if wounded >= whole {
		t.Fatalf("the exchange must land on the creature: %d -> %d", whole, wounded)
	}
	// The fight has to be closed before ordinary care is legal: §三.4 refuses
	// safe recovery while a battle is open, and that refusal is about the
	// protagonist rather than the creature.
	ended := session
	ended.Ended, ended.Result = true, cultivation.ResultTruce
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第2日",
		Session: ended, Outcome: cultivation.ExchangeOutcome{Round: ended.Round, Ended: true, Result: cultivation.ResultTruce},
		CommandID: "cmd-f17-flow-truce", CommandHash: "hash-f17-flow-truce",
	}); err != nil {
		t.Fatal(err)
	}

	// The second part is standing guard. A bond that was expressed and answered,
	// with the creature in the scene, is the only thing the rules accept as a
	// guardian — and it is worth exactly the rules' own five points. The prose
	// says it is helping; this is the number that says the rules agree.
	guarded := f17FlowGuardianChance(t, store, storyID, "main")
	unrelated, ok := cultivation.BreakthroughChance(cultivation.BreakthroughFoundation, cultivation.BreakthroughModifiers{})
	if !ok {
		t.Fatal("the rules must price a foundation attempt")
	}
	if math.Abs((guarded-unrelated)-0.05) > 1e-9 {
		t.Fatalf("a bonded companion standing by must be worth the rules' five points: %v vs %v", guarded, unrelated)
	}

	// The control is the same story cut before the bond was made. The creature is
	// already in the scene there, so the only difference is the bond itself: a
	// bystander the prose called helpful earns nothing.
	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: meetingTurn, Title: "只是遇见"})
	if err != nil {
		t.Fatal(err)
	}
	forkRegistry, err := store.CharacterState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	forkSnapshot, err := store.Snapshot(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if present := storySceneFieldList(storyContextActorState(forkSnapshot.State), storyScenePresentField); !containsActorID(present, f17FlowBeast) {
		t.Fatalf("the control branch must still have the creature in the scene: %#v", present)
	}
	if xiuxianBreakthroughGuardian(forkSnapshot.State, forkRegistry.Relationships) {
		t.Fatalf("a creature merely present must not guard: %+v", forkRegistry.Relationships)
	}
	if forkChance := f17FlowGuardianChance(t, store, storyID, fork.ID); math.Abs(forkChance-unrelated) > 1e-9 {
		t.Fatalf("a creature merely present changed the odds: %v vs %v", forkChance, unrelated)
	}

	// --- 照料：ordinary care is time, not a second set of numbers ------------
	progressBeforeCare := f17BeastProgress(t, store, storyID, f17FlowBeast)

	// Thirty days of keeping it fed and rested is an ordinary interval. The
	// creature keeps growing on its own arrangement, at its own stage; nothing
	// new is measured about it, because 照料 is expressed by the story rather
	// than by a hunger, loyalty or mood system.
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-f17-flow-care")
	var foundSpeciesSettlement bool
	for _, record := range npcGrowthTestRecords(t, store, storyID) {
		if record.Input.ActorID != f17FlowBeast {
			continue
		}
		for _, segment := range record.Input.Interval.Segments {
			if segment.Conditions.GrowthCoefficient != 0.60 {
				t.Fatalf("species interval coefficient = %v, want 0.60", segment.Conditions.GrowthCoefficient)
			}
			foundSpeciesSettlement = true
		}
	}
	if !foundSpeciesSettlement {
		t.Fatal("the formally registered species produced no growth settlement")
	}

	if got := f17BeastProgress(t, store, storyID, f17FlowBeast); got <= progressBeforeCare {
		t.Fatalf("care is real time, so the creature must keep growing: %v -> %v", progressBeforeCare, got)
	}
	if stage := f17BeastStage(t, store, storyID, f17FlowBeast); stage != cultivation.StageKey(cultivation.Qi(2)) {
		t.Fatalf("30 days of care must not lift the creature to its owner's stage: %q", stage)
	}
	// The creature's body is its own: an interval that settles the protagonist's
	// pools does not mend the creature's wound.
	if got := f17BeastHealth(t, store, storyID, f17FlowBeast); got != wounded {
		t.Fatalf("care healed the wound: %d -> %d", wounded, got)
	}
	f17FlowAssertNoHusbandryFields(t, store, storyID, "after 30 days of care")

	// A retried care command is the same interval, not a second one.
	careProgress := f17BeastProgress(t, store, storyID, f17FlowBeast)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-f17-flow-care")
	if got := f17BeastProgress(t, store, storyID, f17FlowBeast); got != careProgress {
		t.Fatalf("a retried care interval settled twice: %v -> %v", careProgress, got)
	}

	// --- 保存恢复：the journal holds the whole creature ----------------------
	if err := store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopened := NewStore(store.root)
	t.Cleanup(func() { _ = reopened.Close() })

	restored, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatalf("read the registry after reopening: %v", err)
	}
	restoredBond, ok := character.CurrentRelationship(f17FlowBeast, restored.Relationships)
	if !ok || restoredBond.Kind != character.RelationshipCloseFriend {
		t.Fatalf("reopening lost the bond: %+v", restored.Relationships)
	}
	if restoredBond.ExpressedBy != bond.ExpressedBy || restoredBond.AnsweredBy != bond.AnsweredBy {
		t.Fatalf("reopening changed the lines the bond was made of: %+v vs %+v", restoredBond, bond)
	}
	if _, ok := restored.Profile(f17FlowBeast); !ok {
		t.Fatalf("reopening lost the creature from the story's people: %+v", restored.Profiles)
	}
	if got := f17BeastHealth(t, reopened, storyID, f17FlowBeast); got != wounded {
		t.Fatalf("reopening healed the creature's wound: %d -> %d", wounded, got)
	}
	if got := f17BeastStage(t, reopened, storyID, f17FlowBeast); got != cultivation.StageKey(cultivation.Qi(2)) {
		t.Fatalf("reopening moved the creature's stage: %q", got)
	}
	if got := f17BeastProgress(t, reopened, storyID, f17FlowBeast); got != careProgress {
		t.Fatalf("reopening changed the creature's growth: %v -> %v", careProgress, got)
	}
	restoredState := combatTestState(t, reopened, storyID)
	if present := storySceneFieldList(storyContextActorState(restoredState), storyScenePresentField); !containsActorID(present, f17FlowBeast) {
		t.Fatalf("reopening lost the creature from the scene: %#v", present)
	}
	f17FlowAssertNoHusbandryFields(t, reopened, storyID, "after reopening")
	// The guardian standing is read from committed facts, so a restart does not
	// take it away.
	if again := f17FlowGuardianChance(t, reopened, storyID, "main"); math.Abs(again-guarded) > 1e-9 {
		t.Fatalf("reopening changed the guardian's standing: %v vs %v", guarded, again)
	}
}

// containsActorID reports whether a scene's present list names this actor.
func containsActorID(present []string, actorID string) bool {
	for _, id := range present {
		if normalizeStatePanelActorID(id) == actorID {
			return true
		}
	}
	return false
}
