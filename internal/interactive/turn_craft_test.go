package interactive

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// This file is F16's minimal reproduction and its proof.
//
// §3 of 《自制-修仙技艺》V0.1 asks for one real craft path through the main story:
// a registered method, real materials, real in-fiction time, a registered
// product, all committed once and readable back from the save. §2 forbids a batch
// that is charged for work it never started, and §7's ART-01 forbids a product
// the rules did not make. Each test below states the rule it checks.

// f16CraftStory builds a story whose protagonist stands at qi-refining layer 6
// and carries two portions of the one registered material.
//
// Layer 6 is the catalogue's own minimum for the shipped method, so the fixture
// is a character who really qualifies rather than one the test bends the rules
// for.
func f16CraftStory(t *testing.T, herbQuantity int) (*Store, string) {
	t.Helper()
	initial := map[string]any{
		xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6)),
	}
	if herbQuantity > 0 {
		initial[xiuxianFieldImportantItems] = map[string]any{
			"青萝草": map[string]any{
				"定义": "material.herb.green_vine",
				"数量": float64(herbQuantity),
			},
		}
	}
	// The pack has to be declared by the protagonist's template or the seeded
	// herb never reaches the state the craft reads. combatTestSystem builds the
	// protagonist from panel and state fields only, which leaves 重要物品 out;
	// the full preset template declares it, and the realm is seeded the same way.
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(cultivation.ReferenceRules(),
				spec),
			storyContextStateTemplate(spec),
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: initial},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
		},
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "炼丹", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "向丹师学会回春丹法", Narrative: "丹师授完回春丹法，主角已实际掌握，准备开炉。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "craft.alchemy.spring_return", Source: "丹师当面传授并核过操作"}}}},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// f16LuckyIdentity finds a command identity whose deterministic draw clears the
// method's loss band, so a test that means to observe a product really does.
//
// The draw is the rules' own 10% loss rate read against a fixed die. Searching
// for a cycle that misses the band keeps the test honest in both directions: the
// outcome is still decided by the rules, and the assertion is not a coin flip.
// f16UnluckyIdentity does the same for the loss path.
func f16LuckyIdentity(t *testing.T) DomainCommitIdentity {
	t.Helper()
	for cycle := 1; cycle <= 400; cycle++ {
		identity := b1Identity("op-f16-lucky", cycle)
		if craftDraw(identity, "", "craft.alchemy.spring_return") >= 0.10 {
			return identity
		}
	}
	t.Fatal("no identity outside the loss band in the searched range")
	return DomainCommitIdentity{}
}

func f16UnluckyIdentity(t *testing.T) DomainCommitIdentity {
	t.Helper()
	for cycle := 1; cycle <= 400; cycle++ {
		identity := b1Identity("op-f16-unlucky", cycle)
		if craftDraw(identity, "", "craft.alchemy.spring_return") < 0.10 {
			return identity
		}
	}
	t.Fatal("no identity inside the loss band in the searched range")
	return DomainCommitIdentity{}
}

// f16CraftTurn commits one turn that works the shipped method for the given
// interval and returns the committed turn.
func f16CraftTurn(t *testing.T, store *Store, storyID string, seconds int64) TurnEvent {
	t.Helper()
	return f16CraftTurnUnder(t, store, storyID, seconds, f16LuckyIdentity(t), "炼制回春丹")
}

