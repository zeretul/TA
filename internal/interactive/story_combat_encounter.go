package interactive

import (
	"errors"
	"sort"
	"strings"

	"ta/internal/cultivation"
)

// ErrStoryTurnDuringBattle means the main story tried to advance through a fight
// that the combat window owns.
//
// The two paths write different things. A battle round writes the pools the
// rules actually settled; a main-story turn writes prose and state the model
// decided. Letting the second run while the first is live is how a live run ended
// up with the prose saying 沈青倒地不起 while the window still showed round 1 and
// full health on both sides. The refusal is not a style rule: it keeps the story
// from claiming a result no rule produced.
var ErrStoryTurnDuringBattle = errors.New("这场交手还没有结果")

// StoryTurnBlockedByBattle refuses a main-story turn that would write the
// opening of a fight the combat window has not yet played.
//
// The gap it closes is the one a live run showed: a turn declared an attack, the
// very next turn narrated 沈青倒地不起, and the window still showed round 1 with
// both sides at full health. The prose had settled a battle no rule had run. That
// is the "one dice_check writes the whole fight" failure in its most direct form.
//
// The refusal is deliberately narrow. It applies when the scene declares an
// engagement the window could open and nobody has traded a blow in it yet. Once
// the window has settled a round, the fight is being played the way it is meant
// to be, and the main story goes back to its ordinary business beside it — a
// player who is fighting can still put a spirit beast back in its bag, and
// refusing that would be a different defect.
//
// Two states deliberately do not count:
//
//   - A declaration naming nobody the rules can fight. The window would refuse to
//     open it, so blocking the story on it would freeze the game with no way out.
//
//   - An enemy the story only has by template, with no declaration. That is the
//     fallback for a frozen schema that predates 本次交战; treating a standing
//     opponent as a fight would stop every old story from advancing again.
//
// Threatening, backing away, and probing that never became a fight are untouched:
// none of them leaves a declaration behind, so the ordinary rules still decide
// them.
func StoryTurnBlockedByBattle(state map[string]any, system StoryDirectorActorStateSystem, combats map[string]cultivation.CombatSession) error {
	if !StoryDeclaresEngagement(state, system) {
		return nil
	}
	// The declaration is only a fight if the window could open it. Anything the
	// encounter derivation cannot put on the enemy side is a scene claim the rules
	// cannot act on, and the story keeps the ordinary rules for it.
	if encounter := CombatEncounterFromState(state, system); encounter.ReasonKey != "" {
		return nil
	}
	// The window has already begun settling exchanges, so the fight is being
	// played where it belongs and the story may continue beside it.
	if combatEngagementHasBeenFought(combats) {
		return nil
	}
	return ErrStoryTurnDuringBattle
}

// combatEngagementHasBeenFought reports whether the player's open battle has
// settled at least one exchange.
//
// The opening record establishes the encounter at round 1 and settles nothing;
// the first real exchange moves the battle past it. Round alone is enough — the
// record's own counter is what the window shows.
func combatEngagementHasBeenFought(combats map[string]cultivation.CombatSession) bool {
	for _, session := range combats {
		if session.Ended || session.Round <= 1 {
			continue
		}
		for _, combatant := range session.Combatants {
			if normalizeStatePanelActorID(combatant.ID) == DefaultActorID {
				return true
			}
		}
	}
	return false
}

