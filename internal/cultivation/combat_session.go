package cultivation

import (
	"sort"
	"strings"
)

// This file implements the battle exchange loop of 《自制-合理战斗_研究与初步方案》
// V0.5 §6 and 《自制-技能与施展》V0.3 §4–§5.
//
// Everything it calls already existed: hit chance, damage, shields, burden,
// casting checks, lasting effects and formations were all implemented and
// unit-tested, and nothing during play ever drove them. This is the orchestrator
// that does, so a battle round is settled by the same rules the panel is settled
// by rather than by the narrative describing a fight.
//
// Two properties are deliberate and load-bearing:
//
//   - It is pure. A session goes in, a session and an outcome come out, and the
//     caller commits the difference through the story's own commit path. There
//     is no second world state, which §8 of the window design forbids.
//
//   - Every random draw is supplied by the caller through DrawSource. The design
//     requires that a reopened window, a retry or a refresh re-use the draws it
//     already used, so a battle cannot be re-rolled by clicking again. Taking
//     the draws as a function makes that structural rather than a convention.

// CombatSide distinguishes the player's party from the opposition.
type CombatSide string

const (
	SidePlayer CombatSide = "player"
	SideEnemy  CombatSide = "enemy"
)

// CombatActionKind is what one combatant does in one exchange.
//
// The set is the one §5.2 of the battle design lists as available: a technique,
// an item, a retreat, or negotiation. A prepared technique also has explicit
// continue and abandon intents so neither completion nor cancellation hides
// inside another primary action. There is deliberately no generic
// attack/defend/feint menu — §6 of the numeric rules removed it, and an ordinary
// attack is expressed as a zero-cost technique instead.
type CombatActionKind string

const (
	// ActionTechnique is performing a registered technique, including the
	// zero-cost ordinary attack.
	ActionTechnique CombatActionKind = "technique"
	// ActionRest is spending the primary action on quiet regulation, which
	// recovers mana only if the character was not wounded this exchange.
	ActionRest CombatActionKind = "rest"
	// ActionEscape is a real attempt to disengage. It requires an established
	// route; the caller must not offer it from a sealed position.
	ActionEscape CombatActionKind = "escape"
	// ActionSurrender is an offer to yield. It is a real action by the offering
	// side, and it does not end the fight: §14.3 requires the other side to be
	// able to accept, refuse, impose conditions or keep acting, so the offer
	// leaves the battle unresolved until they answer with
	// ActionAcceptSurrender.
	ActionSurrender CombatActionKind = "surrender"
	// ActionAcceptSurrender is the answer that takes an outstanding offer. Only
	// an explicit acceptance removes the yielding side from the fight, which is
	// what makes §14.3's "求饶不自动获得同意" structural rather than a convention
	// the caller has to remember.
	ActionAcceptSurrender CombatActionKind = "accept_surrender"
	// ActionUseItem spends one dose of a carried pill, which is §5.2's 物品.
	// The dose is named by instance rather than by effect: §7 of the item design
	// requires that the definition and the real copy be kept apart, so what the
	// action carries is which copy was taken, and the numbers come from the
	// catalogue rather than from the request.
	ActionUseItem CombatActionKind = "use_item"
	// ActionContinuePreparation spends this exchange's primary action completing
	// the actor's already-paid prepared technique. It does not run normal cast
	// affordability checks or recover mana.
	ActionContinuePreparation CombatActionKind = "continue_preparation"
	// ActionCancelPreparation abandons a pending technique without refund and
	// without performing any other action this exchange.
	ActionCancelPreparation CombatActionKind = "cancel_preparation"
	// ActionSwapEquipment takes one carried spare into its slot and puts the
	// copy that was there back in the pack. It is the whole of §2's
	// 战斗中要从行囊把一件备品换入3个随身位，需花一个主要行动收起和取出: the exchange's
	// primary action is spent on the swap itself, and the swapped-in copy cannot
	// also be driven this exchange.
	//
	// It is deliberately a primary action rather than a free step between
	// rounds. §2 requires 关闭窗口、换视图、暂停或重开不能绕过该动作和消耗, and the
	// only way to make that structural is for the swap to be resolved by the
	// engine as one combatant's action, with its own step in the round record.
	ActionSwapEquipment CombatActionKind = "swap_equipment"
)

// EquipmentSlot names one of the positions §1 of 《自制-装备栏与法宝御使》 lists in
// its 随身装配 table.
//
// The identifiers are the engine's own rather than the story's wording, because
// the engine matches a spare to the position it replaces. §4 maps an item's real
// use to its position — 主法器 is the weapon, 护具 is the garment — and the caller
// performs that mapping when it builds the battle, so nothing here guesses a
// category from a name.
//
// 布阵 is deliberately not one of these. §4 keeps a deployed formation a real
// formation rather than turning it into a support treasure, and §1's table has
// five positions with no formation among them; a formation is raised and ended
// through its own state, not swapped in from the pack mid-fight.
type EquipmentSlot string

const (
	// EquipmentSlotWeapon is the one main weapon position.
	EquipmentSlotWeapon EquipmentSlot = "weapon"
	// EquipmentSlotGarment is the one garment position. It grants protection only
	// when the copy really has a protective definition; ordinary clothing does not
	// add defence by its name.
	EquipmentSlotGarment EquipmentSlot = "garment"
	// The three 法宝 positions. §1 allows one copy of each kind at a time, so
	// these are three positions rather than one slot with three copies.
	EquipmentSlotAttackTreasure  EquipmentSlot = "attack_treasure"
	EquipmentSlotDefenseTreasure EquipmentSlot = "defense_treasure"
	EquipmentSlotSupportTreasure EquipmentSlot = "support_treasure"
)

// SwappableEquipmentSlots is the set a mid-fight swap may move a copy into.
//
// It is exported because the settlement has to write back exactly the positions
// a swap can move, and a second list in the caller would drift from this one.
var SwappableEquipmentSlots = []EquipmentSlot{
	EquipmentSlotWeapon,
	EquipmentSlotGarment,
	EquipmentSlotAttackTreasure,
	EquipmentSlotDefenseTreasure,
	EquipmentSlotSupportTreasure,
}

// CombatVariant is an alternative form of a technique.
//
// A variant is a way of casting, not a separate registered technique: it
// replaces the base definition's power for that casting, which is why it lives
// on the action rather than on the definition. §5 fixes exactly two, and they
// are never both attached to one casting.
type CombatVariant string

const (
	VariantNone CombatVariant = ""
	// VariantBurn is 火弹灼烧: the direct hit is weaker and the target keeps
	// burning for two more exchanges.
	VariantBurn CombatVariant = "burn"
	// VariantSlow is 重雷迟滞: the direct hit is weaker and the target loses
	// agility until its next primary action completes.
	VariantSlow CombatVariant = "slow"
)

// A variant is a form of one technique, and which one is a fact about that
// definition rather than a second table: SkillDef.VariantBase records it, so a
// generated technique built from a template keeps the form its template
// publishes. The attachment is explicit rather than inferred from the element,
// because a fire technique that is not the fire bolt must not silently acquire a
// burning form the design never gave it.

// VariantPower returns the power a variant casts at, and whether the variant
// applies to the given built-in technique at all.
//
// The definition is resolved first so the answer comes from its own VariantBase
// rather than from a second table that could disagree with it. A generated
// technique has no entry here — its identity is not a published ID — so callers
// holding a definition use VariantPowerFor instead.
func VariantPower(variant CombatVariant, skillID string) (float64, bool) {
	skill, ok := SkillByID(skillID)
	if !ok {
		return 0, false
	}
	return VariantPowerFor(skill, variant)
}

// VariantPowerFor returns the power a variant casts at for one definition.
//
// The form comes from the definition itself. That is what lets a generated
// technique keep the form its template publishes: §5 attaches 燎痕 to the fire
// template and 迟滞 to the thunder template, and an instance built from either
// carries that attachment in its own definition.
func VariantPowerFor(skill SkillDef, variant CombatVariant) (float64, bool) {
	if variant == VariantNone {
		return 0, false
	}
	if skill.VariantBase != variant {
		return 0, false
	}
	switch variant {
	case VariantBurn:
		return BurnVariantPower, true
	case VariantSlow:
		return SlowVariantPower, true
	}
	return 0, false
}

// VariantsFor returns the published variants of a built-in technique, in a
// stable order.
//
// §5 fixes exactly two forms and attaches each to one base technique. The window
// needs this list to offer them: a form the rules settle but the interface never
// shows is a capability the player has paid for and cannot reach. The order is
// fixed rather than taken from the map so the same technique always lists its
// forms the same way.
func VariantsFor(skillID string) []CombatVariant {
	skill, ok := SkillByID(skillID)
	if !ok {
		return nil
	}
	return VariantsForDefinition(skill)
}

// VariantsForDefinition lists the forms one definition publishes.
func VariantsForDefinition(skill SkillDef) []CombatVariant {
	if skill.VariantBase == VariantNone {
		return nil
	}
	return []CombatVariant{skill.VariantBase}
}

// RetreatTemplate is an enemy's willingness to keep fighting.
//
// §14.2 of the battle design specifies the two published templates and requires
// that the evaluation be a real check against real state rather than a fixed
// health line that always produces the same answer.
type RetreatTemplate string

const (
	// RetreatNone is a combatant with no retreat evaluation at all: a mindless
	// beast, a construct, or anyone the design says gets no surrender template.
	RetreatNone RetreatTemplate = ""
	// RetreatSurvival evaluates retreat, begging or a truce at 30% health or
	// below, or when no usable primary technique remains.
	RetreatSurvival RetreatTemplate = "survival"
	// RetreatCautious evaluates the same options earlier, at 50% health or
	// below, but only while a clear disadvantage has actually been observed.
	RetreatCautious RetreatTemplate = "cautious"
)

