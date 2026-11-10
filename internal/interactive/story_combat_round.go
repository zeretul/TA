package interactive

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"ta/internal/cultivation"
)

// This file makes a battle startable and a round playable.
//
// story_combat.go settles one exchange from a session the caller already has and
// commits it. What it deliberately does not do is decide who fights, whose turn
// it is, or what the opposition does — those are facts about the scene, and the
// engine's own ResolveExchange leaves an undeclared combatant to a plain attack
// rather than choosing for it. That is the right default for a rules engine and
// the wrong one for play: §14.2 requires an enemy to act from its own learned
// techniques, its own resources and its own willingness to keep fighting.
//
// So this file is the missing middle. It reads the battle out of the branch
// projection, fills in the opposition's intent through the engine's own decision
// function, resolves the round against the real numbers, and commits through the
// same side-record path everything else uses. Nothing here is a second world:
// the session it works from is the one the journal already holds, and the result
// it commits is the one AppendCombatRecord already knows how to store.

// ErrCombatActorAlreadyFighting means a caller tried to put someone into a
// second battle while the first one is still open.
//
// One person fights one battle at a time. A second fight built beside a live one
// is not a second scene: both write the same character's pools, so whichever
// round settles last overwrites the other's wounds with its own snapshot. The
// store refuses the opening rather than letting the journal hold two live fights
// for one actor.
var ErrCombatActorAlreadyFighting = errors.New("combat actor is already fighting")

// ErrCombatStaleBattle means the battle a round names has been superseded.
//
// A journal can already hold two live battles for one actor — the guard above
// only stops new ones. Continuing the older fight would write its snapshot of
// the pools over the settlement a later fight really produced, which is a wound
// being undone. The round is refused and the battle that last settled those
// actors is reported instead.
var ErrCombatStaleBattle = errors.New("combat battle was superseded by a later settlement")

// ErrCombatPlayerActionRequired means the round was submitted without an action
// for the player's own side.
//
// §七.1 makes the first exchange the player's own choice, and every exchange
// after it as well. An omitted declaration used to reach the rules engine, whose
// default for an undeclared combatant is a plain attack — so a submission that
// named nobody silently attacked on the player's behalf and charged them for a
// decision they never made. The store refuses it instead, and the window asks
// for an action. The ordinary attack is still a legal action to choose.
var ErrCombatPlayerActionRequired = errors.New("combat round requires the player's action")
var ErrCombatPlayerIntentInvalid = errors.New("combat round has an invalid player intent")
var ErrCombatTakeLifeActionRequired = errors.New("take-life intent requires the protagonist to attack")
var ErrCombatTakeLifeTargetInvalid = errors.New("take-life intent requires an active opponent target")

// CombatActorConflict reports which battle holds the actors a request named, so
// the window can open that fight instead of offering a second one.
type CombatActorConflict struct {
	// Kind is ErrCombatActorAlreadyFighting or ErrCombatStaleBattle.
	Kind error
	// SessionID is the live battle that holds the actors.
	SessionID string
	// Actors are the participants both fights share.
	Actors []string
}

func (c *CombatActorConflict) Error() string {
	return fmt.Sprintf("%v: session=%s actors=%s", c.Kind, c.SessionID, strings.Join(c.Actors, ","))
}

// Unwrap makes errors.Is answer for the kind, so a caller can tell an opening
// refused from a round refused without knowing the concrete type.
func (c *CombatActorConflict) Unwrap() error { return c.Kind }

// BattleStartRequest opens a battle at a real point in the story.
//
// The participants are named rather than derived: §七.1 requires the window to
// show both sides, and which characters are actually in the fight is a scene
// fact the story has, not something the rules can read off the cast list.
type BattleStartRequest struct {
	BranchID string
	// SessionID is the battle's identity. Empty mints one, which is what the
	// window does; a caller that already has an identity supplies it so a retry
	// addresses the same battle rather than opening a second one.
	SessionID string
	// WorldTime is the in-fiction time the battle began at.
	WorldTime string
	// Combatants is every participant, with the side they are on and the two
	// scene facts the numbers do not carry.
	Combatants []CombatantSpec
	// TurnID attaches the battle to a story turn when it began inside one.
	TurnID      string
	CommandID   string
	CommandHash string
}

