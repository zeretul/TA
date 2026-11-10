package interactive

import (
	"encoding/json"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/quest"
)

func membershipRuleFixture() (factionMembershipContext, *TurnCharacterFactsInput) {
	ctx := factionMembershipContext{State: character.State{Profiles: []character.Profile{{ID: "掌门", Name: "钟灵道", SourceKey: "leader_zhong_lingdao", StoryID: "story"}}}, Scene: character.Scene{Present: []string{DefaultActorID, "掌门"}}, PlayerInput: "我申请加入黄枫谷。", Catalog: &PackCatalogSnapshot{ContentVersion: 30, Characters: []PackCatalogPerson{{SourceKey: "leader_zhong_lingdao", Name: "钟灵道", FactionID: "yue.huangfeng", FactionName: "黄枫谷", MembershipAdmission: []string{"formal", "temporary"}}}}}
	facts := &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "掌门", Speaker: "player", Channel: "scene", AddressedTo: "掌门", Text: ctx.PlayerInput}, {CharacterID: "掌门", Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: "我准许你以正式弟子身份入谷。"}}, Memberships: []TurnFactionMembershipInput{{FactionID: "yue.huangfeng", Status: "formal", Statement: "掌门当面接纳为正式弟子", SourceText: ctx.PlayerInput, RequestExchangeIndex: 1, SourceExchangeIndex: 2}}}
	return ctx, facts
}
func TestFactionMembershipRejectsUnqualifiedOrUnseenAcceptance(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*factionMembershipContext, *TurnCharacterFactsInput)
	}{
		{"ordinary self-declared leader", func(c *factionMembershipContext, f *TurnCharacterFactsInput) {
			c.Catalog.Characters[0].MembershipAdmission = nil
			c.Catalog.Characters[0].Role = "掌门"
		}},
		{"legacy catalog", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { c.Catalog = nil }},
		{"unknown actor", func(c *factionMembershipContext, f *TurnCharacterFactsInput) {
			c.State.Profiles = nil
			f.Profiles = []TurnCharacterProfileInput{{CharacterID: "掌门", Name: "钟灵道", SourceKey: "leader_zhong_lingdao"}}
		}},
		{"wrong person", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { c.State.Profiles[0].Name = "冒名者" }},
		{"absent", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { c.Scene.Present = nil }},
		{"dead", func(c *factionMembershipContext, f *TurnCharacterFactsInput) {
			c.StateData = map[string]any{}
			applyActorStateOp(c.StateData, ActorStateOp{Op: "set", ActorID: "掌门", FieldID: xiuxianFieldHealth, Value: 0})
		}},
		{"busy", func(c *factionMembershipContext, f *TurnCharacterFactsInput) {
			c.StateData = map[string]any{}
			applyActorStateOp(c.StateData, ActorStateOp{Op: "set", ActorID: "掌门", FieldID: xiuxianFieldArrangement, Value: xiuxianArrangementTravel})
		}},
		{"corrupt permission", func(c *factionMembershipContext, f *TurnCharacterFactsInput) {
			c.Catalog.Characters[0].MembershipAdmission = []string{"formal", "admin"}
		}},
		{"unsupported standing", func(c *factionMembershipContext, f *TurnCharacterFactsInput) {
			c.Catalog.Characters[0].MembershipAdmission = []string{"temporary"}
		}},
		{"foreign faction", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { f.Memberships[0].FactionID = "other" }},
		{"private whisper", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { f.Exchanges[1].Channel = "private" }},
		{"unaddressed answer", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { f.Exchanges[1].AddressedTo = "" }},
		{"other recipient", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { f.Exchanges[1].AddressedTo = "路人" }},
		{"self acceptance", func(c *factionMembershipContext, f *TurnCharacterFactsInput) {
			f.Memberships[0].SourceExchangeIndex = 1
		}},
		{"answer precedes request", func(c *factionMembershipContext, f *TurnCharacterFactsInput) {
			f.Exchanges[0], f.Exchanges[1] = f.Exchanges[1], f.Exchanges[0]
			f.Memberships[0].RequestExchangeIndex = 2
			f.Memberships[0].SourceExchangeIndex = 1
		}},
		{"forged player source", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { c.PlayerInput = "我只是问路。" }},
		{"source not spoken", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { f.Exchanges[0].Text = "天色不错。" }},
		{"no source", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { f.Memberships[0].SourceText = "" }},
		{"invented status", func(c *factionMembershipContext, f *TurnCharacterFactsInput) { f.Memberships[0].Status = "leader" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, f := membershipRuleFixture()
			tc.mutate(&c, f)
			if err := validateTurnFactionMemberships(c, f); err == nil {
				t.Fatal("unearned membership accepted")
			}
		})
	}
}
func TestFactionMembershipPendingRefusalAndWithdrawal(t *testing.T) {
	ctx, facts := membershipRuleFixture()
	facts.Memberships[0].Status = "pending"
	facts.Memberships[0].SourceExchangeIndex = 1
	if err := validateTurnFactionMemberships(ctx, facts); err != nil {
		t.Fatal(err)
	}
	// A request can be heard by an ordinary named faction member, but cannot
	// become accepted merely because the reply is polite.
	ctx.Catalog.Characters[0].MembershipAdmission = nil
	if err := validateTurnFactionMemberships(ctx, facts); err != nil {
		t.Fatal(err)
	}
	facts.Memberships[0].Status = "formal"
	facts.Memberships[0].SourceExchangeIndex = 2
	if err := validateTurnFactionMemberships(ctx, facts); err == nil {
		t.Fatal("pending contact became admissions officer")
	}
	ctx, facts = membershipRuleFixture()
	ctx.State.FactionMemberships = []character.FactionMembership{{FactionID: "yue.huangfeng", FactionName: "黄枫谷", Status: "pending"}}
	facts.Memberships[0].Status = "left"
	facts.Memberships[0].SourceText = ""
	facts.Memberships[0].RequestExchangeIndex = 0
	facts.Exchanges[1].Text = "这次不予接纳。"
	if err := validateTurnFactionMemberships(ctx, facts); err != nil {
		t.Fatal(err)
	}
	ctx.PlayerInput = "我撤回申请。"
	ctx.Scene.Present = nil
	ctx.Catalog = nil
	facts.Memberships[0].SourceExchangeIndex = 0
	facts.Memberships[0].SourceText = ctx.PlayerInput
	if err := validateTurnFactionMemberships(ctx, facts); err != nil {
		t.Fatal(err)
	}
	ctx.State.FactionMemberships[0].Status = "left"
	if err := validateTurnFactionMemberships(ctx, facts); err == nil {
		t.Fatal("withdrew twice")
	}
}