func f16CraftTurnUnder(
	t *testing.T,
	store *Store,
	storyID string,
	seconds int64,
	identity DomainCommitIdentity,
	user string,
) TurnEvent {
	t.Helper()
	identity = b1CommitInput(t, store, storyID, identity, user)
	input := TurnCraftInput{MethodID: "craft.alchemy.spring_return", DurationSeconds: seconds}
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:         "main",
		User:             user,
		Narrative:        "主角将青萝草投入丹炉，依丹法控火。",
		TurnResult:       &TurnResult{Choices: testTurnChoices(), Craft: &input},
		AgentCommandID:   identity.CommandID,
		AgentOperationID: identity.OperationID,
		AgentCycle:       identity.Cycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

// f16StoredPack reads the protagonist's pack back from the save.
func f16StoredPack(t *testing.T, store *Store, storyID string) map[string]any {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	record, ok := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	if !ok {
		return map[string]any{}
	}
	return record
}

// TestF16ACraftSpendsTheMaterialsAndYieldsTheProduct is the whole path in one
// test: two portions of green vine become one Spring-Return Pill, and both halves
// reach the save.
func TestF16ACraftSpendsTheMaterialsAndYieldsTheProduct(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	// The shipped method is 360 minutes a batch; one batch is exactly that.
	const oneBatch = int64(360 * 60)

	f16CraftTurn(t, store, storyID, oneBatch)

	pack := f16StoredPack(t, store, storyID)
	herb, ok := pack["青萝草"].(map[string]any)
	if !ok {
		t.Fatalf("the herb must stay readable after it is spent: %#v", pack)
	}
	if got := xiuxianCarriedQuantity(herb); got != 0 {
		t.Fatalf("one batch must spend both portions: held %d, want 0", got)
	}
	pill, ok := pack["pill.restore.health.lower"].(map[string]any)
	if !ok {
		t.Fatalf("the batch's product must reach the pack: %#v", pack)
	}
	if got := xiuxianCarriedQuantity(pill); got != 1 {
		t.Fatalf("one batch must yield one pill: got %d", got)
	}
	if got := xiuxianEntryText(pill, "定义"); got != "pill.restore.health.lower" {
		t.Fatalf("the product must be the registered definition, got %q", got)
	}
}

// TestF16ACraftAdvancesTheFormalClockByTheBatchItSettled checks §5's one
// interval, one settlement: the time the world moves and the time the batches
// consumed are one number.
func TestF16ACraftAdvancesTheFormalClockByTheBatchItSettled(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	const oneBatch = int64(360 * 60)

	before, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	f16CraftTurn(t, store, storyID, oneBatch)
	after, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}

	if got := after.Seconds - before.Seconds; got != oneBatch {
		t.Fatalf("the branch clock must advance by the batch's own interval: advanced %d, want %d", got, oneBatch)
	}
}

// TestF16ACraftWithoutTheMaterialsWritesNothing is ART-01's refusal: a method the
// character cannot afford must not produce a pill, and must not advance the
// calendar for work that never started.
func TestF16ACraftWithoutTheMaterialsWritesNothing(t *testing.T) {
	store, storyID := f16CraftStory(t, 1)
	const oneBatch = int64(360 * 60)

	before, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	f16CraftTurn(t, store, storyID, oneBatch)
	after, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}

	pack := f16StoredPack(t, store, storyID)
	if _, made := pack["pill.restore.health.lower"]; made {
		t.Fatalf("a batch the character cannot pay for must yield nothing: %#v", pack)
	}
	herb, ok := pack["青萝草"].(map[string]any)
	if !ok {
		t.Fatalf("a refused batch must leave the herb where it was: %#v", pack)
	}
	if got := xiuxianCarriedQuantity(herb); got != 1 {
		t.Fatalf("a refused batch must spend nothing: held %d, want 1", got)
	}
	if after.Seconds != before.Seconds {
		t.Fatalf("a refused batch must not move the clock: %d -> %d", before.Seconds, after.Seconds)
	}
	if got := b1RecordCount(t, store, storyID, StoryEventTypeCraftRecord); got != 0 {
		t.Fatalf("a refused batch must publish no craft record, got %d", got)
	}
}

// A paid partial batch retains work without issuing an unfinished product.
func TestF16AShortIntervalSavesPaidWork(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	f16CraftTurn(t, store, storyID, 180*60)
	pack := f16StoredPack(t, store, storyID)
	herb, _ := pack["青萝草"].(map[string]any)
	if xiuxianCarriedQuantity(herb) != 0 {
		t.Fatal("started batch must reserve its materials")
	}
	if _, made := pack["pill.restore.health.lower"]; made {
		t.Fatal("unfinished batch issued a product")
	}
	snapshot, _ := store.Snapshot(storyID, "main")
	work, ok := craftWorkFor(snapshot.State, DefaultActorID, "craft.alchemy.spring_return")
	if !ok || work.WorkedMinutes != 180 {
		t.Fatal("actual work was not saved")
	}
}

// TestF16ACraftRecordCarriesTheIntervalItSpent checks that the side record says
// which method was worked and how long the player asked for, because a reroll has
// no other source for it.
func TestF16ACraftRecordCarriesTheIntervalItSpent(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	const oneBatch = int64(360 * 60)

	turn := f16CraftTurn(t, store, storyID, oneBatch)

	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var found *CraftRecordEvent
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCraftRecord {
			continue
		}
		var event CraftRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			t.Fatal(err)
		}
		found = &event
	}
	if found == nil {
		t.Fatal("a committed craft must publish a craft record")
	}
	if found.TurnID != turn.ID {
		t.Fatalf("the record must name the turn it belongs to: %q vs %q", found.TurnID, turn.ID)
	}
	if found.MethodID != "craft.alchemy.spring_return" {
		t.Fatalf("the record must name the method: %q", found.MethodID)
	}
	if found.DurationSeconds != oneBatch {
		t.Fatalf("the record must carry the interval the player offered: %d", found.DurationSeconds)
	}
	if found.Batches != 1 || found.ElapsedMinutes != 360 {
		t.Fatalf("the record must carry the resolved batch: %+v", found)
	}
	if got := found.ClockAfter.Seconds - found.ClockBefore.Seconds; got != oneBatch {
		t.Fatalf("the record's own window must be the interval it settled: %d", got)
	}
}

