package interactive

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"ta/internal/cultivation"
)

// This file makes §6 of 《自制-技能与施展》 real during play: the registry, the
// learned set and the validate-then-register-then-grant sequence.
//
// Before it, the registry and the learned-skill type existed and were tested
// inside internal/cultivation, but nothing in a story turn ever consulted either.
// A generated technique had no path from "the pack described it" to "the
// character can cast it", so the three facts the design insists on separating —
// a definition exists, a definition is registered, a character has learned it —
// were only separated in a package nobody called.
//
// The division of labour mirrors cultivation_advance.go:
//
//   - The registry is built from the rules' own built-in table, so the set of
//     definitions a story can learn from is fixed and versioned rather than
//     invented per turn.
//   - The learned record lives in the actor's own state, because who has learned
//     what is a fact about a character and not about the world.
//   - Learning runs validate → register → grant, in that order, and a definition
//     refused at any step grants nothing. That ordering is the design's, and it
//     is why a technique with an out-of-budget power cannot reach a character by
//     being learned first and checked afterwards.
//
// The numbers are never copied into the record. A cast is priced from the
// registered definition at the version the character learned, so editing the
// record cannot inflate a technique.

// StateOpSourceSkillLearning marks an Actor State operation the skill rules
// produced while granting mastery of a registered technique.
//
// It is its own source kind because the reroll path has to be able to drop a
// learning without touching anything the narrative wrote, exactly as it drops a
// settlement's or an advance's writes.
const StateOpSourceSkillLearning = "skill_learning"

// TurnSkillLearningInput is the model-facing learning module of one story turn.
//
// It is the structured form of a command like "在此闭关，参悟《赤炎诀》" where the
// character genuinely obtains a technique. The model names the definition and the
// version; the rules decide whether it exists, whether it is valid, and whether
// the character may hold it.
//
// It carries no power, no cost and no burden. Those belong to the registered
// definition, which is what makes "AI不能为戏剧需要临时编一套参数" structural: there
// is nothing in this module for the model to put numbers into.
type TurnSkillLearningInput struct {
	// ActorID is who learns. Omit for the protagonist.
	ActorID string `json:"actor_id,omitempty" jsonschema_description:"实际学习的角色，省略则为主角。"`
	// Skills are the definitions being learned this turn.
	Skills []TurnSkillLearning `json:"skills,omitempty" jsonschema_description:"本回合实际学会的功法或制作方法；没有学会就留空，仅提及或翻阅不等于掌握。"`
}

// TurnSkillLearning is one technique being learned.
//
// A learning is either a reference to a published definition (DefID) or a
// generation from an approved template (TemplateID plus Name). The two are
// exclusive: a published definition already has its identity and its wording, so
// naming a template alongside it would be two claims about the same fact.
type TurnSkillLearning struct {
	SkillDesign *GeneratedSkillDesign `json:"skill_design,omitempty" jsonschema_description:"仅新战斗技能的规则内设计。允许威力、目标、命中、穿透或护盾的真实差异，自动核算耗灵/负担/学习成本，不得与craft_design混用。"`
	CraftDesign *GeneratedCraftDesign `json:"craft_design,omitempty" jsonschema_description:"仅制作方法生成使用：在已有方法基础上推导真实新产物、品级内效果与材料，登记不会获得产物，随后用craft结算。"`
	Minutes     float64               `json:"minutes,omitempty" jsonschema_description:"本回合实际投入的学习分钟；普通/良好/高阶技能按1/3/7日与本人资质累计，0分钟只登记或查看，不会学会。多项分钟顺序相加，不重复使用同段时间。制作方法按实际传授登记，不套技能学习天数；登记本身不推进时钟，正文不能据此添加几小时或几日已经过去，制作耗时另由craft结算。"`

	// DefID names a registered definition.
	DefID string `json:"def_id,omitempty" jsonschema_description:"已登记功法或制作方法的稳定标识，例如 fire_bolt、shield、craft.talisman.ward。制作方法仅授予制作资格，不成为战斗招式。"`
	// Version is the definition revision being learned. It must be a version the
	// registry actually holds, so a character cannot learn numbers that were never
	// fixed.
	Version int `json:"version,omitempty" jsonschema_description:"实际学习的已登记版本，省略则使用当前登记版本，不能指定不存在的修订。"`
	// TemplateID names an approved numeric template a new technique is built
	// from. Optional SkillDesign proposals may reallocate supported mechanics;
	// preparation, focus and residual recovery remain the template's contract.
	TemplateID string `json:"template_id,omitempty" jsonschema_description:"新功法的效果模板：fire_bolt、condensed_blade、heavy_thunder、flame_wave、shield、meridian_secret_blade；新制作方法可选已登记 method_id，如 craft.talisman.ward。填写 name 和实际 source，规则沿用模板效果与条件。已登记内容用 def_id。"`
	// Name is the technique's own wording, required with TemplateID and refused
	// without it. The name becomes part of the definition's stable identity, so
	// the same name in the same world resolves to the same definition.
	Name string `json:"name,omitempty" jsonschema_description:"新功法或制作方法的名称，配合 template_id 使用，名称与背景可按故事自由创作。"`
	// Quality and Rarity must be the pair the template publishes when supplied at
	// all. They exist so the model can state a grade it was told, never so it can
	// choose a higher one: a mismatched pair is refused rather than corrected.
	Quality string `json:"quality,omitempty" jsonschema_description:"Optional. The technique's grade in the world's own words: 普通, 良好 or 高阶. It must match what the template publishes, or be omitted."`
	Rarity  string `json:"rarity,omitempty" jsonschema_description:"Optional. The technique's acquisition tier: 常见, 少见, 稀有, 珍稀 or 独特. It must match what the template publishes, or be omitted."`
	// Source records where the technique came from in the story — a manual, a
	// teacher, an inheritance. It is prose for the journal and is never priced.
	Source string `json:"source,omitempty" jsonschema_description:"实际完整可读资料或有效传授的来历。首次实际学习需要本次source或已登记来历；制作方法每次正式授业必须填写。不把缺关键内容的残本当成完整传授，不改变任何数值。"`
	// Description is what the art looks and feels like when it is used, in the
	// story's own words, with a little wit where it fits. It is journal material:
	// every number stays on the registered definition or template, so a vivid
	// sentence cannot buy an unregistered effect. Written once, when the technique
	// is first learned.
	Description string `json:"description,omitempty" jsonschema_description:"Optional. One or two sentences on what this art looks or feels like in the story's own words. Do not restate its power, cost or numbers — those come from the definition or template. Omit for an unremarkable art."`
}

