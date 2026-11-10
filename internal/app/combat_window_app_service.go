package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// This file is the battle window's read and write side.
//
// The rules settle a battle and the journal remembers it; neither of those
// decides what the player is shown or how a click becomes a round. That is this
// file, and three of its choices are deliberate:
//
//   - The window shows the numbers the battle is actually being fought at, not
//     the numbers the panel shows. A character's stored pool is where a wound was
//     last written; a live battle carries the pools as the last exchange left
//     them. §七.1 requires both sides' real pools, so while a battle is open the
//     battle's own numbers are the ones reported.
//
//   - An enemy is described only as far as the player has actually observed them.
//     §三.1 of the NPC information boundary keeps the rules' full truth for
//     settlement and recovery, and the player's view is projected from the same
//     journal rather than from a second store: the protagonist's own side is
//     reported exactly, and everyone else is reported only through what the fight
//     itself put in front of the player. The projection happens here, on the
//     server, because §四 forbids sending the full state and hiding it in the
//     browser.
//
//   - Closing the window is not modelled here at all. §七.1 requires closing to be
//     a display act that neither flees nor ends the fight, and the way to make that
//     structural rather than a rule the interface must remember is to give the
//     close path no operation to call.

// CombatWindowSkill is one learned technique as the window lists it.
type CombatWindowSkill struct {
	cultivation.DefinitionDetails
	InstanceDescription string `json:"instance_description,omitempty"`
	Source              string `json:"source,omitempty"`
	ID                  string `json:"id"`
	Version             int    `json:"version"`
	// NameKey is a localisation key, so the interface supplies the wording.
	NameKey string `json:"name_key,omitempty"`
	// Name is the wording for a technique whose text is its own — a definition this
	// story generated has no localisation key, so the window falls back to this.
	Name  string  `json:"name,omitempty"`
	Power float64 `json:"power,omitempty"`
	// ManaCost and BurdenCost price the cast at this combatant's own stage, so the
	// window reads the cost from the rules rather than from the published table.
	ManaCost   int `json:"mana_cost,omitempty"`
	BurdenCost int `json:"burden_cost,omitempty"`
	// Castable reports whether this combatant could perform it right now, and
	// ReasonKey names the single blocking reason when they could not.
	Castable  bool   `json:"castable"`
	ReasonKey string `json:"reason_key,omitempty"`
	// Targets is how many opponents the technique reaches. The window needs it to
	// offer the extra target choices §9 settles from the player's actual
	// selection, rather than leaving the reach to be discovered after the cast.
	Targets int `json:"targets,omitempty"`
	// Effects are the published properties of this definition, in the rules' own
	// terms. The window renders a sentence from them rather than keeping a second
	// table of what each technique does: a technique whose power or form changes
	// must not leave a hand-written description behind claiming the old numbers.
	Effects CombatWindowSkillEffects `json:"effects"`
	// Variants are the published alternative forms of this technique. §5 fixes
	// exactly two and attaches each to one base technique; §9's scope requires
	// them to be castable, so a form the rules settle has to be reachable from the
	// window rather than only from a hand-written request.
	Variants []CombatWindowVariant `json:"variants,omitempty"`
}

// CombatWindowVariant is one published alternative form of a technique.
//
// A variant is a way of casting rather than a second registered technique, so it
// carries no identifier of its own beyond the action's variant value, and it does
// not change the cost — §5 keeps 耗灵和负担不减 for both forms. What it does change
// is the power, which the window reports so the player can see what the form is
// trading before committing to it.
type CombatWindowVariant struct {
	ID      string  `json:"id"`
	NameKey string  `json:"name_key"`
	Power   float64 `json:"power"`
	// Effect names the lasting effect the form applies on a real hit. It is the
	// rules' own identifier rather than prose, so the window words it from its
	// locale and a form the engine stops applying cannot keep an old sentence.
	Effect string `json:"effect,omitempty"`
}

// CombatWindowSkillEffects is what a technique does beyond its power and cost.
//
// The window needs a sentence of real effect beside every technique (§3 of the
// battle-window design), and the only honest source for it is the definition the
// rules settle. Sending the properties rather than a written sentence keeps one
// authority: a balance revision changes the number, and the window's wording
// follows, instead of a stored description quietly describing a technique that
// no longer exists.
type CombatWindowSkillEffects struct {
	// ExecutionActions is 1 for a technique completed this exchange and 2 for one
	// that prepares first.
	ExecutionActions int `json:"execution_actions,omitempty"`
	// IgnoreDefense is the fraction of the target's defense the technique legally
	// bypasses.
	IgnoreDefense float64 `json:"ignore_defense,omitempty"`
	// RequiresFocus marks a technique that needs its implement in hand.
	RequiresFocus bool `json:"requires_focus,omitempty"`
	// ShieldFraction is the shield size as a fraction of the caster's health
	// maximum, and DurationExchanges is how many exchanges it survives.
	ShieldFraction    float64 `json:"shield_fraction,omitempty"`
	DurationExchanges int     `json:"duration_exchanges,omitempty"`
	// Element is the damage element the rules record for sourced advantage.
	Element string `json:"element,omitempty"`
	// LeavesResidual marks a technique that leaves a meridian residual injury.
	LeavesResidual bool `json:"leaves_residual,omitempty"`
}

// CombatWindowItem is one carried consumable as the window offers it.
//
// The numbers are the catalogue's, not the story's: §4.2 fixes a restoring
// pill's amount to its registered stage and a boosting pill's bonus to its
// grade, so the window shows what the rules will actually pay rather than a
// figure the prose supplied. §3 requires the name, the kind, the grade, the
// rarity and the effect to be shown together.
// CombatWindowEquipment is one position and what really occupies it.
//
// It is reported only for the player's own side, for the same reason the carried
// stock is: what an opponent is holding is a fact the player learns by watching,
// not by reading the fight's state.
//
// An empty position is reported as an entry with no copy rather than omitted, so
// the window can draw all five rows and the player can see that a row is free.
// §1 gives a character five positions; showing three of them because two are
// empty would misdescribe what the sheet allows.
type CombatWindowEquipment struct {
	cultivation.DefinitionDetails
	InstanceDescription string `json:"instance_description,omitempty"`
	Source              string `json:"source,omitempty"`
	// Slot is the engine's position name: weapon, garment, attack_treasure,
	// defense_treasure or support_treasure. The interface supplies the wording.
	Slot string `json:"slot"`
	// InstanceID, DefID and Name name the copy in the position. All three are
	// absent when the position is empty.
	InstanceID string `json:"instance_id,omitempty"`
	DefID      string `json:"def_id,omitempty"`
	Name       string `json:"name,omitempty"`
	// OperatingManaCost is what driving this copy costs for one exchange, priced
	// from its registered definition. It is reported so §1's 行动和灵力消耗真实 is a
	// number the player can read before choosing, not one they discover after.
	OperatingManaCost int `json:"operating_mana_cost,omitempty"`
}

// CombatWindowSpare is one copy carried in the pack that a mid-fight change may
// take into a position.
//
// §1 is explicit that 多余宝物可以放在行囊…不能因为放在包里就加属性, so a spare carries
// no bonus here: it is named, not priced. What it would contribute is answered
// from its definition once it is really in a position.
type CombatWindowSpare struct {
	cultivation.DefinitionDetails
	InstanceDescription string `json:"instance_description,omitempty"`
	Source              string `json:"source,omitempty"`
	// InstanceID is the copy's own identity, which a swap_equipment action names.
	InstanceID string `json:"instance_id"`
	DefID      string `json:"def_id,omitempty"`
	Name       string `json:"name,omitempty"`
	// Slot is the position this copy would occupy, read from its registered
	// definition rather than chosen by the client. §4 maps an item's real use to
	// its row, so a blade is never offered for a treasure position.
	Slot string `json:"slot"`
}