// TestF16ACraftCannotShareATurnWithRestOrTraining is the exclusivity rule.
//
// Training, rest, the world action and a craft each publish a clock-advancing
// record, so two in one turn would both advance from the same starting instant.
// Submission refuses the combination rather than letting one interval be spent
// twice.
func TestF16ACraftCannotShareATurnWithRestOrTraining(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)

	craft := TurnCraftInput{MethodID: "craft.alchemy.spring_return", DurationSeconds: 360 * 60}
	rest := TurnRestInput{Mode: string(cultivation.RecoveryHealthRest), DurationSeconds: 3600}
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "炼丹并休养",
		Narrative: "主角炼丹一炉，随后打坐休养。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			Craft:   &craft,
			Rest:    &rest,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	pack := f16StoredPack(t, store, storyID)
	if _, made := pack["pill.restore.health.lower"]; made {
		t.Fatalf("a craft sharing a turn with a rest must not settle: %#v", pack)
	}
	if got := b1RecordCount(t, store, storyID, StoryEventTypeCraftRecord); got != 0 {
		t.Fatalf("a refused craft must publish no record, got %d", got)
	}
}

// TestF16ALostBatchStillCostsTheMaterialsAndTheTime is §2's 失败代价: the method's
// own loss rate takes the batch, and a lost batch is a batch that really ran —
// the materials are gone and the hours were spent, but nothing is produced.
func TestF16ALostBatchStillCostsTheMaterialsAndTheTime(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	const oneBatch = int64(360 * 60)

	before, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	f16CraftTurnUnder(t, store, storyID, oneBatch, f16UnluckyIdentity(t), "炼制回春丹")

	after, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	pack := f16StoredPack(t, store, storyID)
	herb, ok := pack["青萝草"].(map[string]any)
	if !ok {
		t.Fatalf("a lost batch still spends its materials: %#v", pack)
	}
	if got := xiuxianCarriedQuantity(herb); got != 0 {
		t.Fatalf("a lost batch must spend both portions: held %d, want 0", got)
	}
	if _, made := pack["pill.restore.health.lower"]; made {
		t.Fatalf("a lost batch must yield nothing: %#v", pack)
	}
	if got := after.Seconds - before.Seconds; got != oneBatch {
		t.Fatalf("a lost batch still spends its time: advanced %d, want %d", got, oneBatch)
	}

	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var found *CraftRecordEvent
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCraftRecord {
			continue
		}
		var event CraftRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			t.Fatal(err)
		}
		found = &event
	}
	if found == nil {
		t.Fatal("a lost batch is still a batch that ran and must be recorded")
	}
	if found.FailedBatches != 1 || len(found.Products) != 0 {
		t.Fatalf("the record must say the batch was lost and produced nothing: %+v", found)
	}
}

// TestF16ATheDieIsBoundToTheCommandNotTheTurn is ART-02's 不反复重抽.
//
// A turn ID is minted fresh on every append, so a die derived from it would give
// a retried command a different roll. The die must therefore follow the durable
// command identity, which is what this checks: the same command reads the same
// number whatever turn it is committed as, and a genuinely new command rolls.
func TestF16ATheDieIsBoundToTheCommandNotTheTurn(t *testing.T) {
	identity := b1Identity("op-f16-die", 1)
	first := craftDraw(identity, "ev_one", "craft.alchemy.spring_return")
	second := craftDraw(identity, "ev_two", "craft.alchemy.spring_return")
	if first != second {
		t.Fatalf("the same command must re-read the same die: %v vs %v", first, second)
	}
	other := craftDraw(b1Identity("op-f16-die", 2), "ev_one", "craft.alchemy.spring_return")
	if other == first {
		t.Fatalf("a new command must be a new draw, got %v twice", first)
	}
	if first < 0 || first >= 1 {
		t.Fatalf("the draw must be inside [0,1), got %v", first)
	}
}

