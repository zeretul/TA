package interactive

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/npcgrowth"
)

// This file makes 《自制-NPC自主成长》 real during ordinary play.
//
// Before it, the growth rules existed as a package with no caller: an NPC's
// stage only moved when the narrative happened to rewrite it. The design's §5
// asks for the opposite — one settlement per interval of real world time, owned
// by the rules, with the NPC's own arrangement deciding the rate — and §9 asks
// that a failed commit leave no partial growth behind.
//
// Three properties are deliberate.
//
//   - There is no second clock. The interval settled here is the interval a
//     record on the branch's formal time axis really spent, and the pass runs
//     inside the same call that closes that interval's registered matters. A
//     turn that stands still settles nothing, because nothing here runs outside
//     a clock-moving commit.
//
//   - The interval identity is derived from the branch, the actor and the two
//     instants. A retry, a reopened panel and a re-emitted reroll therefore all
//     present the same identity, and the rules answer 重复 rather than granting
//     the same stretch of life a second time.
//
//   - Only facts the story already committed are submitted. Talent, method
//     grade, place and body condition are read from the actor's own state by the
//     same readers the player's cultivation path uses; a method the profile does
//     not carry is not invented, and neither is a support resource. §3's "画像
//     没有提供的积累、功法或资源不得由结算器任意补齐" is that rule.
//
// The protagonist is deliberately excluded. Their training is the player's own
// interval, committed by the cultivation and rest records; settling them here as
// well would pay the same hour twice under two names.
const StateOpSourceNPCGrowth = "npc_growth"

// NPCGrowthRecordEvent is a side record on the canonical story journal.
//
// It carries one actor's settled growth interval — the input the rules were
// asked, the candidate they answered, and the branch clock around it — without
// creating a story Turn and without consuming a turn number.
type NPCGrowthRecordEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	// TurnID is the main-story turn this interval belongs to, when the interval
	// was spent inside one. Empty when a rest or a road spent it directly.
	TurnID string `json:"turn_id,omitempty"`
	// Cursor is the canonical commit position this record was projected at.
	Cursor string `json:"cursor,omitempty"`
	// WorldTime is the story's own display text for this moment. It is a label;
	// every calculation uses ClockAfter.
	WorldTime string `json:"world_time,omitempty"`
	// Input is what the rules were asked, kept so a reader can see which facts
	// the settlement was actually sourced from rather than trusting a summary.
	Input npcgrowth.Input `json:"input"`
	// Candidate is the rules' answer. Its NextState is proposed state only; the
	// paired state delta is what makes it fact.
	Candidate npcgrowth.Candidate `json:"candidate"`
	// ClockBefore and ClockAfter are the branch's formal time axis around this
	// interval. The growth owns no clock of its own; it spends the story's.
	ClockBefore StoryClock `json:"clock_before"`
	ClockAfter  StoryClock `json:"clock_after"`
}

// npcGrowthEvents settles every tracked non-protagonist actor over one real
// interval and returns the records plus the new head.
//
// It is called from storyIntervalEvents, which every clock-moving commit already
// calls, so a journey, a rest, a training stretch and a turn all settle growth
// through the same path and no window is settled twice.
//
// settledActorID is the one actor the caller has already settled through the
// cultivation rules over exactly this interval. That actor is skipped here for
// the same reason the protagonist is: the interval already has an owner, and a
// second settlement would price the same seconds twice and let whichever record
// folds last win. Every other actor still grows on their own conditions.
func npcGrowthEvents(projection *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now, turnID, settledActorID string, clockBefore, clockAfter StoryClock) ([]any, string) {
	if system.AdventureRules != nil {
		return adventureNPCGrowthEvents(projection, system, branchID, parentID, now, turnID, settledActorID, clockBefore, clockAfter)
	}
	if projection == nil || !ActorStateSystemIsXiuxian(system) || clockAfter.Seconds <= clockBefore.Seconds {
		return nil, parentID
	}
	settledActorID = normalizeStatePanelActorID(settledActorID)
	actors, _ := projection.State[actorStateRoot].(map[string]any)
	if len(actors) == 0 {
		return nil, parentID
	}
	ids := make([]string, 0, len(actors))
	for rawID := range actors {
		actorID := normalizeStatePanelActorID(rawID)
		// The protagonist's time is the player's own interval, already committed
		// by the cultivation and rest records. Settling them here too would pay
		// the same hour twice.
		if actorID == "" || actorID == DefaultActorID {
			continue
		}
		// The story named this actor and settled their interval through the
		// cultivation rules. Those seconds already have an owner; re-pricing them
		// here is the double settlement the fixed package's journal caught.
		if settledActorID != "" && actorID == settledActorID {
			continue
		}
		ids = append(ids, actorID)
	}
	// Sorted so the committed record order is stable across replays and across
	// map iteration, which is what lets two runs of one branch produce the same
	// journal.
	sort.Strings(ids)

	var events []any
	head := parentID
	for _, actorID := range ids {
		actorEvents, actorHead := npcGrowthActorEvents(projection, system, branchID, head, now, turnID, actorID, clockBefore, clockAfter)
		events = append(events, actorEvents...)
		head = actorHead
	}
	return events, head
}

// npcGrowthMaxLegsPerWindow bounds how many settlements one actor's window can
// need. Every leg either advances the watermark or clears a decision the rules
// refuse to make twice at one instant, so the bound guards against a
// pathological answer rather than describing the normal path.
const npcGrowthMaxLegsPerWindow = 8

// xiuxianMethodUnsetText is the default the 功法与专精 field is created with. It
// is the template's "not decided yet", not a method: a character carrying it has
// recorded no method at all, and the growth rules may not read it as one.
const xiuxianMethodUnsetText = "未定"