// Combatant is one participant's live battle state.
//
// The attributes are the settled current values, not a stage key: a battle
// mutates health and mana every exchange, and re-deriving them from the stage
// each time would discard the wound the previous exchange just dealt.
type Combatant struct {
	ID          string      `json:"id"`
	Side        CombatSide  `json:"side"`
	Cultivation Cultivation `json:"cultivation"`
	// Attributes carries the current pools and the fully aggregated effective
	// values, so the combat formulas read one consistent set of numbers.
	Attributes Attributes  `json:"attributes"`
	Burden     BurdenState `json:"burden"`
	// The lasting effects are plain structs, so they are always written. A zero
	// one reads back as "not in force", which is the same answer the rules give
	// it, and the alternative — a pointer per effect — would put a nil check in
	// every rule that touches one.
	Shield    Shield         `json:"shield"`
	Burn      DamageOverTime `json:"burn"`
	Slow      SlowEffect     `json:"slow"`
	Formation FormationState `json:"formation"`
	// Retreat is the enemy's evaluation template.
	Retreat RetreatTemplate `json:"retreat,omitempty"`
	// RouteExists reports whether a real way out of this fight is available to
	// this combatant. §14.2 forbids retreat without one.
	RouteExists bool `json:"route_exists,omitempty"`

	// Skills is what this combatant has actually learned, at the versions they
	// learned. §14.2 requires an enemy to use only its own registered abilities
	// with resources it really has, so the enemy's own technique set is carried
	// here and chosen from — not read off the registry, which holds every
	// definition in the game including ones this opponent never learned.
	Skills []SkillDef `json:"skills,omitempty"`
	// HasImplement reports whether the focus a technique requires is really in
	// this combatant's hands. The caller establishes it from equipment; a
	// combatant that never had one must not be given a free pass on it.
	HasImplement bool `json:"has_implement,omitempty"`
	// ManaReduction is the casting cost reduction this combatant's own
	// circulating methods grant, capped at the rules' 20%. It is a property of
	// the combatant rather than of one declared action, because a method reduces
	// every technique they cast for as long as it circulates — unlike a dose,
	// which applies to the round it was taken in.
	ManaReduction   float64 `json:"mana_reduction,omitempty"`
	StrainReduction float64 `json:"strain_reduction,omitempty"`
	// OperatingManaCost is what the item in hand costs to drive for one
	// exchange, from the equipped definition's own published fraction.
	//
	// §4.1 and 数值规则 line 128 require this cost to be real: 普通攻击的真实法器
	// 操控成本依装备定义，不因按普攻按钮免费施展. It is charged once per offensive
	// action taken while the implement is in hand, not once per technique, because
	// the implement is being driven for the whole exchange either way.
	//
	// It is the definition's fraction of the wielder's bare stage mana, computed
	// by the caller from the catalogue so a recorded copy cannot make its own
	// treasure cheaper.
	OperatingManaCost int `json:"operating_mana_cost,omitempty"`

	// Modifiers are the combatant's own intrinsic, sourced extras: the equipped
	// implement, the worn armour, a deployed formation and any lasting effect
	// already in force. They are carried so a dose taken mid-fight aggregates
	// with them rather than compounding on top of an already-aggregated Attack.
	//
	// §5 of the item design requires every real attribute to be re-summed from
	// the stage base and the currently effective sources. Multiplying a pill's
	// percentage onto Attributes.Attack would do the opposite: the attack would
	// then depend on whether the implement had been folded in before or after,
	// and the global cap would be applied to a number that already had a bonus
	// baked in.
	Modifiers []AttributeModifier `json:"modifiers,omitempty"`
	// Equipment is what this combatant is really wearing or holding in each
	// position, as live battle state.
	//
	// It is carried here rather than left in Modifiers because §2 lets a position
	// change mid-fight: a spare taken from the pack has to stop contributing the
	// copy it replaced and start contributing its own, in the same exchange. A
	// modifier list copied once at the opening would keep paying the old copy for
	// the rest of the battle, which is precisely the failure the design forbids.
	Equipment []EquipmentCopy `json:"equipment,omitempty"`
	// Spares is the equipment this combatant carries but has not taken into a
	// position. §1 is explicit that 多余宝物可以放在行囊…不能因为放在包里就加属性, so
	// nothing here contributes to the numbers while it sits in the pack.
	Spares []EquipmentCopy `json:"spares,omitempty"`
	// Carried is the consumable stock this combatant really holds, by instance.
	// It is a per-battle copy: spending a dose here does not by itself rewrite
	// the character's inventory, which the caller commits through its own
	// settlement path so a refused or repeated round cannot eat a second dose.
	Carried []CarriedDose `json:"carried,omitempty"`
	// ActiveDoses records the temporary effects this combatant's own doses have
	// put in force, so a boost survives the round it was taken in and expires on
	// its own term rather than being re-derived from the inventory each round.
	ActiveDoses []ActiveDose `json:"active_doses,omitempty"`

	// OfferText is an outstanding offer to yield this combatant has made and
	// the other side has not yet answered. It is deliberately separate from
	// Surrendered: an offer is a proposal, and the fight continues until it is
	// accepted.
	OfferText string `json:"offer_text,omitempty"`

	Defeated    bool `json:"defeated,omitempty"`
	Fled        bool `json:"fled,omitempty"`
	Surrendered bool `json:"surrendered,omitempty"`
}

// Active reports whether this combatant can still act and be targeted.
func (c Combatant) Active() bool {
	return !c.Defeated && !c.Fled && !c.Surrendered
}

// CarriedDose is one real consumable copy a combatant has with them in a fight.
//
// It is the battle-side view of §7's distinction: the definition is a
// registered entry with fixed numbers, and this is one real copy with its own
// remaining quantity. The numbers come from the catalogue through Pill, never
// from the story's prose, which is what makes §2's "AI不能因戏剧需要把普通药物写成
// 十倍攻击" hold during a fight rather than only outside one.
type CarriedDose struct {
	// InstanceID identifies the copy, so a retried round spends the same dose
	// rather than a second one.
	InstanceID string `json:"instance_id"`
	// DefID names the registered definition this copy was made from.
	DefID string `json:"def_id"`
	// NameKey is the localisation key, so this layer supplies an identity and the
	// interface supplies the wording.
	NameKey string `json:"name_key,omitempty"`
	// Quantity is how many doses remain in this copy.
	Quantity int `json:"quantity"`
	// Pill is the rules payload, read from the catalogue.
	Pill PillDef `json:"pill"`
	// Talisman is an alternative consumable payload; it never grants pill effects.
	Talisman *TalismanDef `json:"talisman,omitempty"`
}

// EquipmentCopy is one real object in one position, or one real spare in the
// pack.
//
// The numbers are the published ones the caller read from the catalogue, for
// the same reason a dose carries its PillDef rather than trusting the request:
// an item's effect is a property of its registered definition, so a battle never
// re-reads a bonus from the story's wording. The identity is the copy's own, so
// the swap can name one object and the settlement can write the same object
// back.
type EquipmentCopy struct {
	// Rated, definition-derived effects. They apply only when this real copy is driven.
	DefensePierce   int     `json:"defense_pierce,omitempty"`
	StrainReduction float64 `json:"strain_reduction,omitempty"`
	// InstanceID identifies the real copy. It is the pack key for a spare, and
	// the recorded name for a copy already in a position, so one object is
	// recognisable across the pack and the slots.
	InstanceID string `json:"instance_id"`
	// DefID names the registered definition, when the copy came from the
	// catalogue. A copy the story wrote by hand has none, and contributes no
	// published numbers.
	DefID string `json:"def_id,omitempty"`
	// Name is what the story calls this copy.
	Name string `json:"name,omitempty"`
	// Slot is the position it occupies. A spare carries the slot it would go to
	// when taken into hand, so the engine can refuse one that fits nowhere.
	Slot EquipmentSlot `json:"slot"`
	// ApplicableStage is the registered operating floor, supplied by the trusted
	// inventory reader, never by the action declaring a swap.
	ApplicableStage string `json:"applicable_stage,omitempty"`
	// Driven records payment for this exchange's numeric treasure effect. It is
	// reset at the next exchange, not by closing or reopening the window.
	Driven bool `json:"driven,omitempty"`
	// SwappedThisExchange prevents a newly taken-up treasure from also being
	// driven before the next exchange. Reopening a window does not clear it.
	SwappedThisExchange bool `json:"swapped_this_exchange,omitempty"`
	// Modifiers are the sourced extras this copy contributes while it occupies
	// its position. An empty list is the honest answer for a copy the rules never
	// gave a bonus — ordinary clothing, or a treasure with no published number.
	Modifiers []AttributeModifier `json:"modifiers,omitempty"`
	// OperatingManaCost is what driving this copy costs for one exchange, from its
	// definition. A paid treasure contributes only after its effect is driven.
	OperatingManaCost int `json:"operating_mana_cost,omitempty"`
}

// ActiveDose is a temporary effect a dose put in force and that is still running.
//
// It is recorded rather than recomputed from the inventory, because a dose taken
// in an earlier exchange has been spent: re-reading the stock would either lose
// the effect or pay for it a second time.
type ActiveDose struct {
	// InstanceID identifies the copy that produced it, so the same dose is not
	// counted twice.
	InstanceID string `json:"instance_id"`
	// DefID names the definition it was taken from, which is what the window and
	// the narration report rather than an effect invented for the occasion.
	DefID   string `json:"def_id"`
	NameKey string `json:"name_key,omitempty"`
	// Attribute and Value are the sourced change it contributes.
	Attribute AttributeName `json:"attribute,omitempty"`
	Value     float64       `json:"value,omitempty"`
	// Group is the stacking group. §4.2's 同类取最强 is enforced by aggregation
	// through this field, so a second dose of the same kind replaces the first
	// rather than adding to it.
	Group string `json:"group,omitempty"`
	// RemainingExchanges counts down as exchanges pass, the exchange it was taken
	// in included.
	RemainingExchanges int `json:"remaining_exchanges,omitempty"`
}

// doseModifiers turns the doses still in force into the modifiers they
// contribute.
//
// An expired dose contributes nothing, which is how a term that has run out
// stops paying without the caller having to remember to remove it.
func (c Combatant) doseModifiers() []AttributeModifier {
	if len(c.ActiveDoses) == 0 {
		return nil
	}
	modifiers := make([]AttributeModifier, 0, len(c.ActiveDoses))
	for _, dose := range c.ActiveDoses {
		if dose.RemainingExchanges <= 0 || dose.Attribute == "" || dose.Value == 0 {
			continue
		}
		modifiers = append(modifiers, AttributeModifier{
			ID:        "dose:" + dose.InstanceID,
			Kind:      SourceMedicine,
			Attribute: dose.Attribute,
			Value:     dose.Value,
			Group:     dose.Group,
			Temporary: true,
		})
	}
	return modifiers
}

// equipmentModifiers collects what the copies really in each position
// contribute.
//
// The order follows SwappableEquipmentSlots rather than the order the slice
// happens to hold them in, so a replay aggregates the same list every time. A
// position holding nothing contributes nothing, which is the ordinary case.
func (c Combatant) equipmentModifiers() []AttributeModifier {
	if len(c.Equipment) == 0 {
		return nil
	}
	var modifiers []AttributeModifier
	for _, slot := range SwappableEquipmentSlots {
		for _, copy := range c.Equipment {
			if copy.Slot != slot {
				continue
			}
			if (ItemDef{ApplicableStage: copy.ApplicableStage}).UsableBy(c.Cultivation) != nil {
				break
			}
			if copy.Slot != EquipmentSlotWeapon && copy.OperatingManaCost > 0 && !copy.Driven {
				break
			}
			modifiers = append(modifiers, copy.Modifiers...)
			break
		}
	}
	return modifiers
}

