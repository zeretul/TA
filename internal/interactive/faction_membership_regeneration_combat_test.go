package interactive

import (
	"strings"
	"testing"
)

// The battle is a real rule-resolved side record between two main turns, not
// a battle copied into the earlier turn's checkpoint.
func membershipRegenerationCombatStory(t *testing.T) (*Store, string, TurnEvent, string, *TurnCharacterFactsInput) {
	t.Helper()
	ctx, facts := membershipRuleFixture()
	system := combatTestSystem()
	sceneSystem := journeyTestSystem("黄枫谷对外入口")
	for i := range system.Templates {
		if system.Templates[i].ID == ActorStateStoryContextTemplateID {
			for _, template := range sceneSystem.Templates {
				if template.ID == ActorStateStoryContextTemplateID {
					system.Templates[i] = template
				}
			}
		}
	}
	for i := range system.InitialActors {
		if system.InitialActors[i].ID == DefaultStoryContextActorID {
			for _, actor := range sceneSystem.InitialActors {
				if actor.ID == DefaultStoryContextActorID {
					system.InitialActors[i] = actor
					system.InitialActors[i].State[storyScenePresentField] = []any{DefaultActorID, "rival"}
				}
			}
		}
		if system.InitialActors[i].ID == "rival" {
			system.InitialActors[i].Name = "钟灵道"
		}
	}
	for i := range facts.Exchanges {
		facts.Exchanges[i].CharacterID = "rival"
		if facts.Exchanges[i].AddressedTo == "掌门" {
			facts.Exchanges[i].AddressedTo = "rival"
		}
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "战斗中再生成身份", ActorState: &system, PackCatalog: ctx.Catalog})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "拜见掌门", Narrative: "钟灵道在场。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "rival", Name: "钟灵道", SourceKey: "leader_zhong_lingdao"}}}}}); err != nil {
		t.Fatal(err)
	}
	session := combatTestExchange(t, store, story.ID, "membership-combat", "membership-combat")
	if session.Ended {
		t.Fatal("the fixture battle must remain active after one real exchange")
	}
	target, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "交手尚未结束，掌门没有回答入门申请。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	return store, story.ID, target, ctx.PlayerInput, facts
}

func TestFactionMembershipRegenerationParentKeepsInterTurnCombat(t *testing.T) {
	store, id, target, _, _ := membershipRegenerationCombatStory(t)
	view, err := store.TurnWorldView(id, "main", target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if battle, ok := view.Combats["cbt-1"]; !ok || battle.Ended {
		t.Fatalf("the exact parent lost its active inter-turn battle: %+v", view.Combats)
	}
}

func TestFactionMembershipRegenerationRejectsFightingAdmitter(t *testing.T) {
	store, id, target, playerInput, facts := membershipRegenerationCombatStory(t)
	expected := target.ID
	intent, err := NewDomainCommitIntent(AppendTurnWithStateRequest{BranchID: "main", ExpectedParentID: &expected, ReplaceTurnID: target.ID, User: playerInput, Narrative: "掌门接纳了申请。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}, AgentCommandID: "membership-combat-regenerate", AgentOperationID: "membership-combat-replace", AgentCycle: 1})
	if err != nil {
		t.Fatal(err)
	}
	commitPlayerInputForTurnTest(t, store, id, intent.Request)
	if _, err := store.CommitDomainTurn(id, intent); err == nil || !strings.Contains(err.Error(), "active combat") {
		t.Fatalf("regenerated admission by a fighting contact must be rejected, got %v", err)
	}
	state, err := store.CharacterState(id, "main")
	if err != nil || len(state.FactionMemberships) != 0 {
		t.Fatalf("rejected regeneration committed membership: %+v, %v", state.FactionMemberships, err)
	}
}

func TestFactionMembershipRegenerationOnFreshForkDropsReplacedAcceptance(t *testing.T) {
	store, id := membershipStoreFixture(t)
	ctx, facts := membershipRuleFixture()
	accepted, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: ctx.PlayerInput, Narrative: "掌门接纳。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: accepted.ID, Title: "改写接纳"})
	if err != nil {
		t.Fatal(err)
	}
	expected := branch.Head
	intent, err := NewDomainCommitIntent(AppendTurnWithStateRequest{BranchID: branch.ID, ExpectedParentID: &expected, ReplaceTurnID: accepted.ID, User: ctx.PlayerInput, Narrative: "掌门尚未答应。", TurnResult: &TurnResult{Choices: testTurnChoices()}, AgentCommandID: "membership-fork-regenerate", AgentOperationID: "membership-fork-replace", AgentCycle: 1})
	if err != nil {
		t.Fatal(err)
	}
	commitPlayerInputForTurnTest(t, store, id, intent.Request)
	receipt, err := store.CommitDomainTurn(id, intent)
	if err != nil {
		t.Fatal(err)
	}
	restoreIDs := func(s *Store) string {
		t.Helper()
		s.mu.Lock()
		// Audit-only head moves need not survive the cold recent-path cache.
		// Verify persistence and retry uniqueness against the complete journal.
		_, records, err := s.readStoryJournalLocked(id)
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, record := range records {
			if record.Envelope.Type == StoryEventTypeBranchHeadMoved && record.Envelope.BranchID == branch.ID {
				ids = append(ids, record.Envelope.ID)
			}
		}
		return strings.Join(ids, ",")
	}
	firstRestoreIDs := restoreIDs(store)
	if firstRestoreIDs == "" {
		t.Fatal("fresh-fork regeneration must persist its parent restoration")
	}
	retry, err := store.CommitDomainTurn(id, intent)
	if err != nil || retry.Revision != receipt.Revision || retry.Turn.ID != receipt.Turn.ID {
		t.Fatalf("exact retry changed replacement receipt: first=%+v retry=%+v error=%v", receipt, retry, err)
	}
	if got := restoreIDs(store); got != firstRestoreIDs {
		t.Fatalf("exact retry appended another parent restoration: before=%s after=%s", firstRestoreIDs, got)
	}
	state, err := store.CharacterState(id, branch.ID)
	if err != nil || len(state.FactionMemberships) != 0 {
		t.Errorf("fork retained acceptance from its replaced turn: %+v, %v", state.FactionMemberships, err)
	}
	main, err := store.CharacterState(id, "main")
	if err != nil || len(main.FactionMemberships) != 1 || main.FactionMemberships[0].Status != "formal" {
		t.Fatalf("fork regeneration changed original branch: %+v, %v", main.FactionMemberships, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(store.root)
	t.Cleanup(func() { _ = reopened.Close() })
	state, err = reopened.CharacterState(id, branch.ID)
	if err != nil || len(state.FactionMemberships) != 0 {
		t.Errorf("cold reopen retained replaced fork membership: %+v, %v", state.FactionMemberships, err)
	}
	retry, err = reopened.CommitDomainTurn(id, intent)
	if err != nil || retry.Revision != receipt.Revision || retry.Turn.ID != receipt.Turn.ID {
		t.Fatalf("cold exact retry changed replacement receipt: first=%+v retry=%+v error=%v", receipt, retry, err)
	}
	if got := restoreIDs(reopened); got != firstRestoreIDs {
		t.Fatalf("cold exact retry appended another parent restoration: before=%s after=%s", firstRestoreIDs, got)
	}
}
