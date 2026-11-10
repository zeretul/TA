package cultivation

import "errors"

// Errors returned when a definition is refused before registration.
//
// The design requires that an out-of-budget definition be rejected rather than
// registered and then hidden behind the global cap, so these are returned from
// validation rather than being folded into a clamped value. They are sentinel
// values so a caller can distinguish "this definition is invalid" from an
// infrastructure failure.
var (
	ErrUnsupportedGrade = errors.New("cultivation: unsupported item grade")

	ErrNegativeFormationBonus     = errors.New("cultivation: formation bonus must not be negative")
	ErrFormationTwoBonuses        = errors.New("cultivation: a first-version formation grants one main bonus")
	ErrFormationAttackOverBudget  = errors.New("cultivation: formation attack bonus exceeds its grade ceiling")
	ErrFormationDefenseOverBudget = errors.New("cultivation: formation defense bonus exceeds its grade ceiling")

	ErrSkillMissingID               = errors.New("cultivation: skill definition has no id")
	ErrSkillNegativePower           = errors.New("cultivation: skill power must not be negative")
	ErrSkillManaFractionOutOfRange  = errors.New("cultivation: skill mana fraction must be between 0 and 1")
	ErrSkillBurdenOutOfRange        = errors.New("cultivation: skill burden must be between 0 and 100")
	ErrSkillIgnoreDefenseOverCap    = errors.New("cultivation: skill defense ignore exceeds the 20% cap")
	ErrSkillNoTargets               = errors.New("cultivation: skill must reach at least one target")
	ErrSkillUnsupportedPreparation  = errors.New("cultivation: skill preparation longer than two actions is unsupported")
	ErrSkillResidualWithoutFloor    = errors.New("cultivation: a residual injury needs a burden floor")
	ErrSkillResidualWithoutRecovery = errors.New("cultivation: a residual injury needs a safe-recovery requirement")
	ErrSkillShieldOverCap           = errors.New("cultivation: skill shield exceeds the 35% single-target cap")
	ErrSkillNonFiniteValue          = errors.New("cultivation: skill carries a non-finite value")

	// Registration and learning errors. §6 keeps a definition existing, a
	// definition being registered and a character having learned it as three
	// separate facts, so each way of confusing them has its own error rather
	// than a single generic refusal.
	ErrSkillRegistryNotInitialised = errors.New("cultivation: skill registry is not initialised")
	ErrSkillVersionOutOfRange      = errors.New("cultivation: skill version must be one or greater")
	ErrSkillVersionNotAdvancing    = errors.New("cultivation: a new skill version must be higher than the registered one")
	ErrLearnedSkillsNotInitialised = errors.New("cultivation: learned-skill set is not initialised")
	ErrSkillNotRegistered          = errors.New("cultivation: skill definition is not registered")
	ErrSkillVersionNotRegistered   = errors.New("cultivation: skill version is not registered")
	ErrSkillNotLearnable           = errors.New("cultivation: this skill cannot be performed by any character")

	// Generation errors. §6 lets a model fill a name, a presentation and a legal
	// source into a template the rules already validated, and forbids it
	// everything else, so each way of leaving that envelope is named rather than
	// folded into a generic refusal.
	ErrSkillTemplateUnknown       = errors.New("cultivation: no such approved skill template")
	ErrSkillTemplateGradeMismatch = errors.New("cultivation: the grade and rarity do not match what the template publishes")
	ErrSkillGeneratedMissingName  = errors.New("cultivation: a generated technique needs a name")
	ErrSkillGeneratedInvalidName  = errors.New("cultivation: a generated technique's name cannot be used as an identifier")
	ErrSkillGeneratedIDConflict   = errors.New("cultivation: a generated technique cannot take a published definition's identifier")

	// Catalogue errors. §4.1 makes a grade and rarity ceiling a property of the
	// definition, and §7 keeps a definition separate from the instances made
	// from it, so each way a definition or an instance can be misused is named
	// separately.
	ErrItemMissingID             = errors.New("cultivation: item definition has no id")
	ErrItemPayloadMismatch       = errors.New("cultivation: item definition payload does not match its kind")
	ErrItemUnknownKind           = errors.New("cultivation: item definition has an unknown kind")
	ErrItemNegativeBonus         = errors.New("cultivation: item bonus must not be negative")
	ErrItemOverBudget            = errors.New("cultivation: item bonus exceeds its grade and rarity budget")
	ErrItemBudgetCountedTwice    = errors.New("cultivation: an item cannot spend its rarity budget on an effect and count it as a main attribute")
	ErrItemPillMissingPurpose    = errors.New("cultivation: pill definition has no purpose")
	ErrItemPillSupportIncomplete = errors.New("cultivation: a cultivation-support pill needs a published strength and duration")
	ErrItemGradeMismatch         = errors.New("cultivation: item entry and its payload disagree about the grade")
	ErrItemCatalogNotInitialised = errors.New("cultivation: item catalogue is not initialised")
	ErrItemDuplicateID           = errors.New("cultivation: item definition id is already registered")
	ErrItemDefinitionNotFound    = errors.New("cultivation: item definition is not registered")
	ErrItemQuantityOutOfRange    = errors.New("cultivation: item quantity must be positive")
	ErrItemNotConsumable         = errors.New("cultivation: only a pill is consumed by use")
	ErrItemExhausted             = errors.New("cultivation: item instance has no doses left")
	ErrItemUnknownStage          = errors.New("cultivation: item definition names a stage the rules cannot read")
	ErrItemStageTooLow           = errors.New("cultivation: the character has not reached the item's applicable stage")
	ErrItemNotWieldable          = errors.New("cultivation: only an implement, armour, formation focus or treasure is equipped")
	ErrItemInvalidPrice          = errors.New("cultivation: item base price must not be negative")
	ErrItemUnpriced              = errors.New("cultivation: item definition publishes no price")
	ErrItemUnknownAttribute      = errors.New("cultivation: item definition names an attribute the rules cannot read")

	// Technique (功法) errors. §4.1 keeps a manual existing, a method being
	// registered and a character having mastered it as three separate facts, so
	// the ways of confusing them are named separately. The compatibility and
	// replacement refusals are also distinct: one says the two models cannot run
	// together, the other says the new method is not yet an upgrade the character
	// may adopt.
	ErrTechniqueMissingID               = errors.New("cultivation: technique definition has no id")
	ErrTechniqueVersionOutOfRange       = errors.New("cultivation: technique version must be one or greater")
	ErrTechniqueUnknownQuality          = errors.New("cultivation: technique has an unknown quality grade")
	ErrTechniqueUnknownCirculation      = errors.New("cultivation: technique has an unknown circulation model")
	ErrTechniqueUnknownStage            = errors.New("cultivation: technique stage requirement is not a recognised stage")
	ErrTechniqueUnknownAttribute        = errors.New("cultivation: technique specialty names an unknown attribute")
	ErrTechniqueGrowthOutOfRange        = errors.New("cultivation: technique growth coefficient must be positive")
	ErrTechniqueLearningTimeOutOfRange  = errors.New("cultivation: technique learning time must be positive")
	ErrTechniqueQualityAttackOutOfRange = errors.New("cultivation: technique quality attack correction exceeds its grade")
	ErrTechniqueSpecialtyOverCap        = errors.New("cultivation: technique specialty exceeds the method-category ceiling")
	ErrTechniqueCostReductionOverCap    = errors.New("cultivation: technique cost or strain reduction exceeds the 20% ceiling")
	ErrTechniqueNonFiniteValue          = errors.New("cultivation: technique carries a non-finite value")
	ErrTechniqueCirculationConflict     = errors.New("cultivation: these circulation models cannot run together")
	ErrTechniqueStageTooLow             = errors.New("cultivation: character is below this technique's minimum stage")
	ErrTechniqueNotHigherGrade          = errors.New("cultivation: a replacement technique must be of a higher grade")
	ErrTechniqueNotMastered             = errors.New("cultivation: this technique has not been mastered")
	ErrTechniqueNotCirculating          = errors.New("cultivation: this technique is not currently circulating")
)
