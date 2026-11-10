package cultivation

import "math"

// This file implements continuous cultivation and major-realm breakthrough:
// §6 and §7 of 《自制-修仙数值规则》V0.3.
//
// Cultivation advances by real elapsed time, not by turns or by a number of
// "sessions". A single six-month advance and six one-month advances must reach
// the same point, so the accumulator below works in continuous rule-days and
// carries the remainder across stage transitions instead of rounding each
// call.

// DaysPerRuleMonth is one rule month: the integration unit for cultivation.
// A rule year is twelve of them. The design is explicit that a displayed
// calendar date is a separate concern and must not be reinterpreted here.
const DaysPerRuleMonth = 30

// qiProgressRequired returns the accumulated progress needed to fill the given
// refining layer: 100 × L².
func qiProgressRequired(layer int) float64 {
	l := float64(clampInt(layer, 1, QiLayers))
	return 100 * l * l
}

// qiMonthlyProgress returns the ordinary-condition progress per rule month at
// the given refining layer: 20 × L.
func qiMonthlyProgress(layer int) float64 {
	return 20 * float64(clampInt(layer, 1, QiLayers))
}

// realmProgressBase returns the progress needed to fill the early stage of a
// major realm, and the ordinary monthly progress at that stage.
//
// Within a realm the progress threshold doubles per sub-stage and the monthly
// rate rises with it: 10000/20000/40000 at 100/140/190 per month for
// Foundation, and so on. Those pairs come from the parameter set.
func realmProgressBase(realm Realm) (progress float64, monthly float64, ok bool) {
	switch realm {
	case RealmFoundation:
		return 10000, 100, true
	case RealmCore:
		return 100000, 500, true
	case RealmNascent:
		return 1000000, 2500, true
	case RealmSpirit:
		return 10000000, 12500, true
	case RealmVoid:
		return 100000000, 62500, true
	case RealmIntegration:
		return 1000000000, 312500, true
	case RealmMahayana:
		return 10000000000, 1562500, true
	}
	return 0, 0, false
}

// stageProgressScale and stageTrainingScale describe how a sub-stage scales the
// early-stage threshold and monthly rate. Both are 1.0/2.0/4.0 for progress and
// 1.0/1.4/1.9 for training, matching the parameter set.
func stageProgressScale(stage Stage) float64 {
	switch stage {
	case StageMiddle:
		return 2.0
	case StageLate:
		return 4.0
	}
	return 1.0
}

func stageTrainingScale(stage Stage) float64 {
	switch stage {
	case StageMiddle:
		return 1.4
	case StageLate:
		return 1.9
	}
	return 1.0
}

// ProgressRequired returns the accumulated progress needed to complete the
// character's current stage, and the ordinary monthly progress gained there.
//
// The final supported position still reports its authored progress and rate.
// AtTerminalStage separately prevents inventing a next realm.
func ProgressRequired(c Cultivation) (required float64, monthly float64, ok bool) {
	if c.rules != nil {
		row, ok := c.rules.stage(c)
		return row.ProgressRequired, row.MonthlyProgress, ok && row.ProgressRequired > 0
	}
	switch c.Realm {
	case RealmMortal:
		// A mortal does not accumulate cultivation progress at all; becoming a
		// cultivator is a story event, not a threshold crossed by waiting.
		return 0, 0, false
	case RealmQi:
		layer := clampInt(c.Layer, 1, QiLayers)
		return qiProgressRequired(layer), qiMonthlyProgress(layer), true
	default:
		base, baseMonthly, found := realmProgressBase(c.Realm)
		if !found {
			return 0, 0, false
		}
		return base * stageProgressScale(c.Stage), baseMonthly * stageTrainingScale(c.Stage), true
	}
}

// AtTerminalStage reports whether the character has reached the end of what
// this rule version implements: the late stage of the highest realm.
//
// At this point progress still accumulates but no further stage transition is
// available, so a caller must stop at the boundary rather than roll into an
// unimplemented realm.
func AtTerminalStage(c Cultivation) bool {
	if c.rules != nil {
		row, ok := c.rules.stage(c)
		return !ok || row.Terminal
	}
	return c.Realm == RealmMahayana && c.Stage == StageLate
}