// Empty reports whether the module carries nothing to apply.
func (input TurnSkillLearningInput) Empty() bool { return len(input.Skills) == 0 }

// cloneTurnSkillLearningInput copies a learning module so an accepted one can be
// retained in a draft without the caller's slice being aliased.
func cloneTurnSkillLearningInput(input *TurnSkillLearningInput) *TurnSkillLearningInput {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Skills = append([]TurnSkillLearning(nil), input.Skills...)
	for index, request := range cloned.Skills {
		cloned.Skills[index].SkillDesign = cloneGeneratedSkillDesign(request.SkillDesign)
		if request.CraftDesign != nil {
			design := *request.CraftDesign
			design.Inputs = append([]cultivation.CraftInput(nil), request.CraftDesign.Inputs...)
			design.Item.MainBonus = cloneGeneratedFloat(design.Item.MainBonus)
			design.Item.ManaFraction = cloneGeneratedFloat(design.Item.ManaFraction)
			cloned.Skills[index].CraftDesign = &design
		}
	}

	return &cloned
}

// validateTurnSkillLearningModule rejects a learning module the rules could not
// act on, so the model repairs it rather than the turn silently learning nothing.
//
// It checks shape, not outcome. Whether the definition exists and whether the
// character may hold it are the rules' answers and are reported in the result;
// what must not pass is an empty definition ID or a negative version.
func validateTurnSkillLearningModule(input *TurnSkillLearningInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	for index, skill := range input.Skills {
		hasDef := strings.TrimSpace(skill.DefID) != ""
		hasTemplate := strings.TrimSpace(skill.TemplateID) != ""
		switch {
		case !hasDef && !hasTemplate:
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleSkills, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/skills/"+strconv.Itoa(index)+"/def_id", "a registered technique definition identifier or an approved template",
				"", "Each learned technique must name either a registered definition or an approved template.",
			)
		case hasDef && hasTemplate:
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleSkills, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/skills/"+strconv.Itoa(index)+"/template_id", "def_id or template_id, not both",
				skill.TemplateID, "A technique is either an already published definition or a new one built from a template, never both.",
			)
		case hasTemplate && strings.TrimSpace(skill.Name) == "":
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleSkills, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/skills/"+strconv.Itoa(index)+"/name", "the technique's name in the story's words",
				"", "A technique built from a template must be given the name the world calls it.",
			)
		case hasDef && strings.TrimSpace(skill.Name) != "":
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleSkills, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/skills/"+strconv.Itoa(index)+"/name", "no name for an already published definition",
				skill.Name, "A published definition already has its wording; learn it by def_id without renaming it.",
			)
		}
		if skill.Minutes < 0 || math.IsNaN(skill.Minutes) || math.IsInf(skill.Minutes, 0) || skill.Minutes >= float64(math.MaxInt64)/60 {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleSkills, intPointer(index), TurnSubmissionDiagnosticInvalidModule, "/skills/"+strconv.Itoa(index)+"/minutes", "finite nonnegative study minutes", "invalid duration", "学习时长必须为有限且非负的实际分钟。")
		}
		if skill.Version < 0 {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleSkills, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/skills/"+strconv.Itoa(index)+"/version", "a positive version, or omitted for the current one",
				strconv.Itoa(skill.Version), "A technique version must be positive, or omitted to learn the registry's current version.",
			)
		}
		if hasTemplate && skill.Version > 0 {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleSkills, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/skills/"+strconv.Itoa(index)+"/version", "no version for a newly generated technique",
				strconv.Itoa(skill.Version), "A technique built from a template is registered at its first version; a version cannot be requested for it.",
			)
		}
	}
	return nil
}