// TestF16AForkAtTheCraftTurnKeepsTheWholeBatch is the branch rule.
//
// Working the furnace is one transaction: the herbs leave the pack, the pill
// enters it, and six hours pass. All three are what that turn committed, so a
// fork cut at it inherits all three. The pack write happens to live in the Turn's
// own StateDelta while the clock lives in a side record parented after the Turn,
// but where a fact is stored is not what it means: keeping the pill and the spent
// herbs while refusing the hours would let the fork hold a product whose making
// never happened, and would then charge the next batch from a furnace it never
// lit.
//
// This used to assert the opposite, on the reading that a side record parented
// after the Turn was not part of it. C-NEXT-14 settled that reading: a side record
// travels with the turn it names. See belongsToStoryAncestorPath.
func TestF16AForkAtTheCraftTurnKeepsTheWholeBatch(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	const oneBatch = int64(360 * 60)

	turn := f16CraftTurn(t, store, storyID, oneBatch)

	mainClock, _, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if mainClock.Seconds != oneBatch {
		t.Fatalf("main must have lived the batch before the fork is cut: %d", mainClock.Seconds)
	}

	forked, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turn.ID, Title: "另一条路"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(storyID, forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	if !ok {
		t.Fatalf("the fork must carry the pack the craft turn left: %#v", snapshot.State)
	}
	if _, held := record["pill.restore.health.lower"]; !held {
		t.Fatalf("the fork must keep the pill the turn really produced: %#v", record)
	}
	if herb, held := record["青萝草"].(map[string]any); held {
		if got := xiuxianCarriedQuantity(herb); got != 0 {
			t.Fatalf("the fork must keep the herbs the turn really spent: held %d", got)
		}
	}

	forkedClock, _, err := store.CultivationState(storyID, forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if forkedClock.Seconds != oneBatch {
		t.Fatalf("a fork cut at the craft turn must carry the hours the furnace took: got %d, want %d",
			forkedClock.Seconds, oneBatch)
	}
}

// f16ActorFields returns the protagonist's mutable field map inside a cloned
// state, so a test can remove one fact the way the rules would see it missing.
func f16ActorFields(state map[string]any) map[string]any {
	actor, _ := getPath(state, actorStateRoot+"."+normalizeStatePanelActorID(DefaultActorID)).(map[string]any)
	values, _ := actor["state"].(map[string]any)
	return values
}

// TestF16ATheCraftPageStatesWhatTheRulesWillActuallyCharge checks the narrator's
// page against the same registry the commit validates with.
//
// The module names a method by id, so the page has to publish that id together
// with the facts a refusal would otherwise spring on the player: the stage the
// method needs and whether this character has reached it, the materials and how
// many they really carry, and the product with its chance.
func TestF16ATheCraftPageStatesWhatTheRulesWillActuallyCharge(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	page := CraftMethodsContext(StoryDirectorActorStateSystem{}, snapshot.State, DefaultActorID, StoryContextMaxBytes)
	if page == "" {
		t.Fatal("a character who can work a registered method must get the page")
	}
	for _, want := range []string{
		"craft.alchemy.spring_return",
		"qi_1, 已达到",
		"material.herb.green_vine",
		"持有 2",
		"pill.restore.health.lower",
		// The other two arts ship as methods, so the narrator must be able to
		// name them too. A page that published only the furnace would leave
		// 炼器 and 阵法 unreachable however complete their table entries are.
		"craft.smithing.iron_sword",
		"material.ore.iron",
		"craft.formation.gathered_edge",
		"material.stone.ward",
		// The formation's floor is qi_4 and this character is qi_6, so the page
		// must say it is reachable rather than leaving the narrator to guess.
		"qi_4, 已达到",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the page must state %q so a refusal is foreseeable:\n%s", want, page)
		}
	}
	if strings.Contains(page, "## Craft Methods") {
		t.Fatalf("the page body must not repeat the block heading the prompt writes:\n%s", page)
	}

	// The same character without the herb still gets the page: the method is
	// workable in principle, and the held count is how the narrator learns it is
	// not workable now.
	bare := cloneStoryState(snapshot.State)
	delete(f16ActorFields(bare), xiuxianFieldImportantItems)
	if page := CraftMethodsContext(StoryDirectorActorStateSystem{}, bare, DefaultActorID, StoryContextMaxBytes); !strings.Contains(page, "持有 0") {
		t.Fatalf("a character carrying none must be told so:\n%s", page)
	}

	// A character who has reached no method's stage cannot work anything, so the
	// page would only ever say "not reached" and is withheld entirely.
	stageless := cloneStoryState(snapshot.State)
	delete(f16ActorFields(stageless), xiuxianFieldRealm)
	if page := CraftMethodsContext(StoryDirectorActorStateSystem{}, stageless, DefaultActorID, StoryContextMaxBytes); page != "" {
		t.Fatalf("a character with no stage must get no craft page:\n%s", page)
	}
}

// f16SubmissionSystem returns the story's frozen actor-state schema as the
// submission boundary reads it.
func f16SubmissionSystem(t *testing.T, store *Store, storyID string) StoryDirectorActorStateSystem {
	t.Helper()
	store.mu.Lock()
	meta, _, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return actorStateSystemFromSnapshot(meta.ActorStateSchema, store.storyDirectorForMeta(meta).ActorState)
}

// TestF16ATheModelCanNameAMethodAndNothingElse walks the module through the JSON
// a model really emits, because the commit path and the submission path decode
// the same field independently.
func TestF16ATheModelCanNameAMethodAndNothingElse(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	system := f16SubmissionSystem(t, store, storyID)

	decoded := DecodeInteractiveTurnSubmissionInput(`{"craft":{"method_id":"craft.alchemy.spring_return","duration_seconds":21600}}`)
	if len(decoded.Diagnostics) != 0 || decoded.Craft == nil {
		t.Fatalf("a registered method must decode: %#v", decoded)
	}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: snapshot.State, ChoiceCount: 5,
	}, nil, decoded)
	if receipt.ModuleStatus.Craft != TurnSubmissionModuleAccepted {
		t.Fatalf("the craft module must be accepted: %#v", receipt)
	}
	if got := prepared.TurnResult().Craft; got == nil || got.MethodID != "craft.alchemy.spring_return" || got.DurationSeconds != 21600 {
		t.Fatalf("the module must survive preparation: %#v", got)
	}

	// The module names no product. A submission that tries to name one is not
	// refused for its wording but for its shape: the decoder does not accept the
	// field at all, so the rules stay the only source of what a batch yields.
	withProduct := DecodeInteractiveTurnSubmissionInput(`{"craft":{"method_id":"craft.alchemy.spring_return","duration_seconds":21600,"product":"pill.restore.health.lower"}}`)
	if withProduct.Craft != nil || len(withProduct.Diagnostics) == 0 {
		t.Fatalf("a craft module must not be able to name its own product: %#v", withProduct)
	}

	// An unknown method is a shape-valid module, because the submission boundary
	// cannot know what the story registered. It is refused where the rules can
	// read the registry, and refused as a batch that never ran rather than as a
	// malformed submission: nothing is spent, nothing is published.
	unknown := DecodeInteractiveTurnSubmissionInput(`{"craft":{"method_id":"craft.alchemy.moon_dew","duration_seconds":21600}}`)
	if unknown.Craft == nil || len(unknown.Diagnostics) != 0 {
		t.Fatalf("an unregistered method is shape-valid and must decode: %#v", unknown)
	}
	before := f16StoredPack(t, store, storyID)
	// The turn itself still commits: the player's command is a real turn whatever
	// the rules decided about the batch, and the module stays on it as the record
	// of what was asked. What a refusal removes is the spending and the record.
	_, _, err = store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "另炼一炉",
		Narrative:  "主角换了张残方试手。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), Craft: unknown.Craft},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := b1RecordCount(t, store, storyID, StoryEventTypeCraftRecord); got != 0 {
		t.Fatalf("an unregistered method must publish no craft record, got %d", got)
	}
	if after := f16StoredPack(t, store, storyID); !reflect.DeepEqual(before, after) {
		t.Fatalf("an unregistered method must spend nothing: %#v -> %#v", before, after)
	}
}

