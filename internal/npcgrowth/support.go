package npcgrowth

import (
	"math"

	"ta/internal/cultivation"
)

func prepareSupport(state Snapshot, segment Segment) (Snapshot, error) {
	items := cultivation.NewItemCatalogWithFirstVersion(state.Cultivation.Rules())
	if state.ActiveSupport != nil && state.ActiveSupport.EndsAtSeconds <= segment.From {
		state.ActiveSupport = nil
	}
	if segment.SupportUse == nil {
		return state, nil
	}
	if !supportTrainingAvailable(state, segment) {
		return Snapshot{}, diagnostic("support_without_training", "$.interval.segments.support_use", "A cultivation-support dose can only be used when the NPC has a real training window.")
	}
	if state.ActiveSupport != nil {
		return Snapshot{}, diagnostic("support_already_active", "$.interval.segments.support_use", "An active cultivation-support dose cannot be stacked or refreshed.")
	}
	use := *segment.SupportUse
	if !sourceRefValid(use.InstanceID) || !sourceRefValid(use.SourceRef) {
		return Snapshot{}, diagnostic("invalid_support_use", "$.interval.segments.support_use", "The dose use must identify its real item instance and source.")
	}
	for index, dose := range state.SupportDoses {
		if dose.InstanceID != use.InstanceID {
			continue
		}
		if !sourceRefValid(dose.SourceRef) || dose.Quantity <= 0 {
			return Snapshot{}, diagnostic("invalid_support_inventory", "$.state.support_doses", "The selected support item is not a valid held dose.")
		}
		definition, ok := items.Definition(dose.DefinitionID)
		if !ok || definition.Kind != cultivation.KindPill || definition.Pill == nil {
			return Snapshot{}, diagnostic("unsupported_support_item", "$.state.support_doses", "The selected item has no registered cultivation-support definition.")
		}
		strength, ok := definition.Pill.SupportStrengthOf()
		seconds, durationOK := supportDurationSeconds(definition.Pill)
		if !ok || !durationOK || segment.From > math.MaxInt64-seconds {
			return Snapshot{}, diagnostic("unsupported_support_definition", "$.state.support_doses", "The registered cultivation-support definition is incomplete or out of range.")
		}
		instance, err := items.NewItemInstance(dose.InstanceID, dose.DefinitionID, dose.Quantity)
		if err != nil {
			return Snapshot{}, diagnostic("invalid_support_inventory", "$.state.support_doses", "The selected support item could not be read from the NPC inventory.")
		}
		spent, err := instance.Consume(items)
		if err != nil {
			return Snapshot{}, diagnostic("support_dose_unavailable", "$.interval.segments.support_use", "The NPC does not hold a usable cultivation-support dose.")
		}
		state.SupportDoses = append([]SupportDose(nil), state.SupportDoses...)
		if spent.Quantity == 0 {
			state.SupportDoses = append(state.SupportDoses[:index], state.SupportDoses[index+1:]...)
		} else {
			state.SupportDoses[index].Quantity = spent.Quantity
		}
		state.ActiveSupport = &ActiveSupport{
			InstanceID: dose.InstanceID, DefinitionID: dose.DefinitionID, SourceRef: use.SourceRef,
			StartedAtSeconds: segment.From, EndsAtSeconds: segment.From + seconds, Strength: string(strength),
		}
		return state, nil
	}
	return Snapshot{}, diagnostic("support_dose_unavailable", "$.interval.segments.support_use", "The NPC does not hold the selected cultivation-support dose.")
}

func supportTrainingAvailable(state Snapshot, segment Segment) bool {
	if activityTenthsPerWorldSecond(segment.Activity) == 0 || atMajorBoundary(state.Cultivation, state.Accumulated) ||
		float64(segment.Conditions.CurrentHealth)/float64(segment.Conditions.MaximumHealth) < 0.30 {
		return false
	}
	_, _, supportedStage := cultivation.ProgressRequired(state.Cultivation)
	return supportedStage
}

