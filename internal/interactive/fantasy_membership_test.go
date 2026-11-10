package interactive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"ta/internal/character"
	"testing"
)

func fantasyCatalog(t *testing.T) PackCatalogSnapshot {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join("..", "storypack", "assets", "sui-guan-realms.json"))
	if e != nil {
		t.Fatal(e)
	}
	var c PackCatalogSnapshot
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	return c
}

func TestFantasyAllOrganizationsHaveRealLocalAdmission(t *testing.T) {
	catalog := fantasyCatalog(t)
	seen := map[string]bool{}
	for _, person := range catalog.Characters {
		if len(person.MembershipAdmission) == 0 {
			continue
		}
		t.Run(person.SourceKey, func(t *testing.T) {
			user := "我申请加入" + person.FactionName + "。"
			ctx := factionMembershipContext{State: character.State{Profiles: []character.Profile{{ID: "contact", Name: person.Name, SourceKey: person.SourceKey}}}, Catalog: &catalog, Scene: character.Scene{Present: []string{"contact", DefaultActorID}}, PlayerInput: user}
			facts := &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "contact", Speaker: "player", Channel: "scene", AddressedTo: "contact", Text: user}, {CharacterID: "contact", Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: "经当面商议，我接纳你的申请。"}}, Memberships: []TurnFactionMembershipInput{{FactionID: person.FactionID, Status: "formal", Statement: "有权联系人在场同意", SourceText: user, RequestExchangeIndex: 1, SourceExchangeIndex: 2}}}
			if e := validateTurnFactionMemberships(ctx, facts); e != nil {
				t.Fatal(e)
			}
			facts.Memberships[0].Status = "temporary"
			if e := validateTurnFactionMemberships(ctx, facts); e != nil {
				t.Fatal(e)
			}
			ctx.PlayerInput = "我只来问路。"
			if e := validateTurnFactionMemberships(ctx, facts); e == nil {
				t.Fatal("membership fabricated without player application")
			}
		})
		seen[person.FactionID] = true
	}
	if len(seen) != 12 {
		t.Fatalf("only %d organizations have actual authorized contacts", len(seen))
	}
}

func TestFantasyMembershipGatesGuildWorkAndPersists(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-road")
	s, id := fantasyBoundStory(t, r, a, "渡鸦桥镇")
	defer s.Close()
	state, e := s.QuestStateFor(id, "main")
	if e != nil || questAvailable(state, "sg-guild-ashriver-league") {
		t.Fatal("unaffiliated traveler received guild work", e)
	}
	snap, e := s.Snapshot(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	branch, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: snap.CurrentTurn.ID, Title: "Remain independent"})
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "拜访镇长", Narrative: "玛蕾与你在桥头见面。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{"mare"}}}, TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "mare", Name: "玛蕾·灰榆", SourceKey: "sg-p01"}}}}})
	if e != nil {
		t.Fatal(e)
	}
	user := "我申请正式加入烬河自由盟。"
	facts := &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "mare", Speaker: "player", Channel: "scene", AddressedTo: "mare", Text: user}, {CharacterID: "mare", Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: "依照商议好的条件，我接纳你为本地正式成员。"}}, Memberships: []TurnFactionMembershipInput{{FactionID: "sg-faction-ashriver-league", Status: "formal", Statement: "玛蕾当面接纳本地正式身份", SourceText: user, RequestExchangeIndex: 1, SourceExchangeIndex: 2}}}
	_, _, e = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: user, Narrative: "你提出申请，玛蕾当面说明并接纳。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if e != nil {
		t.Fatal(e)
	}
	state, e = s.QuestStateFor(id, "main")
	if e != nil || !questAvailable(state, "sg-guild-ashriver-league") {
		t.Fatal("formal member cannot see local guild work", e)
	}
	other, e := s.QuestStateFor(id, branch.ID)
	if e != nil || questAvailable(other, "sg-guild-ashriver-league") {
		t.Fatal("membership crossed branch", e)
	}
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	cold := NewStore(root)
	defer cold.Close()
	state, e = cold.QuestStateFor(id, "main")
	if e != nil || !questAvailable(state, "sg-guild-ashriver-league") {
		t.Fatal("cold restore lost membership eligibility", e)
	}
	leave := "我主动退出烬河自由盟。"
	_, _, e = cold.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: leave, Narrative: "你明确退出，原先已经发生的经历仍保留。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Memberships: []TurnFactionMembershipInput{{FactionID: "sg-faction-ashriver-league", Status: "left", Statement: "本人明确退出", SourceText: leave}}}}})
	if e != nil {
		t.Fatal(e)
	}
	state, e = cold.QuestStateFor(id, "main")
	if e != nil || questAvailable(state, "sg-guild-ashriver-league") {
		t.Fatal("left member still sees new guild work", e)
	}
}