// SkillLearningResult reports what one turn's learning did, in player-readable
// terms.
type SkillLearningResult struct {
	// ActorID is the character the learning applied to.
	ActorID string
	// Learned lists the definitions actually granted, at the version granted.
	Learned []cultivation.SkillDef
	Studied []TechniqueStudy
	// CraftMethods uses the same learning journal but never becomes a combat technique.
	CraftMethods []cultivation.CraftMethodDef
	// Refused lists the definitions that were not granted, with the reason.
	Refused []SkillLearningRefusal
}

// SkillLearningRefusal is one definition that was not granted.
//
// It is reported rather than silently dropped: a player who read a secret manual
// and was refused mastery needs to see why, and the design requires the reason to
// name the real cause rather than only saying that nothing happened.
type SkillLearningRefusal struct {
	DefID  string
	Reason string
}

// xiuxianSkillQualityWording and xiuxianSkillRarityWording are the published
// wording for a generated technique's grade and acquisition tier. The rules key
// them in English and the story records them in Chinese, so the record renders the
// words a reader expects rather than the rule layer's own keys.
var xiuxianSkillQualityWording = map[cultivation.TechniqueQuality]string{
	cultivation.QualityOrdinary: "普通",
	cultivation.QualityGood:     "良好",
	cultivation.QualityAdvanced: "高阶",
}

var xiuxianSkillRarityWording = map[cultivation.ItemRarity]string{
	cultivation.RarityCommon:   "常见",
	cultivation.RarityUncommon: "少见",
	cultivation.RarityRare:     "稀有",
	cultivation.RarityPrecious: "珍稀",
	cultivation.RarityUnique:   "独特",
}

// XiuxianSkillTemplateGuidanceForModel is xiuxianSkillTemplateGuidance for the
// tool description, which lives outside this package.
func XiuxianSkillTemplateGuidanceForModel() string { return xiuxianSkillTemplateGuidance() }

// xiuxianSkillTemplateGuidance names the approved templates and their published
// grades, so the model can invent a technique the rules will really accept.
//
// §7.2 requires the application layer to assemble the selectable templates rather
// than let the model guess them. The list is generated from the rule table, so a
// template whose implementation has not passed acceptance cannot appear here and a
// template that is withdrawn disappears with it.
func xiuxianSkillTemplateGuidance(profiles ...*cultivation.Rules) string {
	templates := cultivation.SkillTemplates(profiles...)
	parts := make([]string, 0, len(templates))
	for _, template := range templates {
		parts = append(parts, fmt.Sprintf("%s（%s·%s）",
			template.Definition.ID,
			xiuxianSkillQualityWording[template.Quality],
			xiuxianSkillRarityWording[template.Rarity]))
	}
	if len(templates) == 0 {
		return "本局没有可用的生成技能模板。"
	}
	return "需要新造功法时用 template_id 选本局已批准模板，并用 name 写名称：" + strings.Join(parts, "；") + "。省略skill_design时沿用模板；提出设计变化时由既有结算核算实际耗灵、负担和研习时间。品级、适用阶段、资料与法器要求取自该模板，不从别的世界补齐。已登记技能用def_id与minutes继续学习，未完成不能施展；登记不等于取得物品，实物仍须有实际来源。"

}

// xiuxianSkillTemplateRefusalWords turns a template refusal into the sentence a
// player is shown. The reason names the real cause rather than only saying that
// nothing happened, which §7.2 requires when the rules refuse an unsupported
// ability.
func xiuxianSkillTemplateRefusalWords(err error) string {
	switch {
	case errors.Is(err, cultivation.ErrSkillTemplateUnknown):
		return "这个能力目前不能作为正式可用功法：没有对应的已批准模板。"
	case errors.Is(err, cultivation.ErrSkillTemplateGradeMismatch):
		return "品级与稀有度和该模板公布的不一致；不能改标签来获得更高数值。"
	case errors.Is(err, cultivation.ErrSkillGeneratedMissingName):
		return "新造功法需要名称。"
	case errors.Is(err, cultivation.ErrSkillGeneratedInvalidName):
		return "功法名称不能用作稳定标识（过长或含控制字符）。"
	case errors.Is(err, cultivation.ErrSkillGeneratedIDConflict):
		return "该名称会占用一个已公布的功法定义，不能作为新造功法。"
	}
	return err.Error()
}

// xiuxianSkillRarityFromWording and xiuxianSkillQualityFromWording read a grade
// back from the story's own wording, accepting the rules' English key too so a
// story that wrote either form is understood.
func xiuxianSkillQualityFromWording(text string) (cultivation.TechniqueQuality, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", true
	}
	for quality, wording := range xiuxianSkillQualityWording {
		if text == wording || text == string(quality) {
			return quality, true
		}
	}
	return "", false
}

func xiuxianSkillRarityFromWording(text string) (cultivation.ItemRarity, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", true
	}
	for rarity, wording := range xiuxianSkillRarityWording {
		if text == wording || text == string(rarity) {
			return rarity, true
		}
	}
	return "", false
}