// BattleRoundRequest plays one more exchange of a battle that is already open.
type BattleRoundRequest struct {
	// Server-side AI intent is bound to the exact observed committed state.
	ExpectedHead   string
	ExpectedRound  int
	EnemyChoices   map[string]string
	DecisionStatus string
	BranchID       string
	// SessionID names the battle to advance.
	SessionID string
	// Actions is the round's declared intents, by actor ID. The player's side
	// supplies its own; an enemy left out is decided from its own state.
	Actions      map[string]cultivation.CombatAction
	PlayerIntent PlayerCombatIntent
	WorldTime    string
	TurnID       string
	CommandID    string
	CommandHash  string
}

// BattleRoundOutcome is what one resolved round reports back.
type BattleRoundOutcome struct {
	EnemyDecision *EnemyDecisionReceipt `json:"enemy_decision,omitempty"`
	// Session is the battle after the round.
	Session cultivation.CombatSession
	// Outcome is the round that was settled. It is zero when Settled is false.
	Outcome cultivation.ExchangeOutcome
	// EventID is the committed side record, empty when nothing was committed.
	EventID string
	// Settled reports whether this call resolved a round. It is false when the
	// battle had already reached a result, which is a read rather than a round:
	// §七.8 requires a settled outcome to stand, so a reopened window shows the
	// result instead of offering another exchange.
	Settled bool
	// SettlementOps are the Actor State writes the commit produced.
	SettlementOps   []ActorStateOp
	Fatalities      []CombatFatality
	TerminalOutcome *TerminalOutcome
}

// StartBattle establishes a battle and commits the encounter.
//
// It deliberately plays no exchange. §七.1's opening is the encounter itself: the
// fight exists, both sides are shown at the numbers they already have, and the
// player is asked for a first action. Settling round 1 here would charge both
// sides for a round nobody chose — and, before R3, did it with a plain attack the
// player never picked. The first exchange is played through ResolveBattleRound
// like every other one, against the same enemy decision and the same eligibility
// and resource checks.
//
// The encounter is nevertheless committed, as a side record with a zero outcome.
// That is what makes it survive a restart, keep the one-battle-per-actor guard
// honest, and let a reopened window wait on the same first action instead of
// opening a second fight or silently fighting one.
func (s *Store) StartBattle(storyID string, req BattleStartRequest) (BattleRoundOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	defer releaseStory()

	branchID, meta, records, err := s.battleBranchLocked(storyID, req.BranchID)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	// A retry must not open a second battle. The committed record is the durable
	// answer, so it is consulted before anything is built or resolved.
	if committed, found, err := combatRecordForCommandLocked(meta, records, branchID, commandID, commandHash); err != nil {
		return BattleRoundOutcome{}, err
	} else if found {
		return battleOutcomeFromRecord(committed), nil
	}

	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	roster := make(map[string]cultivation.CombatSide, len(req.Combatants))
	for _, spec := range req.Combatants {
		roster[normalizeStatePanelActorID(spec.ActorID)] = spec.Side
	}
	for _, spec := range req.Combatants {
		actorID := normalizeStatePanelActorID(spec.ActorID)
		if actorID == DefaultActorID || spec.Side != cultivation.SidePlayer {
			continue
		}
		known := false
		if projection.Characters != nil {
			_, known = projection.Characters.Profile(actorID)
		}
		if !known || !combatCompanionEligible(projection.State, system, actorID) {
			return BattleRoundOutcome{}, fmt.Errorf("combat companion %s has no present, sourced agreement to this encounter", actorID)
		}
		entry, _ := storyContextEngagementRecords(storyContextActorState(projection.State))[actorID].(map[string]any)
		if roster[normalizeStatePanelActorID(actorArchiveString(entry["target_id"]))] != cultivation.SideEnemy {
			return BattleRoundOutcome{}, fmt.Errorf("combat companion %s did not agree to the requested opposing roster", actorID)
		}
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = newID("cbt")
	}
	if _, ok := projection.Combats[sessionID]; ok {
		// An identity that is already in use names a battle that exists. Opening a
		// second one under it would replace a fight with a different fight.
		return BattleRoundOutcome{}, fmt.Errorf("战斗已存在: %s / a battle already exists under this session id", sessionID)
	}
	// One person fights one battle at a time. The check reads the live battles the
	// branch's own projection holds, under the same write lock the commit takes,
	// so two openings racing each other cannot both pass it.
	if held, actors := liveBattleHolding(projection.Combats, combatantActorIDs(req.Combatants), ""); held != "" {
		if sameCombatants(projection.Combats[held], req.Combatants) {
			// The same encounter opened twice. The fight already exists, so this
			// answers with it and settles nothing: the player reopens the battle
			// they are in rather than starting a second one beside it.
			return BattleRoundOutcome{Session: projection.Combats[held], Settled: false}, nil
		}
		return BattleRoundOutcome{}, &CombatActorConflict{
			Kind: ErrCombatActorAlreadyFighting, SessionID: held, Actors: actors,
		}
	}

	// The session is built from the participants' own stored numbers, which is the
	// whole of what an opening decides. Round stays at 1: that is the exchange the
	// battle is now waiting for.
	session, err := combatSessionFromState(projection.State, system, sessionID, req.Combatants)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	lethalIntents := freezeCombatLethalIntents(records, meta.Branches[branchID].Head, session)
	return s.commitBattleRoundLocked(storyID, meta, records, BattleAppendRequest{
		BranchID: branchID, TurnID: req.TurnID, WorldTime: req.WorldTime,
		Session: session, Outcome: cultivation.ExchangeOutcome{},
		CommandID: commandID, CommandHash: commandHash,
		lethalIntents: lethalIntents,
	})
}

