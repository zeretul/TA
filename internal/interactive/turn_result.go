package interactive

import (
	interactivestate "ta/internal/interactive/state"
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// TurnResult is the complete hidden result produced by the Game Agent. The
// backend compiles StateUpdates into replayable StateDelta operations.
type TurnResult struct {
	StateUpdates []interactivestate.Update `json:"state_updates"`
	Choices      []string                  `json:"choices"`
	// PlanUpdate is committed as a private branch event, never inside the Turn.
	PlanUpdate *string `json:"-"`
	// CharacterFacts is committed as a character side record in the same
	// transaction as the Turn. It is the same shape the character window writes,
	// which is what makes a line spoken in the story and a line spoken in the
	// window the same record to the next turn.
	CharacterFacts *TurnCharacterFactsInput `json:"-"`
	// Cultivation is the turn's authorised cultivation interval and any
	// authorised breakthrough. It is committed as rules-owned Actor State writes
	// in the same transaction as the Turn, so a turn that advances a character's
	// cultivation moves the stored stage rather than only describing it.
	Cultivation *TurnCultivationInput `json:"-"`
	// Skills is the turn's newly learned techniques. It is committed as
	// rules-owned Actor State writes in the same transaction as the Turn, so a
	// character who is taught a technique in the prose gains it as a fact rather
	// than only as a claim.
	Skills *TurnSkillLearningInput `json:"-"`
	// Techniques is the turn's method module: what was studied, what completed,
	// and which methods started or stopped circulating. It is committed as
	// rules-owned Actor State writes in the same transaction as the Turn, so a
	// method the prose says was learned only becomes real when the elapsed study
	// time actually completed it.
	Techniques *TurnTechniqueInput `json:"-"`
	// Items is the turn's item module: what was obtained, what was taken into
	// hand and what was put down. It is committed as rules-owned Actor State
	// writes in the same transaction as the Turn, so an item the prose says was
	// picked up becomes a real copy in the pack rather than a claim.
	Items *TurnItemsInput `json:"-"`
	// Trade is the turn's trade module: what the character really bought and
	// really sold. It is committed as rules-owned Actor State writes in the same
	// transaction as the Turn, so a price the prose quoted becomes a real
	// deduction and a real transfer of goods rather than a claim.
	Trade *TurnTradeInput `json:"-"`
	// Rest is the turn's rest or regulation interval. It is committed as
	// rules-owned Actor State writes in the same transaction as the Turn, so a
	// turn whose prose says the character rested a day really restores the pool
	// and really spends the day rather than only claiming both.
	Rest *TurnRestInput `json:"-"`
	// Craft is the turn's craft module: which registered method was worked and for
	// how long. It is committed as rules-owned Actor State writes plus a side
	// record carrying the spent time, so a turn whose prose says the character
	// brewed a pill really spends the herbs, really advances the clock and really
	// puts a registered product in the pack rather than only claiming all three.
	Craft *TurnCraftInput `json:"-"`
	// World is the turn's one world action: a stretch of road, a task decision,
	// a public-event participation or a realm action. It is committed as a side
	// record in the same transaction as the Turn, so a player who writes 去坊市
	// or 接下这桩差事 really moves, really takes the task and really spends the
	// formal time rather than reading a scene that claims it.
	World *TurnWorldActionInput `json:"-"`
}

func NormalizeTurnResult(result TurnResult) TurnResult {
	result.StateUpdates = interactivestate.NormalizeUpdates(result.StateUpdates)
	result.Choices = normalizeChoiceListLimit(result.Choices, MaxStoryChoiceCount+1)
	if result.PlanUpdate != nil {
		value := normalizeBranchPlanMarkdown(*result.PlanUpdate)
		result.PlanUpdate = &value
	}
	result.CharacterFacts = cloneTurnCharacterFactsInput(result.CharacterFacts)
	result.Cultivation = cloneTurnCultivationInput(result.Cultivation)
	result.Skills = cloneTurnSkillLearningInput(result.Skills)
	result.Techniques = cloneTurnTechniqueInput(result.Techniques)
	result.Items = cloneTurnItemsInput(result.Items)
	result.Trade = cloneTurnTradeInput(result.Trade)
	result.Rest = cloneTurnRestInput(result.Rest)
	result.Craft = cloneTurnCraftInput(result.Craft)
	result.World = cloneTurnWorldActionInput(result.World)
	return result
}

func ValidateTurnResult(result TurnResult, configuredChoiceCount ...int) error {
	choiceCount := DefaultStoryChoiceCount
	if len(configuredChoiceCount) > 0 {
		choiceCount = normalizeStoryChoiceCount(configuredChoiceCount[0])
	}
	return validateTurnResult(result, choiceCount, false)
}

func validateTerminalTurnResult(result TurnResult, configuredChoiceCount int) error {
	return validateTurnResult(result, configuredChoiceCount, true)
}

func validateTurnResult(result TurnResult, configuredChoiceCount int, terminal bool) error {
	choiceCount := normalizeStoryChoiceCount(configuredChoiceCount)
	if err := validateStoryChoiceCount(choiceCount); err != nil {
		return err
	}
	for index, update := range result.StateUpdates {
		if err := interactivestate.ValidateUpdate(update); err != nil {
			return fmt.Errorf("TurnResult state_updates[%d] is invalid: %w", index, err)
		}
	}
	if result.PlanUpdate != nil {
		if err := validateBranchPlanMarkdown(*result.PlanUpdate); err != nil {
			return fmt.Errorf("TurnResult plan_update is invalid: %w", err)
		}
	}
	if terminal {
		if len(result.Choices) != 0 {
			return fmt.Errorf("TurnResult choices must be empty for an explicitly terminal turn")
		}
		return nil
	}
	if len(result.Choices) != choiceCount {
		return fmt.Errorf("TurnResult choices must contain exactly %d distinct action suggestions", choiceCount)
	}
	return nil
}

func normalizeTurnResultPointer(result *TurnResult, configuredChoiceCount int, terminal bool) *TurnResult {
	if result == nil {
		return nil
	}
	normalized := NormalizeTurnResult(*result)
	var err error
	if terminal {
		err = validateTerminalTurnResult(normalized, configuredChoiceCount)
	} else {
		err = ValidateTurnResult(normalized, configuredChoiceCount)
	}
	if err != nil {
		return nil
	}
	return &normalized
}

func normalizedChoiceKey(value string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
}