// storySkillRegistry is the registry a story's turns learn from, built from the
// rules' own published table alone.
//
// It is used where no story state is in scope — the turn module's own shape checks
// and the panel's static catalogue. A reader that has the story's state must use
// storySkillRegistryFromState instead, or a technique the story generated would be
// invisible to it.
func storySkillRegistry(cultivationRules *cultivation.Rules) *cultivation.SkillRegistry {
	return cultivation.NewSkillRegistryWithBuiltins(cultivationRules)
}

// storySkillRegistryFromState resolves the story's registry from its own record.
//
// §6 binds a generated definition to the world pack, rule version, story and
// branch, and requires that a branch inherit only what was registered before the
// fork. Resolving from the branch's journaled record is what makes that
// true: there is no process-global registry a second story could see, and a fork
// simply carries the record it inherited.
//
// A record that no longer validates is skipped rather than repaired. §6 forbids
// silently rewriting what a character knows, so a definition whose numbers cannot
// be re-derived leaves its learned entries unresolvable, and the cast path then
// refuses them honestly instead of casting something else.
func storySkillRegistryFromState(cultivationRules *cultivation.Rules, state map[string]any) *cultivation.SkillRegistry {
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	return storySkillRegistryFromRecords(cultivationRules,
		records)
}

func storySkillRegistryFromRecords(cultivationRules *cultivation.Rules, records map[string]any) *cultivation.SkillRegistry {
	return cultivation.NewSkillRegistryWithBuiltins(cultivationRules).WithLookup(func(id string) (cultivation.SkillDef, bool) {
		record, _ := records[id].(map[string]any)
		def, ok := generatedSkillFromRecord(record, cultivationRules)
		return def, ok && def.ID == id
	}, func() []string { return generatedRecordIDs(records, "") })
}

// generatedSkillFromRecord rebuilds one generated definition from its record.
//
// The record names a template and the identity, and every number is taken from the
// template and any checked design proposal again. Native budgets validate
// the immutable proposal on replay; the actor never writes arbitrary deltas.
func generatedSkillFromRecord(record map[string]any, profiles ...*cultivation.Rules) (cultivation.SkillDef, bool) {
	templateID := strings.TrimSpace(xiuxianEntryText(record, "模板"))
	name := strings.TrimSpace(xiuxianEntryText(record, "名称"))
	if templateID == "" || name == "" || xiuxianEntryText(record, "类别") != "" {
		return cultivation.SkillDef{}, false
	}
	template, ok := cultivation.SkillTemplateByID(templateID, profiles...)
	if !ok {
		return cultivation.SkillDef{}, false
	}
	definition, err := template.Instantiate(name)
	if record["技能设计"] != nil {
		var design GeneratedSkillDesign
		if decodeGeneratedDesign(record, "技能设计", &design) != nil {
			return cultivation.SkillDef{}, false
		}
		definition, err = deriveGeneratedSkill(definition, design)
	}
	if err != nil {
		return cultivation.SkillDef{}, false
	}
	// The recorded identity must be the one the template derives, or the learned
	// entries pointing at it would resolve to a different technique.
	if recorded := strings.TrimSpace(xiuxianEntryText(record, "定义")); recorded != "" && recorded != definition.ID {
		return cultivation.SkillDef{}, false
	}
	if raw, ok := actorStateNumber(record["版本"]); ok && int(raw) > 0 {
		definition.Version = int(raw)
	}
	return definition, true
}

// xiuxianGeneratedSkillEntry adds one newly registered definition to the story's
// registry record and returns the record to write.
//
// The write is its own Actor State field on the story-context actor, with the
// learning's source kind, so the reroll path drops a generated definition together
// with the learning that produced it rather than leaving an orphan registration.
//
// The whole record is passed in and a copy returned rather than reading the state
// again, because two techniques generated in one turn must both survive: a second
// snapshot taken from the pre-turn state would overwrite the first.
func xiuxianGeneratedSkillEntry(
	record map[string]any,
	definition cultivation.SkillDef,
	template cultivation.SkillTemplate,
	source string,
	description string,
	design *GeneratedSkillDesign,
) (map[string]any, bool) {
	// A definition already registered at this version is not rewritten: §6 keeps
	// the old revision in force rather than silently replacing it, and a replayed
	// commit must not duplicate the entry.
	if existing, ok := record[definition.ID].(map[string]any); ok {
		if raw, ok := actorStateNumber(existing["版本"]); ok && int(raw) >= definition.Version {
			return nil, false
		}
	}
	next := make(map[string]any, len(record)+1)
	for key, value := range record {
		next[key] = value
	}
	entry := map[string]any{
		"定义":  definition.ID,
		"版本":  float64(definition.Version),
		"模板":  template.Definition.ID,
		"名称":  definition.Name,
		"品级":  xiuxianSkillQualityWording[template.Quality],
		"稀有度": xiuxianSkillRarityWording[template.Rarity],
	}
	if strings.TrimSpace(source) != "" {
		entry["来源"] = strings.TrimSpace(source)
	}
	if strings.TrimSpace(description) != "" {
		entry["描述"] = strings.TrimSpace(description)
	}
	if design != nil {
		entry["技能设计"] = generatedDesignRecord(design)
	}
	next[definition.ID] = entry
	return next, true
}