// EquipmentInSlot returns the copy occupying one position, if any.
//
// It is exported because the settlement has to ask the same question the engine
// asked when it resolved the swap: which object is really in this position now.
// A second reading of the session's list in the caller would be a second answer
// to one question.
func (c Combatant) EquipmentInSlot(slot EquipmentSlot) (EquipmentCopy, bool) {
	for _, copy := range c.Equipment {
		if copy.Slot == slot {
			return copy, true
		}
	}
	return EquipmentCopy{}, false
}

// drivingCost is what the copies in hand cost to drive for one exchange.
//
// Only the weapon position charges it, which is the rule the implement cost
// already followed: a technique needing focus is driven through the weapon, and
// a treasure's own operating cost belongs to the action that invokes it rather
// than to every exchange.
func (c Combatant) drivingCost() int {
	if weapon, ok := c.EquipmentInSlot(EquipmentSlotWeapon); ok {
		if weapon.OperatingManaCost > 0 {
			return weapon.OperatingManaCost
		}
		return 0
	}
	// A session built before the positions existed carries the price directly.
	// Reading it is not a second source: the caller that fills Equipment derives
	// the same figure from the same definition, so the two never disagree — and a
	// battle built without positions still pays for the implement it holds.
	return c.OperatingManaCost
}

// recomputeAttributes re-derives the six displayed numbers from the stage base
// and every currently effective source.
//
// §5 of the item design requires exactly this: 所有实际属性从阶段基础值与当前有效来源
// 重新汇总. Applying a dose's percentage to the already-aggregated Attack would
// make the result depend on whether equipment had been folded in before or
// after, and would apply the global cap to a number that already carried a
// bonus.
//
// The two pools are carried across and re-clamped rather than reset: a wound
// taken earlier in the fight is a real current value, not something to be
// re-derived from the maximum.
func (c *Combatant) recomputeAttributes() {
	base := BaseAttributes(c.Cultivation)
	modifiers := make([]AttributeModifier, 0, len(c.Modifiers)+len(c.ActiveDoses)+2)
	modifiers = append(modifiers, c.Modifiers...)
	modifiers = append(modifiers, c.doseModifiers()...)
	// The delaying variant's −15% agility is a real modifier, not a note on the
	// session: §5 has it change the target's own agility checks, so it has to
	// enter the same aggregation every other source does. Leaving it out made
	// the effect visible and counted down without ever moving a number.
	modifiers = append(modifiers, c.Slow.AgilityModifier()...)
	// A formation's bonus comes from its live state rather than from the carried
	// modifier list, because §4.3 makes upkeep able to end it mid-fight. The
	// caller keeps the formation out of Modifiers for exactly this reason; a
	// second copy there would keep paying after the operator stopped.
	modifiers = append(modifiers, c.Formation.FormationModifiers()...)
	// What is really in each position contributes here rather than through
	// Modifiers, because §2 lets a spare replace a copy mid-fight. Reading the
	// positions is what makes the swap change a number in the same exchange it
	// was paid for.
	modifiers = append(modifiers, c.equipmentModifiers()...)
	aggregated := AggregateAttributes(base, modifiers, c.Cultivation.rules)
	aggregated.Health = clampInt(c.Attributes.Health, 0, aggregated.HealthMax)
	aggregated.Mana = clampInt(c.Attributes.Mana, 0, aggregated.ManaMax)
	c.Attributes = aggregated
}

// CombatStanding names how a combatant currently stands in a fight.
type CombatStanding string

const (
	// StandingInTheFight is a combatant still able to act.
	StandingInTheFight CombatStanding = "standing"
	// StandingFled left the fight under its own power. It is kept apart from a
	// defeat because the story must not write a corpse for someone who ran.
	StandingFled CombatStanding = "fled"
	// StandingSurrendered yielded and is alive and present. It is kept apart from
	// a defeat for the same reason, and apart from a flight because the winner
	// still has someone standing in front of them.
	StandingSurrendered CombatStanding = "surrendered"
	// StandingDefeated is out of the fight by force.
	StandingDefeated CombatStanding = "defeated"
)

// Standing reports how this combatant currently stands.
//
// The four are kept apart because §7 of the battle design requires the aftermath
// to differ by how someone left the fight: a combatant who yielded is standing
// there alive, and reporting them as defeated would have the story write a
// corpse. It lives here, on the rules' own type, so the battle window and the
// handoff back to the main story read one vocabulary rather than each deciding
// for itself what "surrendered" means.
func (c Combatant) Standing() CombatStanding {
	switch {
	case c.Fled:
		return StandingFled
	case c.Surrendered:
		return StandingSurrendered
	case c.Defeated:
		return StandingDefeated
	default:
		return StandingInTheFight
	}
}

// HealthFraction returns the current health as a fraction of the maximum, which
// is what the retreat templates and the burden capacity both read.
func (c Combatant) HealthFraction() float64 {
	if c.Attributes.HealthMax <= 0 {
		return 0
	}
	return float64(c.Attributes.Health) / float64(c.Attributes.HealthMax)
}

// Preparation is a technique mid-execution.
//
// §4.2 of the casting design fixes the timing as 准备占一轮、完成占下一轮: the
// exchange it is begun in and the following exchange, not one exchange. The
// starting exchange is recorded rather than assumed, so a preparation cannot
// complete in the exchange it started in.
type Preparation struct {
	// Skill is the definition being performed, with any variant's power already
	// substituted. §6 makes a learned technique a versioned fact, so the version
	// being performed is carried here and persisted with the battle: a resumed
	// window completes the technique the character actually began, at the numbers
	// that version defines, rather than at whatever the table says later.
	Skill SkillDef `json:"skill"`
	// Variant is the alternative form being cast, if any. The variant's power is
	// already folded into Skill, so this is kept for what the window has to show
	// — which form of the technique is mid-execution — not to recompute it.
	Variant CombatVariant `json:"variant,omitempty"`
	// TargetID is the combatant the completed technique will resolve against,
	// fixed when the preparation starts so the strike lands on the opponent it
	// was aimed at rather than on whoever is nearest when it completes.
	TargetID string `json:"target_id,omitempty"`
	// StartedRound is the exchange the preparation began in.
	StartedRound int `json:"started_round"`
}

// CombatSession is one battle's full state.
type CombatSession struct {
	// ID is the battle's stable identity in the story journal.
	ID string `json:"id"`
	// Round is the exchange about to be resolved, 1-based.
	Round int `json:"round"`
	// Combatants is every participant, in a fixed order.
	Combatants []Combatant `json:"combatants"`
	// Preparations records techniques mid-execution, by combatant ID.
	Preparations map[string]Preparation `json:"preparations,omitempty"`
	// Ended reports that the battle has reached a result.
	Ended bool `json:"ended,omitempty"`
	// Result is the settled outcome once Ended is true.
	Result CombatResult `json:"result,omitempty"`
}

// CombatResult is how a battle finished.
type CombatResult string

const (
	ResultUnresolved CombatResult = ""
	ResultPlayerWon  CombatResult = "player_won"
	ResultPlayerLost CombatResult = "player_lost"
	ResultPlayerFled CombatResult = "player_fled"
	ResultEnemyFled  CombatResult = "enemy_fled"
	ResultTruce      CombatResult = "truce"
	ResultDraw       CombatResult = "draw"
)

// CombatantByID returns a pointer to a combatant, or nil.
//
// The returned pointer aliases the session's slice, so a caller that intends to
// keep the original must copy the session first — which ResolveExchange does.
func (s *CombatSession) CombatantByID(id string) *Combatant {
	for index := range s.Combatants {
		if s.Combatants[index].ID == id {
			return &s.Combatants[index]
		}
	}
	return nil
}

// CombatAction is one combatant's declared intent for one exchange.
type CombatAction struct {
	Kind CombatActionKind
	// Skill is the technique being performed, for ActionTechnique.
	Skill SkillDef
	// Variant is the alternative form being cast, if any.
	Variant CombatVariant
	// TargetID names the combatant this action is directed at.
	TargetID string
	// ExtraTargetIDs names the further combatants a technique that reaches more
	// than one target is aimed at. §9 of the casting design requires the reach to
	// be settled from the player's actual choice — 多目标按实际选择和上限结算，不默认打满敌队 —
	// so the extra names are a declaration like the primary one, never an
	// inference from who happens to be standing.
	//
	// The primary stays separate because a preparation binds to TargetID, and
	// because every existing consumer reads Resolution as the primary strike.
	ExtraTargetIDs []string
	// ManaReduction is the total real cost reduction in effect, capped by the
	// rules at 20%.
	ManaReduction float64
	// OfferText is the proposal carried by ActionSurrender, recorded so the
	// other side's answer has something to answer.
	OfferText string
	// ItemInstanceID names the carried copy ActionUseItem spends. It is an
	// instance rather than a definition because §7 requires the two to be kept
	// apart: holding a definition is not holding stock, and the copy that is
	// spent is a real one.
	//
	// The effect is deliberately NOT carried on the action. A caller that could
	// send the numbers would be able to write 40% onto a 下品 pill, which is
	// precisely the failure §2 of the item design names. There is no definition
	// field either, for the same reason: the dose is priced from the copy the
	// combatant really carries, so there is nothing here for a caller to point at
	// a stronger entry.
	ItemInstanceID string
	// SwapInstanceID names the carried spare ActionSwapEquipment takes into its
	// position. It is an instance for the same reason ItemInstanceID is: the copy
	// that moves is a real one, and the slot it goes to is read from that copy
	// rather than from the request, so a caller cannot hang a sword in the
	// treasure row by asking to.
	SwapInstanceID string
	// OperatingManaCost is what the combatant's equipped implement costs to drive
	// this exchange, read from its registered definition.
	//
	// It is a declaration only in the sense that it travels with the action; the
	// engine never trusts it. §4.1 requires the real cost to come from the
	// equipment definition, and a caller that could send a smaller number would be
	// able to make a treasure free to swing. resolveTechnique therefore reads
	// actor.OperatingManaCost, not this field, and this one is kept for the record
	// so the step can say what was paid.
	OperatingManaCost int
}