// CombatWindowItem is one carried dose.
type CombatWindowItem struct {
	cultivation.DefinitionDetails
	InstanceDescription string `json:"instance_description,omitempty"`
	Source              string `json:"source,omitempty"`
	// InstanceID names the real copy. It is what a use_item action carries, so
	// §7's definition-versus-instance separation reaches the wire: two copies of
	// one pill are two entries with two counts.
	InstanceID string `json:"instance_id"`
	// DefID names the registered definition this copy was made from.
	DefID   string `json:"def_id"`
	NameKey string `json:"name_key,omitempty"`
	// Quantity is how many doses this copy holds right now.
	Quantity int `json:"quantity"`
	// Purpose is the rules' own pill purpose: restore_health, restore_mana or
	// boost_attack. The interface supplies the wording from it.
	Purpose string `json:"purpose"`
	Grade   string `json:"grade,omitempty"`
	Rarity  string `json:"rarity,omitempty"`
	// RestoreAmount is the absolute amount one dose gives back, priced from the
	// pill's registered stage. It is omitted for a boosting pill.
	RestoreAmount *int `json:"restore_amount,omitempty"`
	// ShieldAmount is the fixed absorption granted by one ward talisman.
	ShieldAmount *int `json:"shield_amount,omitempty"`
	// AttackBonus is the boost a boosting pill grants, as a fraction.
	AttackBonus *float64 `json:"attack_bonus,omitempty"`
	// DurationExchanges is the boost's term in exchanges.
	DurationExchanges *int `json:"duration_exchanges,omitempty"`
	// Usable reports whether this copy could be taken right now, and ReasonKey
	// names the single blocking reason when it could not. §8 requires a refused
	// use to cost nothing, so the window says why rather than hiding the entry.
	Usable    bool   `json:"usable"`
	ReasonKey string `json:"reason_key,omitempty"`
}

// CombatWindowObservation is what the player has actually seen of one opponent.
//
// It is the only channel through which a non-player combatant is described.
// §三.2 of the NPC information boundary fixes what belongs here: the techniques
// that were really performed, a dose that was really taken, the lasting effects
// that are in plain sight, and a coarse condition read from the fight. Everything
// else — the opponent's learned set, its total number of techniques, its costs,
// its pools and its carried stock — is absent, and absent is not zero.
type CombatWindowObservation struct {
	// Condition is the visible state of the opponent in the game's own words.
	// It is a band, not a percentage: §三.2 forbids drawing an estimate as a
	// false-precision figure.
	Condition string `json:"condition"`
	// Techniques is one entry per distinct means the opponent has been seen to
	// use, in the round it was first seen. A technique that was refused still
	// counts as attempted, but it is not identified unless the player really can
	// identify it.
	Techniques []CombatWindowTechnique `json:"techniques,omitempty"`
	// Doses is how many times the opponent has been seen to take a dose. The
	// count is of witnessed acts, not of what is left in the bag.
	Doses int `json:"doses,omitempty"`
	// Shield, BurnTicks and SlowActions are the effects the player can see on
	// them. They are presence, not counts: the player watches a blow turn aside
	// or a wound keep burning, but the size of the shield and how many exchanges
	// it has left are the opponent's own numbers. They are kept apart from the
	// side's own exact fields so an observation can never be mistaken for the
	// fight's settled numbers.
	Shield      bool `json:"shield,omitempty"`
	BurnTicks   bool `json:"burn_ticks,omitempty"`
	SlowActions bool `json:"slow_actions,omitempty"`
}

// CombatWindowTechnique is one means the player has seen an opponent use.
//
// Recognized decides whether the identifier is present at all. An unidentified
// technique still appears — the player watched something happen — but it carries
// no skill_id and no name_key, because either one would name it.
type CombatWindowTechnique struct {
	// Round is the round the player first saw this means used.
	Round int `json:"round"`
	// Recognized reports that a real source identified it for the player.
	Recognized bool `json:"recognized"`
	// SkillID and NameKey are present only when Recognized is true.
	SkillID string `json:"skill_id,omitempty"`
	NameKey string `json:"name_key,omitempty"`
	// Name is the wording for a technique the story generated, which has no
	// localisation key. It is present only alongside a recognition.
	Name string `json:"name,omitempty"`
}

// CombatWindowSide is one combatant as the window shows them.
type CombatWindowSide struct {
	ActorID string `json:"actor_id"`
	// Name is what the story calls this actor. A combatant the story has never
	// introduced falls back to the ID rather than being given an invented name.
	Name string `json:"name"`
	// Side is "player" or "enemy", so the window can lay the two apart.
	Side string `json:"side"`
	// Exact reports that the fields below are the fight's own settled numbers.
	// It is true for the player's own side and false for everyone else, so a
	// reader can tell "no value was settled" apart from "this value is not the
	// player's to know".
	Exact bool `json:"exact"`
	// Realm is the stage key the rules place this combatant at. It is reported as
	// the key, and the interface supplies the wording. Only an exact side reports
	// it: an opponent's realm is the rules' own truth, not an observation.
	Realm string `json:"realm,omitempty"`
	// The pools and attributes are pointers so a value the rules never settled is
	// omitted rather than sent as a zero.
	Health     *int `json:"health,omitempty"`
	HealthMax  *int `json:"health_max,omitempty"`
	Mana       *int `json:"mana,omitempty"`
	ManaMax    *int `json:"mana_max,omitempty"`
	Attack     *int `json:"attack,omitempty"`
	Defense    *int `json:"defense,omitempty"`
	Agility    *int `json:"agility,omitempty"`
	Perception *int `json:"perception,omitempty"`
	// Burden is the meridian burden carried and BurdenFloor the residual injury
	// that floors its recovery.
	Burden      *int `json:"burden,omitempty"`
	BurdenFloor *int `json:"burden_floor,omitempty"`
	// The lasting effects in force. An effect that is not in force is omitted.
	Shield      *int `json:"shield,omitempty"`
	BurnTicks   *int `json:"burn_ticks,omitempty"`
	SlowActions *int `json:"slow_actions,omitempty"`
	// Skills is what this combatant has actually learned, at the versions they
	// learned. It is reported only for the player's own side: §三.2 keeps an
	// opponent's learned set closed no matter how many rounds have been fought.
	Skills []CombatWindowSkill `json:"skills,omitempty"`
	// Items is the consumable stock this combatant really holds, priced from the
	// catalogue. It is reported only for the player's own side: a witnessed dose
	// is recorded as an act under Observed, never as a bag.
	Items []CombatWindowItem `json:"items,omitempty"`
	// Equipment is what really occupies each of the five positions. All five are
	// reported, empty ones included, so the sheet shows what it allows. It is the
	// player's own arrangement, so it is reported only for their side.
	Equipment []CombatWindowEquipment `json:"equipment,omitempty"`
	// Spares is the pack copies a mid-fight change could take up, with the
	// position each would go to. A spare contributes nothing while it sits in the
	// pack; it is listed so the choice is available, not so it adds a number.
	Spares []CombatWindowSpare `json:"spares,omitempty"`
	// HasImplement reports whether the focus a technique needs is in hand. It is
	// the player's own equipment state, so it is reported only for their side.
	HasImplement bool `json:"has_implement"`
	// Standing is how the combatant currently stands.
	Standing string `json:"standing"`
	// OfferText is an outstanding offer to yield which the other side has not yet
	// answered. §14.3 keeps it a proposal: the fight continues until it is answered.
	OfferText string `json:"offer_text,omitempty"`
	// Preparation is the persisted technique this combatant has paid to begin.
	// It is exposed so the window can offer explicit continuation and
	// abandonment rather than inferring state from mana or burden.
	Preparation *CombatWindowPreparation `json:"preparation,omitempty"`
	// Retreat is this combatant's evaluation template. It is a rule the engine
	// applies, not something the player can see, so it is reported only for the
	// player's own side; an opponent's visible willingness to yield arrives as
	// OfferText instead.
	Retreat string `json:"retreat,omitempty"`
	// RouteExists reports whether this combatant really has a way out. §14.2 forbids
	// a retreat without one, so the window shows the fact that turns on — but only
	// for the player's own side, since the alternative is telling the player in
	// advance whether an opponent can be run from.
	RouteExists bool `json:"route_exists"`
	// Observed is how an opponent is described. It is nil for the player's own
	// side, whose exact fields are reported directly.
	Observed *CombatWindowObservation `json:"observed,omitempty"`
}

type CombatWindowPreparation struct {
	SkillID      string `json:"skill_id"`
	NameKey      string `json:"name_key,omitempty"`
	Name         string `json:"name,omitempty"`
	TargetID     string `json:"target_id,omitempty"`
	StartedRound int    `json:"started_round"`
}