// xiuxianLearnedSkillEntry records one art in the character's own learned set.
//
// The definition and the version are what the rules cast from; the description
// and the source are journal material that nothing prices. They are written here
// rather than on the registry because two characters may learn the same art from
// different teachers, and each should read its own account of it.
func xiuxianLearnedSkillEntry(
	definition cultivation.SkillDef,
	description string,
	source string,
) map[string]any {
	entry := map[string]any{
		"定义":   definition.ID,
		"名称":   definition.Name,
		"版本":   float64(definition.Version),
		"掌握程度": "已学会",
	}
	if text := strings.TrimSpace(description); text != "" {
		entry["描述"] = text
	}
	if text := strings.TrimSpace(source); text != "" {
		entry["来历"] = text
	}
	return entry
}

// learnedSkillsFromState reads a character's learned set out of their own state.
//
// An entry whose definition or version the registry does not hold is skipped
// rather than substituted with the current definition. §6 forbids silently
// rewriting what a character knows, so a branch that diverged before a
// registration keeps the honest answer — an entry the rules cannot resolve — and
// the play-time cast path then refuses it rather than casting the wrong numbers.
func learnedSkillsFromState(state map[string]any, actorID string, registry *cultivation.SkillRegistry) *cultivation.LearnedSkills {
	learned := cultivation.NewLearnedSkills()
	entry, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldLearnedSkills)
	if !ok {
		return learned
	}
	// The record is keyed by technique name for the player to read, so the order
	// is sorted to keep a replay's learned list stable across map iteration.
	for _, key := range sortedKeys(entry) {
		record, ok := entry[key].(map[string]any)
		if !ok {
			continue
		}
		defID := strings.TrimSpace(xiuxianEntryText(record, "定义"))
		if defID == "" {
			continue
		}
		version := 0
		if raw, ok := actorStateNumber(record["版本"]); ok {
			version = int(raw)
		}
		if version <= 0 {
			// An entry that names a definition but no version is read at the
			// registry's current version. The field's own instruction asks for a
			// version, so this is the recovery path for a story that wrote only
			// the name, not a second way to learn.
			current, found := registry.Current(defID)
			if !found {
				continue
			}
			version = current.Version
		}
		if skillRecordMastered(record) {
			_ = learned.Learn(registry, defID, version)
		}
	}
	return learned
}