// ResolveBattleRound plays one exchange of an open battle.
//
// The opposition's intent is decided here rather than supplied by the caller,
// because it is the enemy's own decision made from its own state: §14.2 forbids
// an enemy reaching for a technique it never learned, and DecideEnemyAction is
// the function that reads the enemy's own list. A caller that declares an enemy's
// action explicitly still wins, so a scripted opponent stays possible.
func (s *Store) ResolveBattleRound(storyID string, req BattleRoundRequest) (BattleRoundOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	defer releaseStory()

	branchID, meta, records, err := s.battleBranchLocked(storyID, req.BranchID)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	commandHash = battleRoundCommitHash(commandHash, req.PlayerIntent)
	if committed, found, err := combatRecordForCommandLocked(meta, records, branchID, commandID, commandHash); err != nil {
		return BattleRoundOutcome{}, err
	} else if found {
		return battleOutcomeFromRecord(committed), nil
	}

	if req.ExpectedHead != "" && meta.Branches[branchID].Head != req.ExpectedHead {
		return BattleRoundOutcome{}, fmt.Errorf("battle state changed during enemy decision; reload the window")
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return BattleRoundOutcome{}, fmt.Errorf("需要战斗 id / advancing a battle requires a session id")
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	live, ok := projection.Combats[sessionID]
	if !ok {
		return BattleRoundOutcome{}, fmt.Errorf("战斗不存在: %s / no such battle", sessionID)
	}
	if req.ExpectedRound > 0 && live.Round != req.ExpectedRound {
		return BattleRoundOutcome{}, fmt.Errorf("battle round changed during enemy decision")
	}
	if live.Ended {
		// The result stands. Nothing is resolved and nothing is committed, so a
		// player who reopens a finished battle cannot fight it again.
		return BattleRoundOutcome{Session: live, Settled: false}, nil
	}
	// The battle must still be the one that last settled these actors' pools.
	//
	// A journal can already hold two live battles for one person — the opening
	// guard only stops new ones, and an older save may already have them. Both
	// sessions write their pools as absolute values, so continuing the older
	// fight would restore the health it last recorded and silently undo whatever
	// the newer fight did. Comparing the battle's pools against the stored ones
	// would not catch it: the legitimate fight also differs from the stored
	// values, because the stored ones are its own last settlement. What does
	// catch it is which battle settled last, which the journal records in order.
	if superseded := battleSupersededBy(records, branchID, live); superseded != "" {
		return BattleRoundOutcome{}, &CombatActorConflict{
			Kind: ErrCombatStaleBattle, SessionID: superseded, Actors: combatantActorIDsOf(live),
		}
	}

	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	live = battleWithCurrentParticipation(live, projection.State, system)
	if live.Ended {
		return s.commitBattleRoundLocked(storyID, meta, records, BattleAppendRequest{
			BranchID: branchID, TurnID: req.TurnID, WorldTime: req.WorldTime, Session: live,
			Outcome:   cultivation.ExchangeOutcome{Round: live.Round, Ended: true, Result: live.Result},
			CommandID: commandID, CommandHash: commandHash,
		})
	}
	actions := battleActions(live, req.Actions)
	accepted := map[string]string{}
	enemies := 0
	for _, actor := range live.Combatants {
		if actor.Side == cultivation.SideEnemy && actor.Active() {
			enemies++
		}
	}
	for actorID, choice := range req.EnemyChoices {
		actor := live.CombatantByID(actorID)
		if actor == nil || actor.Side != cultivation.SideEnemy || !actor.Active() {
			continue
		}
		if action, legal := cultivation.EnemyActionForChoice(live, actorID, choice); legal {
			actions[actorID] = action
			accepted[actorID] = choice
		}
	}
	// The player has to choose. This is checked before the exchange is resolved so
	// an omitted action cannot become the engine's default plain attack, which
	// would spend pools for a decision the player never made.
	if err := requirePlayerAction(live, actions); err != nil {
		return BattleRoundOutcome{}, err
	}
	playerTakeLifeTargetID, err := playerTakeLifeTarget(live, actions, req.PlayerIntent)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	session, outcome, err := ResolveCombatExchange(
		projection.State, system, sessionID, commandID, nil, &live, actions,
	)
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	for _, step := range outcome.Steps {
		if err := playerActionRejection(session, step); err != nil {
			return BattleRoundOutcome{}, err
		}
	}
	previous := live
	lethalIntents := frozenLethalIntentsForBattle(records, meta.Branches[branchID].Head, sessionID)
	return s.commitBattleRoundLocked(storyID, meta, records, BattleAppendRequest{
		BranchID: branchID, TurnID: req.TurnID, WorldTime: req.WorldTime,
		Session: session, Outcome: outcome,
		CommandID: commandID, CommandHash: commandHash,
		enemyDecision:   enemyDecisionReceipt(req.DecisionStatus, accepted, enemies),
		previousSession: &previous, lethalIntents: lethalIntents,
		playerTakeLifeTargetID: playerTakeLifeTargetID,
	})
}

func playerTakeLifeTarget(session cultivation.CombatSession, actions map[string]cultivation.CombatAction, intent PlayerCombatIntent) (string, error) {
	switch intent {
	case "", PlayerCombatIntentSpare:
		return "", nil
	case PlayerCombatIntentTakeLife:
		player := session.CombatantByID(DefaultActorID)
		action, hasAction := actions[DefaultActorID]
		if player == nil || player.Side != cultivation.SidePlayer || !hasAction || (action.Kind != cultivation.ActionTechnique && action.Kind != cultivation.ActionContinuePreparation) {
			return "", ErrCombatTakeLifeActionRequired
		}
		targetID := normalizeStatePanelActorID(action.TargetID)
		if action.Kind == cultivation.ActionContinuePreparation {
			targetID = normalizeStatePanelActorID(session.Preparations[DefaultActorID].TargetID)
		}
		target := session.CombatantByID(targetID)
		if targetID == "" || target == nil || target.Side != cultivation.SideEnemy || !target.Active() {
			return "", ErrCombatTakeLifeTargetInvalid
		}
		return targetID, nil
	default:
		return "", ErrCombatPlayerIntentInvalid
	}
}

// playerActionRejection turns a refused player action into the correctable error
// the window reports.
//
// Only the player's own side is refused here. The player chose those actions, so
// a declaration the rules cannot carry out is a request to correct; the
// opposition's actions were not chosen by the player and cannot be corrected by
// them, and refusing the round over one would leave the player stuck behind a
// decision they never made. An opponent that cannot take the action it wanted
// simply spends the exchange doing nothing, which the settled round records.
func playerActionRejection(session cultivation.CombatSession, step cultivation.ExchangeStep) error {
	if !step.ActionRejected {
		return nil
	}
	actor := session.CombatantByID(step.ActorID)
	if actor == nil || actor.Side != cultivation.SidePlayer {
		return nil
	}
	return fmt.Errorf("%w: actor=%s reason=%s", ErrCombatActionRejected, step.ActorID, step.ReasonKey)
}

// BattleAppendRequest is the store-internal shape of one round to commit.
type BattleAppendRequest struct {
	enemyDecision          *EnemyDecisionReceipt
	BranchID               string
	TurnID                 string
	WorldTime              string
	Session                cultivation.CombatSession
	Outcome                cultivation.ExchangeOutcome
	CommandID              string
	CommandHash            string
	previousSession        *cultivation.CombatSession
	lethalIntents          []CombatLethalIntent
	playerTakeLifeTargetID string
}

// battleActions completes the round's intents.
//
// An enemy with no declared action is asked what it does. The decision reads only
// the enemy's own pools and the publicly visible standing of its target, so
// §14.2's ban on countering the player's hidden abilities holds by construction:
// there is no hidden information in scope to counter with.
//
// It is the same completion for the first exchange and for every later one. The
// opening used to skip it — the battle had no previous session to ask about — and
// that made the first round the one round in which an opponent acted without
// deciding. There is now always a live session to ask, because the opening
// commits the encounter and the first round is played through the round path.
//
// A technique's focus requirement is not filled here. It used to be: the declared
// action carried a caller-supplied flag, and this function set it from the
// combatant's real equipment — but only for a combatant already in an open
// battle, so a focus technique declared in a new battle's opening round was
// refused for an actor who really was holding the focus. The requirement is now
// answered inside the rules from the combatant the round resolves against, which
// is built from real equipment at both entry points. See CombatAction.
func battleActions(
	live cultivation.CombatSession,
	declared map[string]cultivation.CombatAction,
) map[string]cultivation.CombatAction {
	actions := make(map[string]cultivation.CombatAction, len(declared))
	for actorID, action := range declared {
		actions[normalizeStatePanelActorID(actorID)] = action
	}

	for _, combatant := range live.Combatants {
		if combatant.ID == DefaultActorID || !combatant.Active() {
			continue
		}
		// A companion's agreement is to participate, not to become another
		// player-controlled piece. Scripted enemy actions remain supported, while
		// companions choose from their own real skill and resource set.
		if _, declared := actions[combatant.ID]; declared && combatant.Side == cultivation.SideEnemy {
			continue
		}
		fallback, ok := cultivation.NewSkillRegistryWithBuiltins(combatant.Cultivation.Rules()).Current("strike")
		if !ok {
			continue
		}
		actions[combatant.ID] = cultivation.DecideEnemyAction(live, combatant.ID, fallback)
	}
	return actions
}

// requirePlayerAction refuses a round in which the player's own side declared
// nothing.
//
// §七.1 makes every exchange the player's choice. An undeclared combatant reaches
// the rules engine, which settles it as a plain attack — a legal action, but not
// one the player picked, and one that costs real pools. Refusing here is what
// keeps an omitted action from being read as consent to attack, while the
// ordinary attack remains available as something the player can choose.
//
// A player-side combatant who is not active is not asked: a defeated, fled or
// surrendered actor has no action to take, and demanding one would make a round
// unplayable.
func requirePlayerAction(live cultivation.CombatSession, actions map[string]cultivation.CombatAction) error {
	for _, combatant := range live.Combatants {
		if normalizeStatePanelActorID(combatant.ID) != DefaultActorID || combatant.Side != cultivation.SidePlayer || !combatant.Active() {
			continue
		}
		if _, declared := actions[DefaultActorID]; declared {
			return nil
		}
		return fmt.Errorf("%w: session=%s", ErrCombatPlayerActionRequired, live.ID)
	}
	return nil
}

// battleWithCurrentParticipation keeps a recalled or departed creature from
// acting through a stale roster. Its body and spent pools remain unchanged;
// restoring it later does not reactivate this battle's departed participant.
func battleWithCurrentParticipation(live cultivation.CombatSession, state map[string]any, system StoryDirectorActorStateSystem) cultivation.CombatSession {
	next := cloneCombatSession(live)
	players, enemies := 0, 0
	for index := range next.Combatants {
		actor := &next.Combatants[index]
		actor.Cultivation = actor.Cultivation.WithRules(system.CultivationRules)
		if actor.Active() && (actorIsArchived(state, actor.ID) || actor.ID != DefaultActorID && actor.Side == cultivation.SidePlayer && !combatCompanionEligible(state, system, actor.ID)) {
			actor.Fled = true
			delete(next.Preparations, actor.ID)
		}
		if !actor.Active() {
			continue
		}
		if actor.Side == cultivation.SidePlayer {
			players++
		} else {
			enemies++
		}
	}
	switch {
	case players == 0 && enemies == 0:
		next.Ended, next.Result = true, cultivation.ResultDraw
	case enemies == 0:
		next.Ended, next.Result = true, cultivation.ResultEnemyFled
	case players == 0:
		next.Ended, next.Result = true, cultivation.ResultPlayerFled
	}
	return next
}

// combatantActorIDs lists the actors a request names, normalized the same way a
// battle normalizes its participants.
func combatantActorIDs(specs []CombatantSpec) []string {
	ids := make([]string, 0, len(specs))
	for _, spec := range specs {
		if actorID := normalizeStatePanelActorID(spec.ActorID); actorID != "" {
			ids = append(ids, actorID)
		}
	}
	return ids
}

// combatantActorIDsOf lists a battle's participants.
func combatantActorIDsOf(session cultivation.CombatSession) []string {
	ids := make([]string, 0, len(session.Combatants))
	for _, combatant := range session.Combatants {
		if actorID := normalizeStatePanelActorID(combatant.ID); actorID != "" {
			ids = append(ids, actorID)
		}
	}
	return ids
}

// sameCombatants reports whether a request names exactly the participants of a
// live battle, sides included.
//
// It is what tells "the same encounter opened again" from "someone trying to
// fight a second battle". Only the roster is compared: the declared opening
// actions are deliberately not, because the encounter read is what the window
// sends, and a reopened window has no actions of its own to declare.
func sameCombatants(session cultivation.CombatSession, specs []CombatantSpec) bool {
	if len(session.Combatants) != len(specs) {
		return false
	}
	sides := make(map[string]cultivation.CombatSide, len(specs))
	for _, spec := range specs {
		actorID := normalizeStatePanelActorID(spec.ActorID)
		if actorID == "" {
			return false
		}
		if _, duplicate := sides[actorID]; duplicate {
			return false
		}
		sides[actorID] = spec.Side
	}
	for _, combatant := range session.Combatants {
		if sides[normalizeStatePanelActorID(combatant.ID)] != combatant.Side {
			return false
		}
	}
	return true
}

// liveBattleHolding reports the open battle on this branch that already holds any
// of these actors, and which actors it shares.
//
// A battle that has reached a result holds nobody: its participants are free to
// fight again, which is what makes a rematch possible. ignoreSessionID excludes
// one battle from the search, for a caller asking whether a *different* fight
// holds the actors.
func liveBattleHolding(
	combats map[string]cultivation.CombatSession,
	actorIDs []string,
	ignoreSessionID string,
) (string, []string) {
	if len(actorIDs) == 0 || len(combats) == 0 {
		return "", nil
	}
	wanted := make(map[string]bool, len(actorIDs))
	for _, actorID := range actorIDs {
		wanted[actorID] = true
	}
	// Sorted so the reported battle is the same one on every run of the same
	// journal, rather than whichever the map happened to yield first.
	sessionIDs := make([]string, 0, len(combats))
	for sessionID := range combats {
		sessionIDs = append(sessionIDs, sessionID)
	}
	sort.Strings(sessionIDs)
	for _, sessionID := range sessionIDs {
		if sessionID == ignoreSessionID {
			continue
		}
		session := combats[sessionID]
		if session.Ended {
			continue
		}
		shared := make([]string, 0, len(session.Combatants))
		for _, combatant := range session.Combatants {
			actorID := normalizeStatePanelActorID(combatant.ID)
			if wanted[actorID] {
				shared = append(shared, actorID)
			}
		}
		if len(shared) > 0 {
			sort.Strings(shared)
			return sessionID, shared
		}
	}
	return "", nil
}

// battleSupersededBy reports the battle that settled these actors after this one
// did, if there is one.
//
// The journal is ordered, so the answer is read from it rather than guessed from
// the numbers: the last battle record that contains one of this battle's actors
// is the one whose settlement the stored pools belong to. If that record is this
// battle's own, nothing has superseded it and the round may proceed. If it is a
// different battle's, continuing this one would write a snapshot taken before
// that settlement over it.
//
// It deliberately does not require the other battle to still be open. A finished
// fight's final settlement is equally a later word on these pools, and a round
// replayed from an older snapshot would undo it just as surely.
func battleSupersededBy(records []StoryEventRecord, branchID string, live cultivation.CombatSession) string {
	actors := make(map[string]bool, len(live.Combatants))
	for _, combatant := range live.Combatants {
		if actorID := normalizeStatePanelActorID(combatant.ID); actorID != "" {
			actors[actorID] = true
		}
	}
	if len(actors) == 0 {
		return ""
	}
	// Walk the branch's records backwards. The first record that touches one of
	// these actors is the most recent settlement of them, so it answers the
	// question either way: if it is this battle's, nothing has superseded it; if
	// it is another battle's, that one settled last.
	for index := len(records) - 1; index >= 0; index-- {
		record := records[index]
		if record.Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		if event.BranchID != branchID {
			continue
		}
		for _, combatant := range event.Session.Combatants {
			if !actors[normalizeStatePanelActorID(combatant.ID)] {
				continue
			}
			if event.Session.ID == live.ID {
				return ""
			}
			return event.Session.ID
		}
	}
	return ""
}

// battleBranchLocked resolves the branch a battle belongs to and the metadata
// that branch's rules are read from.
func (s *Store) battleBranchLocked(storyID, branchID string) (string, StoryMeta, []StoryEventRecord, error) {
	meta, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return "", StoryMeta{}, nil, err
	}
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return "", StoryMeta{}, nil, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	return branchID, meta, records, nil
}