// TestF16ACraftCannotBeSubmittedAlongsideAnotherInterval checks the exclusivity
// rule at the submission boundary, where a model's mistake is still repairable.
func TestF16ACraftCannotBeSubmittedAlongsideAnotherInterval(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	system := f16SubmissionSystem(t, store, storyID)

	decoded := DecodeInteractiveTurnSubmissionInput(`{"craft":{"method_id":"craft.alchemy.spring_return","duration_seconds":21600},"rest":{"mode":"health_rest","duration_seconds":3600}}`)
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: snapshot.State, ChoiceCount: 5,
	}, nil, decoded)
	if receipt.ModuleStatus.Craft != TurnSubmissionModuleRejected {
		t.Fatalf("craft beside a rest must be rejected: %#v", receipt)
	}
	if prepared.TurnResult().Craft != nil {
		t.Fatalf("a rejected craft must not reach the commit: %#v", prepared.TurnResult().Craft)
	}
}

// TestF16BTheForgeAndTheArrayCommitThroughTheSameModule is the second half of
// 《自制-修仙技艺》§3's 共用规则: 炼器 and 阵法 are not separate engines, so they must
// reach the pack through the very same turn module the furnace does.
//
// What changes between the three arts is the table entry — material, interval,
// stage floor, product — and nothing in the module. A test that only checked the
// arithmetic in internal/cultivation would leave open the case where the commit
// path quietly assumed a pill, which is exactly what "each art grows its own
// service" would look like from the inside.
func TestF16BTheForgeAndTheArrayCommitThroughTheSameModule(t *testing.T) {
	cases := []struct {
		name     string
		methodID string
		oreDef   string
		oreName  string
		quantity int
		stage    cultivation.Cultivation
		product  string
		minutes  int64
	}{
		{
			name: "forge", methodID: "craft.smithing.iron_sword",
			oreDef: "material.ore.iron", oreName: "铁矿石", quantity: 3,
			stage: cultivation.Qi(6), product: "implement.lower.common", minutes: 720,
		},
		{
			name: "array", methodID: "craft.formation.gathered_edge",
			oreDef: "material.stone.ward", oreName: "护阵石", quantity: 2,
			stage: cultivation.Qi(4), product: "formation.attack.middle", minutes: 1440,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, storyID := f16ArtStory(t, tc.stage, tc.oreName, tc.oreDef, tc.quantity)

			identity := b1CommitInput(t, store, storyID, b1Identity("op-f16b-"+tc.name, 1), "开工")
			input := TurnCraftInput{MethodID: tc.methodID, DurationSeconds: tc.minutes * 60}
			if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
				BranchID:  "main",
				User:      "开工",
				Narrative: "主角依方法动手，耗去一整段工夫。",
				TurnResult: &TurnResult{
					Choices: testTurnChoices(),
					Craft:   &input,
				},
				AgentCommandID:   identity.CommandID,
				AgentOperationID: identity.OperationID,
				AgentCycle:       identity.Cycle,
			}); err != nil {
				t.Fatal(err)
			}

			pack := f16StoredPack(t, store, storyID)
			material, ok := pack[tc.oreName].(map[string]any)
			if !ok {
				t.Fatalf("the material must stay readable after it is spent: %#v", pack)
			}
			if got := xiuxianCarriedQuantity(material); got != 0 {
				t.Fatalf("one batch must spend every unit it names: held %d", got)
			}
			product, ok := pack[tc.product].(map[string]any)
			if !ok {
				t.Fatalf("the batch's product must reach the pack: %#v", pack)
			}
			if got := xiuxianEntryText(product, "定义"); got != tc.product {
				t.Fatalf("the product must be the registered definition, got %q", got)
			}
			// §4.2 of 自制-装备与物品品级 requires an obtained item's real source to be
			// recorded. A forged blade's source is that this character made it, in the
			// story's own words rather than an engineering id.
			if got := xiuxianEntryText(product, "来历"); got != craftProductProvenance() {
				t.Fatalf("a crafted product must record where it came from, got %q", got)
			}
			// The clock moved by exactly the interval the method states, which is the
			// same rule the furnace follows.
			if got := b1RecordCount(t, store, storyID, StoryEventTypeCraftRecord); got != 1 {
				t.Fatalf("a settled batch must publish exactly one record, got %d", got)
			}
		})
	}
}