// npcGrowthActorEvents settles one actor over one real window and returns the
// records plus the new head.
//
// One window can need several settlements. Training up to a bottleneck, the
// breakthrough attempt that spends its own days there, the recovery a failure
// imposes and the training that follows are separate intervals with separate
// identities, because the rules refuse to express a decision and the training
// around it as one span. Each step is a real interval with its own record, and
// each is chained after the writes of the step before it so a replay folds them
// in one order.
//
// Nothing is invented. Talent, method, place and body condition are read from
// the actor's own state; the attempt is made only when the rules' own
// preconditions hold and the recorded method reaches the target realm. When they
// do not hold, the actor stays at the bottleneck and the window stays unpaid —
// §6's "没有合法突破计划时停在瓶颈".
func npcGrowthActorEvents(projection *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now, turnID, actorID string, clockBefore, clockAfter StoryClock) ([]any, string) {
	// The archive is the formal "not in the story" marker. An Actor under it has
	// no body to train and no place to train in — a dead creature, a character who
	// left — so its interval is not settled. Without this the archive only hid the
	// Actor from the panels while its numbers kept climbing.
	if actorIsArchived(projection.State, actorID) {
		return nil, parentID
	}
	templateID, found := actorTemplateIDFromStateOrSystem(projection.State, system, actorID)
	if !found || !actorStateTemplateIsXiuxian(actorStateTemplateByID(system, templateID), system.CultivationRules) {
		return nil, parentID
	}
	speciesMethod := npcGrowthSpeciesMethod(projection, actorID)
	if lifeform := npcGrowthLifeform(projection, actorID); lifeform != "" && lifeform != PackCatalogLifeformHuman && speciesMethod == nil {
		// A non-human identity does not inherit human cultivation rules from a
		// shared state template. Only its fixed, registered species method can
		// qualify it.
		return nil, parentID
	}
	character, ok := xiuxianCharacterForGrowth(system.CultivationRules,
		projection.State, actorID)
	if !ok {
		return nil, parentID
	}

	// The actor's position is read from the live state, not from the watermark:
	// a rule outcome may have moved the stage since the last growth settlement,
	// and the watermark's job is only to say how much time has been paid for. The
	// breakthrough bookkeeping is the opposite — it is not derivable from any
	// actor field, so it is carried from the stored snapshot.
	watermark := npcgrowth.Snapshot{
		SourceRef:             npcGrowthActorRef(actorID, "state"),
		SettledThroughSeconds: clockBefore.Seconds,
	}
	if existing, has := projection.NPCGrowth[actorID]; has {
		watermark.Revision = existing.Revision
		watermark.SettledThroughSeconds = existing.SettledThroughSeconds
		watermark.LastIntervalID = existing.LastIntervalID
		watermark.LastIntervalHash = existing.LastIntervalHash
		watermark.Breakthrough = cloneNPCGrowthBreakthrough(existing.Breakthrough)
		watermark.Recovery = cloneNPCGrowthRecovery(existing.Recovery)
	}
	watermark.Cultivation = character.Cultivation
	watermark.Accumulated = character.Accumulated

	// An interval that begins before the watermark is truncated to it. That is
	// how a character stopped at a major boundary resumes: the rules left their
	// watermark where the stopped interval began, and paying for the same stretch
	// again would be a second grant of time the character never spent training.
	cursor := clockBefore.Seconds
	if watermark.SettledThroughSeconds > cursor {
		cursor = watermark.SettledThroughSeconds
	}
	if cursor >= clockAfter.Seconds {
		return nil, parentID
	}

	// The pools are presented on the formal stage table, not the aggregated one.
	// The breakthrough contract is written against BaseAttributes — it refuses a
	// preparation whose conditions disagree with them — and one leg cannot present
	// two bases, so every leg uses the same one.
	//
	// Presenting the formal basis is a reading, not a rewrite: an ordinary leg
	// writes no pool at all, so a character whose specialisation or equipment
	// lifts their maximum keeps that value in the journal. Only an attempt, which
	// really does spend and resolve health and mana on the formal basis, writes
	// them back.
	base := cultivation.BaseAttributes(watermark.Cultivation)
	health := npcGrowthFormalPool(character.Health, base.HealthMax)
	mana := npcGrowthFormalPool(character.Mana, base.ManaMax)

	var events []any
	head := parentID
	for leg := 0; leg < npcGrowthMaxLegsPerWindow && cursor < clockAfter.Seconds; leg++ {
		watermark.SettledThroughSeconds = cursor
		watermark.Resources = &npcgrowth.ActorPools{
			Health: health, HealthMaximum: base.HealthMax,
			Mana: mana, ManaMaximum: base.ManaMax,
		}
		if watermark.Breakthrough != nil {
			// A wound or a spent pool committed between intervals is the real
			// current value; the carried breakthrough state is re-synced to it
			// rather than overwriting it.
			watermark.Breakthrough.CurrentHealth = health
			watermark.Breakthrough.CurrentMana = mana
		}
		conditions := npcGrowthConditions(projection.State, actorID, character, health, mana, base)
		// The method the segment is priced from is also the method an attempt has
		// to cite: the breakthrough contract refuses an intent whose technique
		// source disagrees with the submitted conditions. A species method is
		// sourced from the frozen catalog entry, everything else from the actor's
		// own record, so the ref is decided once here and handed to both.
		methodRef := conditions.TechniqueRef
		if speciesMethod != nil && len(character.Techniques) == 0 {
			methodRef = npcGrowthSpeciesMethodRef(projection, actorID)
			conditions.GrowthCoefficient = speciesMethod.GrowthCoefficient
			conditions.Technique = string(cultivation.QualityOrdinary)
			conditions.TechniqueRef = methodRef
		}

		// §4's allocation table is read from the character's own committed
		// arrangement, not asserted by the settlement. A body too badly hurt to
		// train is 0% whatever the arrangement says, because the injury rules
		// already own that interval; otherwise the arrangement picks the row, and
		// an arrangement the story never established is ordinary life rather than
		// a claim of seclusion.
		activity, activityRef, opportunityRef := npcGrowthActivity(projection.State, actorID, health, base.HealthMax)
		var intent *npcgrowth.BreakthroughIntent
		switch {
		case watermark.Breakthrough != nil && watermark.Breakthrough.Pending != nil:
			// A paid preparation is already in progress. Its original conditions are
			// resubmitted so the rules can continue it; if the body or the pools
			// moved since, the rules interrupt it themselves rather than rolling.
			activity, activityRef = npcgrowth.ActivityBreakthroughPrep, npcGrowthActorRef(actorID, "arrangement/breakthrough")
			opportunityRef = ""
			pending := watermark.Breakthrough.Pending.Intent
			intent = &pending
		case watermark.Breakthrough != nil && watermark.Breakthrough.RecoveryUntilSeconds > cursor:
			// A failed attempt's recovery is still running. The breakthrough rules
			// own this interval: it grants no training and ends at the deadline.
			activity, activityRef = npcgrowth.ActivityInjuryPrevents, npcGrowthActorRef(actorID, "arrangement/breakthrough-recovery")
			opportunityRef = ""
		case cursor != npcGrowthLastAttemptAt(watermark):
			// §6 forbids repeating an attempt at the same instant. The attempt's
			// own start is the authority on that, so a leg beginning where the last
			// attempt began does not try again — even across two separate windows
			// that happen to meet at that instant.
			if attempt := npcGrowthAttemptFor(system.CultivationRules,
				projection.State, actorID, branchID, watermark, base, health, mana, cursor, clockAfter.Seconds, speciesMethod, methodRef); attempt != nil && xiuxianTemplateSupportsStage(actorStateTemplateByID(system, templateID), cultivation.StageKey(npcGrowthEnteredRealm(attempt.Target))) {
				activity, activityRef, intent = npcgrowth.ActivityBreakthroughPrep, npcGrowthActorRef(actorID, "arrangement/breakthrough"), attempt
				opportunityRef = ""
			}
		}

		input := npcgrowth.Input{
			SchemaVersion: npcgrowth.InputVersion,
			ActorID:       actorID,
			ActorSource:   npcGrowthActorRef(actorID, "actor"),
			BaseRevision:  watermark.Revision,
			State:         watermark,
			Interval: npcgrowth.Interval{
				// Derived from the branch, the actor and the two instants, so a retry,
				// a reopened window and a re-emitted reroll present one identity and
				// the rules answer 重复 instead of granting the interval again.
				ID:        npcGrowthIntervalID(branchID, actorID, cursor, clockAfter.Seconds),
				SourceRef: npcGrowthBranchRef(branchID, "interval"),
				From:      cursor,
				To:        clockAfter.Seconds,
				Segments: []npcgrowth.Segment{{
					From:                 cursor,
					To:                   clockAfter.Seconds,
					Activity:             activity,
					ActivitySourceRef:    activityRef,
					OpportunitySourceRef: opportunityRef,
					Conditions:           conditions,
				}},
			},
			BreakthroughIntent: intent,
		}

		candidate, err := npcgrowth.Calculate(input)
		if err != nil {
			// The committed facts do not meet the contract — an unreadable stage, a
			// clock that does not start at the watermark. Refusing to commit is the
			// design's "提交故障不保留半份成长": the actor simply does not grow over
			// this window rather than growing from numbers the rules rejected.
			break
		}
		if intent != nil && intent.RandomReceipt == nil && candidate.Status == npcgrowth.StatusBreakthroughReady && candidate.Breakthrough != nil {
			// The first answer priced the attempt without moving anything. That
			// preflight is where the condition fingerprint comes from, and the draw
			// is bound to the same attempt identity, so asking again with the
			// receipt is the one committed attempt rather than a second chance at
			// the die.
			paid := *intent
			paid.RandomReceipt = &npcgrowth.RandomReceipt{
				AttemptID:            intent.AttemptID,
				SourceRef:            npcGrowthActorRef(actorID, "breakthrough-random"),
				Roll:                 npcGrowthBreakthroughRoll(branchID, actorID, cursor, clockAfter.Seconds, intent.Target),
				ConditionFingerprint: candidate.Breakthrough.ConditionFingerprint,
			}
			input.BreakthroughIntent = &paid
			candidate, err = npcgrowth.Calculate(input)
			if err != nil {
				break
			}
		}
		if candidate.Status == npcgrowth.StatusDuplicate {
			// This exact interval was already settled on this branch. Re-emitting the
			// record would tell the journal a second interval happened.
			break
		}
		if !candidate.HasStateChange {
			// The rules answered without moving anything: the actor stands at a major
			// boundary and needs a decision this wiring does not make for them, or is a
			// mortal who cannot accumulate at all. Committing a record that moved
			// nothing would invent a settlement the rules did not make, so nothing is
			// written. The next interval is truncated to the unchanged watermark, so
			// the skipped window reads as still-unpaid time rather than as a gap.
			break
		}

		record := NPCGrowthRecordEvent{
			V: schemaVersion, Type: StoryEventTypeNPCGrowthRecord, ID: newID("npg"),
			ParentID: head, BranchID: branchID, Ts: now,
			TurnID:    strings.TrimSpace(turnID),
			WorldTime: storySceneText(projection.State, storySceneTimeField),
			Input:     input,
			Candidate: candidate,
			// The leg's own start instant, not the window's: the record says which
			// stretch of time it really paid for.
			ClockBefore: StoryClock{Version: clockBefore.Version, Anchor: clockBefore.Anchor, Seconds: cursor},
			ClockAfter:  clockAfter,
		}
		events = append(events, record)
		head = record.ID
		if ops := npcGrowthSettlementOps(projection.State, system, record, turnID); len(ops) > 0 {
			// The writes are a separate event parented to the record, for the same
			// reason the recovery's pool write is: an event whose ID is also its
			// parent closes the branch's parent chain into a cycle.
			deltaID := newID("sd")
			events = append(events, newStateDeltaEventWithActorOps(deltaID, record.ID, branchID, now, nil, ops))
			head = deltaID
		}

		if candidate.ConsumedThrough <= cursor || candidate.ConsumedThrough > clockAfter.Seconds {
			break
		}
		cursor = candidate.ConsumedThrough
		watermark.Cultivation = candidate.NextState.Cultivation
		watermark.Accumulated = candidate.NextState.Accumulated
		watermark.Revision = candidate.NextState.Revision
		watermark.LastIntervalID = candidate.NextState.LastIntervalID
		watermark.LastIntervalHash = candidate.NextState.LastIntervalHash
		watermark.Breakthrough = cloneNPCGrowthBreakthrough(candidate.NextState.Breakthrough)
		watermark.Recovery = cloneNPCGrowthRecovery(candidate.NextState.Recovery)
		base = cultivation.BaseAttributes(watermark.Cultivation)
		if candidate.NextState.Resources != nil {
			health, mana = candidate.NextState.Resources.Health, candidate.NextState.Resources.Mana
		}
	}
	return events, head
}

