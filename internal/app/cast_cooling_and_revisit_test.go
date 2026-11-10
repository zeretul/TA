package app

import (
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

// This file is the 降温 → 再升温 half of 自制-NPC出场与升降温 §4 and §6, checked
// through the records the game already keeps:
//
//   与人物A完成一件事 → 离开做B的事 → 点名回访A
//   A暂时减少篇幅但记得往事；回访不复制Actor、关系加分或奖励
//
// "减少篇幅" is not a stored heat value and there is deliberately no new field for
// it. It is what the cast block does when it runs out of room: the person whose
// business is finished loses their full card and keeps a one-line index entry
// with their stable id, while everything already established about them stays in
// the record and is still readable at a normal budget.
//
// The fixture is F15's: a real 黄枫谷 外门弟子, the pack's own world book, and the
// ordinary turn path. The duty is taken up, delivered, and then the protagonist
// goes elsewhere and comes back to the same person by name.

// revisitStatement is what the returning turn records. It is a second, separate
// thread with the same person: coming back to someone is not the same fact as the
// work that was delivered, and the design's 不重复奖励 rule is about the first one.
const revisitStatement = "顾承川从元武国回来后，又到药园找慕沛灵，问起东畦后来的情形。"

// castCardFor returns the full card the cast block wrote for one person, or ""
// when the block only kept their index line.
func castCardFor(cast, displayName string) string {
	header := "### " + displayName + " (id: "
	start := strings.Index(cast, header)
	if start < 0 {
		return ""
	}
	rest := cast[start:]
	if end := strings.Index(rest, "\n### "); end >= 0 {
		return rest[:end]
	}
	return rest
}

// TestADeliveredDutyCoolsToAnIndexAndKeepsItsHistory is the whole 降温 rule in
// one story: after the work is delivered and the protagonist has moved on, a
// tight budget drops the 药园管事's full card and keeps her identity line; a
// normal budget still shows everything she and the protagonist went through.
func TestADeliveredDutyCoolsToAnIndexAndKeepsItsHistory(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)
	fixture.deliverTheDuty(t, original.ID)

	// The protagonist leaves and does something else. The 药园管事 is not in the
	// new scene, and her thread is settled rather than open; the person the new
	// scene does have business with carries an open thread.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main",
		User:     "我收拾行装，随商队去元武国一趟。",
		Narrative: "商队出了越国边境，走了半个多月才到元武国的坊市。" +
			"坊市东头有个替人跑腿问价的陶知远，见你初来，先问你要不要他带路。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点",
				Value: "元武国公共坊市", Reason: "本回合确认的地点",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前时间",
				Value: "第24日 午后", Reason: "本回合确认的时间",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色",
				Value: []any{interactive.DefaultActorID, "tao_zhiyuan"}, Reason: "本回合确认的在场角色",
			},
		},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"去坊市打听消息", "找间客栈住下", "在城外练功"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{{
					CharacterID: "tao_zhiyuan", Name: "陶知远",
					SourceKey:   fixture.pack.CatalogBySourceKey()["parallel_tao_zhiyuan"].SourceKey,
					Role:        "坊市里替人跑腿问价的人",
					Description: "常年在坊市东头替人问价跑腿，说话快，记性好。",
				}},
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "tao_zhiyuan", Kind: "promise", Status: "open",
					Statement:    "陶知远答应明日带顾承川去坊市里认几家收药材的铺子。",
					Counterparty: interactive.DefaultActorID,
					Deadline:     "第25日 午后",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the turn that leaves 越国: %v", err)
	}

	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after leaving: %v", err)
	}
	scene := interactive.StoryScene(interactive.Snapshot{BranchID: "main", State: map[string]any{
		"actors": map[string]any{
			interactive.DefaultStoryContextActorID: map[string]any{
				"state": map[string]any{
					"当前详细地点": "元武国公共坊市",
					"当前时间":   "第24日 午后",
					"在场角色":   []any{interactive.DefaultActorID, "tao_zhiyuan"},
				},
			},
		},
	}})
	members := character.StoryCastAt(state, scene, scene.WorldTime, 0)
	if len(members) != 2 {
		t.Fatalf("the cast holds %d people after the journey, want the 药园管事 and the man in 元武国", len(members))
	}
	// The person with live business in this scene leads; the one whose work is
	// finished and who is not here comes after.
	if members[0].Profile.ID != "tao_zhiyuan" || members[1].Profile.ID != "mu_peiling" {
		t.Fatalf("the roster does not lead with this scene's business: %s then %s",
			members[0].Profile.ID, members[1].Profile.ID)
	}
	profile := members[1].Profile
	// She is no longer present, and her thread is settled rather than open: the
	// two facts that make her the person a long story stops fully describing.
	if members[1].Present {
		t.Fatal("the 药园管事 reads as present in a 元武国 scene")
	}
	if len(members[1].OpenMatters) != 0 {
		t.Fatalf("the delivered duty is still open: %+v", members[1].OpenMatters)
	}
	if len(members[1].ResolvedMatters) == 0 {
		t.Fatal("the cast holds no settled history for her, so the help cannot be acknowledged")
	}

	// A normal budget still shows the full card, with the delivered work on it.
	full := character.CastMarkdown(members, nil, character.DefaultMaxCastBytes)
	if card := castCardFor(full, profile.DisplayName()); card == "" {
		t.Fatalf("the full cast block has no card for %s:\n%s", profile.DisplayName(), full)
	}
	if !strings.Contains(full, dutyDeliveredStatement) {
		t.Fatalf("the full cast block lost the delivered work:\n%s", full)
	}

	// A tight budget that fits only the first card drops hers and keeps her
	// identity line: 降温 means less room, not a person the narrator may not name.
	// The budget sits between the one-card and two-card sizes, the same shape the
	// cast package's own trim test uses.
	one := len([]byte(character.CastMarkdown(members[:1], nil, 1<<20)))
	two := len([]byte(full))
	if two <= one {
		t.Fatal("the fixture has no second card to trim")
	}
	budget := one + (two-one)/2
	cooled := character.CastMarkdown(members, nil, budget)
	if len([]byte(cooled)) > budget {
		t.Fatalf("the cooled block is %d bytes against a %d-byte budget", len([]byte(cooled)), budget)
	}
	if card := castCardFor(cooled, profile.DisplayName()); card != "" {
		t.Fatalf("the cooled block kept the full card:\n%s", cooled)
	}
	indexLine := "- " + profile.DisplayName() + " (id: " + profile.ID + ")"
	if !strings.Contains(cooled, indexLine) {
		t.Fatalf("the cooled block erased her instead of condensing her; want %q:\n%s", indexLine, cooled)
	}
	// Cooling is a budget fact, not a stored one: the same person at a normal
	// budget is fully described again, with the settled work still on the card.
	again := character.CastMarkdown(members, nil, character.DefaultMaxCastBytes)
	if castCardFor(again, profile.DisplayName()) == "" {
		t.Fatalf("a normal budget no longer describes her after a tight one:\n%s", again)
	}
	if !strings.Contains(again, dutyDeliveredStatement) {
		t.Fatalf("the settled work did not come back with the full card:\n%s", again)
	}
}