// DrawSource returns a reproducible uniform draw in [0,1) for one purpose.
//
// The purpose strings the engine uses are "initiative", "hit" and "damage". The
// caller derives them from the battle's identity so a retry re-uses the draw
// rather than re-rolling.
type DrawSource func(purpose, actorID, targetID string) float64

// ExchangeStep records what one combatant did and what it produced.
type ExchangeStep struct {
	ActorID  string           `json:"actor_id"`
	TargetID string           `json:"target_id,omitempty"`
	Kind     CombatActionKind `json:"kind"`
	// SkillID names the technique this step performed, at the definition's stable
	// ID rather than its wording. A refused cast still names it: the record has to
	// say what was attempted, or a round reads as though nothing was chosen. The
	// ID is what the narration and the window both need, since neither is allowed
	// to invent a technique the step did not name.
	SkillID string `json:"skill_id,omitempty"`
	// Variant is the published form the technique was cast as, if any. §5 makes a
	// variant a way of casting rather than a second technique, so the step keeps
	// the base technique in SkillID and the form here: a round that recorded only
	// the technique would read the burning form's weaker hit as the base
	// technique's, and the narration could not say which form was seen.
	Variant CombatVariant `json:"variant,omitempty"`
	// PreparationAbandoned makes an alternate chosen action's cancellation of a
	// pending preparation visible without replacing that action's own result.
	PreparationAbandoned bool `json:"preparation_abandoned,omitempty"`
	// Cast is the precondition check. A refused cast produced no effect and
	// spent nothing, which is what §6 of the casting design requires.
	Cast CastCheck `json:"cast"`
	// OperatingManaCost is what the implement in hand cost to drive this
	// exchange, recorded so the round can show it was paid. It is 0 when nothing
	// equipped declares a cost, which is the ordinary case and not a missing
	// number.
	OperatingManaCost int `json:"operating_mana_cost,omitempty"`
	// Resolution is the settled attack, for a technique that resolved.
	Resolution AttackResolution `json:"resolution,omitempty"`
	// ExtraTargets are the further combatants a technique that reaches more than
	// one target was aimed at. §9 settles them from the player's actual choice,
	// so each one is a real strike with its own check — a miss on the primary
	// does not cancel them, and a target the player did not name is not among
	// them. They are kept apart from Resolution so every reader of the primary
	// strike keeps reading the same field it always did.
	ExtraTargets []ExtraTargetStrike `json:"extra_targets,omitempty"`
	// ManaRecovered is what a rest restored.
	ManaRecovered int `json:"mana_recovered,omitempty"`
	// HealthRecovered is what a restoring dose restored. It is reported apart
	// from ManaRecovered so a round says which pool actually moved.
	HealthRecovered int `json:"health_recovered,omitempty"`
	// ItemApplied reports that a dose took effect, which a refused one does not.
	// A refusal spends nothing, so the two outcomes are kept apart rather than
	// both reading as "an item was used".
	ItemApplied bool `json:"item_applied,omitempty"`
	// EquipmentSwapped reports that the declared spare really moved into its
	// position. §2 spends the exchange's primary action on the swap itself, so a
	// successful swap is the whole of that combatant's action and cannot also
	// drive the copy it just took out.
	EquipmentSwapped bool `json:"equipment_swapped,omitempty"`
	// SwappedIn and SwappedOut name the copies that moved, so the round record and
	// the window can say which object was taken out and which was put away. A
	// refused swap names neither, because nothing moved.
	SwappedIn  string `json:"swapped_in,omitempty"`
	SwappedOut string `json:"swapped_out,omitempty"`
	// SwappedSlot is the position the swap took place in.
	SwappedSlot EquipmentSlot `json:"swapped_slot,omitempty"`
	// BurnApplied and SlowApplied report whether the variant's lasting effect
	// attached, which happens only on a real hit.
	BurnApplied bool `json:"burn_applied,omitempty"`
	SlowApplied bool `json:"slow_applied,omitempty"`
	// Escaped reports a successful disengagement.
	Escaped bool `json:"escaped,omitempty"`
	// Surrendered reports that the combatant offered to yield.
	Surrendered bool `json:"surrendered,omitempty"`
	// Defeated reports that this action dropped the target.
	Defeated bool `json:"defeated,omitempty"`
	// ReasonKey names why nothing happened, when nothing did.
	ReasonKey string `json:"reason_key,omitempty"`
	// ActionRejected is transient product-boundary state; stores must refuse the
	// request before committing the exchange, so it is deliberately not journaled.
	ActionRejected bool `json:"-"`
}

// ExtraTargetStrike is one further combatant a multi-target technique struck.
//
// It is a target and its own resolution, not a second ExchangeStep: the cost,
// the cast check and the lasting-effect bookkeeping all belong to the one cast,
// and repeating them per target would report the same cost several times and
// let a reader think the technique was performed more than once.
type ExtraTargetStrike struct {
	TargetID   string           `json:"target_id"`
	Resolution AttackResolution `json:"resolution"`
	// BurnApplied and SlowApplied report whether the variant's lasting effect
	// attached to this target. They are per target because the effect attaches
	// only where the blow really wounded, which can differ between targets of
	// the same cast.
	BurnApplied bool `json:"burn_applied,omitempty"`
	SlowApplied bool `json:"slow_applied,omitempty"`
	// Defeated reports that this strike dropped the target.
	Defeated bool `json:"defeated,omitempty"`
}

// ExchangeOutcome is one exchange's settled result.
//
// The tags exist because the outcome is persisted alongside the battle: a
// reopened window has to show what just happened without re-resolving the
// exchange, and re-resolving it is exactly what §七.2 forbids on a retry.
type ExchangeOutcome struct {
	Round int `json:"round"`
	// Order is the resolved action order, which the narration reads so the
	// prose follows what actually happened rather than a guess.
	Order []string       `json:"order,omitempty"`
	Steps []ExchangeStep `json:"steps,omitempty"`
	// Ended and Result mirror the session's state after the exchange.
	Ended  bool         `json:"ended,omitempty"`
	Result CombatResult `json:"result,omitempty"`
}

// ResolveExchange settles one exchange.
//
// The session passed in is not modified: a copy is returned. That is what lets
// the caller abandon an exchange, replay it against the same draws, or hold it
// uncommitted without corrupting the live battle.
//
// Order of operations within the exchange is the design's:
//
//  1. initiative order is drawn once per combatant,
//  2. each combatant in that order performs its declared action,
//  3. shields, lasting damage, slows and burden are advanced once at the end.
//
// Advance is last because a shield cast this exchange must survive to absorb the
// next one, and a burn attached this exchange must not tick in the same
// exchange it landed.
func ResolveExchange(session CombatSession, actions map[string]CombatAction, draws DrawSource) (CombatSession, ExchangeOutcome) {
	next := session.clone()
	outcome := ExchangeOutcome{Round: next.Round}
	for i := range next.Combatants {
		next.Combatants[i] = next.Combatants[i].atExchangeStart()
	}
	order := initiativeOrder(next, draws)
	outcome.Order = append([]string{}, order...)
	healthAtStart := make(map[string]int, len(next.Combatants))
	for _, combatant := range next.Combatants {
		healthAtStart[combatant.ID] = combatant.Attributes.Health
	}
	resting := map[string]bool{}
	freshBurn := map[string]bool{}
	interrupted := map[string]bool{}
	// A technique's target is settled when the exchange is declared, not when
	// its actor's turn comes round. The two moments are different facts: a
	// declaration that names nobody has no legal target and is refused, while a
	// target that leaves the fight before the actor's turn is a real exchange in
	// which the blow found nothing. Refusing the second as well would make the
	// fleeing side's own action uncommittable, so the fight could never end.
	refused := illegalTechniqueDeclarations(&next, actions)
	affordable := affordableTechniqueDeclarations(next, actions, refused)
	freezeOtherInputRefusals(next, actions, refused)

	for _, actorID := range order {
		actor := next.CombatantByID(actorID)
		if reason, ok := refused[actorID]; ok {
			action := actions[actorID]
			// A declaration refused before the exchange began costs its actor
			// nothing, so it is reported as a refusal rather than resolved. The
			// actor keeps whatever preparation it already had: the refusal is of
			// the new declaration, not of the commitment already paid for.
			step := ExchangeStep{ActorID: actorID, TargetID: action.TargetID, Kind: action.Kind, ReasonKey: reason, ActionRejected: true}
			if action.Kind == ActionTechnique {
				step.SkillID = action.Skill.ID
				if check, exists := affordable[actorID]; exists && !check.Allowed {
					step.Cast = check
				}
			}
			outcome.Steps = append(outcome.Steps, step)
			continue
		}
		if actor == nil || !actor.Active() {
			continue
		}
		action, ok := actions[actorID]
		if !ok {
			if preparation, preparing := next.Preparations[actorID]; preparing {
				outcome.Steps = append(outcome.Steps, ExchangeStep{ActorID: actorID, SkillID: preparation.Skill.ID, TargetID: preparation.TargetID, ReasonKey: "reason.action_required", ActionRejected: true})
				continue
			}
			if strike, ok := NewSkillRegistryWithBuiltins(actor.Cultivation.Rules()).Current("strike"); ok {
				action = CombatAction{Kind: ActionTechnique, Skill: strike}
			} else {
				action = CombatAction{Kind: ActionRest}
			}
			if target := firstOpponent(&next, actor); target != nil {
				action.TargetID = target.ID
			}
		}
		preparation, hadPreparation := next.Preparations[actorID]
		if interrupted[actorID] {
			continue
		}

		if action.Kind == ActionContinuePreparation {
			if !hadPreparation {
				outcome.Steps = append(outcome.Steps, resolveAction(&next, actor, action, draws))
				continue
			}
			target := next.CombatantByID(preparation.TargetID)
			targetHealthBefore := -1
			if target != nil {
				targetHealthBefore = target.Attributes.Health
			}
			delete(next.Preparations, actorID)
			step := completePreparation(&next, actor, preparation, draws)
			outcome.Steps = append(outcome.Steps, step)
			if target != nil {
				actualLoss := maxInt(0, targetHealthBefore-target.Attributes.Health)
				if actualLoss > 0 || step.Defeated {
					markPreparationInterruptions(&next, target.ID, actualLoss, interrupted, &outcome)
				}
			}
			continue
		}
		if action.Kind == ActionRest {
			resting[actorID] = true
		}
		attackTarget := next.CombatantByID(action.TargetID)
		attackTargetHealthBefore := -1
		if attackTarget != nil {
			attackTargetHealthBefore = attackTarget.Attributes.Health
		}
		// A slow still in force when the actor's turn comes round is spent by
		// that action: §5 keeps it until the target's next primary action
		// completes, so it is captured before the action resolves and cleared
		// afterwards. Clearing it re-derives the numbers, because the penalty
		// lives in the attribute aggregation rather than beside it.
		wasSlowed := actor.Slow.RemainingTargetActions > 0
		step := resolveAction(&next, actor, action, draws)
		if step.ActionRejected && affordable[actorID].Allowed && interruptedCastReason(step.ReasonKey) {
			// The intent was legal when declared. Its later failure spends this
			// turn, but not a cast that never happened, and keeps all earlier
			// wounds/costs. It is not a request to roll the exchange again.
			step.ActionRejected = false
		}
		if wasSlowed {
			actor.Slow = actor.Slow.TickSlow()
			actor.recomputeAttributes()
		}
		if hadPreparation {
			if step.ActionRejected {
				if _, replaced := next.Preparations[actorID]; !replaced {
					next.Preparations[actorID] = preparation
				}
			} else if !step.PreparationAbandoned {
				if current, stillOld := next.Preparations[actorID]; stillOld && current.StartedRound == preparation.StartedRound {
					delete(next.Preparations, actorID)
					step.PreparationAbandoned = true
				}
			}
		}
		if step.BurnApplied {
			freshBurn[step.TargetID] = true
		}
		for _, extra := range step.ExtraTargets {
			if extra.BurnApplied {
				freshBurn[extra.TargetID] = true
			}
		}
		outcome.Steps = append(outcome.Steps, step)
		actualLoss := 0
		if attackTarget != nil {
			actualLoss = maxInt(0, attackTargetHealthBefore-attackTarget.Attributes.Health)
		}
		if actualLoss > 0 || step.Defeated {
			markPreparationInterruptions(&next, step.TargetID, actualLoss, interrupted, &outcome)
		}
		// A further target takes the same consequences as the primary one: a real
		// wound interrupts whatever it was preparing, and a fresh burn does not
		// tick in the exchange it landed. §5 does not make the second target of a
		// cast less struck than the first.
		for _, extra := range step.ExtraTargets {
			if extra.Resolution.HealthLost <= 0 && !extra.Defeated {
				continue
			}
			markPreparationInterruptions(&next, extra.TargetID, extra.Resolution.HealthLost, interrupted, &outcome)
		}
	}
	outcome.Steps = append(outcome.Steps, advanceExchange(&next, draws, freshBurn, interrupted)...)
	for actorID := range resting {
		actor := next.CombatantByID(actorID)
		if actor == nil || !actor.Active() || healthAtStart[actorID]-actor.Attributes.Health > 0 {
			continue
		}
		recovered := RestManaRecovery(actor.Attributes.ManaMax, false)
		actor.Attributes.Mana = minInt(actor.Attributes.ManaMax, actor.Attributes.Mana+recovered)
		for index := range outcome.Steps {
			if outcome.Steps[index].ActorID == actorID {
				outcome.Steps[index].ManaRecovered = recovered
				break
			}
		}
	}
	next.Round++
	settleEnd(&next)
	outcome.Ended, outcome.Result = next.Ended, next.Result
	return next, outcome
}