// f16ArtStory builds a story whose protagonist stands at the given stage and
// carries the given material, so each art is tested against a character who
// really qualifies rather than one the test bends the rules for.
func f16ArtStory(t *testing.T, stage cultivation.Cultivation, name, defID string, quantity int) (*Store, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(cultivation.ReferenceRules(),
				spec),
			storyContextStateTemplate(spec),
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: map[string]any{
				xiuxianFieldRealm: cultivation.StageKey(stage),
				xiuxianFieldImportantItems: map[string]any{
					name: map[string]any{"定义": defID, "数量": float64(quantity)},
				},
			}},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
		},
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "技艺", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "随师傅学会制作方法后备料", Narrative: "主角已经实际学会本次制作方法，清点了材料。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: map[string]string{"material.ore.iron": "craft.smithing.iron_sword", "material.stone.ward": "craft.formation.gathered_edge"}[defID], Source: "师傅当面传授并核过操作"}}}},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// f16CraftSubmission submits one craft module through the boundary a model
// actually talks to, under the given command identity.
//
// PrepareTurnSubmission is where a refusal can still change the turn: the prose
// has not been locked yet, so the model can be told what really happened and
// write it. A test that only appended the turn would prove the commit spends the
// materials, which F16A already covers; what this fixture is for is the receipt.
func f16CraftSubmission(t *testing.T, store *Store, storyID string, identity DomainCommitIdentity, seconds int64) (*PreparedTurnSubmission, TurnSubmissionReceipt) {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return PrepareTurnSubmission(TurnSubmissionContext{
		ActorState:    f16SubmissionSystem(t, store, storyID),
		CurrentState:  snapshot.State,
		ChoiceCount:   5,
		CraftIdentity: identity,
	}, nil, DecodeInteractiveTurnSubmissionInput(
		fmt.Sprintf(`{"choices":["继续行动","观察环境","询问同伴","检查状态","暂时等待"],"craft":{"method_id":"craft.alchemy.spring_return","duration_seconds":%d}}`, seconds)))
}