// TestANamedRevisitLandsOnTheSameActorAndScoresNothingNew is 再升温: the
// protagonist comes back and names her. The return must reuse the actor the
// story already has — not mint a second one — keep the earlier name, keep the
// earlier settled history, and pay nothing for restating the delivered work.
func TestANamedRevisitLandsOnTheSameActorAndScoresNothingNew(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)
	fixture.deliverTheDuty(t, original.ID)

	// The delivery turn is also where the help is paid for: one complete
	// interaction, settled once, in the same commit that closes the thread.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我查完东畦，把册子交回给慕管事。",
		Narrative: "你把东畦的病株一株株记进册子，收工时交回慕沛灵手里。她翻了一遍，点了点头。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问她明日还有什么差事", "回居所歇一歇", "去外门事务处走一趟"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "settled",
					Statement:    dutyDeliveredStatement,
					Counterparty: interactive.DefaultActorID,
					Resolves:     original.ID,
				}},
				Affinities: []interactive.TurnCharacterAffinityInput{{
					CharacterID: "mu_peiling", Kind: "meaningful", Delta: 5,
					Basis: "顾承川把东畦的病株一株株查完，收工前把册子交回慕沛灵手里。",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the delivery and its one settlement: %v", err)
	}

	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after the delivery: %v", err)
	}
	established, ok := state.Profile("mu_peiling")
	if !ok {
		t.Fatal("the story lost the 药园管事 after the delivery")
	}
	beforeAffinity := character.CurrentAffinity("mu_peiling", state.Affinities)
	if beforeAffinity == 0 {
		t.Fatal("the delivery scored nothing, so the revisit cannot be shown to add nothing")
	}
	settledBefore := len(character.ResolvedMatters("mu_peiling", state.Matters))

	// The returning turn is the shape a model produces when it reads a title
	// instead of the name: a newly minted id, the author's key copied from the
	// story's own record, no new introduction, and the delivered help described
	// again as though it were this turn's fact.
	mu := fixture.pack.CatalogBySourceKey()["npc_mu_peiling"]
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我从元武国回来了，再到药园找慕管事问问东畦的事。",
		Narrative: "慕沛灵还在药园里。见你回来，她先问了一句元武国那边的事，才说起东畦。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点",
				Value: "越国黄枫谷药园", Reason: "本回合确认的地点",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前时间",
				Value: "第26日 午后", Reason: "本回合确认的时间",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色",
				Value: []any{interactive.DefaultActorID, "mu_peiling"}, Reason: "本回合确认的在场角色",
			},
		},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问东畦后来如何", "说起元武国的见闻", "问她还有没有差事"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{{
					CharacterID: "mu_peiling_revisit", Name: "慕管事",
					SourceKey: mu.SourceKey,
				}},
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling_revisit", Kind: "action", Status: "open",
					Statement:    revisitStatement,
					Counterparty: interactive.DefaultActorID,
				}},
				Affinities: []interactive.TurnCharacterAffinityInput{{
					CharacterID: "mu_peiling_revisit", Kind: "meaningful", Delta: 7,
					Basis: "上回顾承川把东畦的病株一株株查完，收工前把册子交回给慕沛灵。",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the returning turn: %v", err)
	}

	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after the revisit: %v", err)
	}
	// One actor, not two. This is the rule §6 states as 回访不复制Actor.
	if len(state.Profiles) != 1 {
		t.Fatalf("the revisit cast a second actor for the same person: %+v", state.Profiles)
	}
	after, ok := state.Profile("mu_peiling")
	if !ok {
		t.Fatalf("the revisit moved the person off her established actor: %+v", state.Profiles)
	}
	if after.Name != established.Name {
		t.Fatalf("the revisit renamed her from %q to %q", established.Name, after.Name)
	}
	if after.SourceKey != mu.SourceKey {
		t.Fatalf("the revisit changed the author key to %q", after.SourceKey)
	}
	// The earlier history is still hers, and the return added one thread rather
	// than replacing what was already there.
	if got := len(character.ResolvedMatters("mu_peiling", state.Matters)); got != settledBefore {
		t.Fatalf("the revisit changed the settled history from %d to %d records", settledBefore, got)
	}
	if !strings.Contains(matterStatements(state.Matters), dutyDeliveredStatement) {
		t.Fatal("the revisit erased the delivered work")
	}
	if !strings.Contains(matterStatements(state.Matters), revisitStatement) {
		t.Fatal("the revisit recorded no thread of its own")
	}
	// The new thread is on the same actor, not on the id the model minted.
	for _, matter := range state.Matters {
		if strings.TrimSpace(matter.CharacterID) == "mu_peiling_revisit" {
			t.Fatalf("a thread was recorded against the minted id instead of the established actor: %+v", matter)
		}
	}

	// And nothing was paid twice. The delivered work is not a new fact, so
	// restating it changes no feeling.
	if got := character.CurrentAffinity("mu_peiling", state.Affinities); got != beforeAffinity {
		t.Fatalf("the revisit moved her feeling from %d to %d without a new fact", beforeAffinity, got)
	}
}