func preparationInterruptThreshold(maxHealth int) int {
	if maxHealth <= 0 {
		return 1
	}
	threshold := maxHealth / 5
	if maxHealth%5 != 0 {
		threshold++
	}
	return threshold
}

func markPreparationInterruptions(session *CombatSession, targetID string, healthLost int, interrupted map[string]bool, outcome *ExchangeOutcome) {
	if targetID == "" {
		return
	}
	target := session.CombatantByID(targetID)
	if target == nil {
		return
	}
	if healthLost < preparationInterruptThreshold(target.Attributes.HealthMax) && target.Active() {
		return
	}
	preparation, preparing := session.Preparations[targetID]
	if !preparing {
		return
	}
	delete(session.Preparations, targetID)
	interrupted[targetID] = true
	outcome.Steps = append(outcome.Steps, ExchangeStep{ActorID: targetID, TargetID: preparation.TargetID, SkillID: preparation.Skill.ID, Kind: ActionContinuePreparation, PreparationAbandoned: true, ReasonKey: "reason.preparation_interrupted"})
}

// resolveAction performs one combatant's action against the session in place.
//
// The actor and target pointers alias the session being mutated, so the caller
// must be operating on the copy it owns.
func resolveAction(session *CombatSession, actor *Combatant, action CombatAction, draws DrawSource) ExchangeStep {
	step := ExchangeStep{ActorID: actor.ID, TargetID: action.TargetID, Kind: action.Kind}
	switch action.Kind {
	case ActionRest:
		// The recovery itself is settled by the exchange, once the real health
		// damage of the whole exchange is known.
		return step

	case ActionEscape:
		step.Escaped = resolveEscape(session, actor, action, draws)
		return step

	case ActionSurrender:
		// An offer to yield is recorded, not resolved. The offering combatant is
		// still standing and can still be attacked until the other side answers.
		actor.OfferText = action.OfferText
		step.Surrendered = true
		return step

	case ActionAcceptSurrender:
		step.Surrendered = resolveAcceptSurrender(session, actor, action)
		return step

	case ActionUseItem:
		step := resolveUseItem(actor, action)
		if !step.ItemApplied && step.ReasonKey != "reason.pool_already_full" && step.ReasonKey != "reason.talisman_shield_active" {
			step.ActionRejected = true
		}
		// A held, qualified consumable can be declared in anticipation of a
		// wound or a broken ward. If that need never arises, preserve the dose
		// but spend the action: the enemy's already chosen turn is not rerolled.
		return step

	case ActionContinuePreparation:
		step := ExchangeStep{ActorID: actor.ID, Kind: action.Kind}
		if _, preparing := session.Preparations[actor.ID]; !preparing {
			step.ReasonKey = "reason.no_preparation"
			step.ActionRejected = true
		}
		return step

	case ActionCancelPreparation:
		step := ExchangeStep{ActorID: actor.ID, Kind: action.Kind}
		if _, preparing := session.Preparations[actor.ID]; preparing {
			delete(session.Preparations, actor.ID)
			step.PreparationAbandoned = true
			step.ReasonKey = "reason.preparation_abandoned"
		} else {
			step.ReasonKey = "reason.no_preparation"
			step.ActionRejected = true
		}
		return step

	case ActionSwapEquipment:
		step := resolveSwapEquipment(actor, action)
		if !step.EquipmentSwapped {
			step.ActionRejected = true
		}
		return step

	case ActionTechnique:
		step := resolveTechnique(session, actor, action, draws)
		// A no-target result is deliberately absent here. Declarations are
		// checked before the exchange begins, so by this point a technique with
		// no target can only mean the target left the fight earlier in the same
		// exchange: a blow that found nothing, which is an outcome to record
		// rather than a round to refuse.
		if step.ReasonKey == ReasonUnknownSkill || step.ReasonKey == ReasonMortalNoTechniques || step.ReasonKey == ReasonSkillStageTooLow || step.ReasonKey == ReasonInsufficientMana || step.ReasonKey == ReasonBurdenCapacity || step.ReasonKey == ReasonMeridianUnrecovered || step.ReasonKey == ReasonImplementRequired || step.ReasonKey == "reason.variant_unsupported" {
			step.ActionRejected = true
		}
		return step
	}
	step.ReasonKey = "reason.unknown_action"
	step.ActionRejected = true
	return step
}

// resolveSwapEquipment moves one carried spare into its position and puts the
// copy that was there back in the pack.
//
// §2 spends a primary action on the exchange itself: 需花一个主要行动收起和取出，不能
// 在同一动作额外发动它. The engine enforces that structurally by treating the swap
// as the combatant's action for the exchange — resolveAction returns after this
// and nothing else is performed, so the swapped-in copy cannot also be driven.
//
// Every refusal returns before anything moves, for the same reason a refused
// dose spends nothing: a repeated command must find the same pre-state and the
// same refusal rather than a pack the first attempt had already rearranged.
//
// The position comes from the spare's own record, never from the request. A
// caller that could name a slot would be able to hang a sword in a treasure row,
// and §1 says 同一实物只能占一个位置、执行一次动作.
func resolveSwapEquipment(actor *Combatant, action CombatAction) ExchangeStep {
	step := ExchangeStep{ActorID: actor.ID, Kind: action.Kind}

	instanceID := strings.TrimSpace(action.SwapInstanceID)
	if instanceID == "" {
		step.ReasonKey = "reason.spare_not_named"
		return step
	}
	index := -1
	for i := range actor.Spares {
		if actor.Spares[i].InstanceID == instanceID {
			index = i
			break
		}
	}
	if index < 0 {
		// The character does not carry this copy. §4 keeps 取得、转移、装备 as facts
		// about one real object, so a spare the battle never received cannot be
		// swapped in.
		step.ReasonKey = "reason.spare_not_carried"
		return step
	}
	spare := actor.Spares[index]
	if err := (ItemDef{ApplicableStage: spare.ApplicableStage}).UsableBy(actor.Cultivation); err != nil {
		step.ReasonKey = "reason.equipment_stage_too_low"
		return step
	}
	if !swappableSlot(spare.Slot) {
		// A copy with no position of its own is not equipment this ruleset can
		// hang anywhere. Refusing is the honest answer for an item the catalogue
		// never gave a slot to.
		step.ReasonKey = "reason.spare_not_equipment"
		return step
	}
	spare.Driven = false
	spare.SwappedThisExchange = true

	// Take the copy that was in the position out of it and into the pack. §1 makes
	// the limit a limit on what is equipped, not on what is owned: the replaced
	// copy is kept, not destroyed.
	next := make([]EquipmentCopy, 0, len(actor.Equipment)+1)
	for _, current := range actor.Equipment {
		if current.Slot != spare.Slot {
			next = append(next, current)
			continue
		}
		step.SwappedOut = current.InstanceID
		actor.Spares = append(actor.Spares, current)
	}
	next = append(next, spare)
	actor.Equipment = next
	// The copy that moved leaves the pack. It is the same object, so it is not
	// also still listed as a spare — which would let one sword be swapped in twice.
	remaining := make([]EquipmentCopy, 0, len(actor.Spares)-1)
	skipped := false
	for _, current := range actor.Spares {
		if !skipped && current.InstanceID == instanceID {
			skipped = true
			continue
		}
		remaining = append(remaining, current)
	}
	actor.Spares = remaining

	// The numbers change in the same exchange the swap was paid for: the copy
	// that left stops contributing and the copy that arrived starts. §5 requires
	// every real attribute to be re-summed from the stage base and the currently
	// effective sources, which is exactly this.
	actor.recomputeAttributes()

	step.EquipmentSwapped = true
	step.SwappedIn = instanceID
	step.SwappedSlot = spare.Slot
	return step
}