func supportDurationSeconds(pill *cultivation.PillDef) (int64, bool) {
	if pill == nil {
		return 0, false
	}
	days := pill.SupportDuration()
	seconds := days * float64(24*60*60)
	if days <= 0 || !finite(seconds) || seconds > float64(math.MaxInt64) {
		return 0, false
	}
	rounded := math.Round(seconds)
	if math.Abs(seconds-rounded) > 1e-9 || rounded <= 0 {
		return 0, false
	}
	return int64(rounded), true
}

func supportResourceAt(state Snapshot, at int64) cultivation.SupportResource {
	if state.ActiveSupport == nil || at < state.ActiveSupport.StartedAtSeconds || at >= state.ActiveSupport.EndsAtSeconds {
		return cultivation.SupportNone
	}
	return cultivation.SupportResource(state.ActiveSupport.Strength)
}

func splitSegmentAtSupportExpiry(state Snapshot, segment Segment) []Segment {
	if state.ActiveSupport == nil || segment.From >= state.ActiveSupport.EndsAtSeconds || segment.To <= state.ActiveSupport.EndsAtSeconds {
		return []Segment{segment}
	}
	first, second := segment, segment
	first.To = state.ActiveSupport.EndsAtSeconds
	second.From = state.ActiveSupport.EndsAtSeconds
	first.SupportUse, second.SupportUse = nil, nil
	return []Segment{first, second}
}

func remainingIntentWithoutPastSupport(interval Interval, segmentIndex int, from int64, reason string) *RemainingIntent {
	continuation := interval
	continuation.Segments = append([]Segment(nil), interval.Segments...)
	for index := segmentIndex; index < len(continuation.Segments); index++ {
		if continuation.Segments[index].From < from {
			continuation.Segments[index].SupportUse = nil
		}
	}
	intent := remainingIntent(continuation, segmentIndex, from, reason)
	if intent == nil {
		return nil
	}
	return intent
}

func validateSupportState(state Snapshot) error {
	items := cultivation.NewItemCatalogWithFirstVersion(state.Cultivation.Rules())
	seen := make(map[string]bool, len(state.SupportDoses))
	for _, dose := range state.SupportDoses {
		if !sourceRefValid(dose.InstanceID) || !sourceRefValid(dose.SourceRef) || dose.Quantity <= 0 || seen[dose.InstanceID] {
			return diagnostic("invalid_support_inventory", "$.state.support_doses", "Support inventory must contain uniquely sourced positive item stacks.")
		}
		definition, ok := items.Definition(dose.DefinitionID)
		if !ok || definition.Kind != cultivation.KindPill || definition.Pill == nil {
			return diagnostic("unsupported_support_item", "$.state.support_doses", "Support inventory references an item without a registered cultivation-support definition.")
		}
		if _, ok := definition.Pill.SupportStrengthOf(); !ok {
			return diagnostic("unsupported_support_item", "$.state.support_doses", "Support inventory references an item without a supported cultivation effect.")
		}
		if _, ok := supportDurationSeconds(definition.Pill); !ok {
			return diagnostic("unsupported_support_item", "$.state.support_doses", "Support inventory references an item without a valid registered duration.")
		}
		seen[dose.InstanceID] = true
	}
	if state.ActiveSupport != nil {
		active := state.ActiveSupport
		definition, ok := items.Definition(active.DefinitionID)
		if !ok || definition.Pill == nil || !sourceRefValid(active.InstanceID) || !sourceRefValid(active.SourceRef) || active.StartedAtSeconds < 0 || active.EndsAtSeconds <= active.StartedAtSeconds {
			return diagnostic("invalid_active_support", "$.state.active_support", "Active support must cite a complete registered dose and valid effect interval.")
		}
		strength, strengthOK := definition.Pill.SupportStrengthOf()
		seconds, durationOK := supportDurationSeconds(definition.Pill)
		if !strengthOK || !durationOK || active.Strength != string(strength) || active.EndsAtSeconds-active.StartedAtSeconds != seconds {
			return diagnostic("invalid_active_support", "$.state.active_support", "Active support strength and duration must match the registered item definition.")
		}
	}
	return nil
}