// This file answers one question the rules cannot: which of the people the
// scene has established are actually in this fight, and on which side.
//
// It is derived rather than asked. The window used to lay the whole cast out and
// make the player side every name, which turned bystanders into enemies and let
// someone who is elsewhere join a fight they are not in. Three committed facts
// settle it:
//
//   - 在场角色, the story-context actor's own present list. It is the same list
//     StoryScene reads for the character window, incoming messages and main-story
//     speech, so the fight is fought by the people the scene on screen is about.
//     Anyone not on it is elsewhere and does not join.
//
//   - 本次交战, the story-context actor's declaration of who is fighting the
//     protagonist in this scene, keyed by the attacker's own Actor ID with the
//     engagement's basis as the value. It is what a person's identity cannot
//     say: the same important character can be a friend in one scene and an
//     attacker in the next, and a template fixed at creation cannot express
//     that. The declaration is written by the turn that settles the attack, so
//     the fight exists in committed state before anyone opens the window.
//
//   - The actor's template. 敌对对象状态表 marks what someone is — 敌人、怪物、
//     Boss、陷阱或异常实体 — and is what the story used before the declaration
//     existed. A template alone no longer starts a fight: 中立不变敌人, and a
//     fight that has ended stays ended rather than reopening on every scene.
//     It still decides the fight for a story whose frozen schema predates the
//     declaration field and therefore cannot write it.
//
// A companion joins only through this encounter's sourced participation record.
// A relationship or a favorable number cannot conscript a bystander. The turn
// validates the companion's own reply, their current presence and the named
// opponent; the record expires with this fight rather than creating a party.
//
// Lifecycle is read from the archive, which is the formal inactive marker
// ("presence in actor_archives makes it inactive until an explicit restore").
// The free-text 存续与位置 / 出场与位置 fields are not parsed: reading them would
// be guessing at prose, and the archive is the fact the rest of the system uses.

// CombatEncounter is the fight the scene already settles.
type CombatEncounter struct {
	// Combatants are the derived participants with the side the scene puts them
	// on. Their RouteExists and Retreat are the conservative facts this
	// derivation can honestly state; see combatEncounterSpec.
	Combatants []CombatantSpec
	// Bystanders are present, rules-tracked people the scene does not put in the
	// fight. They are reported so the window can say who is standing there
	// without turning a neutral into an enemy.
	Bystanders []string
	// ReasonKey names why no battle can be opened here, and is empty when one can.
	// It is a localisation key so the interface supplies the wording.
	ReasonKey string
}

const (
	// combatEncounterNoPlayer means the story has no rules-tracked protagonist to
	// fight as.
	combatEncounterNoPlayer = "combatWindow.encounter.noPlayer"
	// combatEncounterNoPresence means the scene has not settled who is present,
	// so there is no encounter to read off it.
	combatEncounterNoPresence = "combatWindow.encounter.noPresence"
	// combatEncounterNoEnemy means the scene is settled but nobody present is an
	// opponent.
	combatEncounterNoEnemy = "combatWindow.encounter.noEnemy"
)

// StoryCombatEncounter derives the encounter from a committed story snapshot.
func StoryCombatEncounter(snapshot Snapshot, fallback StoryDirectorActorStateSystem) CombatEncounter {
	return CombatEncounterFromState(
		snapshot.State,
		actorStateSystemFromSnapshot(snapshot.ActorStateSchema, fallback),
	)
}

// CombatEncounter reads the current encounter on a branch.
//
// It resolves the frozen actor schema the same way the battle itself does, so the
// preview and the fight it opens are read from one system.
func (s *Store) CombatEncounter(storyID, branchID string) (CombatEncounter, error) {
	context, err := s.StoryContext(storyID, branchID)
	if err != nil {
		return CombatEncounter{}, err
	}
	return StoryCombatEncounter(context.Snapshot, s.storyDirectorForMeta(context.Meta).ActorState), nil
}

// CombatEncounterFromState is the same derivation against a bare state map, for
// callers that hold committed state without a snapshot.
func CombatEncounterFromState(state map[string]any, system StoryDirectorActorStateSystem) CombatEncounter {
	encounter := CombatEncounter{}
	if state == nil {
		encounter.ReasonKey = combatEncounterNoPlayer
		return encounter
	}

	// The protagonist is the player's own side whether or not the scene listed
	// them: 在场角色 describes who else is in view, and a story that has not
	// written the field yet still has someone to play.
	if spec, ok := combatEncounterSpec(state, system, DefaultActorID); ok {
		spec.Side = cultivation.SidePlayer
		encounter.Combatants = append(encounter.Combatants, spec)
	} else {
		encounter.ReasonKey = combatEncounterNoPlayer
		return encounter
	}

	present := StorySceneFromState(state, "").Present
	if len(present) == 0 {
		encounter.ReasonKey = combatEncounterNoPresence
		return encounter
	}

	// A story whose frozen schema has the declaration field states its own
	// engagement. One frozen before that field existed cannot write it, and
	// falling back to "nobody is fighting" would leave an existing story unable
	// to open any battle at all, so it keeps the rule it was created under.
	engagement, declared := combatEncounterEngagement(state, system)

	seen := map[string]bool{DefaultActorID: true}
	for _, rawID := range present {
		actorID := normalizeStatePanelActorID(rawID)
		if actorID == "" || seen[actorID] {
			continue
		}
		// The scene's own ledgers are state objects, not people, even if a turn
		// mistakenly listed them among the present.
		if actorID == DefaultStoryContextActorID || actorID == DefaultWorldEntitiesActorID {
			continue
		}
		seen[actorID] = true
		spec, tracked := combatEncounterSpec(state, system, actorID)
		if !tracked {
			// Not rules-tracked, or archived: either way the rules cannot settle a
			// fight with them, so they are not a participant.
			continue
		}
		if !combatEncounterIsOpponent(state, system, actorID, engagement, declared) {
			if combatCompanionEligible(state, system, actorID) {
				spec.Side = cultivation.SidePlayer
				encounter.Combatants = append(encounter.Combatants, spec)
				continue
			}
			encounter.Bystanders = append(encounter.Bystanders, actorID)
			continue
		}
		spec.Side = cultivation.SideEnemy
		encounter.Combatants = append(encounter.Combatants, spec)
	}

	if !combatEncounterHasEnemy(encounter.Combatants) {
		encounter.ReasonKey = combatEncounterNoEnemy
	}
	return encounter
}