// AtTrainingBoundary reports whether ordinary training can no longer move this
// character, so the next step is a decision rather than more time.
//
// There are two such positions and they are different findings. The terminal
// stage is the end of the implemented table, where progress still banks but no
// transition exists. The last stage before a major realm is a threshold already
// met, where the only way forward is an authorised breakthrough. A reader that
// asked only the first question would tell a character at the top refining layer
// that they could still train, which is exactly what the interval rules refuse
// to spend time on.
func AtTrainingBoundary(c Cultivation, accumulated float64) bool {
	if AtTerminalStage(c) {
		return true
	}
	required, _, ok := ProgressRequired(c)
	if !ok || accumulated < required {
		return false
	}
	_, advanced := nextStage(c)
	return !advanced
}

// TrainingModifiers are the real conditions that scale cultivation speed.
// Every field is required to be one of the enumerated values; an unrecognised
// value falls back to the neutral 1.0 rather than being ignored silently.
type TrainingModifiers struct {
	Talent      Talent
	Quality     TechniqueQuality
	Environment Environment
	Resource    SupportResource
	// HealthFraction is current health over maximum health, used to pick the
	// body-condition scale.
	HealthFraction float64
	// GrowthCoefficient is the selected method's training multiplier (§4.3).
	// Only one method's coefficient applies to a given training segment, and a
	// zero value means no method was selected, which is the neutral 1.0.
	GrowthCoefficient float64
}

// Talent is a cultivation aptitude.
type Talent string

const (
	TalentPoor        Talent = "poor"
	TalentNormal      Talent = "normal"
	TalentGood        Talent = "good"
	TalentExcellent   Talent = "excellent"
	TalentExceptional Talent = "exceptional"
)

// TechniqueQuality is the grade of the primary cultivation technique.
type TechniqueQuality string

const (
	QualityOrdinary TechniqueQuality = "ordinary"
	QualityGood     TechniqueQuality = "good"
	QualityAdvanced TechniqueQuality = "advanced"
)

// Environment is the spiritual quality of the place of cultivation.
type Environment string

const (
	EnvironmentPoor     Environment = "poor"
	EnvironmentOrdinary Environment = "ordinary"
	EnvironmentStable   Environment = "stable_spiritual_site"
)

// SupportResource is the strength of the real auxiliary resources being
// consumed. It is a property of resources that actually exist and are actually
// consumed, never a permanent bonus from a single pill.
type SupportResource string

const (
	SupportNone   SupportResource = "none"
	SupportNormal SupportResource = "normal_support"
	SupportStrong SupportResource = "strong_support"
)

func talentScale(t Talent) float64 {
	switch t {
	case TalentPoor:
		return 0.50
	case TalentGood:
		return 1.30
	case TalentExcellent:
		return 1.60
	case TalentExceptional:
		return 2.00
	}
	return 1.00
}

func qualityScale(q TechniqueQuality) float64 {
	switch q {
	case QualityGood:
		return 1.15
	case QualityAdvanced:
		return 1.30
	}
	return 1.00
}

func environmentScale(e Environment) float64 {
	switch e {
	case EnvironmentPoor:
		return 0.70
	case EnvironmentStable:
		return 1.25
	}
	return 1.00
}

func resourceScale(r SupportResource) float64 {
	switch r {
	case SupportNormal:
		return 1.20
	case SupportStrong:
		return 1.40
	}
	return 1.00
}

// healthConditionScale is 1.00 at 90% health or above, 0.80 down to 60%,
// 0.40 down to 30%, and 0 below that.
func healthConditionScale(fraction float64) float64 {
	switch {
	case fraction >= 0.90:
		return 1.00
	case fraction >= 0.60:
		return 0.80
	case fraction >= 0.30:
		return 0.40
	}
	return 0
}

// combinedScale returns the product of every modifier, clamped to the
// permitted 0.20–4.00 range.
//
// The clamp is applied to the product, not to each factor, so the individual
// values stay readable while the overall pace stays bounded. A body condition
// of zero survives the clamp as zero rather than being lifted to the minimum:
// the rules say a badly injured character cannot cultivate normally, and that
// is a prohibition rather than a slow rate.
func combinedScale(m TrainingModifiers, rules ...*Rules) float64 {
	if len(rules) > 0 && rules[0] != nil {
		return rules[0].trainingScale(m)
	}
	growth := m.GrowthCoefficient
	if growth <= 0 {
		growth = qualityScale(m.Quality)
	}
	// A registered method already carries its quality in its coefficient.
	// Grade is the legacy fallback, never a second multiplier for that method.
	scale := talentScale(m.Talent) *
		growth *
		environmentScale(m.Environment) *
		resourceScale(m.Resource) *
		healthConditionScale(m.HealthFraction)
	if scale <= 0 {
		return 0
	}
	return clampFloat(scale, 0.20, 4.00)
}