func npcGrowthCatalogPerson(projection *storyBranchProjection, actorID string) (*PackCatalogPerson, bool) {
	if projection == nil || projection.PackCatalog == nil || projection.Characters == nil {
		return nil, false
	}
	profile, ok := projection.Characters.Profile(actorID)
	if !ok || strings.TrimSpace(profile.SourceKey) == "" {
		return nil, false
	}
	for i := range projection.PackCatalog.Characters {
		person := &projection.PackCatalog.Characters[i]
		if person.SourceKey == profile.SourceKey {
			return person, true
		}
	}
	return nil, false
}

func npcGrowthSpeciesMethod(projection *storyBranchProjection, actorID string) *cultivation.TechniqueDef {
	methodID := ""
	if projection != nil && projection.Characters != nil {
		if profile, ok := projection.Characters.Profile(actorID); ok && profile.Lifeform != "" {
			if profile.Lifeform != PackCatalogLifeformSpiritBeast {
				return nil
			}
			methodID = profile.CultivationMethodID
		}
	}
	if methodID == "" {
		person, ok := npcGrowthCatalogPerson(projection, actorID)
		if !ok || person.Lifeform != PackCatalogLifeformSpiritBeast {
			return nil
		}
		methodID = person.CultivationMethodID
	}
	if methodID == "" || !packCatalogAllowsSpeciesMethod(projection, methodID) {
		return nil
	}
	method, ok := cultivation.TechniqueByID(methodID, projection.cultivationRules())
	if !ok {
		return nil
	}
	return &method
}