// xiuxianSkillLearningOps applies one turn's learning and returns the writes that
// record it.
//
// The sequence is the design's: validate the definition, register it, and only
// then grant mastery. A definition refused by any step grants nothing, and the
// refusal is reported rather than swallowed, so a player is told why a technique
// did not take.
//
// The registry here is built fresh from the rules' published table. That is
// deliberate for this version: the design requires registration to be a real step
// with validation in front of it, and building the registry from the table means
// the validation is exercised on every learning rather than being skipped because
// the definitions were trusted at start-up.
func xiuxianSkillLearningOps(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	actorID string,
	input TurnSkillLearningInput,
	turnID string,
) ([]ActorStateOp, SkillLearningResult, bool) {
	result := SkillLearningResult{ActorID: normalizeStatePanelActorID(actorID)}
	if result.ActorID == "" || !ActorStateSystemIsXiuxian(system) {
		return nil, result, false
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, result.ActorID)
	if !found {
		return nil, result, false
	}
	template := actorStateTemplateByID(system, templateID)
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return nil, result, false
	}
	if !actorStateTemplateDeclaresField(template, xiuxianFieldLearnedSkills) {
		// A template without the field cannot record what was learned, and
		// inventing the field would put a value in a schema that never declared it.
		return nil, result, false
	}
	if input.Empty() {
		return nil, result, true
	}

	registry := storySkillRegistryFromState(system.CultivationRules,
		state)
	learned := learnedSkillsFromState(state, result.ActorID, registry)

	record, _ := xiuxianEquipmentEntry(state, result.ActorID, xiuxianFieldLearnedSkills)
	next := make(map[string]any, len(record)+len(input.Skills))
	for key, value := range record {
		next[key] = value
	}
	registryRecord, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	registryRecord = maps.Clone(registryRecord)
	if registryRecord == nil {
		registryRecord = map[string]any{}
	}
	changed := false
	definitionsChanged := false
	for _, request := range input.Skills {
		if request.SkillDesign != nil && (request.TemplateID == "" || request.CraftDesign != nil) {
			result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.DefID, Reason: "技能设计只用于新战斗技能，不能与制作设计混用。"})
			continue
		}
		defID := strings.TrimSpace(request.DefID)
		var generatedRecord map[string]any
		var generatedProduct map[string]any
		if request.CraftDesign != nil && (request.TemplateID == "" || request.DefID != "") {
			result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.DefID, Reason: "制作设计必须与新的制作方法模板一起提交。"})
			continue
		}
		if _, found := storyCraftRegistry(system).Method(strings.TrimSpace(request.TemplateID)); found && request.DefID == "" {
			if request.SkillDesign != nil {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.TemplateID, Reason: "制作方法不能附带战斗技能设计。"})
				continue
			}
			id, err := generatedContentID("craft", strings.TrimSpace(request.TemplateID), request.Name)
			generatedRecord = generatedContentRecord("craft", id, strings.TrimSpace(request.TemplateID), request.Name, request.Source, request.Description)
			if request.CraftDesign != nil {
				generatedRecord["制作设计"] = generatedDesignRecord(request.CraftDesign)
				base, _ := storyCraftRegistry(system).Method(strings.TrimSpace(request.TemplateID))
				_, product, designErr := deriveGeneratedCraftDesign(system, base, generatedRecord, *request.CraftDesign)
				if designErr != nil {
					result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.TemplateID, Reason: "新制作设计不满足产物效果、境界、材料或工时预算。"})
					continue
				}
				generatedProduct = product
				productID := xiuxianEntryText(product, "定义")
				if existing := registryRecord[productID]; existing != nil && !reflect.DeepEqual(existing, product) {
					result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.TemplateID, Reason: "同名产物已登记，不能用新设计改写原有物品。"})
					continue
				}
			}
			if existing := registryRecord[id]; existing != nil && !reflect.DeepEqual(existing, generatedRecord) {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.TemplateID, Reason: "同名方法已登记，不能用新设计改写原有方法。"})
				continue
			}
			if _, valid := generatedCraftFromRecord(system, generatedRecord); err != nil || !valid || !generatedRegistryCanAdd(registryRecord, generatedRecord, generatedProduct) {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.TemplateID, Reason: "新制作方法需要已登记模板、名称与实际传授来历，且本局定义目录尚有容量。"})
				continue
			}
			defID = id
		}
		method, found := storyCraftRegistryFromState(system, state).Method(defID)
		if generatedRecord != nil {
			method, found = generatedCraftFromRecord(system, generatedRecord)
		}
		if found {
			learningRequest := request
			if generatedRecord != nil {
				learningRequest.TemplateID = ""
				learningRequest.Name = ""
			}

			if refusal := craftLearningRefusal(state, result.ActorID, method, learningRequest); refusal != "" {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: defID, Reason: refusal})
				continue
			}
			if !generatedRegistryCanAdd(registryRecord, generatedRecord, generatedProduct) {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: defID, Reason: "本局新定义目录已满，已有内容保留。"})
				continue
			}
			if !craftMethodKnown(state, result.ActorID, method) {
				next[method.ID] = map[string]any{"定义": method.ID, "版本": float64(method.Version), "掌握程度": "已学会", "来历": strings.TrimSpace(request.Source), "描述": strings.TrimSpace(request.Description)}
				if strings.HasPrefix(method.ID, "gen.craft.") {
					next[method.ID].(map[string]any)["名称"] = method.NameKey
				}
				result.CraftMethods = append(result.CraftMethods, method)
				if generatedRecord != nil && registryRecord[method.ID] == nil {
					registryRecord[method.ID] = generatedRecord
					if generatedProduct != nil {
						registryRecord[xiuxianEntryText(generatedProduct, "定义")] = generatedProduct
					}
					definitionsChanged = true
				}
				changed = true
			}
			continue
		}
		if request.CraftDesign != nil {
			result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.TemplateID, Reason: "制作设计只可用于已登记的制作方法模板。"})
			continue
		}
		if request.TemplateID != "" && registryRecord[cultivation.GeneratedSkillID(request.TemplateID, request.Name)] == nil && len(registryRecord) >= generatedDefinitionLimit {
			result.Refused = append(result.Refused, SkillLearningRefusal{DefID: request.TemplateID, Reason: "本局新定义目录已满，已有内容保留。"})
			continue
		}
		// Each proposal starts from accepted records only. A refused local
		// registration must not let a later same-ID request bypass archive checks.
		registry = storySkillRegistryFromRecords(system.CultivationRules,
			registryRecord)
		definition, template, generated, refusal := resolveSkillLearningRequest(registry, request, system.CultivationRules)
		if refusal != "" {
			refusedID := defID
			if refusedID == "" {
				refusedID = strings.TrimSpace(request.TemplateID)
			}
			result.Refused = append(result.Refused, SkillLearningRefusal{DefID: refusedID, Reason: refusal})
			continue
		}
		var pendingRegistry map[string]any
		if generated {
			if merged, added := xiuxianGeneratedSkillEntry(registryRecord, definition, template, request.Source, request.Description, request.SkillDesign); added {
				candidate, _ := merged[definition.ID].(map[string]any)
				if !generatedRegistryCanAdd(registryRecord, candidate) {
					result.Refused = append(result.Refused, SkillLearningRefusal{DefID: definition.ID, Reason: "本局生成档案达到安全容量；已有定义仍可使用。"})
					continue
				}
				pendingRegistry = merged
			}
		}
		// Granting is the third fact and only runs after the first two hold. A
		// character who already knows this version or a newer one is a no-op, so a
		// replayed commit does not duplicate the entry.
		if err := skillLearnableBy(definition, state, result.ActorID); err != nil {
			result.Refused = append(result.Refused, SkillLearningRefusal{DefID: definition.ID, Reason: err.Error()})
			continue
		}
		// The wording is the art's first impression and is written once. A later
		// version of the same art, or a replayed commit, keeps the sentence the
		// player already read rather than restyling it under them.
		description := request.Description
		source := request.Source
		key, previous := skillLearningRecord(next, definition.ID)
		if previous != nil {
			if recorded := xiuxianEntryText(previous, "描述"); recorded != "" {
				description = recorded
			}
			if recorded := xiuxianEntryText(previous, "来历"); recorded != "" {
				source = recorded
			}
		}
		entry := xiuxianLearnedSkillEntry(definition, description, source)
		mastered := learned.Knows(definition.ID) || skillRecordMastered(previous)
		minutes := skillRecordStudyMinutes(previous)
		if !mastered {
			if request.Minutes > 0 && !cultivationManualHeld(state, result.ActorID, definition.RequiredManualID) {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: definition.ID, Reason: "尚未持有这门传承的完整实物注本；登记或口述来历不授予学习资格。"})
				continue
			}
			if request.Minutes > 0 && strings.TrimSpace(source) == "" {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: definition.ID, Reason: "学习需要完整可读资料或有效传授的实际来历。"})
				continue
			}
			required := skillStudyMinutes(definition, state, result.ActorID, system.CultivationRules)
			if required <= 0 {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: definition.ID, Reason: "这个技能没有可用的学习参数。"})
				continue
			}
			minutes = min(required, minutes+request.Minutes)
			mastered = minutes >= required
			entry["掌握程度"] = "修习中"
			if mastered {
				entry["掌握程度"] = "已学会"
			}
			result.Studied = append(result.Studied, TechniqueStudy{DefID: definition.ID, Progress: minutes / required, Mastered: mastered})
		}
		if minutes > 0 {
			entry["已修习分钟"] = minutes
		}
		if mastered {
			if err := learned.Learn(registry, definition.ID, definition.Version); err != nil {
				result.Refused = append(result.Refused, SkillLearningRefusal{DefID: definition.ID, Reason: err.Error()})
				continue
			}
			result.Learned = append(result.Learned, definition)
		}
		next[key] = entry
		// A generated definition is registered in the story's own record only once
		// the character may really hold it, so a refused technique leaves no
		// orphan definition behind.
		if pendingRegistry != nil {
			registryRecord = pendingRegistry
			definitionsChanged = true
		}
		changed = true
	}
	if !changed {
		return nil, result, true
	}

	ops := make([]ActorStateOp, 0, 2)
	// Definitions are written before the learned entries that reference them, so a
	// reader that applies the operations in order never sees a learned record
	// pointing at a definition the journal does not yet hold.
	if definitionsChanged {
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      DefaultStoryContextActorID,
			FieldID:      normalizeActorStateFieldName(xiuxianFieldSkillRegistry),
			Value:        registryRecord,
			Reason:       "由技能登记结算写入，不由叙述改写。",
			SourceTurnID: turnID,
			SourceKind:   StateOpSourceSkillLearning,
			SourceID:     "skill_definitions",
		})
	}
	ops = append(ops, ActorStateOp{
		Op:           "set",
		ActorID:      result.ActorID,
		FieldID:      normalizeActorStateFieldName(xiuxianFieldLearnedSkills),
		Value:        next,
		Reason:       "由技能登记与掌握结算写入，不由叙述改写。",
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceSkillLearning,
		SourceID:     "skills",
	})
	return ops, result, true
}