// Progress is the result of advancing cultivation by some elapsed time.
type Progress struct {
	// Cultivation is where the character ended up, after any automatic
	// small-stage transitions.
	Cultivation Cultivation
	// Accumulated is the progress banked toward the current stage. It is a
	// float64 because the rules keep full precision and only round for
	// display; rounding here would make a long advance differ from the same
	// advance split into pieces.
	Accumulated float64
	// StagesAdvanced counts completed stage transitions during this advance.
	StagesAdvanced int
	// StoppedAtBoundary is true when the advance ran to the end of the
	// implemented rule set and stopped there.
	StoppedAtBoundary bool
	// Gained is the total progress added before any transition reset. It is
	// reported for the change summary, not used in further arithmetic.
	Gained float64
	// ConsumedDays is how much of the requested interval the rules really spent.
	//
	// A character who has already filled the last stage before a major realm
	// cannot train past it: the next step is an authorised breakthrough, which
	// is a decision rather than more time. The interval then stops at that
	// boundary and the remainder is not spent, so the caller can preserve the
	// player's remaining intent. Reporting the consumed part is what lets the
	// story clock advance by the interval that was really spent instead of by
	// the interval that was merely named.
	//
	// It is not the same as "time passed". A mortal, or a character too badly
	// hurt to cultivate, consumes the whole interval while gaining nothing;
	// only the major-realm bottleneck leaves time unspent.
	ConsumedDays float64
}

// AdvanceCultivation advances cultivation by the given number of real days
// under the supplied modifiers, starting from no banked progress.
func AdvanceCultivation(c Cultivation, days float64, m TrainingModifiers) Progress {
	return AdvanceCultivationFrom(c, 0, days, m)
}

// AdvanceCultivationFrom advances cultivation from a position that already has
// progress banked toward its current stage.
//
// This is the entry point a real caller uses, because a character almost never
// starts a training period exactly on a threshold. Splitting a period into
// several calls through this function must reach the same result as one call
// for the whole period — that is the design's stated consistency requirement,
// and it is why the accumulator is carried rather than rounded per call.
//
// Small-stage transitions happen automatically inside an authorised training
// period: on reaching a threshold the character moves to the next stage, the
// accumulator resets, and the remaining time continues at the new stage's
// rate. No die is rolled per layer — that is reserved for major-realm
// breakthrough, which this function never performs.
func AdvanceCultivationFrom(c Cultivation, accumulated float64, days float64, m TrainingModifiers) Progress {
	if accumulated < 0 {
		accumulated = 0
	}
	result := Progress{Cultivation: c, Accumulated: accumulated}
	if days <= 0 {
		return result
	}
	// Every interval that is not stopped by the major-realm bottleneck spends all
	// of its time. The bottleneck is the one place time is left unspent, and it
	// overwrites this below with the part it really consumed.
	result.ConsumedDays = days
	if c.Realm == RealmMortal {
		// Mortals do not accumulate progress; there is nothing to advance.
		return result
	}

	scale := combinedScale(m, c.rules)
	if scale <= 0 {
		// Too injured to cultivate: time passes, progress does not.
		return result
	}

	remainingDays := days
	// The loop is bounded by the number of stages that exist, so a
	// pathological rate cannot spin forever; each iteration consumes time or
	// stops at the terminal stage.
	for remainingDays > 0 {
		required, monthly, ok := ProgressRequired(c)
		if !ok {
			result.StoppedAtBoundary = true
			break
		}
		daysPerMonth := float64(DaysPerRuleMonth)
		if c.rules != nil {
			daysPerMonth = c.rules.Training.DaysPerMonth
		}
		perDay := monthly / daysPerMonth * scale
		if perDay <= 0 {
			break
		}
		if AtTerminalStage(c) {
			// The threshold is met and nothing further is implemented: bank
			// the remaining time at the terminal rate and stop.
			gain := perDay * remainingDays
			result.Accumulated += gain
			result.Gained += gain
			result.StoppedAtBoundary = true
			remainingDays = 0
			break
		}

		needed := required - result.Accumulated
		if needed <= 0 {
			// The threshold is already met. If no next small stage exists this
			// is the last position before a major breakthrough, and no amount
			// of ordinary training moves it: the interval stops here rather
			// than spending time on progress that cannot be banked.
			next, advanced := nextStage(c)
			if !advanced {
				result.StoppedAtBoundary = true
				break
			}
			c = next
			result.Cultivation = c
			result.Accumulated = 0
			result.StagesAdvanced++
			continue
		}

		daysToFill := needed / perDay
		if daysToFill > remainingDays {
			gain := perDay * remainingDays
			result.Accumulated += gain
			result.Gained += gain
			remainingDays = 0
			break
		}

		// This stage fills within the remaining time. Check whether a next
		// stage exists BEFORE banking the result: at the top refining layer
		// there is none, and discarding the filled threshold there would leave
		// the character permanently one step short of the major breakthrough
		// they had in fact earned.
		next, advanced := nextStage(c)
		if !advanced {
			result.Gained += needed
			result.Accumulated = required
			result.StoppedAtBoundary = true
			remainingDays = 0
			break
		}
		result.Gained += needed
		remainingDays -= daysToFill
		result.Accumulated = 0
		c = next
		result.Cultivation = c
		result.StagesAdvanced++
	}
	// Whatever the loop could not spend is the remainder the player still holds.
	result.ConsumedDays = days - remainingDays
	return result
}

