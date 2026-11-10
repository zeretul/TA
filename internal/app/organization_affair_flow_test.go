package app

import (
	"errors"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// This file closes 第19项's remaining question: what an organization affair
// actually authorises, whose resources it moves, and where its result comes
// from.
//
// 自制-家族与宗门经营 §5 states the acceptance in one sentence — 用一个真实组织
// 贯通「与成员商议→权限内安排一件事→经过必要时间→收到结果→继续故事」— and
// then names four things to check beside it: 无权限不能调物资、不能重复支出、
// 未执行不算完成、读档保留结果.
//
// The duty fixtures already carry the middle of that chain: an assignment handed
// over by the person who runs the 药园, kept open, lapsing in real time, and
// delivered late without being cancelled. What they never show is the money —
// whether the sect's stores can be moved by prose, whether a promised reward
// comes out of the promiser's own purse, and whether a retry pays twice.
//
// Everything here runs on records the game already keeps. No approval form, no
// expense report, no second permission system.

const (
	// The steward's own allowance, stated by her in the scene. It is her purse,
	// not the 药园's, and the rules move it from her Actor.
	orgRewardAmount = 5
	// What the protagonist's 外门月例 already holds, from the sect-disciple basis.
	orgOpeningPurse  = 40
	orgStewardActorID = "慕沛灵"
	orgRewardPromise  = "慕沛灵许下东畦查验的赏格五枚灵石，从她自己的份例里出。"
	orgRewardPaid     = "慕沛灵把东畦查验的赏格五枚灵石交到顾承川手里。"
	// The rules-owned field names, written as the story writes them. They are
	// unexported in the rules package, and the point of the assertions below is
	// that these exact strings are what the state handbook shows the model.
	orgCurrencyField  = "灵石"
	orgMerchantsField = "商贾"
	orgRealmField     = "当前境界"
)

// orgTrackSteward commits the turn in which 慕沛灵 becomes a tracked Actor with
// her own purse and promises the reward, exactly the way the model's create
// branch writes one: the actor_id is her story name, and the opening amount is
// what the scene stated.
func orgTrackSteward(t *testing.T, fixture sectDuty) interactive.TurnEvent {
	t.Helper()
	turn, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我问她这趟差事有没有赏格。",
		Narrative: "慕沛灵合上册子，说东畦查完，赏五枚灵石，从她自己的份例里出。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问清赏格怎么领", "先不接话", "直接去东畦"},
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Create, Path: "/" + orgStewardActorID,
				Value: map[string]any{
					"template_id": interactive.ActorStateImportantCharacterTemplateID,
					"name":        orgStewardActorID,
					"state": map[string]any{
						orgRealmField:    "qi_4",
						orgCurrencyField: orgRewardAmount,
					},
				},
			}},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{{
					CharacterID: "mu_peiling", Name: "慕沛灵",
					SourceKey: "npc_mu_peiling", Role: "药园管事",
				}},
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "promise", Status: "open",
					Statement:    orgRewardPromise,
					Counterparty: interactive.DefaultActorID,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("commit the promise and the steward's own purse: %v", err)
	}
	return turn
}

// orgPurse reads a tracked character's purse off the committed state, so the
// assertions are about what the rules wrote rather than what the prose said.
func orgPurse(t *testing.T, fixture sectDuty, branchID, actorID string) (int, bool) {
	t.Helper()
	snapshot, err := fixture.store.Snapshot(fixture.story.ID, branchID)
	if err != nil {
		t.Fatalf("read the branch snapshot: %v", err)
	}
	actors, _ := snapshot.State["actors"].(map[string]any)
	entry, ok := actors[actorID].(map[string]any)
	if !ok {
		return 0, false
	}
	inner, _ := entry["state"].(map[string]any)
	value, present := inner[orgCurrencyField]
	if !present {
		return 0, false
	}
	number, _ := value.(float64)
	return int(number), true
}