// CombatWindowRound is one committed exchange as the history shows it.
type CombatWindowRound struct {
	EnemyDecision *interactive.EnemyDecisionReceipt `json:"enemy_decision,omitempty"`
	EventID       string                            `json:"event_id"`
	WorldTime     string                            `json:"world_time,omitempty"`
	Outcome       cultivation.ExchangeOutcome       `json:"outcome"`
	Fatalities    []interactive.CombatFatality      `json:"fatalities,omitempty"`
	// Narration is the prose written from this exchange, when one was written.
	// It is omitted rather than sent empty: §8 lets a round exist without its
	// passage, and an empty string would read as a round the model described as
	// nothing rather than one it has not described yet.
	Narration string `json:"narration,omitempty"`
	// NarrationStyleID names the combat style the passage was written in, so the
	// window can show that a style switch left existing prose alone.
	NarrationStyleID string `json:"narration_style_id,omitempty"`
	// NarrationEventID is the committed narration record, so a client can tell a
	// passage that exists from one that does not.
	NarrationEventID string `json:"narration_event_id,omitempty"`
}

// CombatWindowScene is the scene block the window displays.
type CombatWindowScene struct {
	NodeID    string `json:"node_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Location  string `json:"location,omitempty"`
	WorldTime string `json:"world_time,omitempty"`
}

// CombatWindowEncounter is the fight the scene already settles, offered before it
// begins.
//
// The window used to hand the interface the whole cast and ask the player to side
// each name. That made a bystander into an enemy and let someone who is elsewhere
// join. The scene's own facts decide it now, so this block is what the panel
// opens with: who is fighting, on which side, and who is merely standing there.
type CombatWindowEncounter struct {
	// Combatants are the participants with the side the scene puts them on.
	Combatants []CombatWindowCombatant `json:"combatants,omitempty"`
	// Bystanders are present people the scene does not put in the fight.
	Bystanders []string `json:"bystanders,omitempty"`
	// ReasonKey says why no battle can be opened here. It is a localisation key.
	ReasonKey string `json:"reason_key,omitempty"`
}

// CombatWindowCombatant is one derived participant.
type CombatWindowCombatant struct {
	ActorID string `json:"actor_id"`
	Name    string `json:"name,omitempty"`
	Side    string `json:"side"`
	// RouteExists and Retreat are the scene's own facts about this participant,
	// reported so the interface can send back what it was shown when it opens the
	// fight. A client that re-typed only the ids would drop the protagonist's way
	// out, and the escape it is entitled to would be refused by the rules.
	//
	// They are reported for the player's own side only: §三.2 keeps whether an
	// opponent can be run from closed until the fight shows it.
	RouteExists bool   `json:"route_exists,omitempty"`
	Retreat     string `json:"retreat,omitempty"`
}

// CombatWindow is the whole battle window.
type CombatWindow struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	// Scene is where the story currently stands, so the window shows that the fight
	// belongs to the scene on screen.
	Scene CombatWindowScene `json:"scene"`
	// Encounter is the fight the scene settles, reported on the entry read so the
	// panel can open one without asking the player to side the cast. It is omitted
	// once a battle is open, because then Sides is the fight.
	Encounter *CombatWindowEncounter `json:"encounter,omitempty"`
	// Sessions lists every live battle on this branch, so a story with more than one
	// fight open can offer the choice.
	Sessions []string `json:"sessions,omitempty"`
	// SessionID is the battle the window is open on.
	SessionID string `json:"session_id,omitempty"`
	// Round is the exchange about to be resolved, 1-based.
	Round int `json:"round,omitempty"`
	// Ended and Result report a battle that has reached a result. §七.8 requires a
	// settled result to stand, so a reopened window shows it rather than offering
	// another round.
	Ended  bool   `json:"ended"`
	Result string `json:"result,omitempty"`
	// LethalRisk is derived from the intent frozen when this battle opened.
	LethalRisk bool `json:"lethal_risk,omitempty"`
	// PlayerActorID names the actor whose actions the player composes. It is the
	// side the interface offers controls for, so it is decided here rather than
	// guessed from a standing that may have changed.
	PlayerActorID string `json:"player_actor_id,omitempty"`
	// Sides is every combatant of the open battle.
	Sides []CombatWindowSide `json:"sides,omitempty"`
	// LastRound is the exchange just settled, so the window can show what happened
	// without re-resolving it. It carries its own prose, so the passage and the
	// numbers it was written from are read together rather than from two places
	// that could disagree.
	LastRound *CombatWindowRound `json:"last_round,omitempty"`
	// History is the committed rounds, oldest first, bounded to the requested page.
	History []CombatWindowRound `json:"history,omitempty"`
	// HasMore says the battle continues past this page of history.
	HasMore bool `json:"has_more"`
	// Cursor pages further back when HasMore is set.
	Cursor string `json:"cursor,omitempty"`
	// NarrationError reports that the last round's prose is missing. §8 keeps the
	// settled result: the round is committed and stands, and only its expression
	// failed, so the window shows the round and says the passage is missing
	// rather than pretending the round did not happen.
	NarrationError string `json:"narration_error,omitempty"`
	// NarrationState says which of the two missing-prose cases this is:
	// CombatNarrationPending when the round is committed and nothing has been
	// attempted or written yet, CombatNarrationFailed when a call was made and
	// did not produce a passage. §3 requires "待生成" and "失败" to be visibly
	// different, and a reader of the journal can only tell them apart because the
	// failed case is the one that carries a reason.
	NarrationState string `json:"narration_state,omitempty"`
}

// The two states a committed round without prose can be in.
const (
	// CombatNarrationPending means no passage exists and no attempt is recorded
	// against this read. A process interrupted between settlement and prose
	// leaves exactly this, which is why it is derived from the journal.
	CombatNarrationPending = "pending"
	// CombatNarrationFailed means an attempt was made and produced no passage.
	CombatNarrationFailed = "failed"
)

// ErrCombatUnavailableForWorld means the story's frozen package declares no
// independent combat mode. The request is rejected at the application boundary
// even when it arrives without the combat button.
var ErrCombatUnavailableForWorld = fmt.Errorf("independent combat is unavailable in this world")

// CombatWindowRequest selects what to read.
type CombatWindowRequest struct {
	Locale   string
	StoryID  string
	BranchID string
	// SessionID selects a battle. Empty reports the scene and the list of live
	// battles, which is what the entry point needs before the player picks one.
	SessionID string
	// BeforeCursor pages backwards through the battle's history.
	BeforeCursor string
	Limit        int
}

// CombatRoundRequest is one round the player plays.
type CombatRoundRequest struct {
	StoryID  string
	BranchID string
	// SessionID is empty when the player is opening a new battle.
	SessionID string
	// Combatants names the participants when a battle is being opened. It is ignored
	// when SessionID names an open battle.
	Combatants []interactive.CombatantSpec
	// Actions is the round's declared intents, by actor ID. The player's side
	// supplies its own; an enemy left out is decided from its own state.
	//
	// An opening carries no actions: §七.1 makes the first exchange the player's
	// own decision, and it can only be made once the encounter is on screen.
	Actions map[string]cultivation.CombatAction
	// PlayerIntent is a deliberate consequence choice for the action submitted
	// in this round. It defaults to spare and is only accepted for a settled turn.
	PlayerIntent interactive.PlayerCombatIntent
	// CommandID is the caller-owned idempotency key. A retry with the same command ID
	// must not resolve a second round or draw again.
	CommandID string
	// TurnID attaches the battle to a story turn when it began inside one.
	TurnID string
	Locale string
	// SkipNarration commits the exchange without asking for its prose.
	//
	// §6.6 orders settlement before expression, and §3 requires the settled result
	// to be visible while the prose is still being written. The window needs a
	// call that stops at the committed round to show it, then asks for the passage
	// through the narration route under the same command identity. Skipping never
	// changes what was settled: the round is the same one an inline call would
	// have produced, and the prose written afterwards is written about it.
	SkipNarration bool
	// Context carries the caller's cancellation and tracing into the round's
	// narration. A round is settled and committed before the model is asked for
	// its passage, so cancelling here abandons the prose and never the round.
	Context context.Context
}

// ErrCombatUnknown means the requested battle is not live on this branch. The
// window must not invent a fight to show.
var ErrCombatUnknown = fmt.Errorf("battle is not live on this branch")

// ErrCombatNoEncounter means the scene does not settle a fight: nobody hostile is
// present, or presence itself is unsettled. The reason key carried with it names
// which, so the window can say what is missing instead of opening an empty battle.
var ErrCombatNoEncounter = fmt.Errorf("the scene does not settle an encounter")

// ErrCombatOpeningHasActions means an opening request also declared a round.
//
// §七.1 splits the two: opening establishes the encounter and shows both sides at
// the numbers they already have, and the first exchange happens only when the
// player has seen that and chosen an action. Accepting both in one request would
// settle an exchange the player could not have seen the state of, which is the
// defect this split exists to remove — so the request is refused rather than
// silently interpreted one way or the other.
var ErrCombatOpeningHasActions = fmt.Errorf("opening a battle must not declare a round's actions")

const (
	defaultCombatWindowLimit = 40
	maxCombatWindowLimit     = 200
)

// CombatWindow reads the battle window.
//
// It is one read, so the scene, the battle and its history cannot disagree with
// each other, and it reads the same projection the main story's own state is
// projected from.
func (a *App) CombatWindow(request CombatWindowRequest) (CombatWindow, error) {
	if a == nil {
		return CombatWindow{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return CombatWindow{}, fmt.Errorf("the combat window requires a story")
	}
	a.mu.RLock()
	store := a.interactive
	projectID := ""
	if a.cfg != nil {
		projectID = a.cfg.ProjectID
	}
	a.mu.RUnlock()
	if store == nil {
		return CombatWindow{}, ErrNoWorkspace
	}

	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return CombatWindow{}, err
	}
	if !storyUsesCultivationCombat(storyContext) {
		return CombatWindow{}, ErrCombatUnavailableForWorld
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	sessions, err := store.CombatSessions(storyID, branchID)
	if err != nil {
		return CombatWindow{}, err
	}
	scene := interactive.StoryScene(storyContext.Snapshot)

	window := CombatWindow{
		StoryID: storyID, BranchID: branchID,
		Scene: CombatWindowScene{
			NodeID: scene.NodeID, Title: scene.Title, Location: scene.Location,
			WorldTime: characterWorldTime(storyContext),
		},
		Sessions: combatWindowSessionIDs(sessions),
	}

	sessionID := strings.TrimSpace(request.SessionID)
	if sessionID == "" {
		// The entry read reports the encounter the scene already settles, so the
		// panel can show the fight rather than a roster to side. The names come from
		// the same cast a battle reports.
		characterState, err := store.CharacterState(storyID, branchID)
		if err != nil {
			return CombatWindow{}, err
		}
		encounter, err := store.CombatEncounter(storyID, branchID)
		if err != nil {
			return CombatWindow{}, err
		}
		window.Encounter = combatWindowEncounter(encounter, characterState)
		return window, nil
	}
	session, ok := sessions[sessionID]
	if !ok {
		return CombatWindow{}, fmt.Errorf("%w: %s", ErrCombatUnknown, sessionID)
	}
	// The names come from the story's own cast, so a battle reports the names the
	// story has actually introduced.
	characterState, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return CombatWindow{}, err
	}

	window.SessionID = sessionID
	window.Round = session.Round
	window.Ended = session.Ended
	window.Result = string(session.Result)
	window.PlayerActorID = combatWindowPlayerActorID(session)

	history, err := store.CombatHistory(storyID, branchID, sessionID)
	if err != nil {
		return CombatWindow{}, err
	}
	lethalIntents, err := store.CombatLethalIntents(storyID, branchID, sessionID)
	if err != nil {
		return CombatWindow{}, err
	}
	for _, intent := range lethalIntents {
		attacker, target := session.CombatantByID(intent.ActorID), session.CombatantByID(intent.TargetID)
		if attacker != nil && target != nil && attacker.Side != target.Side && attacker.Side == cultivation.SideEnemy && target.Side == cultivation.SidePlayer {
			window.LethalRisk = true
			break
		}
	}
	// The player's view of everyone else is derived from the same committed
	// rounds the history shows, so what the window says an opponent did and what
	// the player is offered to act on cannot disagree.
	window.Sides = combatWindowSides(session, characterState, combatWindowObservations(
		session, history, combatWindowPlayerSkills(session),
	))
	if entries, detailErr := store.CultivationEntryDetails(storyID, branchID, request.Locale); detailErr == nil {
		var frozen map[string]cultivation.DefinitionDetails
		if schema := storyContext.Snapshot.ActorStateSchema; schema != nil {
			frozen = schema.System.DefinitionDetails
		}
		attachCombatEntryDetails(&window, entries, frozen, request.Locale, projectID)
	}
	// The prose is read alongside the rounds and joined by round record, so a
	// round shows the passage written from it rather than the latest passage
	// written about the fight.
	narrations, err := store.CombatNarrations(storyID, branchID, sessionID)
	if err != nil {
		return CombatWindow{}, err
	}
	page := combatWindowHistoryPage(combatWindowRounds(history, narrations, combatWindowPlayerSide(session)), request.BeforeCursor, combatWindowLimit(request.Limit))
	window.History = page.rounds
	window.HasMore = page.hasMore
	window.Cursor = page.cursor
	if len(page.rounds) > 0 {
		last := page.rounds[len(page.rounds)-1]
		window.LastRound = &last
		// §8 splits the round from its expression, and §3 C01 requires the retry
		// entry to survive a restart. The two only work together if the recovery
		// state is derived from the journal rather than remembered by the process
		// that wrote the round: a process interrupted after settlement and before
		// its passage left no in-memory flag to carry, so the fact "this round is
		// committed and has no prose" is read back here. Setting the flag is all
		// this does — the passage is written only when the player asks for it, so
		// an ordinary read never calls the model.
		if last.Narration == "" {
			window.NarrationError = combatNarrationMissingMessage
			window.NarrationState = CombatNarrationPending
		}
	}
	return window, nil
}

// combatNarrationMissingMessage reports a committed round whose passage is
// absent.
//
// It states the durable fact rather than an error from a call this read did not
// make: nothing failed here, the round simply has no prose yet, and the passage
// can be written without replaying the round. The wording is deliberately about
// what is true of the journal, because that is what the read actually knows.
const combatNarrationMissingMessage = "这一段叙述尚未写入。补写不会重新结算本回合，也不会改变已有结果。"

// PlayCombatRound resolves and commits one exchange.
//
// It is the only write the window has. Closing the window has none, which is what
// makes §七.1's "关闭只是收起" structural rather than a rule the interface has to
// remember: there is no operation a close could call.
//
// The round is idempotent on CommandID. A repeated click, a lost response or a
// recovery re-read returns the exchange the first attempt settled, so a player
// cannot re-roll a bad round by clicking again.
func (a *App) PlayCombatRound(request CombatRoundRequest) (CombatWindow, error) {
	if a == nil {
		return CombatWindow{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return CombatWindow{}, fmt.Errorf("a combat round requires a story")
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return CombatWindow{}, ErrNoWorkspace
	}

	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return CombatWindow{}, err
	}
	if !storyUsesCultivationCombat(storyContext) {
		return CombatWindow{}, ErrCombatUnavailableForWorld
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	commandID := strings.TrimSpace(request.CommandID)
	sessionID := strings.TrimSpace(request.SessionID)
	if request.PlayerIntent == "" {
		request.PlayerIntent = interactive.PlayerCombatIntentSpare
	}
	// The fingerprint is resolved against the branch the store will actually write
	// to, so an empty branch ID and an explicit current branch are the same command
	// rather than a conflict.
	commandHash := combatRoundCommandHash(request, branchID)
	if request.PlayerIntent != "" && request.PlayerIntent != interactive.PlayerCombatIntentSpare && request.PlayerIntent != interactive.PlayerCombatIntentTakeLife {
		return CombatWindow{}, interactive.ErrCombatPlayerIntentInvalid
	}

	var settled interactive.BattleRoundOutcome
	if sessionID == "" {
		// An opening establishes the encounter and nothing else. The first
		// exchange waits for the player to see the fight and choose an action, so a
		// request that carries both is refused rather than resolved unseen.
		if len(request.Actions) > 0 || request.PlayerIntent == interactive.PlayerCombatIntentTakeLife {
			return CombatWindow{}, ErrCombatOpeningHasActions
		}
		// A new battle is opened from the encounter the scene already settles, so
		// the player is never asked to side the whole cast. Combatants still wins
		// when the caller names one: an explicit choice is a fact the interface may
		// hold and the derivation may not, and the rules must not overrule it.
		combatants := request.Combatants
		if len(combatants) == 0 {
			encounter, err := store.CombatEncounter(storyID, branchID)
			if err != nil {
				return CombatWindow{}, err
			}
			if encounter.ReasonKey != "" || len(encounter.Combatants) == 0 {
				return CombatWindow{}, fmt.Errorf("%w: %s", ErrCombatNoEncounter, encounter.ReasonKey)
			}
			combatants = encounter.Combatants
		}
		result, err := store.StartBattle(storyID, interactive.BattleStartRequest{
			BranchID: branchID, SessionID: sessionID,
			WorldTime:  characterWorldTime(storyContext),
			Combatants: combatants,
			TurnID:     request.TurnID, CommandID: commandID, CommandHash: commandHash,
		})
		if err != nil {
			return CombatWindow{}, err
		}
		settled = result
		sessionID = result.Session.ID
	} else {
		// Only the protagonist's declared action crosses the public boundary.
		// Scripted enemies remain a Store capability, not a client override of AI.
		playerActions := make(map[string]cultivation.CombatAction)
		for actorID, action := range request.Actions {
			if strings.TrimSpace(actorID) == interactive.DefaultActorID {
				playerActions[interactive.DefaultActorID] = action
			}
		}
		roundRequest := interactive.BattleRoundRequest{
			BranchID: branchID, SessionID: sessionID,
			Actions: playerActions, PlayerIntent: request.PlayerIntent, WorldTime: characterWorldTime(storyContext),
			TurnID: request.TurnID, CommandID: commandID, CommandHash: commandHash,
		}
		settled, err = a.resolveBattleWithEnemyDecision(request.Context, store, storyID, roundRequest)
		if err != nil {
			return CombatWindow{}, err
		}
		sessionID = settled.Session.ID
	}

	window, err := a.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: branchID, SessionID: sessionID, Locale: request.Locale,
	})
	if err != nil {
		return CombatWindow{}, err
	}
	// §6.6 orders the round before its prose: the exchange is settled and
	// committed, and only then is a description asked for. §8 keeps the two
	// separate outcomes apart — a failure here is reported on a window whose
	// round is already durable, so the player never loses a settled result
	// because its expression failed.
	if settled.Settled && settled.EventID != "" && !request.SkipNarration {
		ctx := request.Context
		if ctx == nil {
			ctx = context.Background()
		}
		// The prose is written under the round's own command identity, so a retry
		// of this round is answered from the committed passage rather than
		// rendering the same exchange a second time.
		if _, err := a.NarrateCombatRound(ctx, CombatNarrationRequest{
			StoryID: storyID, BranchID: branchID, SessionID: sessionID,
			RoundEventID: settled.EventID, CommandID: commandID, Locale: request.Locale,
		}); err != nil {
			window.NarrationError = err.Error()
			window.NarrationState = CombatNarrationFailed
		} else {
			window, err = a.CombatWindow(CombatWindowRequest{
				StoryID: storyID, BranchID: branchID, SessionID: sessionID, Locale: request.Locale,
			})
			if err != nil {
				return CombatWindow{}, err
			}
		}
	}
	return window, nil
}

// combatWindowSessionIDs lists the live battles in stable order, so a window
// reopened on the same branch offers the same choice.
func combatWindowSessionIDs(sessions map[string]cultivation.CombatSession) []string {
	ids := make([]string, 0, len(sessions))
	for id := range sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// combatWindowSides presents every combatant of a battle.
//
// §三.1 of the NPC information boundary splits the rules' truth from the player's
// view: the fight keeps every number so it can be settled and recovered, and the
// window reports the protagonist exactly and everyone else (including allies) through
// what the player has actually observed. The split is made here rather than in
// the interface because §四 forbids sending the full state and hiding it in the
// browser.
//
// The pools come from the battle rather than from the character's stored state,
// because a battle's pools are what the exchanges have been settling against: a
// wound the last round dealt is on the battle and not yet on the stored value, and
// showing the stored value would report a health the fight is not using.
func combatWindowSides(session cultivation.CombatSession, characters character.State, observed map[string]*CombatWindowObservation) []CombatWindowSide {
	sides := make([]CombatWindowSide, 0, len(session.Combatants))
	for _, combatant := range session.Combatants {
		exact := combatant.ID == interactive.DefaultActorID && combatant.Side == cultivation.SidePlayer
		side := CombatWindowSide{
			ActorID: combatant.ID, Name: combatWindowName(characters, combatant.ID),
			Side:     string(combatant.Side),
			Exact:    exact,
			Standing: combatWindowStanding(combatant),
		}
		if exact {
			side.Realm = cultivation.StageKey(combatant.Cultivation)
			side.Health = intPointer(combatant.Attributes.Health)
			side.HealthMax = intPointer(combatant.Attributes.HealthMax)
			side.Mana = intPointer(combatant.Attributes.Mana)
			side.ManaMax = intPointer(combatant.Attributes.ManaMax)
			side.Attack = intPointer(combatant.Attributes.Attack)
			side.Defense = intPointer(combatant.Attributes.Defense)
			side.Agility = intPointer(combatant.Attributes.Agility)
			side.Perception = intPointer(combatant.Attributes.Perception)
			side.Burden = intPointer(combatant.Burden.Current)
			side.BurdenFloor = intPointer(combatant.Burden.Residual)
			side.HasImplement = combatant.HasImplement
			side.Retreat = string(combatant.Retreat)
			side.RouteExists = combatant.RouteExists
			side.Skills = combatWindowSkills(combatant)
			side.Items = combatWindowItems(combatant)
			side.Equipment = combatWindowEquipment(combatant)
			side.Spares = combatWindowSpares(combatant)
			// A preparation is the player's own paid commitment, so the window
			// learns it for their side only. §三.2 keeps an opponent's pending
			// technique closed: what the player can see of it is the observation
			// the round actually produced, not a queue read off the session.
			if preparation, ok := session.Preparations[combatant.ID]; ok {
				side.Preparation = &CombatWindowPreparation{
					SkillID: preparation.Skill.ID, NameKey: preparation.Skill.NameKey,
					Name:     preparation.Skill.Name,
					TargetID: preparation.TargetID, StartedRound: preparation.StartedRound,
				}
			}
		} else {
			// An opponent is described only through the observation, so the
			// exact fields above stay absent rather than zero. The visible
			// effects are the ones the fight put in plain sight, and they are
			// kept inside the observation so they cannot be read as settled
			// numbers.
			side.Observed = observed[combatant.ID]
			if side.Observed == nil {
				side.Observed = &CombatWindowObservation{
					Condition: combatWindowVisibleCondition(combatant),
				}
			}
		}
		// An offer to yield is an act the other side made and the player is
		// answering, so it is reported for both sides: withholding it would hide
		// a decision the player has to make.
		side.OfferText = combatant.OfferText
		// A lasting effect that is not in force is left out rather than sent as a
		// zero, so the window shows the effects the fight is actually under. On an
		// exact side they are the fight's own numbers; on an observed side they
		// have already been folded into the observation.
		if exact {
			if combatant.Shield.Amount > 0 {
				side.Shield = intPointer(combatant.Shield.Amount)
			}
			if combatant.Burn.RemainingTicks > 0 {
				side.BurnTicks = intPointer(combatant.Burn.RemainingTicks)
			}
			if combatant.Slow.RemainingTargetActions > 0 {
				side.SlowActions = intPointer(combatant.Slow.RemainingTargetActions)
			}
		}
		sides = append(sides, side)
	}
	return sides
}

// combatWindowObservations derives what the player has seen of each non-player
// combatant from the rounds the fight actually committed.
//
// §三.3 makes the committed exchange the source: the player watches an act, not
// a dossier. So this reads the same journal the history is read from and takes
// only what a witness would have seen — a means that was used, a dose that was
// swallowed, an effect that is visible on the body. Nothing here consults the
// opponent's learned set, its stock or its realm.
//
// The protagonist's own learned set is the one real identification this game
// has: the player recognises an art they have learned themselves. A technique the
// player has never learned is recorded as an act with no name, which is why the
// identifier is dropped rather than merely flagged.
func combatWindowObservations(session cultivation.CombatSession, history []interactive.CombatRoundRecord, playerSkills map[string]cultivation.SkillDef) map[string]*CombatWindowObservation {
	observations := make(map[string]*CombatWindowObservation, len(session.Combatants))
	for _, combatant := range session.Combatants {
		if combatant.ID == interactive.DefaultActorID && combatant.Side == cultivation.SidePlayer {
			continue
		}
		observations[combatant.ID] = &CombatWindowObservation{
			Condition:   combatWindowVisibleCondition(combatant),
			Shield:      combatant.Shield.Amount > 0,
			BurnTicks:   combatant.Burn.RemainingTicks > 0,
			SlowActions: combatant.Slow.RemainingTargetActions > 0,
		}
	}
	for _, round := range history {
		for _, step := range round.Outcome.Steps {
			observation, ok := observations[step.ActorID]
			if !ok {
				continue
			}
			switch step.Kind {
			case cultivation.ActionUseItem:
				// §三.3: watching a dose go down says an act happened, not what
				// was in the bottle or what else is in the bag.
				if step.ItemApplied {
					observation.Doses++
				}
			case cultivation.ActionTechnique:
				if strings.TrimSpace(step.SkillID) == "" || step.SkillID == cultivation.OrdinaryAttackSkillID {
					// A plain attack is not a learned means, and the ordinary
					// strike every combatant can make says nothing about what
					// they know.
					continue
				}
				seen := combatObservedTechnique(step.SkillID, playerSkills)
				seen.Round = round.Outcome.Round
				if known, exists := observedTechniqueAt(observation.Techniques, seen); exists {
					// One means seen twice is still one entry, at the round it
					// was first seen: §三.2 keeps the observation time stable.
					*known = mergeObservedTechnique(*known, seen)
					continue
				}
				observation.Techniques = append(observation.Techniques, seen)
			}
		}
	}
	return observations
}

// combatObservedTechnique words one seen means for the player.
//
// The identifier is present only when the player really can name it. The learned
// set is the protagonist's own, so this is the same evidence the story uses when
// it says a character recognises a move — not an inference from having watched
// it.
func combatObservedTechnique(skillID string, playerSkills map[string]cultivation.SkillDef) CombatWindowTechnique {
	held, known := playerSkills[skillID]
	if !known {
		return CombatWindowTechnique{Recognized: false}
	}
	return CombatWindowTechnique{
		Recognized: true, SkillID: skillID, NameKey: held.NameKey, Name: held.Name,
	}
}

// observedTechniqueAt finds an existing entry for the same means, matched by
// identifier when the player can name it and by round-order otherwise.
func observedTechniqueAt(techniques []CombatWindowTechnique, seen CombatWindowTechnique) (*CombatWindowTechnique, bool) {
	if !seen.Recognized {
		return nil, false
	}
	for index := range techniques {
		if techniques[index].Recognized && techniques[index].SkillID == seen.SkillID {
			return &techniques[index], true
		}
	}
	return nil, false
}

// mergeObservedTechnique keeps the earlier observation and lets a later
// recognition name it, without moving the round it was first seen in.
func mergeObservedTechnique(existing, seen CombatWindowTechnique) CombatWindowTechnique {
	if seen.Recognized && !existing.Recognized {
		existing.Recognized = true
		existing.SkillID = seen.SkillID
		existing.NameKey = seen.NameKey
	}
	return existing
}

// combatWindowVisibleCondition words an opponent's condition as a band rather
// than a figure.
//
// §三.2 forbids laying out an enemy's exact backend pools and forbids drawing an
// estimate as a false-precision percentage, so the fight's real health fraction
// is reduced to the few bands a witness could actually tell apart.
func combatWindowVisibleCondition(combatant cultivation.Combatant) string {
	switch combatant.Standing() {
	case cultivation.StandingFled:
		return "combatWindow.condition.fled"
	case cultivation.StandingSurrendered:
		return "combatWindow.condition.surrendered"
	case cultivation.StandingDefeated:
		return "combatWindow.condition.defeated"
	}
	switch fraction := combatant.HealthFraction(); {
	case fraction <= 0.25:
		return "combatWindow.condition.critical"
	case fraction <= 0.5:
		return "combatWindow.condition.wounded"
	case fraction <= 0.85:
		return "combatWindow.condition.hurt"
	default:
		return "combatWindow.condition.unharmed"
	}
}

// combatWindowPlayerActorID names the actor whose actions the player composes.
//
// The player's side is a property of the battle, not of how a combatant happens
// to be standing, so it is read from the side rather than from a standing that a
// round may have changed.
func combatWindowPlayerActorID(session cultivation.CombatSession) string {
	for _, combatant := range session.Combatants {
		if combatant.ID == interactive.DefaultActorID && combatant.Side == cultivation.SidePlayer {
			return combatant.ID
		}
	}
	return ""
}

// combatWindowPlayerSkills reads the protagonist's own learned set out of the
// battle, which is the only source of recognition this game has for a technique
// someone else used. The battle already resolved that set from the protagonist's
// real state when it opened, so this is the same list the player's own side is
// shown rather than a second reading of the character sheet.
//
// The definitions are returned rather than a set of IDs because a recognised
// technique has to be named, and a definition this story generated has no
// localisation key to name it with.
func combatWindowPlayerSkills(session cultivation.CombatSession) map[string]cultivation.SkillDef {
	for _, combatant := range session.Combatants {
		if combatant.ID != interactive.DefaultActorID || combatant.Side != cultivation.SidePlayer {
			continue
		}
		skills := make(map[string]cultivation.SkillDef, len(combatant.Skills))
		for _, skill := range combatant.Skills {
			if id := strings.TrimSpace(skill.ID); id != "" {
				skills[id] = skill
			}
		}
		return skills
	}
	return nil
}

// combatWindowName is the name the story knows an actor by.
//
// A combatant the cast has never introduced keeps its ID, which is honest: the
// player has not been told a name, and inventing one would put a name in front of
// them that the story never gave.
func combatWindowName(characters character.State, actorID string) string {
	if profile, ok := characters.Profile(actorID); ok {
		if name := strings.TrimSpace(profile.DisplayName()); name != "" {
			return name
		}
	}
	return actorID
}

// combatWindowEncounter projects the derived encounter.
//
// Names come from the story's own cast, so the panel shows the names the story
// introduced. A participant the cast has not named keeps its actor ID, which is
// the name the story language itself uses.
func combatWindowEncounter(encounter interactive.CombatEncounter, characters character.State) *CombatWindowEncounter {
	if encounter.ReasonKey == "" && len(encounter.Combatants) == 0 {
		return nil
	}
	projected := &CombatWindowEncounter{
		Bystanders: encounter.Bystanders, ReasonKey: encounter.ReasonKey,
	}
	for _, spec := range encounter.Combatants {
		combatant := CombatWindowCombatant{
			ActorID: spec.ActorID, Name: combatWindowName(characters, spec.ActorID),
			Side: string(spec.Side),
		}
		if spec.Side == cultivation.SidePlayer {
			combatant.RouteExists = spec.RouteExists
			combatant.Retreat = string(spec.Retreat)
		}
		projected.Combatants = append(projected.Combatants, combatant)
	}
	return projected
}

// combatWindowStanding names how a combatant currently stands.
//
// The wording is the rules' own, so the window and the handoff back to the main
// story cannot drift apart on what a yielded opponent is.
func combatWindowStanding(combatant cultivation.Combatant) string {
	return string(combatant.Standing())
}

// combatWindowSkills prices a combatant's own learned techniques.
//
// The list is the combatant's learned set, resolved at the version they learned,
// and every cast check is made against their own pools — so an opponent's list
// shows what that opponent can actually do rather than what the game's table
// contains. §七.1 requires exactly that: a technique the opponent never learned
// must not appear in their list.
func combatWindowSkills(combatant cultivation.Combatant) []CombatWindowSkill {
	skills := make([]CombatWindowSkill, 0, len(combatant.Skills))
	for _, skill := range combatant.Skills {
		if skill.ID == "" {
			continue
		}
		// The battle snapshot carries the actual circulating reductions. Preview
		// and resolution must use the same mana and burden costs.
		check := cultivation.NextExchangeCastPlanFor(combatant, skill, 0).Check
		skills = append(skills, CombatWindowSkill{
			DefinitionDetails: skill.DefinitionDetails.ForLocale("zh-CN"), ID: skill.ID, Version: skill.Version, NameKey: skill.NameKey, Name: skill.Name,
			Power:    skill.Power,
			ManaCost: check.ManaCost, BurdenCost: check.BurdenCost,
			Castable: check.Allowed, ReasonKey: check.ReasonKey, Targets: skill.Targets,
			Effects: CombatWindowSkillEffects{
				ExecutionActions:  skill.ExecutionActions,
				IgnoreDefense:     skill.IgnoreDefense,
				RequiresFocus:     skill.RequiresFocus,
				ShieldFraction:    skill.ShieldBaseHealthFraction,
				DurationExchanges: skill.DurationExchanges,
				Element:           skill.Element,
				LeavesResidual:    skill.LeavesResidual,
			},
			Variants: combatWindowVariants(skill),
		})
	}
	return skills
}

// combatWindowVariants lists the published forms of one technique.
//
// The power is the variant's own, because that is what the cast will use; the
// cost is not repeated here because §5 keeps it at the base technique's. The
// name key is derived from the technique's rather than stored, so a form cannot
// be listed under a name the content never published.
func combatWindowVariants(skill cultivation.SkillDef) []CombatWindowVariant {
	variants := cultivation.VariantsForDefinition(skill)
	if len(variants) == 0 {
		return nil
	}
	projected := make([]CombatWindowVariant, 0, len(variants))
	for _, variant := range variants {
		power, ok := cultivation.VariantPowerFor(skill, variant)
		if !ok {
			continue
		}
		entry := CombatWindowVariant{ID: string(variant), Power: power, Effect: combatWindowVariantEffect(variant)}
		// A generated technique has no localisation key, so a form's key derived
		// from its ID could never resolve. The window words the form itself from
		// the published form, and sending an unresolvable key would only invite a
		// translation that does not exist.
		if skill.NameKey != "" {
			entry.NameKey = "skill." + skill.ID + ".variant." + string(variant)
		}
		projected = append(projected, entry)
	}
	return projected
}

// combatWindowVariantEffect names the lasting effect a form applies.
//
// It reports the engine's own variant identifier, not a sentence: the window
// words it, and a form the engine stopped applying cannot keep a description
// here claiming it still does.
func combatWindowVariantEffect(variant cultivation.CombatVariant) string {
	switch variant {
	case cultivation.VariantBurn, cultivation.VariantSlow:
		return string(variant)
	default:
		return ""
	}
}

// combatWindowEquipment lists all five positions with what really occupies each.
//
// The list is built from the engine's own position order rather than from what
// the session happens to hold, so a free row is still a row. §1 gives the sheet
// five positions and §4 forbids guessing a category from a name, so nothing here
// invents an occupant for an empty position.
func combatWindowEquipment(combatant cultivation.Combatant) []CombatWindowEquipment {
	positions := make([]CombatWindowEquipment, 0, len(cultivation.SwappableEquipmentSlots))
	for _, slot := range cultivation.SwappableEquipmentSlots {
		position := CombatWindowEquipment{Slot: string(slot)}
		if copy, ok := combatant.EquipmentInSlot(slot); ok {
			position.InstanceID = copy.InstanceID
			position.DefID = copy.DefID
			position.Name = copy.Name
			position.OperatingManaCost = copy.OperatingManaCost
		}
		positions = append(positions, position)
	}
	return positions
}

// combatWindowSpares lists the pack copies a mid-fight change could take up.
//
// Nothing is priced here. §1's 不能因为放在包里就加属性 is structural: a spare carries
// no bonus on the wire, so the window cannot show a number the rules would not
// pay. What the copy would contribute is answered from its definition once it is
// really in a position.
func combatWindowSpares(combatant cultivation.Combatant) []CombatWindowSpare {
	if len(combatant.Spares) == 0 {
		return nil
	}
	spares := make([]CombatWindowSpare, 0, len(combatant.Spares))
	for _, spare := range combatant.Spares {
		spares = append(spares, CombatWindowSpare{
			InstanceID: spare.InstanceID, DefID: spare.DefID,
			Name: spare.Name, Slot: string(spare.Slot),
		})
	}
	return spares
}

// combatWindowItems lists the consumable stock a combatant holds, with the
// effect the rules would actually pay.
//
// The amount is priced from the pill's own registered stage rather than from the
// user's pool, which is §4.2's rule that a stronger character does not get a
// stronger dose. The usability check mirrors the engine's own refusals exactly —
// an exhausted copy, a pill that is not usable, a pool already at its maximum —
// so the window never offers a use the round would refuse, and §8's "a refused
// use costs nothing" is a fact the player can see rather than discover.
func combatWindowItems(combatant cultivation.Combatant) []CombatWindowItem {
	if len(combatant.Carried) == 0 {
		return nil
	}
	items := make([]CombatWindowItem, 0, len(combatant.Carried))
	for _, dose := range combatant.Carried {
		pill := dose.Pill
		item := CombatWindowItem{
			InstanceID: dose.InstanceID, DefID: dose.DefID, NameKey: dose.NameKey,
			Quantity: dose.Quantity,
			Purpose:  string(pill.Purpose),
			Grade:    string(pill.Grade), Rarity: string(pill.Rarity),
		}
		if dose.Talisman != nil {
			ward := dose.Talisman.ShieldFor(combatant.Cultivation, "talisman:"+dose.DefID)
			item.Purpose = "shield"
			if definition, ok := cultivation.NewItemCatalogWithFirstVersion().Definition(dose.DefID); ok {
				item.Grade, item.Rarity = string(definition.Grade), string(definition.Rarity)
			}
			item.ShieldAmount = intPointer(ward.Amount)
			item.DurationExchanges = intPointer(ward.RemainingExchanges)
		}
		switch pill.Purpose {
		case cultivation.PillRestoreHealth, cultivation.PillRestoreMana:
			amount := pill.RestoreAmount(combatant.Attributes)
			item.RestoreAmount = intPointer(amount)
		case cultivation.PillBoostAttack:
			if bonus, ok := pill.AttackBonus(); ok {
				item.AttackBonus = floatPointer(bonus)
			}
			item.DurationExchanges = intPointer(pill.DurationExchanges)
		case cultivation.PillSupportCultivation:
			// The window still lists the copy, because a player may want to know
			// what they are carrying into a fight, but it is not a battle action
			// and carries no exchange effect to price.
		}
		item.Usable, item.ReasonKey = combatWindowItemUsability(combatant, dose, pill)
		items = append(items, item)
	}
	return items
}

// combatWindowItemUsability answers whether a copy could be taken right now.
//
// The reasons are the engine's own keys, in the engine's own order, so a refusal
// the window shows and a refusal the round reports are the same answer rather
// than two independently maintained lists.
func combatWindowItemUsability(combatant cultivation.Combatant, dose cultivation.CarriedDose, pill cultivation.PillDef) (bool, string) {
	if strings.TrimSpace(dose.InstanceID) == "" {
		return false, "reason.item_not_named"
	}
	if dose.Quantity <= 0 {
		return false, "reason.item_exhausted"
	}
	if dose.Talisman != nil {
		reason := cultivation.TalismanUseReason(combatant, *dose.Talisman, dose.DefID)
		return reason == "", reason
	}
	switch pill.Purpose {
	case cultivation.PillRestoreHealth:
		if pill.RestoreAmount(combatant.Attributes) <= 0 ||
			combatant.Attributes.Health >= combatant.Attributes.HealthMax {
			return false, "reason.pool_already_full"
		}
	case cultivation.PillRestoreMana:
		if pill.RestoreAmount(combatant.Attributes) <= 0 ||
			combatant.Attributes.Mana >= combatant.Attributes.ManaMax {
			return false, "reason.pool_already_full"
		}
	case cultivation.PillBoostAttack:
		if bonus, ok := pill.AttackBonus(); !ok || bonus <= 0 {
			return false, "reason.item_not_usable"
		}
	case cultivation.PillSupportCultivation:
		// Spent by a training interval rather than by an exchange, so the window
		// never offers it as a battle action.
		return false, "reason.item_not_usable"
	default:
		return false, "reason.item_not_usable"
	}
	return true, ""
}

// combatWindowRounds presents the committed rounds as the window's own shape.
func combatWindowRounds(history []interactive.CombatRoundRecord, narrations map[string]interactive.CombatNarration, playerSide map[string]bool) []CombatWindowRound {
	rounds := make([]CombatWindowRound, 0, len(history))
	for _, round := range history {
		presented := CombatWindowRound{
			EnemyDecision: round.EnemyDecision,
			EventID:       round.EventID, WorldTime: round.WorldTime,
			Outcome:    combatWindowProjectedOutcome(round.Outcome, playerSide),
			Fatalities: append([]interactive.CombatFatality(nil), round.Fatalities...),
		}
		if narration, ok := narrations[round.EventID]; ok {
			presented.Narration = narration.Narrative
			presented.NarrationStyleID = narration.StyleID
			presented.NarrationEventID = narration.EventID
		}
		rounds = append(rounds, presented)
	}
	return rounds
}

// combatWindowProjectedOutcome is the committed round as the player may see it.
//
// §四 names the nested case explicitly: the round is part of the payload, so its
// steps are a surface too, and sending the raw exchange would put an opponent's
// technique identifier, cast cost and recovered amount in the browser behind a
// collapsed row. The settlement keeps the whole step; the player's copy of an
// opponent's step is built from the acts a witness saw and nothing else.
//
// The opponent's step is constructed field by field rather than copied and
// blanked, so a field added to the settlement later is withheld by default
// instead of leaking the moment somebody forgets to clear it.
func combatWindowProjectedOutcome(outcome cultivation.ExchangeOutcome, playerSide map[string]bool) cultivation.ExchangeOutcome {
	if len(outcome.Steps) == 0 {
		return outcome
	}
	steps := make([]cultivation.ExchangeStep, 0, len(outcome.Steps))
	for _, step := range outcome.Steps {
		if playerSide[step.ActorID] {
			steps = append(steps, step)
			continue
		}
		// What the player watched: who acted, at whom, what sort of act it was,
		// whether it came off, and what was plainly left on the bodies afterwards.
		// Withheld: the technique or pill that names the act, its form, what it
		// cost, what it gave back, why it was refused, and what an implement cost
		// to drive. A further target's amount is withheld too — the player saw a
		// companion take a blow, not how much health it cost them.
		projected := cultivation.ExchangeStep{
			ActorID:              step.ActorID,
			TargetID:             step.TargetID,
			Kind:                 step.Kind,
			PreparationAbandoned: step.PreparationAbandoned,
			Cast:                 cultivation.CastCheck{Allowed: step.Cast.Allowed},
			Resolution:           step.Resolution,
			ItemApplied:          step.ItemApplied,
			BurnApplied:          step.BurnApplied,
			SlowApplied:          step.SlowApplied,
			Escaped:              step.Escaped,
			Surrendered:          step.Surrendered,
			Defeated:             step.Defeated,
		}
		// A blow that landed on the player's own side keeps its amount: that loss
		// is the player's to know exactly. A blow on anyone else keeps only whether
		// it landed, for the same reason the further targets do.
		if !playerSide[step.TargetID] {
			projected.Resolution = cultivation.AttackResolution{Hit: step.Resolution.Hit}
		}
		if len(step.ExtraTargets) > 0 {
			extra := make([]cultivation.ExtraTargetStrike, 0, len(step.ExtraTargets))
			for _, strike := range step.ExtraTargets {
				extra = append(extra, cultivation.ExtraTargetStrike{
					TargetID: strike.TargetID,
					Resolution: cultivation.AttackResolution{
						Hit: strike.Resolution.Hit,
					},
					BurnApplied: strike.BurnApplied,
					SlowApplied: strike.SlowApplied,
					Defeated:    strike.Defeated,
				})
			}
			projected.ExtraTargets = extra
		}
		steps = append(steps, projected)
	}
	outcome.Steps = steps
	return outcome
}

// combatWindowPlayerSide identifies the protagonist for exact history filtering.
// Sharing a battle side grants neither control nor access to an ally's dossier.
func combatWindowPlayerSide(session cultivation.CombatSession) map[string]bool {
	player := make(map[string]bool, 1)
	for _, combatant := range session.Combatants {
		if combatant.ID == interactive.DefaultActorID && combatant.Side == cultivation.SidePlayer {
			player[combatant.ID] = true
		}
	}
	return player
}

type combatWindowPage struct {
	rounds  []CombatWindowRound
	hasMore bool
	cursor  string
}

// combatWindowHistoryPage returns the most recent rounds, or the ones before a
// cursor when the player has scrolled back.
//
// It pages on commit order, which is the order CombatHistory reads back from the
// journal, so a restart pages the same way.
func combatWindowHistoryPage(history []CombatWindowRound, beforeCursor string, limit int) combatWindowPage {
	ordered := history
	before := strings.TrimSpace(beforeCursor)
	if before != "" {
		cut := len(ordered)
		for index, round := range ordered {
			if round.EventID == before {
				cut = index
				break
			}
		}
		ordered = ordered[:cut]
	}
	if limit <= 0 {
		limit = defaultCombatWindowLimit
	}
	page := combatWindowPage{}
	if len(ordered) <= limit {
		page.rounds = ordered
		return page
	}
	page.rounds = ordered[len(ordered)-limit:]
	page.hasMore = true
	page.cursor = page.rounds[0].EventID
	return page
}

func combatWindowLimit(value int) int {
	if value <= 0 {
		return defaultCombatWindowLimit
	}
	if value > maxCombatWindowLimit {
		return maxCombatWindowLimit
	}
	return value
}

// combatRoundCommandHash fingerprints the content behind a round's command ID.
//
// It covers what actually decides the round — the battle, the declared intents and
// the participants — so reusing one command ID for a genuinely different round is
// refused rather than silently answered with the wrong one.
func combatRoundCommandHash(request CombatRoundRequest, branchID string) string {
	parts := []string{
		"denova.combat.round",
		strings.TrimSpace(request.StoryID),
		strings.TrimSpace(branchID),
		strings.TrimSpace(request.SessionID),
		strings.TrimSpace(request.TurnID),
		string(normalizeCombatPlayerIntent(request.PlayerIntent)),
	}
	parts = append(parts, combatActionFingerprint(request.Actions)...)
	for _, spec := range request.Combatants {
		parts = append(parts, fmt.Sprintf("spec\x00%s\x00%s\x00%t\x00%s",
			spec.ActorID, spec.Side, spec.RouteExists, spec.Retreat))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x01")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func normalizeCombatPlayerIntent(intent interactive.PlayerCombatIntent) interactive.PlayerCombatIntent {
	if intent == "" {
		return interactive.PlayerCombatIntentSpare
	}
	return intent
}

// combatActionFingerprint orders the declared intents deterministically, because a
// map's iteration order is not stable and a fingerprint that changed between two
// identical rounds would refuse a legitimate retry.
//
// It also covers the inputs that decide an action's effect without being part of
// its shape: the carried copy a use_item action spends, the spare a swap takes
// into a position, and the text a surrender offers. Each is read the way the
// engine reads it (the copies are trimmed, the offer is kept verbatim) and only
// for the kind that consumes it, so a field a kind ignores cannot make two
// equivalent requests look different. An empty value adds nothing, which keeps
// every fingerprint that existed before these fields were covered byte-for-byte
// unchanged.
func combatActionFingerprint(actions map[string]cultivation.CombatAction) []string {
	if len(actions) == 0 {
		return nil
	}
	actorIDs := make([]string, 0, len(actions))
	for actorID := range actions {
		actorIDs = append(actorIDs, actorID)
	}
	sort.Strings(actorIDs)
	parts := make([]string, 0, len(actorIDs))
	for _, actorID := range actorIDs {
		action := actions[actorID]
		part := fmt.Sprintf("action\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s",
			actorID, action.Kind, action.Skill.ID, action.Variant, action.TargetID,
			strings.Join(action.ExtraTargetIDs, ","))
		switch action.Kind {
		case cultivation.ActionUseItem:
			if instanceID := strings.TrimSpace(action.ItemInstanceID); instanceID != "" {
				part += "\x00item\x00" + instanceID
			}
		case cultivation.ActionSwapEquipment:
			// Which copy comes in decides the whole action: two requests naming
			// different spares are different rounds, and a retry that names the
			// same one must fingerprint the same.
			if instanceID := strings.TrimSpace(action.SwapInstanceID); instanceID != "" {
				part += "\x00swap\x00" + instanceID
			}
		case cultivation.ActionSurrender:
			if action.OfferText != "" {
				part += "\x00offer\x00" + action.OfferText
			}
		}
		parts = append(parts, part)
	}
	return parts
}

// intPointer lets a settled value be reported while an unset one is omitted. It is
// the difference between "zero health" and "no health recorded", which the window
// must not blur.
func intPointer(value int) *int {
	return &value
}

// floatPointer is intPointer's counterpart for a fraction such as a dose's boost.
func floatPointer(value float64) *float64 {
	return &value
}