func membershipStoreFixture(t *testing.T) (*Store, string) {
	t.Helper()
	ctx, _ := membershipRuleFixture()
	system := journeyTestSystem("越国坊市外围散修人家")
	for i := range system.InitialActors {
		if system.InitialActors[i].ID == DefaultStoryContextActorID {
			system.InitialActors[i].State[storyScenePresentField] = []any{DefaultActorID, "掌门"}
		}
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "身份规则", ActorState: &system, PackCatalog: ctx.Catalog})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "拜见掌门", Narrative: "钟灵道在场听你说话。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "掌门", Name: "钟灵道", SourceKey: "leader_zhong_lingdao"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}
func TestFactionMembershipProposalCommitConsumerForkAndReopen(t *testing.T) {
	store, id := membershipStoreFixture(t)
	ctx, facts := membershipRuleFixture()
	before, _ := store.CharacterState(id, "main")
	snap, _ := store.Snapshot(id, "main")
	validation := TurnSubmissionContext{KnownCharacters: before.CharacterIDs(), ExistingProfiles: before.Profiles, CurrentState: snap.State, CurrentPlayerInput: ctx.PlayerInput, Meta: StoryMeta{PackCatalog: ctx.Catalog}, Scene: StoryScene(snap), ChoiceCount: 5}
	facts.Memberships[0].SourceExchangeIndex = 1
	prepared, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{CharacterFacts: facts})
	if prepared.TurnResult().CharacterFacts != nil || len(receipt.Diagnostics) == 0 {
		t.Fatalf("invalid tool proposal accepted: %+v", receipt)
	}
	if _, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "尚未答复。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}}); err == nil {
		t.Fatal("final commit accepted invalid proposal")
	}
	after, _ := store.CharacterState(id, "main")
	if len(after.Exchanges) != len(before.Exchanges) || len(after.FactionMemberships) != 0 {
		t.Fatal("partial invalid commit")
	}
	facts.Memberships[0].SourceExchangeIndex = 2
	prepared, receipt = PrepareTurnSubmission(validation, prepared, TurnSubmissionInput{CharacterFacts: facts})
	if prepared.TurnResult().CharacterFacts == nil || len(receipt.Diagnostics) != 0 {
		t.Fatalf("valid repair refused: %+v", receipt)
	}
	turn, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "掌门当面接纳。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	after, _ = store.CharacterState(id, "main")
	m := character.CurrentFactionMemberships(after.FactionMemberships)
	if len(m) != 1 || m[0].Status != "formal" || m[0].TurnID != turn.ID || m[0].SourceExchangeID != after.Exchanges[1].ID || m[0].AuthorityID != "掌门" {
		t.Fatalf("bad formal record: %+v", m)
	}
	def := quest.Definition{ID: "member-only", RequiresFactionMembership: "yue.huangfeng"}
	p := &storyBranchProjection{Characters: &after, Quests: map[string]quest.Instance{def.ID: {}}}
	if !questActionDiscovered(p, def, QuestActionAccept) {
		t.Fatal("formal member cannot accept disclosed task")
	}
	branch, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: turn.ID, Title: "仍是弟子"})
	if err != nil {
		t.Fatal(err)
	}
	leave := &TurnCharacterFactsInput{Memberships: []TurnFactionMembershipInput{{FactionID: "yue.huangfeng", Status: "left", Statement: "主角主动退出", SourceText: "我退出黄枫谷。"}}}
	_, _, err = store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "我退出黄枫谷。", Narrative: "主角明确退出。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: leave}})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(store.root)
	t.Cleanup(func() { _ = reopened.Close() })
	current, err := reopened.CharacterState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	p.Characters = &current
	if questActionDiscovered(p, def, QuestActionAccept) || questMembershipGateOpen(p, def) {
		t.Fatal("left member still receives new task")
	}
	if !questActionDiscovered(p, def, QuestActionSettle) {
		t.Fatal("leaving confiscates previously earned settlement")
	}
	if len(current.FactionMemberships) != 2 || character.CurrentFactionMemberships(current.FactionMemberships)[0].Status != "left" {
		t.Fatalf("lost left/history: %+v", current)
	}
	fork, err := reopened.CharacterState(id, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fork.FactionMemberships) != 1 || fork.FactionMemberships[0].Status != "formal" {
		t.Fatalf("later exit leaked into fork: %+v", fork)
	}
	world, err := reopened.WorldStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(world.FactionMemberships) != 1 || world.FactionMemberships[0].Status != "left" || len(world.MembershipContacts) != 1 {
		t.Fatalf("next context lost current state: %+v", world)
	}
	encoded, _ := json.Marshal(world)
	if strings.Contains(string(encoded), "Statuses") || strings.Contains(string(encoded), "membership_admission") {
		t.Fatal("internal contact permissions leaked in public world view")
	}
}
func TestFactionMembershipTemporaryDoesNotUnlockFormalTask(t *testing.T) {
	ctx, facts := membershipRuleFixture()
	facts.Memberships[0].Status = "temporary"
	if err := validateTurnFactionMemberships(ctx, facts); err != nil {
		t.Fatal(err)
	}
	def := quest.Definition{RequiresFactionMembership: "yue.huangfeng"}
	for _, status := range []string{"pending", "temporary", "left"} {
		p := &storyBranchProjection{Characters: &character.State{FactionMemberships: []character.FactionMembership{{FactionID: "yue.huangfeng", Status: status}}}}
		if questMembershipGateOpen(p, def) {
			t.Fatalf("%s unlocked formal task", status)
		}
	}
}
func TestFactionMembershipConfirmedCreationIsJournalFact(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	req := CreateStoryRequest{Title: "已确认身份", IdentityConfirmed: true, InitialFactionMembership: &character.FactionMembership{FactionID: "yue.huangfeng", FactionName: "黄枫谷", Statement: "建角确认"}}
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(story.ID, "main")
	if err != nil || len(state.FactionMemberships) != 1 || state.FactionMemberships[0].SourceKind != "identity" {
		t.Fatalf("confirmed identity lost: %+v %v", state, err)
	}
	var decoded CreateStoryRequest
	if err = json.Unmarshal([]byte(`{"initial_faction_membership":{"faction_id":"forged"}}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.InitialFactionMembership != nil {
		t.Fatal("client minted server-side identity fact")
	}
	req.IdentityConfirmed = false
	if _, err = store.CreateStory(req); err == nil {
		t.Fatal("unconfirmed initial identity accepted")
	}
}

func TestFactionMembershipSourceAliasKeepsEstablishedAuthority(t *testing.T) {
	ctx, facts := membershipRuleFixture()
	facts.Profiles = []TurnCharacterProfileInput{{CharacterID: "钟掌门", Name: "钟灵道", SourceKey: "leader_zhong_lingdao"}}
	for i := range facts.Exchanges {
		facts.Exchanges[i].CharacterID = "钟掌门"
	}
	facts.Exchanges[0].AddressedTo = "钟掌门"
	if err := validateTurnFactionMemberships(ctx, facts); err != nil {
		t.Fatal(err)
	}
	got, err := buildTurnFactionMemberships(ctx, facts, []character.Exchange{{ID: "one"}, {ID: "two"}}, TurnEvent{ID: "turn"}, "record", "午后", 0)
	if err != nil || len(got) != 1 || got[0].AuthorityID != "掌门" {
		t.Fatalf("alias made another authority: %+v %v", got, err)
	}
}
func TestFactionMembershipRegenerationRemovesReplacedAcceptance(t *testing.T) {
	store, id := membershipStoreFixture(t)
	ctx, facts := membershipRuleFixture()
	turn, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "掌门接纳。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	characters, err := store.CharacterState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	expected := characters.Cursor
	intent, err := NewDomainCommitIntent(AppendTurnWithStateRequest{BranchID: "main", ExpectedParentID: &expected, ReplaceTurnID: turn.ID, User: ctx.PlayerInput, Narrative: "掌门尚未答应，请你先等候。", TurnResult: &TurnResult{Choices: testTurnChoices()}, AgentCommandID: "membership-regenerate", AgentOperationID: "membership-replace", AgentCycle: 1})
	if err != nil {
		t.Fatal(err)
	}
	commitPlayerInputForTurnTest(t, store, id, intent.Request)
	if _, err = store.CommitDomainTurn(id, intent); err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(id, "main")
	if err != nil || len(state.FactionMemberships) != 0 {
		t.Fatalf("replaced answer retained standing: %+v %v", state.FactionMemberships, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(store.root)
	t.Cleanup(func() { _ = reopened.Close() })
	state, err = reopened.CharacterState(id, "main")
	if err != nil || len(state.FactionMemberships) != 0 {
		t.Fatalf("cold replay revived replaced identity: %+v %v", state.FactionMemberships, err)
	}
}

func TestFactionMembershipAdmitterCannotInferExpulsionAuthority(t *testing.T) {
	ctx, facts := membershipRuleFixture()
	ctx.State.FactionMemberships = []character.FactionMembership{{FactionID: "yue.huangfeng", Status: "formal"}}
	facts.Memberships[0].Status = "left"
	facts.Memberships[0].SourceText = ""
	facts.Exchanges[1].Text = "我将你逐出。"
	if err := validateTurnFactionMemberships(ctx, facts); err == nil {
		t.Fatal("admission permission silently expanded into expulsion")
	}
}
func TestFactionMembershipRegenerationContextUsesCommittedParent(t *testing.T) {
	store, id := membershipStoreFixture(t)
	ctx, facts := membershipRuleFixture()
	turn, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "掌门接纳。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.StoryContextAtTurnParent(id, "main", turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.TurnWorldView(id, "main", turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	memberships, contacts := MembershipContextFromView(parent.Snapshot.State, view, parent.Meta.PackCatalog)
	if len(memberships) != 0 || len(contacts) != 1 {
		t.Fatalf("replacement context contains its own answer or loses earlier contact: %+v %+v", memberships, contacts)
	}
}

func TestFactionMembershipCombatBlocksFormalAnswer(t *testing.T) {
	ctx, facts := membershipRuleFixture()
	ctx.Combats = map[string]cultivation.CombatSession{"battle": {Combatants: []cultivation.Combatant{{ID: "掌门"}}}}
	if err := validateTurnFactionMemberships(ctx, facts); err == nil {
		t.Fatal("fighting actor admitted a member")
	}
}

func TestFactionMembershipCannotUseProtagonistAsAdmissionsOfficer(t *testing.T) {
	ctx, facts := membershipRuleFixture()
	ctx.State.Profiles[0].ID = DefaultActorID
	for i := range facts.Exchanges {
		facts.Exchanges[i].CharacterID = DefaultActorID
		facts.Exchanges[i].AddressedTo = DefaultActorID
	}
	if err := validateTurnFactionMemberships(ctx, facts); err == nil {
		t.Fatal("protagonist admitted themselves by borrowing an author key")
	}
}

func TestFactionMembershipFreshForkParentContextDoesNotLoseScene(t *testing.T) {
	store, id := membershipStoreFixture(t)
	ctx, facts := membershipRuleFixture()
	accepted, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "接纳。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: accepted.ID, Title: "改写"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.StoryContextAtTurnParent(id, branch.ID, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !character.ContainsID(StoryScene(parent.Snapshot).Present, "掌门") {
		t.Fatal("missing cache erased the parent's real scene")
	}
	world, err := store.WorldStateAtTurnParent(id, branch.ID, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(world.FactionMemberships) != 0 || len(world.MembershipContacts) != 1 {
		t.Fatalf("fork parent context lost contact or retained future membership: %+v", world)
	}
}