// TestOneOrganizationAffairRunsOnItsOwnPermissionAndPurse is 第19项's chain.
func TestOneOrganizationAffairRunsOnItsOwnPermissionAndPurse(t *testing.T) {
	fixture := startSectDutyStory(t)
	dutyTurn := fixture.takeUpTheDuty(t)

	// --- 商议：the arrangement comes from the person who runs the 药园 ---------
	orgTrackSteward(t, fixture)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	duty := dutyMatter(t, state)
	if duty.Status != character.MatterOpen {
		t.Fatalf("the assignment was not recorded as outstanding: %q", duty.Status)
	}

	// The promise is a second thread with its own kind. §5 keeps 申请、承诺和真正
	// 交付分开, so it must not be the same record as the assignment.
	promise := orgMatterByStatement(t, state, orgRewardPromise)
	if promise.Kind != character.MatterPromise || promise.Status != character.MatterOpen {
		t.Fatalf("the promised reward is %+v, want an open promise", promise)
	}
	if promise.ID == duty.ID {
		t.Fatal("the reward promise was folded into the assignment")
	}

	// --- 资源归属：the reward is her own purse, not the sect's -----------------
	if got, ok := orgPurse(t, fixture, "main", orgStewardActorID); !ok || got != orgRewardAmount {
		t.Fatalf("the steward's own purse = %d (present=%v), want the %d the scene stated", got, ok, orgRewardAmount)
	}
	if got, _ := orgPurse(t, fixture, "main", interactive.DefaultActorID); got != orgOpeningPurse {
		t.Fatalf("the protagonist's purse = %d, want the %d the identity opened with", got, orgOpeningPurse)
	}

	// --- 无权限不能调物资：prose cannot move the stores ------------------------
	//
	// The turn claims the protagonist took coin out of the garden's account and
	// recorded a shop for it. Neither is a trade the rules ran, so both must be
	// refused rather than written, and the refusal must name the field so the
	// narrator can repair the sentence.
	_, _, err = fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我从药园账上划二十枚灵石到自己名下。",
		Narrative: "你从药园的账上划走二十枚灵石，记在自己名下。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"装作没发生", "去跟慕管事说", "把灵石放回去"},
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Replace, Path: "/protagonist/" + orgCurrencyField, Value: float64(orgOpeningPurse + 20)},
				{Op: interactivestate.Replace, Path: "/story/" + orgMerchantsField,
					Value: map[string]any{"药园账房": map[string]any{"现银": 20}}},
			},
		},
	})
	var validationError *interactive.StateUpdateValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("a turn that moves the sect's coin by prose was accepted: %v", err)
	}
	if validationError.Code != "cultivation_owned_state_update" {
		t.Fatalf("the refusal code = %q, want cultivation_owned_state_update", validationError.Code)
	}
	if !strings.Contains(validationError.Path, orgCurrencyField) &&
		!strings.Contains(validationError.Path, orgMerchantsField) {
		t.Fatalf("the refusal does not name the field it refused: %q", validationError.Path)
	}
	if got, _ := orgPurse(t, fixture, "main", interactive.DefaultActorID); got != orgOpeningPurse {
		t.Fatalf("the refused turn still moved the purse to %d", got)
	}

	// --- 未执行不算完成：the promise is not the payment ------------------------
	//
	// Nothing has been handed over yet. The promise stays open and no purse has
	// moved, which is what stops a line of dialogue from counting as a wage.
	if got, _ := orgPurse(t, fixture, "main", orgStewardActorID); got != orgRewardAmount {
		t.Fatalf("the steward paid before the work was handed over: purse = %d", got)
	}
	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	if orgMatterByStatement(t, state, orgRewardPromise).Status != character.MatterOpen {
		t.Fatal("the promise was settled before any coin changed hands")
	}

	// --- 经过必要时间 --------------------------------------------------------
	secludeFor(t, fixture, 30*24*60*60)

	// --- 收到结果：the work is delivered and the promise is paid ---------------
	//
	// The delivery settles the assignment. The payment is a separate line the
	// trade rules run, and it comes out of the steward's purse because the
	// payment says who paid.
	payIdentity := interactive.DomainCommitIdentity{
		CommandID: "cmd-org-pay", OperationID: "op-org-pay", Cycle: 1,
	}
	payText := "我把东畦查完，把册子交回，她数出五枚灵石。"
	intent, err := interactive.NewPlayerInputIntent(payIdentity, "main", payText)
	if err != nil {
		t.Fatalf("form the accepted player input: %v", err)
	}
	if _, err := fixture.store.CommitPlayerInput(fixture.story.ID, intent); err != nil {
		t.Fatalf("accept the player input: %v", err)
	}
	payRequest := interactive.AppendTurnWithStateRequest{
		BranchID:         "main",
		User:             payText,
		Narrative:        "你把东畦的病株一株株记进册子，隔日交回慕沛灵手里。她翻了一遍，从袖里数出五枚灵石推过来。",
		AgentCommandID:   payIdentity.CommandID,
		AgentOperationID: payIdentity.OperationID,
		AgentCycle:       payIdentity.Cycle,
		TurnResult: &interactive.TurnResult{
			Choices: []string{"收下灵石", "问她往后还有没有差事", "回居所打坐"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "settled",
					Statement:    dutyDeliveredStatement,
					Counterparty: interactive.DefaultActorID,
					Resolves:     duty.ID,
				}, {
					CharacterID: "mu_peiling", Kind: "promise", Status: "settled",
					Statement:    orgRewardPaid,
					Counterparty: interactive.DefaultActorID,
					Resolves:     promise.ID,
				}},
			},
			Trade: &interactive.TurnTradeInput{Payments: []interactive.TurnTradePayment{{
				From: orgStewardActorID, To: interactive.DefaultActorID,
				Amount: orgRewardAmount, Reason: "东畦查验的赏格",
			}}},
		},
	}
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, payRequest); err != nil {
		t.Fatalf("commit the delivery and the promised payment: %v", err)
	}

	if got, _ := orgPurse(t, fixture, "main", orgStewardActorID); got != 0 {
		t.Fatalf("the steward's purse = %d after paying %d, want 0: the coin came from someone else's purse",
			got, orgRewardAmount)
	}
	if got, _ := orgPurse(t, fixture, "main", interactive.DefaultActorID); got != orgOpeningPurse+orgRewardAmount {
		t.Fatalf("the protagonist's purse = %d, want %d + %d", got, orgOpeningPurse, orgRewardAmount)
	}

	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after delivery: %v", err)
	}
	if delivered := dutyMatter(t, state); delivered.Status != character.MatterSettled {
		t.Fatalf("the delivered assignment is still %q", delivered.Status)
	}
	if paid := orgMatterByStatement(t, state, orgRewardPromise); paid.Status != character.MatterSettled {
		t.Fatalf("the promised reward is still %q after it was paid", paid.Status)
	}

	// --- 不能重复支出：the same command replays instead of paying twice --------
	//
	// The identical request under the same command identity is what a lost
	// response looks like. The store must hand back the committed turn and run
	// no second trade.
	paidBefore, _ := orgPurse(t, fixture, "main", orgStewardActorID)
	earnedBefore, _ := orgPurse(t, fixture, "main", interactive.DefaultActorID)
	replayed, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, payRequest)
	if err != nil {
		t.Fatalf("replaying the committed payment failed: %v", err)
	}
	if replayed.ID == "" {
		t.Fatal("the replay returned no committed turn")
	}
	if got, _ := orgPurse(t, fixture, "main", orgStewardActorID); got != paidBefore {
		t.Fatalf("the repeated payment took %d more from the steward", paidBefore-got)
	}
	if got, _ := orgPurse(t, fixture, "main", interactive.DefaultActorID); got != earnedBefore {
		t.Fatalf("the repeated payment credited the protagonist %d more", got-earnedBefore)
	}

	// A payment she cannot cover does not move her purse either. She is empty
	// now, so the same reward promised again has nothing behind it.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "她又说了一遍赏格已经给了。",
		Narrative: "慕沛灵说灵石已经给了，不必再提。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"应一声", "问下一件差事", "离开药园"},
			Trade: &interactive.TurnTradeInput{Payments: []interactive.TurnTradePayment{{
				From: orgStewardActorID, To: interactive.DefaultActorID,
				Amount: orgRewardAmount, Reason: "东畦查验的赏格",
			}}},
		},
	}); err != nil {
		t.Fatalf("commit the turn that asks her to pay again: %v", err)
	}
	if got, _ := orgPurse(t, fixture, "main", orgStewardActorID); got != 0 {
		t.Fatalf("an unfunded payment still took coin: her purse = %d", got)
	}
	if got, _ := orgPurse(t, fixture, "main", interactive.DefaultActorID); got != earnedBefore {
		t.Fatalf("an unfunded payment still credited the protagonist: %d", got)
	}

	// --- 继续故事：the next turn reads the result from the records -------------
	action := "我揣着灵石回居所。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	all := dutyMessageText(assembled)
	if strings.Contains(all, "- Unfinished with the protagonist:") {
		t.Fatalf("a delivered and paid affair is still listed as outstanding:\n%s", all)
	}
	if !strings.Contains(all, dutyDeliveredStatement) {
		t.Fatalf("the next turn lost what was actually done:\n%s", all)
	}

	// --- 读档保留结果 --------------------------------------------------------
	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopened := interactive.NewStore(fixture.workspace)
	t.Cleanup(func() { _ = reopened.Close() })
	fixture.store = reopened

	if got, _ := orgPurse(t, fixture, "main", orgStewardActorID); got != 0 {
		t.Fatalf("reopening changed the steward's purse to %d", got)
	}
	if got, _ := orgPurse(t, fixture, "main", interactive.DefaultActorID); got != orgOpeningPurse+orgRewardAmount {
		t.Fatalf("reopening changed the protagonist's purse to %d", got)
	}
	state, err = reopened.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after reopening: %v", err)
	}
	if delivered := dutyMatter(t, state); delivered.Status != character.MatterSettled {
		t.Fatalf("reopening reopened the delivered assignment: %q", delivered.Status)
	}
	if paid := orgMatterByStatement(t, state, orgRewardPromise); paid.Status != character.MatterSettled {
		t.Fatalf("reopening reopened the paid promise: %q", paid.Status)
	}

	// --- 分支：a fork cut before the payment has neither the coin nor the result
	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: dutyTurn.ID,
		Title:         "还没拿到赏格的那一支",
	})
	if err != nil {
		t.Fatalf("fork at the duty turn: %v", err)
	}
	if got, ok := orgPurse(t, fixture, branch.ID, orgStewardActorID); ok {
		t.Fatalf("a branch cut before the steward was tracked holds her purse: %d", got)
	}
	if got, _ := orgPurse(t, fixture, branch.ID, interactive.DefaultActorID); got != orgOpeningPurse {
		t.Fatalf("the fork's protagonist already holds %d, want the opening %d", got, orgOpeningPurse)
	}
	forked, err := fixture.store.CharacterState(fixture.story.ID, branch.ID)
	if err != nil {
		t.Fatalf("read the fork's character registry: %v", err)
	}
	// The promise was made after the fork point, so this branch never had it —
	// neither open nor paid.
	for _, matter := range forked.Matters {
		if strings.TrimSpace(matter.Normalize().Statement) == orgRewardPromise {
			t.Fatalf("a branch cut before the promise was made holds it: %+v", matter)
		}
	}
}

// orgMatterByStatement finds one committed thread by the sentence it states.
func orgMatterByStatement(t *testing.T, state character.State, statement string) character.Matter {
	t.Helper()
	for _, matter := range state.Matters {
		if strings.TrimSpace(matter.Normalize().Statement) == statement {
			return matter.Normalize()
		}
	}
	t.Fatalf("the story holds no thread for %q; it holds %d matters", statement, len(state.Matters))
	return character.Matter{}
}