func npcGrowthLifeform(projection *storyBranchProjection, actorID string) string {
	if projection != nil && projection.Characters != nil {
		if profile, ok := projection.Characters.Profile(actorID); ok && profile.Lifeform != "" {
			return profile.Lifeform
		}
	}
	if person, ok := npcGrowthCatalogPerson(projection, actorID); ok {
		return person.Lifeform
	}
	return ""
}

func npcGrowthSpeciesMethodRef(projection *storyBranchProjection, actorID string) string {
	if projection != nil && projection.Characters != nil {
		if profile, ok := projection.Characters.Profile(actorID); ok && profile.CultivationMethodID != "" {
			return "character_profile:" + profile.ID + "/cultivation_method"
		}
	}
	return npcGrowthCatalogRef(projection.Characters, actorID)
}

func packCatalogAllowsSpeciesMethod(projection *storyBranchProjection, methodID string) bool {
	if projection == nil || projection.PackCatalog == nil {
		return false
	}
	for _, person := range projection.PackCatalog.Characters {
		if person.Lifeform == PackCatalogLifeformSpiritBeast && person.CultivationMethodID == methodID {
			return true
		}
	}
	return false
}

func npcGrowthCatalogRef(characters *character.State, actorID string) string {
	if characters == nil {
		return npcGrowthActorRef(actorID, "method")
	}
	profile, ok := characters.Profile(actorID)
	if !ok || profile.SourceKey == "" {
		return npcGrowthActorRef(actorID, "method")
	}
	return "pack_catalog:" + profile.SourceKey + "/cultivation_method"
}

// npcGrowthConditions is the sourced condition set for one leg.
//
// It is rebuilt from the actor's own state each time rather than carried from
// the previous leg, because the two facts that change inside a window — the
// current pools and the stage the maxima come from — are exactly the ones the
// breakthrough contract checks.
func npcGrowthConditions(state map[string]any, actorID string, character cultivation.CharacterState, health, mana int, base cultivation.Attributes) npcgrowth.Conditions {
	growthCoefficient := 0.0
	if character.TechniquesStructured {
		growthCoefficient = character.GrowthCoefficient("")
	}
	return npcgrowth.Conditions{
		SourceRef:         npcGrowthActorRef(actorID, "conditions"),
		Talent:            string(xiuxianTalentFromState(state, actorID)),
		Technique:         string(npcGrowthTechniqueQuality(character)),
		TechniqueRef:      npcGrowthActorRef(actorID, "method"),
		GrowthCoefficient: growthCoefficient,
		Environment:       string(npcGrowthEnvironment(state, actorID)),
		Resource:          string(cultivation.SupportNone),
		CurrentHealth:     health,
		MaximumHealth:     base.HealthMax,
		CurrentMana:       mana,
		MaximumMana:       base.ManaMax,
	}
}

// npcGrowthEnvironment is the spiritual quality of the ground this character
// trains on.
//
// It is the character's own ground, not the protagonist's. §三 is explicit: "NPC
// 自己的正式位置／安排有来源时读该来源；主角场景不能当远处NPC的地点。明确同场者可用
// 该场景条件。位置不明只使用已成立的保守普通条件，不凭缺省生成灵地加成."
//
// Two sources are legitimate, in this order:
//
//   - the scene, but only for someone the scene itself lists in 在场角色. That
//     list is the one committed signal that a character is standing where the
//     protagonist is; it is the same list combat reads to decide who is fighting.
//   - otherwise nothing: a distant character's ground is not recorded anywhere
//     the rules can read. The prose fields 出场与位置 and 存续与位置 are
//     deliberately not parsed — the combat encounter reader refuses to guess at
//     prose for the same reason — so the honest answer is ordinary ground, which
//     claims no bonus rather than a wrong one.
func npcGrowthEnvironment(state map[string]any, actorID string) cultivation.Environment {
	if npcGrowthSharesScene(state, actorID) {
		return xiuxianTrainingEnvironment(state)
	}
	return cultivation.EnvironmentOrdinary
}

// npcGrowthSharesScene reports whether the scene lists this character as present.
//
// The comparison goes through the same normalization the combat encounter uses,
// so an Actor ID written with different spacing or case still matches the list.
func npcGrowthSharesScene(state map[string]any, actorID string) bool {
	wanted := normalizeStatePanelActorID(actorID)
	if wanted == "" || wanted == DefaultActorID {
		return false
	}
	for _, rawID := range storySceneFieldList(storyContextActorState(state), storyScenePresentField) {
		if normalizeStatePanelActorID(rawID) == wanted {
			return true
		}
	}
	return false
}

