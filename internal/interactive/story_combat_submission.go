package interactive

import (
	"fmt"
	"reflect"
	"strings"

	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// combatCompanionSubmissionDiagnostic binds a new ally declaration to that
// creature's own current response. Keeping an unchanged declaration preserves
// the original consent; changing its opponent requires a new response.
func combatCompanionSubmissionDiagnostic(system StoryDirectorActorStateSystem, current map[string]any, compiled CompiledTurnStateUpdates, facts *TurnCharacterFactsInput, known []string) *TurnSubmissionDiagnostic {
	state := cloneStoryState(current)
	for _, op := range compiled.Ops {
		applyStateOp(state, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(state, op)
	}
	prior := storyContextEngagementRecords(storyContextActorState(current))
	for actorID, raw := range storyContextEngagementRecords(storyContextActorState(state)) {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		side := actorArchiveString(entry["side"])
		if side == "" || side == string(cultivation.SideEnemy) {
			continue
		}
		refuse := func(reason string) *TurnSubmissionDiagnostic {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleStateChanges, nil, TurnSubmissionDiagnosticInvalidModule,
				"/story/本次交战/"+actorID, "present companion's sourced agreement to this opponent", actorID, reason)
		}
		targetID := normalizeStatePanelActorID(actorArchiveString(entry["target_id"]))
		lastDecisionIndex := 0
		if facts != nil {
			for index, response := range facts.Exchanges {
				if normalizeStatePanelActorID(response.CharacterID) != actorID || response.Speaker != "character" || response.Channel != "scene" || normalizeStatePanelActorID(response.AddressedTo) != DefaultActorID || response.ParticipationDecision == nil {
					continue
				}
				decision := response.ParticipationDecision.Normalize()
				if decision.Kind == character.ParticipationCombat && normalizeStatePanelActorID(decision.TargetID) == targetID {
					lastDecisionIndex = index + 1
				}
			}
		}
		if lastDecisionIndex > 0 && facts.Exchanges[lastDecisionIndex-1].ParticipationDecision.Normalize().Decision == character.ParticipationRefuse {
			return refuse("本人最新回应已明确拒绝这场助战。移除对应本次交战友方声明，不能沿用先前接受或挑选更早的回应。")
		}
		// An unchanged record keeps its original source. A later archive or
		// departure makes it ineligible at the consumer, but must not prevent the
		// departure itself from committing.
		if reflect.DeepEqual(prior[actorID], raw) {
			continue
		}
		if side != string(cultivation.SidePlayer) || !combatCompanionEligible(state, system, actorID) {
			return refuse("助战须用 side=player、非空 basis、真实敌方 target_id 和正整数 source_exchange_index；本人和该敌方必须在场，拥有可用的真实战斗状态。")
		}
		knownIdentity := combatContainsActorID(known, actorID)
		if actorRecordExists(current, actorID) || knownIdentity {
			if !knownIdentity || !combatContainsActorID(StorySceneFromState(current, "").Present, actorID) || actorIsArchived(current, actorID) {
				return refuse("已登记的助战者须确实同场，不能把远处或归档生灵只改在场名单便召入战斗。")
			}
		} else if !combatCompanionIntroduced(compiled, facts, actorID) {
			return refuse("首次出现者须在同一正式提交中建立自己的真实Actor、角色档案、在场事实和本人回应；目录本身不是已经相遇。")
		}
		index, _ := actorStateNumber(entry["source_exchange_index"])
		if facts == nil || int(index) > len(facts.Exchanges) {
			return refuse("新助战意愿须引用本回合 character_facts.exchanges 中本人的实际回应，不能用好感、关系或主角的命令代替。")
		}
		exchange := facts.Exchanges[int(index)-1]
		if normalizeStatePanelActorID(exchange.CharacterID) != actorID || strings.TrimSpace(exchange.Speaker) != "character" || strings.TrimSpace(exchange.Channel) != "scene" || normalizeStatePanelActorID(exchange.AddressedTo) != DefaultActorID || strings.TrimSpace(exchange.Text) == "" {
			return refuse("source_exchange_index 必须指向助战者本人向主角作出的公开场景回应；可记录符合物种灵智的明确行为回应，不要求人类语言。")
		}
		if exchange.ParticipationDecision == nil || exchange.ParticipationDecision.Normalize() != (character.ParticipationDecision{Kind: character.ParticipationCombat, TargetID: targetID, Decision: character.ParticipationAccept}) || lastDecisionIndex != int(index) {
			return refuse("助战须引用本回合本人对该target_id最后一条participation_decision，kind=combat且decision=accept。拒绝、未作答、其他用途或旧回应不能授权参战。")
		}
	}
	return nil
}

func combatCompanionIntroduced(compiled CompiledTurnStateUpdates, facts *TurnCharacterFactsInput, actorID string) bool {
	if facts == nil {
		return false
	}
	profiled := false
	for _, profile := range facts.Profiles {
		profiled = profiled || normalizeStatePanelActorID(profile.CharacterID) == actorID
	}
	if !profiled {
		return false
	}
	for _, update := range compiled.Updates {
		path, err := interactivestate.ParsePath(update.Path)
		if err == nil && update.Op == interactivestate.Create && len(path) == 1 && normalizeStatePanelActorID(path[0]) == actorID {
			return true
		}
	}
	return false
}

// combatEngagementSubmissionDiagnostic checks the same post-turn scene that the
// battle window will read. A profile gives a character identity, not combat
// numbers; accepting a fight with no rules Actor would lock the main story while
// leaving the battle window unable to open. Same-turn Actor creation is valid.
func combatEngagementSubmissionDiagnostic(system StoryDirectorActorStateSystem, current map[string]any, compiled CompiledTurnStateUpdates) *TurnSubmissionDiagnostic {
	state := cloneStoryState(current)
	for _, op := range compiled.Ops {
		applyStateOp(state, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(state, op)
	}
	if _, supported := combatEncounterSpec(state, system, DefaultActorID); !supported {
		return nil
	}
	engaged, declared := combatEncounterEngagement(state, system)
	if !declared {
		return nil
	}
	for _, rawID := range StorySceneFromState(state, "").Present {
		id := normalizeStatePanelActorID(rawID)
		if id == DefaultActorID || !engaged[id] {
			continue
		}
		if _, found := combatEncounterSpec(state, system, id); found {
			continue
		}
		return newTurnSubmissionDiagnostic(TurnSubmissionModuleStateChanges, nil,
			TurnSubmissionDiagnosticInvalidModule, "/story/本次交战", "registered combat Actor", id,
			fmt.Sprintf("The scene declares %q fighting the protagonist, but that person has no active rules Actor for this world's combat. Register their own Actor with an appropriate available template and source-supported initial stage in the same state_changes module; a character_facts profile alone has no combat state. Reuse the exact character ID. Do not invent progression or change existing stages. If no real fight occurred, correct the engagement declaration instead.", id))
	}
	return nil
}