// TestARestatedDeliveredWorkIsNotPaidAgain is the 不重复奖励 rule stated on its
// own: the same help, described again in the returning turn, settles nothing.
func TestARestatedDeliveredWorkIsNotPaidAgain(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)

	// The delivery turn is also where the help is paid for: one complete
	// interaction, settled once, in the same commit that closes the thread.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我查完东畦，把册子交回给慕管事。",
		Narrative: "你把东畦的病株一株株记进册子，收工时交回慕沛灵手里。她翻了一遍，点了点头。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问她明日还有什么差事", "回居所歇一歇", "去外门事务处走一趟"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "settled",
					Statement:    dutyDeliveredStatement,
					Counterparty: interactive.DefaultActorID,
					Resolves:     original.ID,
				}},
				Affinities: []interactive.TurnCharacterAffinityInput{{
					CharacterID: "mu_peiling", Kind: "meaningful", Delta: 5,
					Basis: "顾承川把东畦的病株一株株查完，收工前把册子交回慕沛灵手里。",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the delivery and its one settlement: %v", err)
	}

	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	scored := character.CurrentAffinity("mu_peiling", state.Affinities)
	if scored == 0 {
		t.Fatal("the delivery scored nothing, so the fixture cannot test a repeat")
	}

	// The returning turn describes the same help again. The wording changes the way
	// a second telling of one act does — a "上回" in front, one more 给 — and the
	// settlement still recognises it as the same fact. This is the range the rule
	// promises: AffinityChange.Restates says in its own comment that it catches a
	// restatement and not a free paraphrase, and that limit is why the model is
	// shown what has already been counted.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "我跟她提起上回查东畦的事。", Narrative: "她说那本册子她还留着。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问她册子上记了什么", "说起元武国", "问她还有没有差事"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Affinities: []interactive.TurnCharacterAffinityInput{{
					CharacterID: "mu_peiling", Kind: "meaningful", Delta: 6,
					Basis: "上回顾承川把东畦的病株一株株查完，收工前把册子交回给慕沛灵。",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the restated help: %v", err)
	}

	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	if got := character.CurrentAffinity("mu_peiling", state.Affinities); got != scored {
		t.Fatalf("restating the delivered work moved her feeling from %d to %d", scored, got)
	}
	scoredRecords := 0
	for _, change := range state.Affinities {
		if change.CharacterID == "mu_peiling" {
			scoredRecords++
		}
	}
	if scoredRecords != 1 {
		t.Fatalf("the same help was recorded %d times", scoredRecords)
	}
}

// matterStatements joins every thread statement on record, so a test can ask
// whether one survived rather than which index it sits at.
func matterStatements(matters []character.Matter) string {
	var sb strings.Builder
	for _, matter := range matters {
		sb.WriteString(matter.Normalize().Statement)
		sb.WriteString("\n")
	}
	return sb.String()
}