// swappableSlot reports whether a copy may occupy this position.
func swappableSlot(slot EquipmentSlot) bool {
	for _, candidate := range SwappableEquipmentSlots {
		if candidate == slot {
			return true
		}
	}
	return false
}

// resolveUseItem spends one dose of a carried pill.//
// §4.2 fixes what one dose does, and §8 fixes what a refused one does: an
// illegal parameter, a missing target or a failed check must not deduct the
// resource. So every refusal here returns before anything is spent, and a
// repeated command therefore cannot eat a second dose — the second call finds
// the same pre-state and the same refusal.
//
// The dose is matched against the combatant's own carried stock rather than
// trusted from the action, so an action naming a copy the character does not
// have is refused instead of served.
func resolveUseItem(actor *Combatant, action CombatAction) ExchangeStep {
	step := ExchangeStep{ActorID: actor.ID, Kind: action.Kind}

	instanceID := strings.TrimSpace(action.ItemInstanceID)
	if instanceID == "" {
		step.ReasonKey = "reason.item_not_named"
		return step
	}
	index := -1
	for i := range actor.Carried {
		if actor.Carried[i].InstanceID == instanceID {
			index = i
			break
		}
	}
	if index < 0 {
		// The character does not hold this copy. §7: a definition's existence is
		// not stock, and neither is a client's claim about it.
		step.ReasonKey = "reason.item_not_carried"
		return step
	}
	dose := &actor.Carried[index]
	if dose.Quantity <= 0 {
		step.ReasonKey = "reason.item_exhausted"
		return step
	}
	if dose.Talisman != nil {
		if reason := TalismanUseReason(*actor, *dose.Talisman, dose.DefID); reason != "" {
			step.ReasonKey = reason
			return step
		}
		actor.Shield = StackShield(actor.Shield, dose.Talisman.ShieldFor(actor.Cultivation, "talisman:"+dose.DefID), actor.Cultivation)
		dose.Quantity--
		step.SkillID, step.ItemApplied, step.ReasonKey = dose.DefID, true, "reason.talisman_applied"
		return step
	}
	// The definition is taken from the carried copy, not from the action. A
	// caller cannot price its own pill.
	pill := dose.Pill
	if pill.Purpose == "" {
		step.ReasonKey = "reason.item_not_usable"
		return step
	}
	step.SkillID = dose.DefID

	switch pill.Purpose {
	case PillRestoreHealth:
		amount := pill.RestoreAmount(actor.Attributes)
		if amount <= 0 || actor.Attributes.Health >= actor.Attributes.HealthMax {
			// Nothing to restore. The dose is not spent on a full pool, which is
			// the honest reading of a real action that would have no effect.
			step.ReasonKey = "reason.pool_already_full"
			return step
		}
		actor.Attributes.Health = minInt(actor.Attributes.HealthMax, actor.Attributes.Health+amount)
		step.HealthRecovered = amount
		step.ItemApplied = true
	case PillRestoreMana:
		amount := pill.RestoreAmount(actor.Attributes)
		if amount <= 0 || actor.Attributes.Mana >= actor.Attributes.ManaMax {
			step.ReasonKey = "reason.pool_already_full"
			return step
		}
		actor.Attributes.Mana = minInt(actor.Attributes.ManaMax, actor.Attributes.Mana+amount)
		step.ManaRecovered = amount
		step.ItemApplied = true
	case PillBoostAttack:
		bonus, ok := pill.AttackBonus()
		if !ok || bonus <= 0 {
			step.ReasonKey = "reason.item_not_usable"
			return step
		}
		duration := pill.DurationExchanges
		if duration <= 0 {
			duration = 1
		}
		// §4.2: 同类取最强, and 重服可刷新相同期限但不叠强度和永久时长. 同类 is
		// the purpose, not one definition: two differently named 增攻丹 are the
		// same kind of effect, so they compete for one slot instead of stacking.
		// Keying the group on the definition let a second attack pill of another
		// grade add a whole extra bonus, which is what the rule forbids.
		group := "pill:" + string(pill.Purpose)
		replaced := false
		for i := range actor.ActiveDoses {
			if actor.ActiveDoses[i].Group != group {
				continue
			}
			// Refresh the term, keep the stronger value. A weaker re-dose does not
			// lower an effect already in force.
			actor.ActiveDoses[i].RemainingExchanges = duration
			if bonus > actor.ActiveDoses[i].Value {
				actor.ActiveDoses[i].Value = bonus
			}
			replaced = true
			break
		}
		if !replaced {
			actor.ActiveDoses = append(actor.ActiveDoses, ActiveDose{
				InstanceID: dose.InstanceID, DefID: dose.DefID, NameKey: dose.NameKey,
				Attribute: AttrAttack, Value: bonus, Group: group,
				RemainingExchanges: duration,
			})
		}
		actor.recomputeAttributes()
		step.ItemApplied = true
		step.ReasonKey = "reason.item_boost_applied"
	case PillSupportCultivation:
		// A cultivation-support dose is spent by a training interval, not by an
		// exchange. §6.2 gives it a rate over days; a fight has no such clock, so
		// taking one here would buy nothing and cost a dose.
		step.ReasonKey = "reason.item_not_usable"
		return step
	default:
		step.ReasonKey = "reason.item_not_usable"
		return step
	}

	dose.Quantity--
	return step
}

// resolveTechnique performs a technique, which is the only action that deals
// damage.
func resolveTechnique(session *CombatSession, actor *Combatant, action CombatAction, draws DrawSource) ExchangeStep {
	step := ExchangeStep{ActorID: actor.ID, TargetID: action.TargetID, Kind: action.Kind}

	skill := action.Skill
	// The attempted technique is recorded before any check, so a refused or
	// interrupted cast still says which one it was. A record that named only the
	// outcome would leave the round unable to say what was tried.
	step.SkillID = skill.ID
	// The form is recorded the same way and for the same reason: a refused form is
	// still the form the combatant declared, and a settled one has to be
	// distinguishable from the base technique it shares an ID with.
	step.Variant = action.Variant
	// A variant replaces the base power. The substitution happens before the
	// cast check so the cost and burden reported to the player are the ones the
	// casting actually uses.
	if action.Variant != VariantNone {
		power, ok := VariantPowerFor(skill, action.Variant)
		if !ok {
			step.ReasonKey = "reason.variant_unsupported"
			return step
		}
		skill.Power = power
	}

	plan := CastPlanFor(*actor, skill, action.ManaReduction)
	check := plan.Check
	step.Cast = check
	step.OperatingManaCost = plan.WeaponManaCost + plan.ArmorManaCost
	if !check.Allowed {
		step.ReasonKey = check.ReasonKey
		return step
	}
	// This marks the armor's already-included payment, so receiving a later
	// strike in the same exchange cannot charge it a second time.
	actor.applyCastArmor(plan)

	// A technique that takes more than one exchange only starts here. §4.2 fixes
	// the timing as 准备占一轮、完成占下一轮, and §4.2's own worked example puts the
	// cost, the burden and the residual injury all at the start: 开始准备时扣耗灵
	// 并增加85负担，同时留下冲脉余伤. Abandoning a preparation does not refund any
	// of it.
	if skill.IsPrepared() {
		actor.Attributes, actor.Burden = ApplyCast(check, actor.Attributes, actor.Burden)
		if skill.LeavesResidual {
			actor.Burden.Residual = skill.ResidualBurdenFloor
			if actor.Burden.Current < actor.Burden.Residual {
				actor.Burden.Current = actor.Burden.Residual
			}
		}
		if session.Preparations == nil {
			session.Preparations = map[string]Preparation{}
		}
		session.Preparations[actor.ID] = Preparation{
			Skill:        skill,
			TargetID:     step.TargetID,
			StartedRound: session.Round,
		}
		step.ReasonKey = "reason.preparing"
		return step
	}

	actor.Attributes, actor.Burden = ApplyCast(check, actor.Attributes, actor.Burden)

	// A shielding technique has no target to damage: it builds a shield on the
	// caster and completes. A target may be named to shield an ally, which §5
	// allows at the caster's own 20% base rather than the ally's.
	if skillShields(skill) {
		if step.TargetID != "" {
			if target := session.CombatantByID(step.TargetID); target != nil && target.Active() {
				fresh := NewShield(skill, actor.Cultivation, target.Cultivation, actor.ID+":"+skill.ID)
				target.Shield = StackShield(target.Shield, fresh, target.Cultivation)
				return step
			}
		}
		fresh := NewShield(skill, actor.Cultivation, actor.Cultivation, actor.ID+":"+skill.ID)
		actor.Shield = StackShield(actor.Shield, fresh, actor.Cultivation)
		return step
	}

	target := session.CombatantByID(step.TargetID)
	if target == nil || !target.Active() {
		// Unreachable now that the target is checked before the cost is paid, and
		// kept as the engine's own guard: a technique that somehow reaches here
		// still resolves against nothing rather than being redirected.
		step.ReasonKey = "reason.no_target"
		return step
	}
	treasureCost := actor.driveTreasure(AttrAttack)
	step.OperatingManaCost += treasureCost
	step.Cast.ManaCost += treasureCost
	target.driveTreasure(AttrDefense)

	context := NeutralDamageContext()
	context.DefensePierce = actor.weaponDefensePierce()
	rolls := CombatRolls{
		Initiative: draws("initiative", actor.ID, ""),
		Hit:        draws("hit", actor.ID, target.ID),
		Damage:     draws("damage", actor.ID, target.ID),
	}
	resolution := ResolveAttack(actor.Attributes, target.Attributes, actor.Cultivation.Realm, target.Cultivation.Realm, skill, context, target.Shield, rolls)
	step.Resolution = resolution

	// The further targets are settled whatever the primary blow did. Each is its
	// own check against its own opponent, so a miss on the first does not cancel
	// the rest — and the reach is only real if a player who chose two opponents
	// can see both outcomes.
	step.ExtraTargets = resolveExtraTargets(session, actor, action, skill, context, draws)

	if !resolution.Hit {
		return step
	}

	applyDamage(target, resolution)
	if resolution.ShieldAbsorbed > 0 {
		target.Shield.Amount -= resolution.ShieldAbsorbed
	}

	// Lasting effects attach only on a real hit and only from real health loss.
	// A blow that a shield absorbed entirely is not a wound, so it does not set
	// the target alight. §5 fixes the burn's base as the damage that already
	// passed through the shield — 穿过护盾后的伤害 — so a partly absorbed hit
	// burns for 15% of the wound it actually dealt, not of the full blow.
	if resolution.HealthLost > 0 {
		switch action.Variant {
		case VariantBurn:
			target.Burn = StackBurn(target.Burn, NewBurn(resolution.HealthLost, actor.ID+":"+skill.ID))
			step.BurnApplied = true
		case VariantSlow:
			target.Slow = NewSlow(actor.ID + ":" + skill.ID)
			// The penalty has to reach the numbers before the target's own turn
			// comes round in this exchange: §5 has it affect the checks that
			// still lie ahead, while the order already drawn stays as drawn.
			target.recomputeAttributes()
			step.SlowApplied = true
		}
	}
	step.Defeated = target.Defeated
	return step
}

