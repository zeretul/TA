package interactive

import (
	"reflect"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

func journeyCompanionStoryIter4(t *testing.T) (*Store, string) {
	t.Helper()
	s, id := journeyTestStory(t, "越国坊市外围散修人家")
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "在门前停下", Narrative: "小兽与看客都在门前，小兽尚未决定同行。", TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{
		xiuxianCreateUpdate("小兽", map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(1)), "出场与位置": "越国坊市外围散修人家"}),
		xiuxianCreateUpdate("看客", map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(1)), "出场与位置": "越国坊市外围散修人家"}),
		{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, "小兽", "看客"}},
	}, CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "小兽", Name: "小兽", Lifeform: "animal", Description: "一只棕色小兽，耳尖留着浅疤。"}, {CharacterID: "看客", Name: "看客", Description: "门边的过路人，提着一只竹篮。"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return s, id
}

func journeyCompanionTurnIter4() AppendTurnWithStateRequest {
	return AppendTurnWithStateRequest{BranchID: "main", User: "问小兽愿不愿同行，愿意就一起走十分钟", Narrative: "小兽向你轻鸣，主动跟到你身侧，与你沿路走了十分钟。看客留在原地。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "route_036:reverse", TargetMinutes: 10, JourneyCompanions: []JourneyCompanionInput{{CharacterID: "小兽", SourceExchangeIndex: 1}}}, CharacterFacts: &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "小兽", Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, ParticipationDecision: &character.ParticipationDecision{Kind: character.ParticipationJourney, TargetID: "route_036:reverse", Decision: character.ParticipationAccept}, Text: "（向你轻鸣，主动跟到你身侧。）"}}}}}
}

func TestJourneyCompanionSourceContinuationColdReplayAndBranchIter4(t *testing.T) {
	s, id := journeyCompanionStoryIter4(t)
	before, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "Still at home"})
	if err != nil {
		t.Fatal(err)
	}
	req := journeyCompanionTurnIter4()
	identity := DomainCommitIdentity{CommandID: "journey-companion", OperationID: "journey-companion-turn", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", req.User)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitPlayerInput(id, input); err != nil {
		t.Fatal(err)
	}
	req.AgentCommandID, req.AgentOperationID, req.AgentCycle = identity.CommandID, identity.OperationID, identity.Cycle
	first, _, err := s.AppendTurnWithState(id, req)
	if err != nil {
		t.Fatal(err)
	}
	partial, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if partial.Clock.Seconds != 600 || !reflect.DeepEqual(StoryScene(partial).Present, []string{DefaultActorID, "小兽"}) {
		t.Fatalf("partial journey brought the wrong participants: %+v", StoryScene(partial))
	}
	if place, _ := actorStateFieldValue(partial.State, "小兽", "出场与位置").(string); !strings.HasPrefix(place, "途中：") {
		t.Fatalf("actor location was not kept with the real journey: %q", place)
	}
	root := s.Root()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(root)
	t.Cleanup(func() { _ = s.Close() })
	retry, _, err := s.AppendTurnWithState(id, req)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("cold same-turn retry: %v", err)
	}
	finish := journeyTestApply(t, s, id, JourneyRequest{BranchID: "main", Arrive: true, CommandID: "continue-companion", CommandHash: "continue-companion"})
	if finish.AddedMinutes != 20 || !reflect.DeepEqual(finish.Journey.TravelerIDs, []string{DefaultActorID, "小兽"}) {
		t.Fatalf("continuation lost its frozen travelers: %+v", finish)
	}
	after, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(StoryScene(after).Present, []string{DefaultActorID, "小兽"}) || actorStateFieldValue(after.State, "小兽", "出场与位置") != "公共坊市" || actorStateFieldValue(after.State, "看客", "出场与位置") != "越国坊市外围散修人家" {
		t.Fatal("arrival location and presence disagree")
	}
	old, err := s.Snapshot(id, fork.ID)
	if err != nil || !reflect.DeepEqual(StoryScene(old), StoryScene(before)) {
		t.Fatal("travel leaked into earlier branch")
	}
	journeyTestApply(t, s, id, JourneyRequest{BranchID: "main", RouteID: "route_036:forward", Arrive: true, CommandID: "new-solo", CommandHash: "new-solo"})
	final, err := s.Snapshot(id, "main")
	if err != nil || !reflect.DeepEqual(StoryScene(final).Present, []string{DefaultActorID}) {
		t.Fatal("one trip turned into permanent party membership")
	}
}