// resolveSkillLearningRequest turns one request into the definition to grant,
// the template it was built from (if any), and a refusal when it cannot be granted.
//
// A request naming DefID resolves an existing definition. A request naming a
// template builds one, after checking the template is approved and that the grade
// the caller stated is the one it publishes. §6 permits exactly that much and no
// more: the model supplies a name, a presentation and a legal source, and every
// number comes from the template.
func resolveSkillLearningRequest(
	registry *cultivation.SkillRegistry,
	request TurnSkillLearning,
	profiles ...*cultivation.Rules,
) (cultivation.SkillDef, cultivation.SkillTemplate, bool, string) {
	defID := strings.TrimSpace(request.DefID)
	templateID := strings.TrimSpace(request.TemplateID)
	switch {
	case defID != "" && templateID != "":
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false,
			"同一项不能同时给出 def_id 与 template_id：已公布的功法定义与新造功法是两件事"
	case templateID != "":
		return generateSkillLearning(registry, request, profiles...)
	}
	if defID == cultivation.OrdinaryAttackSkillID {
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false, "普通攻击无需学习，不是可登记的已学功法"
	}
	definition, refusal := registerAndValidateSkill(registry, defID, request.Version)
	return definition, cultivation.SkillTemplate{}, false, refusal
}

// generateSkillLearning builds and registers one technique from an approved
// template.
func generateSkillLearning(
	registry *cultivation.SkillRegistry,
	request TurnSkillLearning,
	profiles ...*cultivation.Rules,
) (cultivation.SkillDef, cultivation.SkillTemplate, bool, string) {
	template, ok := cultivation.SkillTemplateByID(request.TemplateID, profiles...)
	if !ok {
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false,
			xiuxianSkillTemplateRefusalWords(cultivation.ErrSkillTemplateUnknown)
	}
	quality, ok := xiuxianSkillQualityFromWording(request.Quality)
	if !ok {
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false,
			xiuxianSkillTemplateRefusalWords(cultivation.ErrSkillTemplateGradeMismatch)
	}
	rarity, ok := xiuxianSkillRarityFromWording(request.Rarity)
	if !ok {
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false,
			xiuxianSkillTemplateRefusalWords(cultivation.ErrSkillTemplateGradeMismatch)
	}
	if !template.SupportsGrade(quality, rarity) {
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false,
			xiuxianSkillTemplateRefusalWords(cultivation.ErrSkillTemplateGradeMismatch)
	}
	definition, err := template.Instantiate(request.Name)
	if err == nil && request.SkillDesign != nil {
		definition, err = deriveGeneratedSkill(definition, *request.SkillDesign)
	}
	if err != nil {
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false, xiuxianSkillTemplateRefusalWords(err)
	}
	// A technique this story already generated under the same name is the same
	// definition, not a new revision. §6 keeps the registered revision in force, so
	// re-proposing it resolves to what the story already holds instead of failing as
	// a non-advancing version.
	if existing, registered := registry.Current(definition.ID); registered {
		if request.SkillDesign != nil && !reflect.DeepEqual(existing, definition) {
			return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false, "同名技能已有固定效果，不能用新设计改写。"
		}
		return existing, template, false, ""
	}
	// The definition faces registration before it can be learned. A template the
	// rules refuse — because its numbers no longer validate — is reported as the
	// unsupported ability it is rather than as a character's failure.
	if err := registry.Register(definition); err != nil {
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false, err.Error()
	}
	registered, ok := registry.Version(definition.ID, definition.Version)
	if !ok {
		return cultivation.SkillDef{}, cultivation.SkillTemplate{}, false,
			cultivation.ErrSkillNotRegistered.Error()
	}
	return registered, template, true, ""
}