// StoryDeclaresEngagement reports whether the committed scene declares anyone
// fighting the protagonist right now.
//
// It reads only the declaration, never the template fallback. The fallback
// exists for a story frozen before 本次交战 could be written, and treating an
// opponent template as "a fight is happening" would put every old story's
// ordinary scene under a permanent battle. What the turn needs to know is the
// narrower fact: this scene has committed that blows are being exchanged.
func StoryDeclaresEngagement(state map[string]any, system StoryDirectorActorStateSystem) bool {
	engaged, declared := combatEncounterEngagement(state, system)
	if !declared || len(engaged) == 0 {
		return false
	}
	// A declaration naming someone who is not in the scene is not a fight the
	// window could open, so it must not block the turn either. The presence gate
	// is the same one the encounter derivation applies.
	for _, rawID := range StorySceneFromState(state, "").Present {
		actorID := normalizeStatePanelActorID(rawID)
		if actorID == "" || actorID == DefaultActorID {
			continue
		}
		if engaged[actorID] {
			return true
		}
	}
	return false
}

// StoryEngagementActorIDs names the declared opponents, so a refusal can say who
// the fight is with instead of describing it in the abstract.
func StoryEngagementActorIDs(state map[string]any, system StoryDirectorActorStateSystem) []string {
	engaged, declared := combatEncounterEngagement(state, system)
	if !declared {
		return nil
	}
	ids := make([]string, 0, len(engaged))
	for _, rawID := range StorySceneFromState(state, "").Present {
		actorID := normalizeStatePanelActorID(rawID)
		if actorID == "" || actorID == DefaultActorID || !engaged[actorID] {
			continue
		}
		ids = append(ids, actorID)
	}
	sort.Strings(ids)
	return ids
}

// combatEncounterEngagement reads the scene's declaration of who is attacking
// the protagonist, and reports whether this story has the field at all.
//
// Presence still gates the result: a declaration naming someone who is not in
// the scene cannot drag them in from another room, and the loop above is what
// applies that. The map here is only the scene's own claim.
func combatEncounterEngagement(state map[string]any, system StoryDirectorActorStateSystem) (map[string]bool, bool) {
	if !storyContextHasEngagementField(state, system) {
		return nil, false
	}
	engaged := map[string]bool{}
	for rawID, raw := range storyContextEngagementRecords(storyContextActorState(state)) {
		if fields, ok := raw.(map[string]any); ok && strings.TrimSpace(actorArchiveString(fields["side"])) == string(cultivation.SidePlayer) {
			continue
		}
		if actorID := normalizeStatePanelActorID(rawID); actorID != "" {
			engaged[actorID] = true
		}
	}
	return engaged, true
}