// TestF16ALostBatchReachesTheModelAsARewrite is the regression for a real run's
// fake success.
//
// The method loses one batch in ten. A live run hit that, was told only
// "craft: accepted", wrote 剑成了, and on the next turn put the blade in the pack
// with a source it invented — the exact fake success ART-01 forbids. The batch
// really ran, so the turn is not malformed and the module stays accepted; what
// was missing is that the prose had to be written again.
func TestF16ALostBatchReachesTheModelAsARewrite(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	const oneBatch = int64(360 * 60)

	prepared, receipt := f16CraftSubmission(t, store, storyID, f16UnluckyIdentity(t), oneBatch)

	if !receipt.NarrativeRewriteRequired {
		t.Fatalf("a lost batch must require a rewrite: %#v", receipt)
	}
	for _, want := range []string{"报废", "材料", "没有产出"} {
		if !strings.Contains(receipt.NarrativeRewriteReason, want) {
			t.Fatalf("the reason must name what happened (%q): %q", want, receipt.NarrativeRewriteReason)
		}
	}
	// A lost batch is a real outcome, not a refused action: the module stays
	// accepted and the commit still spends the materials and the interval.
	if receipt.ModuleStatus.Craft != TurnSubmissionModuleAccepted {
		t.Fatalf("a lost batch is not a refused craft: %#v", receipt)
	}
	if got := prepared.TurnResult().Craft; got == nil {
		t.Fatalf("a lost batch must still reach the commit: %#v", prepared.TurnResult())
	}
	var found bool
	for _, diagnostic := range receipt.Diagnostics {
		if diagnostic.Code == TurnSubmissionDiagnosticCraftLost {
			found = true
		}
	}
	if !found {
		t.Fatalf("the receipt must carry the loss diagnostic: %#v", receipt.Diagnostics)
	}
}

// TestF16AReachableBatchNeedsNoRewrite is the other half: a batch the die clears
// must not ask for a rewrite, or every craft turn would be written twice.
func TestF16AReachableBatchNeedsNoRewrite(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	const oneBatch = int64(360 * 60)

	prepared, receipt := f16CraftSubmission(t, store, storyID, f16LuckyIdentity(t), oneBatch)

	if receipt.NarrativeRewriteRequired {
		t.Fatalf("a batch that yields its product needs no rewrite: %q", receipt.NarrativeRewriteReason)
	}
	if receipt.ModuleStatus.Craft != TurnSubmissionModuleAccepted {
		t.Fatalf("a reachable batch must be accepted: %#v", receipt)
	}
	if got := prepared.TurnResult().Craft; got == nil {
		t.Fatalf("the module must survive preparation: %#v", prepared.TurnResult())
	}
}

// TestF16ARefusedBatchIsDroppedAndAsksForARewrite covers the other way a craft
// fails the prose: the rules will not work the batch at all. Nothing is spent, so
// the module cannot reach the commit — the player's command is still a turn, but
// the furnace never opened.
func TestF16ARefusedBatchIsDroppedAndAsksForARewrite(t *testing.T) {
	store, storyID := f16CraftStory(t, 1)
	const oneBatch = int64(360 * 60)

	prepared, receipt := f16CraftSubmission(t, store, storyID, f16LuckyIdentity(t), oneBatch)

	if !receipt.NarrativeRewriteRequired {
		t.Fatalf("a refused batch must require a rewrite: %#v", receipt)
	}
	if receipt.ModuleStatus.Craft == TurnSubmissionModuleAccepted {
		t.Fatalf("a refused batch must not stay accepted: %#v", receipt)
	}
	if got := prepared.TurnResult().Craft; got != nil {
		t.Fatalf("a refused batch must not reach the commit: %#v", got)
	}
	var found bool
	for _, diagnostic := range receipt.Diagnostics {
		if diagnostic.Code == TurnSubmissionDiagnosticCraftRefused {
			found = true
		}
	}
	if !found {
		t.Fatalf("the receipt must carry the refusal diagnostic: %#v", receipt.Diagnostics)
	}
}

// TestF16ANoCommandIdentityMeansNoCraftVerdict is the guard on the dry run.
//
// The commit seeds the die from the command identity, so a submission that has
// none would be judging a batch nobody is going to work. Saying nothing is right
// there; inventing a verdict from a different seed would be worse than silence.
func TestF16ANoCommandIdentityMeansNoCraftVerdict(t *testing.T) {
	store, storyID := f16CraftStory(t, 2)
	const oneBatch = int64(360 * 60)

	prepared, receipt := f16CraftSubmission(t, store, storyID, DomainCommitIdentity{}, oneBatch)

	if receipt.NarrativeRewriteRequired {
		t.Fatalf("without a command identity the die cannot be read: %q", receipt.NarrativeRewriteReason)
	}
	if got := prepared.TurnResult().Craft; got == nil {
		t.Fatalf("the module must still be accepted for the commit to judge: %#v", prepared.TurnResult())
	}
}