// registerAndValidateSkill resolves one requested technique to a definition the
// registry holds, running the design's validate-then-register order.
//
// A requested version of zero means the registry's current version. A requested
// version the registry does not hold is refused with the reason named, rather
// than being substituted with the current definition: §6 forbids silently
// rewriting what a character learns.
func registerAndValidateSkill(registry *cultivation.SkillRegistry, defID string, version int) (cultivation.SkillDef, string) {
	if defID == "" {
		return cultivation.SkillDef{}, "definition identifier is empty"
	}
	if version > 0 {
		definition, ok := registry.Version(defID, version)
		if !ok {
			if _, registered := registry.Current(defID); registered {
				return cultivation.SkillDef{}, "version " + strconv.Itoa(version) + " of " + defID + " is not registered"
			}
			return cultivation.SkillDef{}, defID + " is not a registered technique definition"
		}
		// A registered definition is already validated — the registry refuses an
		// out-of-budget one at registration — but the check is repeated here so a
		// registry seeded from an untrusted source cannot smuggle one through.
		if err := cultivation.ValidateSkillDefinition(definition); err != nil {
			return cultivation.SkillDef{}, err.Error()
		}
		return definition, ""
	}
	definition, ok := registry.Current(defID)
	if !ok {
		return cultivation.SkillDef{}, defID + " is not a registered technique definition"
	}
	if err := cultivation.ValidateSkillDefinition(definition); err != nil {
		return cultivation.SkillDef{}, err.Error()
	}
	return definition, ""
}

// skillLearnableBy reports whether the character may hold this technique at all.
//
// A mortal is refused every technique, including a zero-cost one: the design
// requires that zero mana not become free casting merely because a cost formula
// rounds to zero, and the same reasoning refuses a mortal a technique they could
// never perform. The check reads the character's stored stage, so a mortal who
// has not yet become a cultivator cannot be handed a technique by the narrative.
//
// CanCast checks only stable stage eligibility, never current mana, burden or
// held equipment. The ordinary swing is excluded separately: its casting
// exemption for mortals must not make it a learnable technique.
func skillLearnableBy(definition cultivation.SkillDef, state map[string]any, actorID string) error {
	stageKey := xiuxianStoredStageKey(state, actorID)
	cultivationStage, ok := cultivation.ParseStageKey(stageKey)
	if !ok {
		return &cultivation.UnknownStageKeyError{Key: stageKey}
	}
	if !definition.CanCast(cultivationStage) || definition.ID == "" || cultivation.IsOrdinaryAttack(definition) {
		return cultivation.ErrSkillNotLearnable
	}
	return nil
}

// removeSkillLearningActorOps drops a previous learning's writes.
//
// It mirrors the settlement's and the advance's removers so the reroll path can
// discard a learning along with the rule outcome it was derived from.
func removeSkillLearningActorOps(ops []ActorStateOp) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceSkillLearning {
			continue
		}
		out = append(out, op)
	}
	return normalizeActorStateOps(out)
}

// sortedKeys returns a map's keys in ascending order.
//
// Every read of a player-authored record goes through it so a replay produces the
// same learned list, and therefore the same settled numbers, every time rather
// than following Go's map iteration order.
func sortedKeys(entry map[string]any) []string {
	keys := make([]string, 0, len(entry))
	for key := range entry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