// nextStage returns the next position on the stage table, and whether one
// exists within the implemented rule set.
//
// Qi refining walks its numbered layers; the major realms walk early → middle
// → late. The step from the last refining layer into Foundation is deliberately
// NOT taken here: that is a major breakthrough, which requires authorisation
// and a probability roll.
func nextStage(c Cultivation) (Cultivation, bool) {
	if c.rules != nil {
		row, ok := c.rules.stage(c)
		if !ok || row.Next == "" {
			return c, false
		}
		next, ok := ParseStageKey(row.Next)
		return next.WithRules(c.rules), ok
	}
	switch c.Realm {
	case RealmQi:
		if c.Layer >= QiLayers {
			// Full at the top refining layer; waiting for breakthrough.
			return c, false
		}
		return Qi(c.Layer + 1), true
	default:
		if _, _, ok := realmProgressBase(c.Realm); !ok {
			return c, false
		}
		switch c.Stage {
		case StageEarly:
			return Cultivation{Realm: c.Realm, Stage: StageMiddle}, true
		case StageMiddle:
			return Cultivation{Realm: c.Realm, Stage: StageLate}, true
		}
		return c, false
	}
}

// BreakthroughTarget is the realm a character may attempt to enter.
type BreakthroughTarget string

const (
	BreakthroughFoundation  BreakthroughTarget = "foundation"
	BreakthroughCore        BreakthroughTarget = "core"
	BreakthroughNascent     BreakthroughTarget = "nascent"
	BreakthroughSpirit      BreakthroughTarget = "spirit"
	BreakthroughVoid        BreakthroughTarget = "void"
	BreakthroughIntegration BreakthroughTarget = "integration"
	BreakthroughMahayana    BreakthroughTarget = "mahayana"
)

// breakthroughBaseChance is the base success rate per target realm.
var breakthroughBaseChance = map[BreakthroughTarget]float64{
	BreakthroughFoundation:  0.30,
	BreakthroughCore:        0.20,
	BreakthroughNascent:     0.12,
	BreakthroughSpirit:      0.08,
	BreakthroughVoid:        0.06,
	BreakthroughIntegration: 0.05,
	BreakthroughMahayana:    0.05,
}

// breakthroughMedicineBonus is the bonus from one appropriate auxiliary pill.
var breakthroughMedicineBonus = map[BreakthroughTarget]float64{
	BreakthroughFoundation:  0.25,
	BreakthroughCore:        0.20,
	BreakthroughNascent:     0.20,
	BreakthroughSpirit:      0.15,
	BreakthroughVoid:        0.12,
	BreakthroughIntegration: 0.10,
	BreakthroughMahayana:    0.08,
}

// breakthroughDays is the real time one attempt takes.
var breakthroughDays = map[BreakthroughTarget]float64{
	BreakthroughFoundation:  7,
	BreakthroughCore:        30,
	BreakthroughNascent:     90,
	BreakthroughSpirit:      180,
	BreakthroughVoid:        360,
	BreakthroughIntegration: 720,
	BreakthroughMahayana:    1080,
}