// combatCompanionEligible reads only a turn-validated, encounter-local choice.
// A missing target, absence, archive or ended engagement cannot summon an ally.
func combatCompanionEligible(state map[string]any, system StoryDirectorActorStateSystem, actorID string) bool {
	actorID = normalizeStatePanelActorID(actorID)
	if actorID == "" || actorID == DefaultActorID {
		return false
	}
	records := storyContextEngagementRecords(storyContextActorState(state))
	entry, ok := records[actorID].(map[string]any)
	if !ok || actorArchiveString(entry["side"]) != string(cultivation.SidePlayer) || actorArchiveString(entry["basis"]) == "" {
		return false
	}
	index, validIndex := actorStateNumber(entry["source_exchange_index"])
	if !validIndex || index < 1 || index != float64(int(index)) {
		return false
	}
	targetID := normalizeStatePanelActorID(actorArchiveString(entry["target_id"]))
	if targetID == "" || targetID == actorID || targetID == DefaultActorID {
		return false
	}
	present := StorySceneFromState(state, "").Present
	if !combatContainsActorID(present, actorID) || !combatContainsActorID(present, targetID) {
		return false
	}
	if _, active := combatEncounterSpec(state, system, actorID); !active {
		return false
	}
	if health, recorded := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldHealth)); recorded && health <= 0 {
		return false
	}
	if _, active := combatEncounterSpec(state, system, targetID); !active {
		return false
	}
	opponents, _ := combatEncounterEngagement(state, system)
	return opponents[targetID]
}

// storyContextHasEngagementField reports whether the frozen story-context
// template declares 本次交战. A field the story cannot write must not change the
// rule it was created under.
func storyContextHasEngagementField(state map[string]any, system StoryDirectorActorStateSystem) bool {
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, DefaultStoryContextActorID)
	if !found || templateID != ActorStateStoryContextTemplateID {
		return false
	}
	_, exists := actorStateFieldByID(actorStateTemplateByID(system, templateID), storySceneEngagementField)
	return exists
}

// combatEncounterSpec builds one participant's scene-side facts.
//
// The numbers come from the same reader the battle itself uses, so a preview and
// the fight it opens cannot disagree. Two facts are stated conservatively:
//
//   - RouteExists is true for the player and false for an opponent. A player who
//     has not been confined by anything the scene states can disengage; an
//     opponent's way out is a scene fact this derivation does not have, and
//     asserting one would let a cornered enemy slip away on an invented route.
//
//   - Retreat is left at RetreatNone. §14.2 requires the survival and cautious
//     templates to come from a real declared source, and the opponent preset
//     records its disposition as prose (行动模式). Defaulting everyone to 求生
//     would put a begging template on a mindless construct, which §14.2's
//     "无思维敌人不套求饶模板" forbids.
func combatEncounterSpec(state map[string]any, system StoryDirectorActorStateSystem, actorID string) (CombatantSpec, bool) {
	actorID = normalizeStatePanelActorID(actorID)
	if actorID == "" || actorIsArchived(state, actorID) {
		return CombatantSpec{}, false
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !found {
		return CombatantSpec{}, false
	}
	template := actorStateTemplateByID(system, templateID)
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return CombatantSpec{}, false
	}
	return CombatantSpec{
		ActorID:     actorID,
		RouteExists: actorID == DefaultActorID,
		Retreat:     cultivation.RetreatNone,
	}, true
}

// combatEncounterIsOpponent reports whether the scene's own facts put this actor
// on the hostile side.
//
// A declared engagement is the fact that settles it: the person is attacking
// the protagonist in this scene, whatever their template says. Without one, the
// answer is the template — but only for a story that has no declaration field to
// write. 中立不变敌人: a present important character is not an enemy for standing
// in the room, and a negative favorability is a dislike, not a declaration that
// a fight has started.
func combatEncounterIsOpponent(state map[string]any, system StoryDirectorActorStateSystem, actorID string, engagement map[string]bool, declared bool) bool {
	if declared {
		return engagement[actorID]
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !found {
		return false
	}
	return templateID == ActorStateOpponentTemplateID
}

func combatEncounterHasEnemy(combatants []CombatantSpec) bool {
	for _, spec := range combatants {
		if spec.Side == cultivation.SideEnemy {
			return true
		}
	}
	return false
}

// combatContainsActorID compares scene and registry references by actor identity.
func combatContainsActorID(ids []string, actorID string) bool {
	actorID = normalizeStatePanelActorID(actorID)
	for _, id := range ids {
		if normalizeStatePanelActorID(id) == actorID {
			return true
		}
	}
	return false
}