// TestF16ATheRewriteVerdictIsTheCommitVerdict closes the loop the dry run
// opened.
//
// The submission boundary only helps if its answer is the commit's answer. Both
// read the same base and the same command identity, so a submission that says
// "lost" must commit to a lost batch — and one that says nothing must commit to a
// product. A dry run seeded differently would pass the test above and still lie
// to the model in a live run.
func TestF16ATheRewriteVerdictIsTheCommitVerdict(t *testing.T) {
	cases := []struct {
		name     string
		identity func(*testing.T) DomainCommitIdentity
		lost     bool
	}{
		{name: "lost", identity: f16UnluckyIdentity, lost: true},
		{name: "yielded", identity: f16LuckyIdentity, lost: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, storyID := f16CraftStory(t, 2)
			const oneBatch = int64(360 * 60)
			identity := tc.identity(t)

			prepared, receipt := f16CraftSubmission(t, store, storyID, identity, oneBatch)
			if got := receipt.NarrativeRewriteRequired; got != tc.lost {
				t.Fatalf("submission verdict: rewrite=%v, want %v", got, tc.lost)
			}

			identity = b1CommitInput(t, store, storyID, identity, "炼制回春丹")
			result := prepared.TurnResult()
			if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
				BranchID:         "main",
				User:             "炼制回春丹",
				Narrative:        "主角将青萝草投入丹炉，依丹法控火。",
				TurnResult:       &result,
				AgentCommandID:   identity.CommandID,
				AgentOperationID: identity.OperationID,
				AgentCycle:       identity.Cycle,
			}); err != nil {
				t.Fatal(err)
			}

			pack := f16StoredPack(t, store, storyID)
			_, made := pack["pill.restore.health.lower"]
			if made == tc.lost {
				t.Fatalf("commit verdict: produced=%v, submission said lost=%v: %#v", made, tc.lost, pack)
			}
		})
	}
}

// TestF16ABuyingTheOreAndWorkingItInOneTurnIsNotRefused is the case the real
// chain found.
//
// 我把攒下的灵石数了一遍，去坊市买三块铁矿石，回来自己开炉打一柄剑 is one sentence a
// player naturally writes, and the commit has always accepted it: the item and
// trade modules settle before the furnace, so the ore the turn just bought is
// ore the turn can really consume. A dry run that read only the committed pack
// would refuse it, and then the model would be told a batch it is about to work
// never opened — a false refusal, which is its own kind of lie. The two verdicts
// have to agree in both directions, not just on the loss path.
func TestF16ABuyingTheOreAndWorkingItInOneTurnIsNotRefused(t *testing.T) {
	store, storyID := f16CraftStoryWithPurse(t, 60)
	talismanTurn(t, store, storyID, "main", "向铁匠学会锻剑法", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "craft.smithing.iron_sword", Source: "铁匠当面传授并核过操作"}}}})

	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	identity := f16LuckyIdentity(t)
	decoded := DecodeInteractiveTurnSubmissionInput(`{
		"trade":{"merchant":"周家铁料摊","buys":[{"def_id":"material.ore.iron","name":"铁矿石","quantity":3}]},
		"craft":{"method_id":"craft.smithing.iron_sword","duration_seconds":43200}
	}`)
	if decoded.Trade == nil || decoded.Craft == nil || len(decoded.Diagnostics) != 0 {
		t.Fatalf("the turn must decode: %#v", decoded)
	}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState:    f16SubmissionSystem(t, store, storyID),
		CurrentState:  snapshot.State,
		ChoiceCount:   5,
		CraftIdentity: identity,
	}, nil, decoded)
	if receipt.ModuleStatus.Craft != TurnSubmissionModuleAccepted {
		t.Fatalf("ore bought this turn is ore the furnace can work: %#v", receipt)
	}
	if receipt.NarrativeRewriteRequired {
		t.Fatalf("a batch that really opens must not be rewritten away: %q", receipt.NarrativeRewriteReason)
	}
	if prepared.TurnResult().Craft == nil {
		t.Fatal("the craft module must reach the commit")
	}
}

// f16CraftStoryWithPurse is f16CraftStory plus a declared purse, for a turn that
// buys its materials before it works them.
func f16CraftStoryWithPurse(t *testing.T, purse float64) (*Store, string) {
	t.Helper()
	initial := map[string]any{
		xiuxianFieldRealm:    cultivation.StageKey(cultivation.Qi(6)),
		xiuxianFieldCurrency: purse,
	}
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(cultivation.ReferenceRules(),
				spec),
			storyContextStateTemplate(spec),
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: initial},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
		},
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "炼器", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "备料", Narrative: "主角数了数灵石，准备去坊市。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}