// npcGrowthActivity reads §4's allocation row from the character's own committed
// arrangement, and reports the source the rules require for each row.
//
// The order matters. A body that cannot train is 0% whatever the arrangement
// says, because the injury rules already own that interval and the design forbids
// stacking a second training on top of it. Past that, the arrangement decides:
// 未定 is ordinary life, which §三 permits as the conservative default for a
// character whose plan the story has not established.
func npcGrowthActivity(state map[string]any, actorID string, health, healthMax int) (npcgrowth.Activity, string, string) {
	if healthFraction(health, healthMax) < 0.30 {
		return npcgrowth.ActivityInjuryPrevents, npcGrowthActorRef(actorID, "arrangement/injury"), ""
	}
	arrangement, _ := actorStateFieldValue(state, actorID, xiuxianFieldArrangement).(string)
	switch strings.TrimSpace(arrangement) {
	case xiuxianArrangementSeclusion:
		return npcgrowth.ActivityFocusedSeclusion, npcGrowthActorRef(actorID, "arrangement/seclusion"), ""
	case xiuxianArrangementTravel:
		// 20% is paid only "且确有安全适用机会". The window is a real fact about
		// this character's outing; without one the interval is an outing that
		// trains not at all, rather than an opportunity the settlement invents.
		if window := npcGrowthTrainingWindow(state, actorID); window != "" {
			return npcgrowth.ActivityTravelOpportunity, npcGrowthActorRef(actorID, "arrangement/travel"), window
		}
		return npcgrowth.ActivityTravelNoWindow, npcGrowthActorRef(actorID, "arrangement/travel-without-window"), ""
	case xiuxianArrangementOccupied:
		return npcgrowth.ActivityOccupied, npcGrowthActorRef(actorID, "arrangement/occupied"), ""
	case xiuxianArrangementRecovering:
		// §4's 0% row for a character resting under the formal recovery rules.
		// Health and mana already recover through the recovery path; this row
		// exists so the training interval does not quietly add a second one.
		return npcgrowth.ActivityInjuryPrevents, npcGrowthActorRef(actorID, "arrangement/recovering"), ""
	default:
		// 未定, 正常生活兼修行, and any value the schema could not have accepted:
		// all of them are "living their life and training when they can".
		return npcgrowth.ActivityOrdinaryTraining, npcGrowthActorRef(actorID, "arrangement/ordinary"), ""
	}
}

// npcGrowthTrainingWindow is the established safe window an outing really offers.
//
// It is returned as a source reference rather than the prose itself, because the
// rules contract wants a stable identity for the opportunity and the prose is
// only its explanation. An empty window yields no reference, which is what makes
// the outing unpriced.
func npcGrowthTrainingWindow(state map[string]any, actorID string) string {
	window, _ := actorStateFieldValue(state, actorID, xiuxianFieldTrainingWindow).(string)
	if strings.TrimSpace(window) == "" {
		return ""
	}
	return npcGrowthActorRef(actorID, "arrangement/travel-window")
}

// npcGrowthAttemptFor is the actor's own sourced decision to attempt the
// breakthrough its position allows, or nil when no legal attempt exists.
//
// §6 lists what the program verifies: real accumulation, a suitable method,
// health and mana, recovery state, resources, time, and the character's own
// intent. Every one of those is read from committed facts here, and a missing
// condition yields nil rather than a roll — "缺条件不掷骰、不先扣资源".
//
// The intent is the NPC's, not the player's. §5 is explicit that an NPC decides
// for itself and needs no approval; what is shared with the player is the
// domain rules, not the authorship of the decision.
//
// methodRef is the same technique source the interval's conditions carry. The
// breakthrough contract compares the two, so an intent that named a different
// source would be refused however well-sourced it was — which is exactly what
// happened to a species method, whose conditions cite the frozen catalog entry
// while the actor has no method record of its own.
func npcGrowthAttemptFor(cultivationRules *cultivation.Rules, state map[string]any, actorID, branchID string, watermark npcgrowth.Snapshot, base cultivation.Attributes, health, mana int, from, to int64, speciesMethod *cultivation.TechniqueDef, methodRef string) *npcgrowth.BreakthroughIntent {
	target, ok := npcGrowthBreakthroughTarget(watermark.Cultivation)
	if !ok || !npcGrowthSupportsTarget(cultivationRules,
		state, actorID, target, speciesMethod) {
		return nil
	}
	attrs := base
	attrs.Health, attrs.Mana = health, mana
	convalescing := watermark.Breakthrough != nil && watermark.Breakthrough.RecoveryUntilSeconds > from
	if !cultivation.CanAttemptBreakthrough(watermark.Cultivation, watermark.Accumulated, attrs, target, convalescing).Eligible {
		return nil
	}
	return &npcgrowth.BreakthroughIntent{
		// Derived from the branch, the actor, the window and the target, so a retry
		// of the same window presents the same attempt rather than a fresh gamble.
		AttemptID:               npcGrowthAttemptID(branchID, actorID, from, to, target),
		SourceRef:               npcGrowthActorRef(actorID, "breakthrough-intent"),
		Target:                  target,
		TechniqueSupportsTarget: true,
		TechniqueSourceRef:      methodRef,
		ApplicabilitySourceRef:  npcGrowthActorRef(actorID, "breakthrough-applicability"),
	}
}

// npcGrowthBreakthroughTarget is the realm a character at a major boundary may
// attempt to enter.
func npcGrowthBreakthroughTarget(c cultivation.Cultivation) (cultivation.BreakthroughTarget, bool) {
	switch {
	case c.Realm == cultivation.RealmQi && c.Layer == cultivation.QiLayers:
		return cultivation.BreakthroughFoundation, true
	case c.Realm == cultivation.RealmFoundation && c.Stage == cultivation.StageLate:
		return cultivation.BreakthroughCore, true
	case c.Realm == cultivation.RealmCore && c.Stage == cultivation.StageLate:
		return cultivation.BreakthroughNascent, true
	case c.Realm == cultivation.RealmNascent && c.Stage == cultivation.StageLate:
		return cultivation.BreakthroughSpirit, true
	case c.Realm == cultivation.RealmSpirit && c.Stage == cultivation.StageLate:
		return cultivation.BreakthroughVoid, true
	case c.Realm == cultivation.RealmVoid && c.Stage == cultivation.StageLate:
		return cultivation.BreakthroughIntegration, true
	case c.Realm == cultivation.RealmIntegration && c.Stage == cultivation.StageLate:
		return cultivation.BreakthroughMahayana, true
	}
	return "", false
}