// battleOutcomeFromRecord presents a committed record as this call's answer, so a
// retry reports the round that was actually fought rather than a fresh one.
//
// Settled is read from the record rather than assumed. An opening record commits
// the encounter with a zero outcome, and reporting it as settled would make a
// retried opening look like a round the player had played — the window would show
// a result for an exchange that never happened.
func battleOutcomeFromRecord(committed CombatRecordResult) BattleRoundOutcome {
	return BattleRoundOutcome{
		EnemyDecision: committed.EnemyDecision,
		Session:       committed.Session, Outcome: committed.Outcome,
		EventID: committed.EventID, Settled: combatRecordSettlesExchange(committed.Outcome),
		SettlementOps: committed.SettlementOps,
		Fatalities:    append([]CombatFatality(nil), committed.Fatalities...), TerminalOutcome: normalizeTerminalOutcomePointer(committed.TerminalOutcome),
	}
}

// commitBattleRoundLocked writes one resolved round through the side-record path.
//
// The store lock is held and the branch's records are already in hand, so the
// commit goes through the locked form rather than re-entering AppendCombatRecord.
func (s *Store) commitBattleRoundLocked(storyID string, meta StoryMeta, records []StoryEventRecord, req BattleAppendRequest) (BattleRoundOutcome, error) {
	committed, err := s.appendCombatRecordLocked(storyID, meta, records, AppendCombatRecordRequest{
		BranchID: req.BranchID, TurnID: req.TurnID, WorldTime: req.WorldTime,
		enemyDecision: req.enemyDecision,
		Session:       req.Session, Outcome: req.Outcome,
		CommandID: req.CommandID, CommandHash: req.CommandHash,
		previousSession: req.previousSession, lethalIntents: req.lethalIntents,
		playerTakeLifeTargetID: req.playerTakeLifeTargetID,
	})
	if err != nil {
		return BattleRoundOutcome{}, err
	}
	return battleOutcomeFromRecord(committed), nil
}