// resolveExtraTargets settles the further combatants a multi-target technique
// was aimed at.
//
// Each is a full attack: its own hit draw and its own damage draw, keyed by that
// target, so the result is reproducible from the battle's identity and a miss on
// one opponent does not decide another's fate. The primary target is never among
// them — the declaration's own legality check refuses a repeat — and an opponent
// who left the fight between the declaration and this turn resolves as a blow
// that found nothing, which is the same honest outcome the primary strike has.
//
// The lasting effects attach per target and only on a real wound, because §5
// bases a burn on the damage that passed the shield and a shield can absorb one
// target's blow while another's goes through.
func resolveExtraTargets(session *CombatSession, actor *Combatant, action CombatAction, skill SkillDef, context DamageContext, draws DrawSource) []ExtraTargetStrike {
	if len(action.ExtraTargetIDs) == 0 {
		return nil
	}
	strikes := make([]ExtraTargetStrike, 0, len(action.ExtraTargetIDs))
	for _, targetID := range action.ExtraTargetIDs {
		target := session.CombatantByID(targetID)
		if target == nil || !target.Active() {
			// The declaration was legal when it was made, so this is a target that
			// left the fight since. It resolves against nothing rather than being
			// redirected onto whoever is nearest.
			strikes = append(strikes, ExtraTargetStrike{TargetID: targetID})
			continue
		}
		target.driveTreasure(AttrDefense)
		rolls := CombatRolls{
			Initiative: draws("initiative", actor.ID, ""),
			Hit:        draws("hit", actor.ID, target.ID),
			Damage:     draws("damage", actor.ID, target.ID),
		}
		resolution := ResolveAttack(actor.Attributes, target.Attributes, actor.Cultivation.Realm, target.Cultivation.Realm, skill, context, target.Shield, rolls)
		strike := ExtraTargetStrike{TargetID: target.ID, Resolution: resolution}
		if !resolution.Hit {
			strikes = append(strikes, strike)
			continue
		}
		applyDamage(target, resolution)
		if resolution.ShieldAbsorbed > 0 {
			target.Shield.Amount -= resolution.ShieldAbsorbed
		}
		if resolution.HealthLost > 0 {
			switch action.Variant {
			case VariantBurn:
				target.Burn = StackBurn(target.Burn, NewBurn(resolution.HealthLost, actor.ID+":"+skill.ID))
				strike.BurnApplied = true
			case VariantSlow:
				target.Slow = NewSlow(actor.ID + ":" + skill.ID)
				target.recomputeAttributes()
				strike.SlowApplied = true
			}
		}
		strike.Defeated = target.Defeated
		strikes = append(strikes, strike)
	}
	return strikes
}

// applyDamage removes health and marks a combatant defeated at zero.
//
// §7 of the battle design separates being downed from being dead: reaching zero
// health removes the combatant from the fight and nothing more. Whether the
// character actually died is a story decision the caller makes from the real
// result, not something this function assumes.
func applyDamage(target *Combatant, resolution AttackResolution) {
	if resolution.HealthLost <= 0 {
		return
	}
	target.Attributes.Health -= resolution.HealthLost
	if target.Attributes.Health <= 0 {
		target.Attributes.Health = 0
		target.Defeated = true
	}
}

// resolveAcceptSurrender settles one side's answer to an outstanding offer to
// yield.
//
// §14.3 requires that 求饶不自动获得同意: the offer is the enemy's action, and only
// the other side's acceptance turns it into a real removal from the fight. An
// acceptance with no outstanding offer resolves against nothing, which is the
// honest outcome of answering a question nobody asked — it costs the accepting
// side its action and changes no one's standing.
//
// The offer being answered is the first standing opponent's, so an acceptance
// cannot be used to remove a combatant who never offered.
func resolveAcceptSurrender(session *CombatSession, actor *Combatant, action CombatAction) bool {
	target := session.CombatantByID(action.TargetID)
	if target == nil || !target.Active() || target.Side == actor.Side || target.OfferText == "" {
		return false
	}
	target.Surrendered = true
	target.OfferText = ""
	return true
}

// resolveEscape settles a disengagement attempt.
//
// A combatant with no established route cannot escape at all, however high the
// roll: §14.2 forbids escaping from a sealed position, and the chance floor is
// not a licence to ignore that.
func resolveEscape(session *CombatSession, actor *Combatant, action CombatAction, draws DrawSource) bool {
	if !actor.RouteExists {
		return false
	}
	pursuer := firstOpponent(session, actor)
	if pursuer == nil {
		// Nobody is pursuing, so the attempt simply succeeds.
		actor.Fled = true
		return true
	}
	actor.driveTreasure(AttrPerception)
	pursuer.driveTreasure(AttrPerception)
	chance := EscapeChance(actor.Attributes, pursuer.Attributes, actor.Cultivation.Realm, pursuer.Cultivation.Realm, 0, 0)
	if draws("escape", actor.ID, pursuer.ID) >= chance {
		return false
	}
	actor.Fled = true
	return true
}

// advanceExchange ticks lasting effects and recovers burden once.
//
// It runs after every initiative action. Prepared techniques have already had
// their explicit continuation action at the actor's position in that order.
// Residual damage can still interrupt a preparation that remains pending.
func advanceExchange(
	session *CombatSession,
	draws DrawSource,
	freshBurn map[string]bool,
	interrupted map[string]bool,
) []ExchangeStep {
	completed := make([]ExchangeStep, 0)
	for index := range session.Combatants {
		combatant := &session.Combatants[index]
		if !combatant.Active() {
			// A combatant who is out of the fight still has their lasting
			// effects expire, so a downed character does not keep burning.
			combatant.Shield = Shield{}
			combatant.Burn = DamageOverTime{}
			combatant.Slow = SlowEffect{}
			continue
		}

		// A burning effect attached this exchange waits: §5 puts its two
		// residual damages in 后续两个回合 — the two rounds after the hit.
		if !freshBurn[combatant.ID] {
			if damage, remaining := TickDamageOverTime(combatant.Burn); damage > 0 {
				combatant.Burn = remaining
				actualLoss := minInt(damage, combatant.Attributes.Health)
				combatant.Attributes.Health -= actualLoss
				if combatant.Attributes.Health <= 0 {
					combatant.Attributes.Health = 0
					combatant.Defeated = true
				}
				if actualLoss >= preparationInterruptThreshold(combatant.Attributes.HealthMax) || !combatant.Active() {
					if preparation, preparing := session.Preparations[combatant.ID]; preparing {
						delete(session.Preparations, combatant.ID)
						interrupted[combatant.ID] = true
						completed = append(completed, ExchangeStep{ActorID: combatant.ID, TargetID: preparation.TargetID, SkillID: preparation.Skill.ID, Kind: ActionContinuePreparation, PreparationAbandoned: true, ReasonKey: "reason.preparation_interrupted"})
					}
				}
			} else {
				combatant.Burn = remaining
			}
		}

		// A shield does advance in the exchange it was cast in: §5 counts the
		// casting round as the first of its term — 2轮护盾包括施展当轮，到下一轮结束解除.
		combatant.Shield = TickShield(combatant.Shield)
		// A slow is not ticked here: §5 expires it when the target's next primary
		// action completes, which is the target's own turn in the order rather
		// than a count of exchanges. A target that never got to act keeps it.
		// Every surviving cultivator sheds 6 burden per exchange, with no rest
		// action required, and never below a residual floor.
		combatant.Burden = RecoverBurdenPerExchange(combatant.Burden, true)

		// A formation costs its upkeep every exchange it stays up. §4.3 fixes it
		// at 2% of the operator's stage base mana, rounded up, and makes losing the
		// ability to maintain it one of the three ways a formation stops working.
		// An operator who cannot pay ends the formation here, and the bonus has to
		// leave the numbers in the same exchange rather than lingering on the
		// attribute total it was folded into.
		if combatant.Formation.Active {
			wasActive := combatant.Formation.Active
			combatant.Formation, combatant.Attributes = combatant.Formation.Upkeep(combatant.Attributes, combatant.Cultivation)
			if wasActive && !combatant.Formation.Active {
				combatant.recomputeAttributes()
			}
		}

		// A dose's term runs down as exchanges pass, the exchange it was taken in
		// counted. The boost was applied through the modifier aggregation when it
		// was taken, so losing it has to re-derive the numbers rather than
		// subtract the percentage back out: the global cap and the same-type
		// replacement both live in that aggregation.
		if len(combatant.ActiveDoses) > 0 {
			remaining := combatant.ActiveDoses[:0]
			for _, dose := range combatant.ActiveDoses {
				dose.RemainingExchanges--
				if dose.RemainingExchanges > 0 {
					remaining = append(remaining, dose)
				}
			}
			expired := len(remaining) != len(combatant.ActiveDoses)
			combatant.ActiveDoses = remaining
			if expired {
				combatant.recomputeAttributes()
			}
		}
	}

	return completed
}