// npcGrowthSupportsTarget reports whether the character's recorded method
// carries them into the target realm.
//
// The answer comes from the rules' own ceiling, never from an assertion. A
// structured method set answers with the highest SupportsThrough among the
// methods actually circulating; a character whose record predates structured
// methods is read through the free-text field, and since every method the rules
// publish reaches foundation late, that is the ceiling the prose carries. A
// character with no recorded method — the default "未定" — supports nothing,
// which is §3's "画像没有提供的功法不得由结算器任意补齐".
func npcGrowthSupportsTarget(cultivationRules *cultivation.Rules, state map[string]any, actorID string, target cultivation.BreakthroughTarget, speciesMethod *cultivation.TechniqueDef) bool {
	ceiling, ok := npcGrowthMethodCeiling(cultivationRules,
		state, actorID, speciesMethod)
	if !ok {
		return false
	}
	entered := npcGrowthEnteredRealm(target)
	if entered.Realm == "" {
		return false
	}
	return stageReaches(ceiling, cultivation.StageKey(entered))
}

// npcGrowthMethodCeiling is the furthest stage the character's method supports.
func npcGrowthMethodCeiling(cultivationRules *cultivation.Rules, state map[string]any, actorID string, speciesMethod *cultivation.TechniqueDef) (cultivation.Cultivation, bool) {
	if records, ok := readTechniqueRecords(state, actorID); ok && len(records) > 0 {
		// A record that holds entries is authoritative even when nothing
		// circulates: the character deliberately stopped every method, and
		// falling back to the prose would resurrect a method they put down.
		best, found := cultivation.Cultivation{}, false
		for _, definition := range circulatingTechniques(cultivationRules,
			records, state) {
			ceiling, ok := cultivation.ParseStageKey(definition.SupportsThrough, cultivationRules)
			if !ok {
				continue
			}
			if !found || stageReaches(ceiling, cultivation.StageKey(best)) {
				best, found = ceiling, true
			}
		}
		return best, found
	}
	if speciesMethod != nil {
		return cultivation.ParseStageKey(speciesMethod.SupportsThrough, cultivationRules)
	}
	method, _ := actorStateFieldValue(state, actorID, xiuxianFieldMethod).(string)
	method = strings.TrimSpace(method)
	if method == "" || method == xiuxianMethodUnsetText {
		return cultivation.Cultivation{}, false
	}
	return cultivation.ParseStageKey("foundation_late", cultivationRules)
}

// npcGrowthEnteredRealm is the early stage of the realm a target opens.
func npcGrowthEnteredRealm(target cultivation.BreakthroughTarget) cultivation.Cultivation {
	switch target {
	case cultivation.BreakthroughFoundation:
		return cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}
	case cultivation.BreakthroughCore:
		return cultivation.Cultivation{Realm: cultivation.RealmCore, Stage: cultivation.StageEarly}
	case cultivation.BreakthroughNascent:
		return cultivation.Cultivation{Realm: cultivation.RealmNascent, Stage: cultivation.StageEarly}
	case cultivation.BreakthroughSpirit:
		return cultivation.Cultivation{Realm: cultivation.RealmSpirit, Stage: cultivation.StageEarly}
	case cultivation.BreakthroughVoid:
		return cultivation.Cultivation{Realm: cultivation.RealmVoid, Stage: cultivation.StageEarly}
	case cultivation.BreakthroughIntegration:
		return cultivation.Cultivation{Realm: cultivation.RealmIntegration, Stage: cultivation.StageEarly}
	case cultivation.BreakthroughMahayana:
		return cultivation.Cultivation{Realm: cultivation.RealmMahayana, Stage: cultivation.StageEarly}
	}
	return cultivation.Cultivation{}
}

// npcGrowthAttemptID is the stable identity of one attempt over one window.
func npcGrowthAttemptID(branchID, actorID string, from, to int64, target cultivation.BreakthroughTarget) string {
	return fmt.Sprintf("npcbt:%s:%s:%s:%d-%d", branchID, actorID, target, from, to)
}

// npcGrowthBreakthroughRoll derives the draw from the attempt identity.
//
// §6 requires the random result of an attempt to be bound to a stable identity
// and recorded, so a refresh, a retry or a reopened window cannot re-roll. A
// genuinely new attempt is a new window and therefore a new identity, which is
// what makes it a real gamble rather than a replay of the last one.
func npcGrowthBreakthroughRoll(branchID, actorID string, from, to int64, target cultivation.BreakthroughTarget) float64 {
	sum := sha256.Sum256([]byte(npcGrowthAttemptID(branchID, actorID, from, to, target) + "\x00npc-breakthrough"))
	return float64(binary.BigEndian.Uint64(sum[:8])) / float64(1<<64)
}

// npcGrowthFormalPool reads the pool a leg will present.
//
// An unwritten pool means no wound has been recorded, so it is the table's
// maximum. A written one is a real fact; it is trimmed to the table's maximum
// because the breakthrough contract refuses a snapshot whose current value
// exceeds the basis it is compared against, and refusing the whole interval over
// a bonus the growth pass cannot see would cost the character its training. The
// trim is presented, not committed: an ordinary leg writes no pool, so the
// journal keeps the larger number for the paths that do understand the bonus.
func npcGrowthFormalPool(value, maximum int) int {
	if value < 0 {
		return 0
	}
	if value > maximum {
		return maximum
	}
	return value
}

// npcGrowthSettlementOps turns one committed candidate into the Actor State
// writes that make it fact.
//
// The stage and the banked progress are written as absolute values, exactly as
// the player's settlement writes them: the number is derived from the rules, and
// a delta would accumulate the difference between two derivations and drift.
//
// The pools are written only when the candidate really moved them, which in this
// pass means a breakthrough: the preparation deducts its mana cost at the start
// and the outcome writes the resolved health and mana. Training neither wounds
// nor heals, so an ordinary leg writes nothing there and the committed value
// stands.
//
// The six derived attributes follow the stage, so a leg that changed the stage
// re-runs the settlement on a clone of the projection with the growth's own
// writes projected in. That clone is what lets the settlement see the new stage
// and its new pools while every other field it reads stays exactly as committed.
func npcGrowthSettlementOps(state map[string]any, system StoryDirectorActorStateSystem, event NPCGrowthRecordEvent, turnID string) []ActorStateOp {
	if !event.Candidate.HasStateChange {
		return nil
	}
	next := event.Candidate.NextState
	actorID := event.Candidate.ActorID
	ops := make([]ActorStateOp, 0, 10)
	if stage := cultivation.StageKey(next.Cultivation); stage != "" {
		ops = append(ops, npcGrowthSetOp(actorID, xiuxianFieldRealm, stage, turnID))
	}
	ops = append(ops, npcGrowthSetOp(actorID, xiuxianFieldRealmProgress, next.Accumulated, turnID))
	if npcGrowthPoolsMoved(event.Input.State.Resources, next.Resources) {
		ops = append(ops,
			npcGrowthSetOp(actorID, xiuxianFieldHealth, next.Resources.Health, turnID),
			npcGrowthSetOp(actorID, xiuxianFieldMana, next.Resources.Mana, turnID),
		)
	}

	// The derived numbers are settled rather than written here, so the one code
	// path that produces them stays the only one. The projection is cloned and
	// the growth's writes applied to the clone: the settlement then derives from
	// the stage this leg reached, without this function having to know how the
	// six attributes are computed or which fields the template declares.
	if settled, ok := npcGrowthDerivedOps(state, system, actorID, ops, turnID); ok {
		ops = append(ops, settled...)
	}
	return xiuxianChangedOps(state, actorID, ops)
}