// breakthroughFailureRecoveryDays is the recovery imposed by a failed attempt.
var breakthroughFailureRecoveryDays = map[BreakthroughTarget]float64{
	BreakthroughFoundation:  7,
	BreakthroughCore:        30,
	BreakthroughNascent:     90,
	BreakthroughSpirit:      180,
	BreakthroughVoid:        360,
	BreakthroughIntegration: 720,
	BreakthroughMahayana:    1080,
}

// BreakthroughModifiers are the real conditions of one attempt.
type BreakthroughModifiers struct {
	Talent  Talent
	Quality TechniqueQuality
	// AuxiliaryMedicine is true only when one appropriate dose actually exists
	// and is actually consumed by this attempt.
	AuxiliaryMedicine bool
	// StableSite is a genuinely qualified cave dwelling.
	StableSite bool
	// QualifiedGuardian is a guardian whose qualification is real, not merely
	// asserted in narration.
	QualifiedGuardian bool
}

// BreakthroughChance returns the success probability for one attempt, clamped
// to the permitted 5%–90%.
//
// The chance is computed from real conditions only. Nothing here consults the
// narrative, so a story cannot talk its way to a higher probability.
func BreakthroughChance(target BreakthroughTarget, m BreakthroughModifiers, rules ...*Rules) (float64, bool) {
	if len(rules) > 0 && rules[0] != nil {
		return rules[0].breakthroughChance(target, m)
	}
	base, ok := breakthroughBaseChance[target]
	if !ok {
		return 0, false
	}
	chance := base
	switch m.Talent {
	case TalentPoor:
		chance -= 0.10
	case TalentGood:
		chance += 0.08
	case TalentExcellent:
		chance += 0.15
	case TalentExceptional:
		chance += 0.20
	}
	switch m.Quality {
	case QualityGood:
		chance += 0.05
	case QualityAdvanced:
		chance += 0.10
	}
	if m.AuxiliaryMedicine {
		chance += breakthroughMedicineBonus[target]
	}
	if m.StableSite {
		chance += 0.05
	}
	if m.QualifiedGuardian {
		chance += 0.05
	}
	return clampFloat(chance, 0.05, 0.90), true
}

// BreakthroughRequirement describes what an attempt needs before it may roll.
type BreakthroughRequirement struct {
	// Eligible reports whether every precondition is met.
	Eligible bool
	// Reasons lists, in player-readable terms, which preconditions failed.
	Reasons []string
	// ManaCost is the mana consumed when preparation begins.
	ManaCost int
	// Days is the real time the attempt takes.
	Days float64
}

// CanAttemptBreakthrough checks the preconditions for entering target from the
// character's current position and current pools.
//
// A missing precondition means no roll and no cost: the attempt is not made at
// all, rather than made and failed. That distinction matters because a failed
// attempt carries real penalties.
//
// convalescing reports that a failed attempt's recovery has not yet elapsed. §7.1
// makes "不存在尚未解除的突破恢复状态" one of the preconditions, so an attempt made
// inside that window is refused before the die is cast rather than rolled and
// penalised a second time.
func CanAttemptBreakthrough(c Cultivation, accumulated float64, attrs Attributes, target BreakthroughTarget, convalescing bool) BreakthroughRequirement {
	if c.rules != nil {
		return c.rules.breakthroughRequirement(c, accumulated, attrs, target, convalescing)
	}
	required, _, ok := ProgressRequired(c)
	if !ok {
		return BreakthroughRequirement{Reasons: []string{"当前阶段不积累修炼进度"}}
	}
	if accumulated < required {
		return BreakthroughRequirement{Reasons: []string{"积累尚未达到门槛"}}
	}
	if convalescing {
		return BreakthroughRequirement{Reasons: []string{"上次突破失败后的恢复尚未结束"}}
	}
	// The character must be at the last stage before the target realm.
	if !atLastStageBefore(c, target) {
		return BreakthroughRequirement{Reasons: []string{"必须处于前一大境界的最后阶段"}}
	}
	if attrs.HealthMax <= 0 || float64(attrs.Health) < 0.90*float64(attrs.HealthMax) {
		return BreakthroughRequirement{Reasons: []string{"当前生命低于上限的90%"}}
	}
	if attrs.ManaMax <= 0 || float64(attrs.Mana) < 0.80*float64(attrs.ManaMax) {
		return BreakthroughRequirement{Reasons: []string{"当前灵力低于上限的80%"}}
	}
	days, ok := breakthroughDays[target]
	if !ok {
		return BreakthroughRequirement{Reasons: []string{"目标境界未实现"}}
	}
	return BreakthroughRequirement{
		Eligible: true,
		ManaCost: int(ceilNonNegative(0.80 * float64(attrs.ManaMax))),
		Days:     days,
	}
}