// completePreparation resolves a technique whose preparation has finished.
//
// The cost, the burden and the residual injury were all settled when the
// preparation started, per §4.2's worked example, so completing it deals the
// strike and nothing else. Its target is the opponent it was aimed at when it
// began; if that opponent is no longer standing, the strike resolves against
// nothing rather than being redirected.
//
// The step it returns reports the completion as a technique action, so the
// exchange says which technique finished and what it did. Its cast check carries
// no cost: the cast was checked and paid for in the exchange the preparation
// started, and pricing it here would charge the same technique twice. It is
// marked allowed because it was — an empty check would read as a refusal.
func completePreparation(session *CombatSession, actor *Combatant, preparation Preparation, draws DrawSource) ExchangeStep {
	step := ExchangeStep{
		ActorID: actor.ID, TargetID: preparation.TargetID,
		Kind: ActionTechnique, SkillID: preparation.Skill.ID,
		Cast: CastCheck{Allowed: true},
	}
	target := session.CombatantByID(preparation.TargetID)
	if target == nil || !target.Active() {
		step.Cast = CastCheck{ReasonKey: "reason.no_target"}
		step.ReasonKey = "reason.no_target"
		return step
	}
	skill := preparation.Skill
	switch {
	case !skill.CanCast(actor.Cultivation):
		reason := ReasonMortalNoTechniques
		if CanUseTechniques(actor.Cultivation) {
			reason = ReasonSkillStageTooLow
		}
		step.Cast = CastCheck{ReasonKey: reason}
	case skill.ID == "":
		step.Cast = CastCheck{ReasonKey: ReasonUnknownSkill}
	case skill.RequiresFocus && !actor.HasImplement:
		step.Cast = CastCheck{ReasonKey: ReasonImplementRequired}
	case actor.Burden.Current > BurdenCapacity(actor.Attributes.Health, actor.Attributes.HealthMax):
		step.Cast = CastCheck{ReasonKey: ReasonBurdenCapacity}
	default:
		step.Cast = CastCheck{Allowed: true}
	}
	if !step.Cast.Allowed {
		step.ReasonKey = step.Cast.ReasonKey
		return step
	}
	treasureCost := actor.driveTreasure(AttrAttack)
	step.OperatingManaCost += treasureCost
	step.Cast.ManaCost += treasureCost
	target.driveTreasure(AttrDefense)
	rolls := CombatRolls{
		Initiative: draws("initiative", actor.ID, ""),
		Hit:        draws("hit", actor.ID, target.ID),
		Damage:     draws("damage", actor.ID, target.ID),
	}
	context := NeutralDamageContext()
	context.DefensePierce = actor.weaponDefensePierce()
	resolution := ResolveAttack(actor.Attributes, target.Attributes, actor.Cultivation.Realm, target.Cultivation.Realm, preparation.Skill, context, target.Shield, rolls)
	step.Resolution = resolution
	if !resolution.Hit {
		return step
	}
	applyDamage(target, resolution)
	if resolution.ShieldAbsorbed > 0 {
		target.Shield.Amount -= resolution.ShieldAbsorbed
	}
	step.Defeated = target.Defeated
	return step
}

// initiativeOrder returns the combatants sorted by drawn initiative, highest
// first, with the ID as a tie-break so the order is stable across runs.
func initiativeOrder(session CombatSession, draws DrawSource) []string {
	type entry struct {
		id    string
		value float64
	}
	entries := make([]entry, 0, len(session.Combatants))
	for _, combatant := range session.Combatants {
		if !combatant.Active() {
			continue
		}
		entries = append(entries, entry{
			id:    combatant.ID,
			value: Initiative(combatant.Attributes.Agility, draws("initiative", combatant.ID, "")),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].value != entries[j].value {
			return entries[i].value > entries[j].value
		}
		return entries[i].id < entries[j].id
	})
	order := make([]string, 0, len(entries))
	for _, item := range entries {
		order = append(order, item.id)
	}
	return order
}

// firstOpponent returns the first live combatant on the other side.
func firstOpponent(session *CombatSession, actor *Combatant) *Combatant {
	for index := range session.Combatants {
		other := &session.Combatants[index]
		if other.Side != actor.Side && other.Active() {
			return other
		}
	}
	return nil
}

// illegalTechniqueDeclarations names the declarations this exchange must refuse
// before resolving anything.
//
// §4.2 prices a technique when it is performed, and a damaging technique
// performed at nobody has no legal target to be performed at. The refusal has to
// happen here, against the session as the exchange was declared, because that is
// the session the player was looking at: checking later, once initiative has
// begun to move combatants out of the fight, would turn a real exchange in which
// the blow found nothing into a round the store refuses to commit — and if the
// target that left was the one who fled, the refusal would also block the flee.
//
// A shielding technique is exempt because §5 lets it be laid on the caster. A
// preparation is not: it must name a live opponent when it begins, and that is
// what fixes the target its completion stays bound to.
func illegalTechniqueDeclarations(session *CombatSession, actions map[string]CombatAction) map[string]string {
	refused := map[string]string{}
	for actorID, action := range actions {
		if action.Kind != ActionTechnique || skillShields(action.Skill) {
			continue
		}
		actor := session.CombatantByID(actorID)
		if actor == nil || !actor.Active() {
			continue
		}
		if !legalTechniqueTarget(actor, session.CombatantByID(action.TargetID)) {
			refused[actorID] = "reason.no_target"
			continue
		}
		if reason := illegalExtraTargets(session, actor, action); reason != "" {
			refused[actorID] = reason
		}
	}
	return refused
}

// illegalExtraTargets checks the further combatants a multi-target declaration
// names.
//
// §9 requires the reach to be settled from the actual choice and the cap, never
// from who is standing, so the same three facts that make the primary target
// legal have to hold for each extra one — and the count may not exceed what the
// technique reaches. An extra name that repeats the primary or another extra is
// not a second target, so it is refused rather than silently collapsed: a client
// that sent the same opponent twice would otherwise be charged for one strike
// and believe it had made two.
func illegalExtraTargets(session *CombatSession, actor *Combatant, action CombatAction) string {
	if len(action.ExtraTargetIDs) == 0 {
		return ""
	}
	// A technique with no declared reach reaches one target. The primary already
	// fills it, so any extra name is past the cap.
	reach := action.Skill.Targets
	if reach < 1 {
		reach = 1
	}
	if len(action.ExtraTargetIDs) > reach-1 {
		return ReasonTooManyTargets
	}
	seen := map[string]bool{action.TargetID: true}
	for _, extraID := range action.ExtraTargetIDs {
		if seen[extraID] {
			return "reason.no_target"
		}
		seen[extraID] = true
		if !legalTechniqueTarget(actor, session.CombatantByID(extraID)) {
			return "reason.no_target"
		}
	}
	return ""
}

// skillShields reports whether a technique builds a shield rather than striking.
//
// §5 lets a shield be laid on the caster, so it is the one technique that is
// legal without a target. The test lives here rather than at each call site so
// the cast path and the window's own validation cannot disagree about it.
func skillShields(skill SkillDef) bool { return skill.ShieldBaseHealthFraction > 0 }

// legalTechniqueTarget reports whether a damaging technique may be aimed at this
// combatant.
//
// A target is legal when it exists, is still in the fight, and is on the other
// side. All three are facts about the session, so the check reads the session
// rather than trusting the declaration: a caller that could name a fallen
// opponent, an ally or nobody at all could otherwise make a technique cost its
// caster a cast that had nowhere to land.
func legalTechniqueTarget(actor *Combatant, target *Combatant) bool {
	if actor == nil || target == nil {
		return false
	}
	if target.Side == actor.Side {
		return false
	}
	return target.Active()
}

// settleEnd decides whether the battle is over and how it finished.
//
// The result distinguishes a side that was beaten from one that left or yielded,
// because §7 requires the main story to write a different aftermath for each and
// forbids treating zero health as an automatic death.
func settleEnd(session *CombatSession) {
	playerStanding := sideStanding(session, SidePlayer)
	enemyStanding := sideStanding(session, SideEnemy)

	switch {
	case playerStanding == 0 && enemyStanding == 0:
		session.Ended, session.Result = true, ResultDraw
	case enemyStanding == 0:
		session.Ended, session.Result = true, sideOutcome(session, SideEnemy, ResultPlayerWon, ResultEnemyFled)
	case playerStanding == 0:
		session.Ended, session.Result = true, sideOutcome(session, SidePlayer, ResultPlayerLost, ResultPlayerFled)
	}
	// A battle where both sides still have someone standing is not over, and
	// stays unresolved: §14.3 requires the story to be able to stop at a
	// surrender or a truce the player has not yet answered, and inventing a
	// result here would take that decision away from them.
}

// sideOutcome names how one side actually left the fight.
//
// §7 requires the aftermath to differ by how the battle ended, and the three
// ways are not interchangeable: a side that was beaten, a side that got away and
// a side that yielded each leave a different situation behind. Reporting a
// surrender as a defeat would have the story write a corpse for someone who is
// standing there unarmed, and reporting a retreat as a defeat would have it
// write a capture that never happened.
//
// A side with mixed endings is reported by the most decisive one, in the order
// fled, yielded, because a side that got away and a side that yielded each leave
// a different situation behind than a side that was beaten.
func sideOutcome(session *CombatSession, side CombatSide, defeatedResult, fledResult CombatResult) CombatResult {
	fled, surrendered := false, false
	for _, combatant := range session.Combatants {
		if combatant.Side != side {
			continue
		}
		if combatant.Fled {
			fled = true
		}
		if combatant.Surrendered {
			surrendered = true
		}
	}
	if fled {
		return fledResult
	}
	if surrendered {
		// §14.3 keeps the decision with the player: a yielded enemy is still
		// alive to be dealt with, so this is a truce rather than a victory.
		return ResultTruce
	}
	return defeatedResult
}

// sideStanding counts the combatants of one side still in the fight.
func sideStanding(session *CombatSession, side CombatSide) int {
	count := 0
	for _, combatant := range session.Combatants {
		if combatant.Side == side && combatant.Active() {
			count++
		}
	}
	return count
}

// clone returns a deep copy of the session.
func (s CombatSession) clone() CombatSession {
	next := s
	next.Combatants = append([]Combatant{}, s.Combatants...)
	// The per-combatant slices have to be copied too. A shallow copy of the
	// struct shares the backing array, so spending a dose or expiring one in the
	// copy would move the number in the session the caller still holds — and
	// ResolveExchange promises not to modify the session it was given.
	for index := range next.Combatants {
		combatant := &next.Combatants[index]
		combatant.Modifiers = append([]AttributeModifier(nil), combatant.Modifiers...)
		combatant.Carried = append([]CarriedDose(nil), combatant.Carried...)
		combatant.ActiveDoses = append([]ActiveDose(nil), combatant.ActiveDoses...)
		// Equipment and spares move during a fight, so they have to be copied too:
		// a swap in the copy would otherwise append to the caller's own spare list
		// and leave the session it promised not to modify holding a rearranged pack.
		combatant.Equipment = append([]EquipmentCopy(nil), combatant.Equipment...)
		combatant.Spares = append([]EquipmentCopy(nil), combatant.Spares...)
	}
	if s.Preparations != nil {
		next.Preparations = make(map[string]Preparation, len(s.Preparations))
		for id, preparation := range s.Preparations {
			next.Preparations[id] = preparation
		}
	}
	return next
}

// mustSkill returns a built-in definition, panicking only if the published table
// has been edited to remove a technique the engine requires.
func mustSkill(id string) SkillDef {
	skill, ok := SkillByID(id)
	if !ok {
		panic("cultivation: the built-in technique table must contain " + id)
	}
	return skill
}