// npcGrowthDerivedOps runs the settlement on a projected clone and returns the
// derived-attribute writes it produced beyond the ones growth already has.
func npcGrowthDerivedOps(state map[string]any, system StoryDirectorActorStateSystem, actorID string, growthOps []ActorStateOp, turnID string) ([]ActorStateOp, bool) {
	projected := cloneStoryState(state)
	for _, op := range growthOps {
		applyActorStateOp(projected, op)
	}
	settled, ok := settleXiuxianActorState(projected, system, actorID, turnID)
	if !ok {
		return nil, false
	}
	// The settlement re-writes the stage and progress it read, and the pools it
	// clamped. Growth owns those three answers here: its stage and progress are
	// the rules' direct output, and its pools are the outcome of a real attempt
	// rather than a clamp of a stored number.
	owned := map[string]bool{xiuxianFieldRealm: true, xiuxianFieldRealmProgress: true}
	for _, op := range growthOps {
		owned[op.FieldID] = true
	}
	derived := make([]ActorStateOp, 0, len(settled))
	for _, op := range settled {
		if owned[op.FieldID] {
			continue
		}
		derived = append(derived, op)
	}
	return derived, true
}

// npcGrowthPoolsMoved reports whether the leg really changed the actor's pools.
//
// The input always carries pools because the rules require a current snapshot,
// so presence alone cannot be the test. An ordinary training leg neither wounds
// nor heals and therefore leaves them exactly as it found them, and writing that
// unchanged number back would commit the formal basis the leg presented — which
// is a reading of the stage table, not the actor's own maximum.
func npcGrowthPoolsMoved(before, after *npcgrowth.ActorPools) bool {
	if after == nil {
		return false
	}
	if before == nil {
		return true
	}
	return before.Health != after.Health || before.Mana != after.Mana
}

// npcGrowthSetOp builds one growth write.
func npcGrowthSetOp(actorID, fieldID string, value any, turnID string) ActorStateOp {
	return ActorStateOp{
		Op:           "set",
		ActorID:      actorID,
		FieldID:      normalizeActorStateFieldName(fieldID),
		Value:        value,
		Reason:       "由 NPC 成长规则按真实区间结算，不由叙述改写。",
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceNPCGrowth,
		SourceID:     "npc_growth",
	}
}

// applyNPCGrowthRecord merges a side record into the branch projection.
//
// It writes the watermark and nothing else: the stage and progress are applied
// by the paired state delta, which is the record the replay reads for state.
func applyNPCGrowthRecord(branch *storyBranchProjection, event NPCGrowthRecordEvent) error {
	if err := validateNPCGrowthRecordEvent(event); err != nil {
		return err
	}
	if branch.NPCGrowth == nil {
		branch.NPCGrowth = map[string]npcgrowth.Snapshot{}
	}
	branch.NPCGrowth[event.Candidate.ActorID] = event.Candidate.NextState
	return nil
}

// validateNPCGrowthRecordEvent rejects a record that could not have been
// honestly attributed. A malformed row is an error rather than a silent drop,
// because a dropped record is growth the character never received.
func validateNPCGrowthRecordEvent(event NPCGrowthRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("成长记录缺少 branch_id / a growth record requires a branch")
	}
	if normalizeStatePanelActorID(event.Candidate.ActorID) == "" {
		return fmt.Errorf("成长记录缺少 actor_id / a growth record requires an actor")
	}
	if event.Input.ActorID != event.Candidate.ActorID {
		return fmt.Errorf("成长记录的人与输入不一致 / a growth record must answer for the actor it was asked about")
	}
	if event.ClockBefore.Validate() != nil {
		return fmt.Errorf("成长记录缺少合法的起始时刻 / a growth record needs a valid clock before it")
	}
	if err := event.ClockAfter.Validate(); err != nil {
		return err
	}
	if event.ClockAfter.Seconds < event.ClockBefore.Seconds {
		return fmt.Errorf("成长记录让时间倒流 / a growth record must not move the story clock backwards")
	}
	return nil
}

// cloneNPCGrowthSnapshots copies the watermark map so one branch's settled
// intervals cannot be mutated through an alias into another.
//
// The breakthrough bookkeeping is deep-copied too. It is pointer-bearing, and a
// branch that shares a pending preparation with its parent would have one
// branch's attempt spend the other's mana.
func cloneNPCGrowthSnapshots(snapshots map[string]npcgrowth.Snapshot) map[string]npcgrowth.Snapshot {
	if len(snapshots) == 0 {
		return nil
	}
	cloned := make(map[string]npcgrowth.Snapshot, len(snapshots))
	for id, snapshot := range snapshots {
		if snapshot.Resources != nil {
			pools := *snapshot.Resources
			snapshot.Resources = &pools
		}
		snapshot.Breakthrough = cloneNPCGrowthBreakthrough(snapshot.Breakthrough)
		snapshot.Recovery = cloneNPCGrowthRecovery(snapshot.Recovery)
		cloned[id] = snapshot
	}
	return cloned
}

func cloneNPCGrowthBreakthrough(state *npcgrowth.BreakthroughState) *npcgrowth.BreakthroughState {
	if state == nil {
		return nil
	}
	cloned := *state
	if state.Pending != nil {
		pending := *state.Pending
		cloned.Pending = &pending
	}
	if state.LastAttempt != nil {
		attempt := *state.LastAttempt
		cloned.LastAttempt = &attempt
	}
	return &cloned
}