// atLastStageBefore reports whether c is the final position of the realm
// immediately below target.
func atLastStageBefore(c Cultivation, target BreakthroughTarget) bool {
	switch target {
	case BreakthroughFoundation:
		return c.Realm == RealmQi && c.Layer == QiLayers
	case BreakthroughCore:
		return c.Realm == RealmFoundation && c.Stage == StageLate
	case BreakthroughNascent:
		return c.Realm == RealmCore && c.Stage == StageLate
	case BreakthroughSpirit:
		return c.Realm == RealmNascent && c.Stage == StageLate
	case BreakthroughVoid:
		return c.Realm == RealmSpirit && c.Stage == StageLate
	case BreakthroughIntegration:
		return c.Realm == RealmVoid && c.Stage == StageLate
	case BreakthroughMahayana:
		return c.Realm == RealmIntegration && c.Stage == StageLate
	}
	return false
}

// BreakthroughOutcome is the settled result of one attempt.
type BreakthroughOutcome struct {
	// Succeeded reports whether the character entered the target realm.
	Succeeded bool
	// Cultivation is the resulting position.
	Cultivation Cultivation
	// Accumulated is the resulting progress.
	Accumulated float64
	// Health and Mana are the resulting current pools. A success does NOT
	// refill them; a failure reduces the health maximum, so the current value
	// is clamped to the new maximum instead.
	Health int
	Mana   int
	// RecoveryDays is the real recovery imposed by a failure, zero on success.
	RecoveryDays float64
}

// ResolveBreakthrough settles one attempt from an already-drawn random value.
//
// The random draw is passed in rather than taken here so the caller can bind
// it to the command identity: a retried or refreshed request must re-use the
// same draw instead of re-rolling. The same reason applies to the committed
// attempt as a whole.
func ResolveBreakthrough(c Cultivation, accumulated float64, attrs Attributes, target BreakthroughTarget, m BreakthroughModifiers, roll float64) (BreakthroughOutcome, bool) {
	if c.rules != nil {
		return c.rules.resolveBreakthrough(c, accumulated, attrs, target, m, roll)
	}
	chance, ok := BreakthroughChance(target, m)
	if !ok {
		return BreakthroughOutcome{}, false
	}
	if roll < chance {
		next := enteredRealm(target)
		return BreakthroughOutcome{
			Succeeded:   true,
			Cultivation: next,
			Accumulated: 0,
			// Current pools carry over and are clamped to the new maxima; the
			// design is explicit that a breakthrough does not refill them.
			Health: minInt(attrs.Health, BaseAttributes(next).HealthMax),
			Mana:   minInt(attrs.Mana, BaseAttributes(next).ManaMax),
		}, true
	}

	required, _, _ := ProgressRequired(c)
	healthLoss := int(roundHalfUp(0.10 * float64(attrs.HealthMax)))
	return BreakthroughOutcome{
		Succeeded:    false,
		Cultivation:  c,
		Accumulated:  math.Max(0, accumulated-0.10*required),
		Health:       maxInt(0, attrs.Health-healthLoss),
		Mana:         attrs.Mana,
		RecoveryDays: breakthroughFailureRecoveryDays[target],
	}, true
}

// enteredRealm returns the early stage of the realm a successful breakthrough
// reaches.
func enteredRealm(target BreakthroughTarget) Cultivation {
	switch target {
	case BreakthroughFoundation:
		return Cultivation{Realm: RealmFoundation, Stage: StageEarly}
	case BreakthroughCore:
		return Cultivation{Realm: RealmCore, Stage: StageEarly}
	case BreakthroughNascent:
		return Cultivation{Realm: RealmNascent, Stage: StageEarly}
	case BreakthroughSpirit:
		return Cultivation{Realm: RealmSpirit, Stage: StageEarly}
	case BreakthroughVoid:
		return Cultivation{Realm: RealmVoid, Stage: StageEarly}
	case BreakthroughIntegration:
		return Cultivation{Realm: RealmIntegration, Stage: StageEarly}
	case BreakthroughMahayana:
		return Cultivation{Realm: RealmMahayana, Stage: StageEarly}
	}
	return Mortal()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