func TestJourneyCompanionInvalidSourcesAreAtomicIter4(t *testing.T) {
	for _, name := range []string{"player command", "private", "remote", "other speaker", "other addressee", "missing source", "bystander without response", "same-turn creation", "archived", "no decision", "refusal", "different route", "later refusal"} {
		t.Run(name, func(t *testing.T) {
			s, id := journeyCompanionStoryIter4(t)
			req := journeyCompanionTurnIter4()
			switch name {
			case "no decision":
				req.TurnResult.CharacterFacts.Exchanges[0].ParticipationDecision = nil
			case "refusal":
				req.TurnResult.CharacterFacts.Exchanges[0].ParticipationDecision.Decision = character.ParticipationRefuse
			case "different route":
				req.TurnResult.CharacterFacts.Exchanges[0].ParticipationDecision.TargetID = "route_036:forward"
			case "later refusal":
				later := req.TurnResult.CharacterFacts.Exchanges[0]
				later.ParticipationDecision = &character.ParticipationDecision{Kind: character.ParticipationJourney, TargetID: "route_036:reverse", Decision: character.ParticipationRefuse}
				later.Text = "（摇头退回门边。）"
				req.TurnResult.CharacterFacts.Exchanges = append(req.TurnResult.CharacterFacts.Exchanges, later)
			case "player command":
				req.TurnResult.CharacterFacts.Exchanges[0].Speaker = "player"
			case "private":
				req.TurnResult.CharacterFacts.Exchanges[0].Channel = "private"
			case "remote":
				req.TurnResult.CharacterFacts.Exchanges[0].Channel = "message"
			case "other speaker":
				req.TurnResult.CharacterFacts.Exchanges[0].CharacterID = "看客"
			case "other addressee":
				req.TurnResult.CharacterFacts.Exchanges[0].AddressedTo = "看客"
			case "missing source":
				req.TurnResult.World.JourneyCompanions[0].SourceExchangeIndex = 0
			case "bystander without response":
				req.TurnResult.World.JourneyCompanions = append(req.TurnResult.World.JourneyCompanions, JourneyCompanionInput{CharacterID: "看客", SourceExchangeIndex: 1})
			case "same-turn creation":
				req.TurnResult.World.JourneyCompanions[0].CharacterID = "新兽"
				req.TurnResult.CharacterFacts.Exchanges[0].CharacterID = "新兽"
				req.TurnResult.StateUpdates = []interactivestate.Update{xiuxianCreateUpdate("新兽", map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(1))}), {Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, "小兽", "看客", "新兽"}}}
			case "archived":
				req.TurnResult.StateUpdates = []interactivestate.Update{{Op: interactivestate.Archive, Path: "/小兽", Value: map[string]any{"reason": "留在原地休息"}}}
			}
			before, _ := s.Snapshot(id, "main")
			if _, _, err := s.AppendTurnWithState(id, req); err == nil {
				t.Fatal("invalid participation committed")
			}
			after, _ := s.Snapshot(id, "main")
			if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) || before.CurrentTurn.ID != after.CurrentTurn.ID {
				t.Fatal("refusal moved a participant or charged time")
			}
		})
	}
}

func TestJourneyCompanionDepartureDuringTravelIsNotReversedIter4(t *testing.T) {
	for _, archive := range []bool{false, true} {
		t.Run(map[bool]string{false: "left scene", true: "archived"}[archive], func(t *testing.T) {
			s, id := journeyCompanionStoryIter4(t)
			if _, _, err := s.AppendTurnWithState(id, journeyCompanionTurnIter4()); err != nil {
				t.Fatal(err)
			}
			updates := []interactivestate.Update{{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID}}}
			if archive {
				updates = append(updates, interactivestate.Update{Op: interactivestate.Archive, Path: "/小兽", Value: map[string]any{"reason": "退出这次同行"}})
			}
			if _, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "让小兽在岔路离开", Narrative: "小兽转向岔路，不再同行。", TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: updates}}); err != nil {
				t.Fatal(err)
			}
			finish := journeyTestApply(t, s, id, JourneyRequest{Arrive: true, CommandID: "after-departure", CommandHash: "after-departure"})
			if !reflect.DeepEqual(finish.Journey.TravelerIDs, []string{DefaultActorID}) {
				t.Fatal("departed companion was carried to the destination")
			}
		})
	}
}

func TestJourneyFirstRealEncounterCanAcceptWithoutExtraTurnIter4(t *testing.T) {
	s, id := journeyTestStory(t, "越国坊市外围散修人家")
	req := journeyCompanionTurnIter4()
	req.TurnResult.StateUpdates = []interactivestate.Update{xiuxianCreateUpdate("小兽", map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(1))}), {Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{DefaultActorID, "小兽"}}}
	req.TurnResult.CharacterFacts.Profiles = []TurnCharacterProfileInput{{CharacterID: "小兽", Name: "小兽", Lifeform: "animal", Description: "一只在门边初次遇见的棕色小兽，耳尖有一道浅疤。"}}
	if _, _, err := s.AppendTurnWithState(id, req); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Snapshot(id, "main")
	if err != nil || !reflect.DeepEqual(StoryScene(snapshot).Present, []string{DefaultActorID, "小兽"}) {
		t.Fatalf("legitimate first encounter could not travel: %v", err)
	}
}