func cloneNPCGrowthRecovery(recovery *npcgrowth.RecoveryActivity) *npcgrowth.RecoveryActivity {
	if recovery == nil {
		return nil
	}
	cloned := *recovery
	return &cloned
}

// intervalRewindNPCGrowth is the growth watermark as it stood before one
// interval's settlement folded in.
//
// A reroll re-emits the pass from a replacement record, and by then the live
// projection already reports that interval as settled — the rules would answer
// 重复 and the branch would lose the growth along with the record it replaced.
// Rewinding only the actors whose watermark names this window restores the
// position the pass is written from, without resurrecting growth some other
// interval really committed.
//
// "Names this window" is not the same as "ended at its close". An actor that
// stands at a major boundary, or that failed a breakthrough, is settled only
// partway through the window: the rules record the interval identity but stop
// the watermark where the decision fell. That is exactly the case a reroll most
// needs to undo, so the test is the identity plus a watermark inside the window
// rather than an equality with its close.
func intervalRewindNPCGrowth(projection *storyBranchProjection, branchID string, clockBefore, clockAfter StoryClock) map[string]npcgrowth.Snapshot {
	if projection == nil || len(projection.NPCGrowth) == 0 || clockAfter.Seconds <= clockBefore.Seconds {
		return nil
	}
	rewound := make(map[string]npcgrowth.Snapshot, len(projection.NPCGrowth))
	for actorID, snapshot := range projection.NPCGrowth {
		if snapshot.LastIntervalID != "" &&
			snapshot.LastIntervalID == npcGrowthIntervalID(branchID, actorID, clockBefore.Seconds, clockAfter.Seconds) &&
			snapshot.SettledThroughSeconds > clockBefore.Seconds &&
			snapshot.SettledThroughSeconds <= clockAfter.Seconds {
			// This is the settlement the interval being re-emitted wrote. Nothing
			// earlier could have used the same identity, because the identity names
			// this window.
			snapshot.SettledThroughSeconds = clockBefore.Seconds
			snapshot.LastIntervalID, snapshot.LastIntervalHash = "", ""
			// The attempt bookkeeping is carried by the interval too, but only the
			// part this window produced. A preparation paid here, and the recovery a
			// failure here imposed, are consequences of the pass being re-emitted; an
			// attempt started before the window belongs to an earlier interval and is
			// kept, or the reroll would forget a decision the story already paid for.
			snapshot.Breakthrough = npcGrowthRewindBreakthrough(snapshot.Breakthrough, clockBefore.Seconds)
			if snapshot.Revision > 0 {
				snapshot.Revision--
			}
		}
		rewound[actorID] = snapshot
	}
	return rewound
}

// npcGrowthLastAttemptAt is the instant the carried attempt state began, or a
// value no clock can equal when nothing is pending or completed.
//
// The rules date an attempt by the instant its preparation started, and both a
// pending preparation and the most recent completed one carry that instant. A
// leg that begins exactly there would repeat the decision §6 forbids repeating
// at one moment, so the instant is what the caller compares against.
func npcGrowthLastAttemptAt(snapshot npcgrowth.Snapshot) int64 {
	if snapshot.Breakthrough == nil {
		return -1
	}
	if pending := snapshot.Breakthrough.Pending; pending != nil {
		return pending.StartedAtSeconds
	}
	if last := snapshot.Breakthrough.LastAttempt; last != nil {
		return last.StartedAtSeconds
	}
	return -1
}

// npcGrowthRewindBreakthrough drops the attempt state an interval produced.
// An attempt is dated by the instant its preparation started, so that instant
// is what decides whether it belongs to the window being re-emitted. A pending
// preparation or a completed attempt from before the window is real history and
// survives; one from inside it is exactly what the replacement pass will
// recreate, and leaving it would let the rules answer 重复 or refuse the window
// as still in recovery.
func npcGrowthRewindBreakthrough(state *npcgrowth.BreakthroughState, from int64) *npcgrowth.BreakthroughState {
	if state == nil {
		return nil
	}
	rewound := *state
	if rewound.Pending != nil && rewound.Pending.StartedAtSeconds >= from {
		rewound.Pending = nil
	}
	if rewound.LastAttempt != nil && rewound.LastAttempt.StartedAtSeconds >= from {
		rewound.LastAttempt = nil
		rewound.RecoveryUntilSeconds = 0
	}
	if rewound.Pending == nil && rewound.LastAttempt == nil && rewound.RecoveryUntilSeconds == 0 {
		return nil
	}
	return &rewound
}

// xiuxianCharacterForGrowth reads one actor's rules position with its pools
// defaulted the way the settlement defaults them.
//
// An unwritten pool means no wound has been recorded, so it starts full rather
// than at zero. Reading the absence as zero would make every newly seen NPC
// permanently too badly hurt to train.
func xiuxianCharacterForGrowth(cultivationRules *cultivation.Rules, state map[string]any, actorID string) (cultivation.CharacterState, bool) {
	character, err := xiuxianCharacterFromState(cultivationRules,
		state, actorID)
	if err != nil {
		return character, false
	}
	attributes := character.Attributes()
	if _, ok := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldHealth)); !ok {
		character.Health = attributes.HealthMax
	}
	if _, ok := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldMana)); !ok {
		character.Mana = attributes.ManaMax
	}
	return character, true
}

// npcGrowthTechniqueQuality narrows the method grade to the three values the
// growth contract accepts.
//
// An empty grade is the rules' own baseline rather than a missing fact: a
// character with no method recorded trains at the ordinary rate, which is what
// the settlement already prices them at.
func npcGrowthTechniqueQuality(character cultivation.CharacterState) cultivation.TechniqueQuality {
	switch quality := character.PrimaryTechniqueQuality(); quality {
	case cultivation.QualityGood, cultivation.QualityAdvanced:
		return quality
	default:
		return cultivation.QualityOrdinary
	}
}

// npcGrowthIntervalID is the stable identity of one actor's settlement over one
// window on one branch. It is what makes a retry a duplicate rather than a
// second grant.
func npcGrowthIntervalID(branchID, actorID string, from, to int64) string {
	return fmt.Sprintf("npcgrow:%s:%s:%d-%d", branchID, actorID, from, to)
}

func npcGrowthActorRef(actorID, suffix string) string {
	return "actor:" + actorID + "/" + suffix
}

func npcGrowthBranchRef(branchID, suffix string) string {
	return "branch:" + branchID + "/" + suffix
}
