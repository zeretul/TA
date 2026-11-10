package interactive

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

const maxTurnStateUpdateValueBytes = maxInteractiveTextBytes

// TurnStateUpdateCompileOptions keeps validation inputs explicit at the
// model-to-state boundary.
type TurnStateUpdateCompileOptions struct {
	SourceTurnID             string
	RuleResolution           *RuleResolution
	RuleStateConsumptionMode string
	// RestActorID and RestPoolField name the one pool a rest module in this same
	// turn restores. A write to that pool is refused: the interval's result is
	// the rules' answer, and a model that states it would be deciding the
	// character's resources itself.
	RestActorID   string
	RestPoolField string
	// WorldOwnsSceneLocation reports that this turn's world module is the path
	// that writes 当前详细地点: a stretch of road lands the character somewhere,
	// and leaving a realm puts them back where they entered. When it is set, a
	// state_changes write to that field is refused for the same reason a rest's
	// pool write is — two writers would leave the scene claiming one place and
	// the record another.
	WorldOwnsSceneLocation bool
	// WorldSettlesMovement reports that this turn's world module really moves the
	// character: a road, or a realm action that changes which area they stand in.
	// It is what distinguishes a prose refinement of the scene from a prose move.
	// A realm entry leaves the field to the narrator — the instance records the
	// area, and the scene text is the only place the cave's own wording can live
	// — so that turn may write 当前详细地点. A turn that settles no movement may
	// not, because the place a character stands in is a world fact.
	WorldSettlesMovement  bool
	actorLifecycleIntents map[string]actorLifecycleIntent
}

// CompiledTurnStateUpdates contains canonical audit input and deterministic
// reducer operations. Reducer operations, rather than model paths, remain the
// replay source of truth.
type CompiledTurnStateUpdates struct {
	Updates  []interactivestate.Update
	Ops      []interactivestate.Op
	ActorOps []ActorStateOp
}

// StateUpdateValidationError identifies the exact operation that made the
// atomic state_updates module invalid.
type StateUpdateValidationError struct {
	Index          int
	Code           string
	Path           string
	DiagnosticPath string
	Expected       string
	Actual         string
	Cause          error
}

func (e *StateUpdateValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Code
}

// StateUpdateValidationErrors keeps independent failures from one atomic
// module together so callers can repair them in one retry. It still unwraps to
// individual errors for existing fail-fast callers using errors.As.
type StateUpdateValidationErrors struct {
	Items []*StateUpdateValidationError
}

func (e *StateUpdateValidationErrors) Error() string {
	if e == nil || len(e.Items) == 0 {
		return ""
	}
	return e.Items[0].Error()
}

func (e *StateUpdateValidationErrors) Unwrap() []error {
	if e == nil {
		return nil
	}
	errors := make([]error, 0, len(e.Items))
	for _, item := range e.Items {
		if item != nil {
			errors = append(errors, item)
		}
	}
	return errors
}

// CompileTurnStateUpdates validates the complete state_updates module against
// the frozen Actor schema and current replayed state. Any invalid operation
// rejects the whole module.
func CompileTurnStateUpdates(system StoryDirectorActorStateSystem, currentState map[string]any, updates []interactivestate.Update, options TurnStateUpdateCompileOptions) (CompiledTurnStateUpdates, error) {
	updates = interactivestate.NormalizeUpdates(updates)
	system = normalizeActorStateSystem(system)
	lifecycleIntents := options.actorLifecycleIntents
	if lifecycleIntents == nil {
		var err error
		lifecycleIntents, err = planActorLifecycleUpdates(system, currentState, updates)
		if err != nil {
			return CompiledTurnStateUpdates{}, err
		}
	}
	workingState := cloneActorStateRoot(currentState)
	compiled := CompiledTurnStateUpdates{Updates: []interactivestate.Update{}, Ops: []interactivestate.Op{}, ActorOps: []ActorStateOp{}}
	canonicalPaths := make([][]string, 0, len(updates))

	for index, update := range updates {
		deltaNormalized := false
		if update.Op == interactivestate.Delta {
			if converted, changed := normalizeTurnSubmissionFieldValue(ActorStateField{Type: "number"}, update.Value); changed {
				update.Value = converted
				deltaNormalized = true
			}
		}
		if err := interactivestate.ValidateUpdate(update); err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_state_update", update.Path, "replace, delta, create, archive, or restore with a non-null value", stateUpdateActual(update.Value), err)
		}
		segments, err := interactivestate.ParsePath(update.Path)
		if err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_state_path", update.Path, "schema-bound JSON Pointer", update.Path, err)
		}
		actorID := segments[0]
		if actorID == playerAttentionRoot {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "player_owned_state", update.Path, "玩家关注只能由玩家操作", actorID, ErrAttentionInvalid)
		}
		if actorID == "" || normalizeStatePanelActorID(actorID) != actorID {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_actor_id", update.Path, "normalized actor_id", actorID, fmt.Errorf("状态路径包含无效 actor_id: %q", actorID))
		}
		if err := validateStateUpdateValueSize(update.Value); err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "state_value_too_large", update.Path, fmt.Sprintf("at most %d JSON bytes", maxTurnStateUpdateValueBytes), stateUpdateActual(update.Value), err)
		}
		if update.Op == interactivestate.Archive || update.Op == interactivestate.Restore {
			intent := lifecycleIntents[actorID]
			canonical := []string{actorArchiveRoot, actorID}
			if conflict := interactivestate.OverlappingPath(canonicalPaths, canonical); conflict != "" {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "overlapping_state_path", update.Path, "one lifecycle operation per Actor", conflict, fmt.Errorf("同一次提交不能包含重复或相互覆盖的状态路径: %s", conflict))
			}
			stateOp := interactivestate.Op{
				Path:         actorArchiveStatePath(actorID),
				Reason:       intent.Reason,
				SourceTurnID: options.SourceTurnID,
				SourceKind:   interactivestate.SourceTurnResult,
			}
			if update.Op == interactivestate.Archive {
				stateOp.Op = "set"
				stateOp.Value = map[string]any{"reason": intent.Reason, "source_turn_id": options.SourceTurnID}
			} else {
				stateOp.Op = "unset"
			}
			applyStateOp(workingState, stateOp)
			canonicalPaths = append(canonicalPaths, canonical)
			compiled.Updates = append(compiled.Updates, interactivestate.Update{Op: update.Op, Path: interactivestate.FormatPath([]string{actorID}), Value: map[string]any{"reason": intent.Reason}})
			compiled.Ops = append(compiled.Ops, stateOp)
			continue
		}

		if update.Op == interactivestate.Create {
			if len(segments) != 1 {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_create_path", update.Path, "/<actor_id>", update.Path, fmt.Errorf("create 只能作用于 Actor 根路径"))
			}
			canonical := []string{actorID}
			if conflict := interactivestate.OverlappingPath(canonicalPaths, canonical); conflict != "" {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "overlapping_state_path", update.Path, "non-overlapping paths", conflict, fmt.Errorf("同一次提交不能包含重复或相互覆盖的状态路径: %s", conflict))
			}
			patch, err := actorPatchFromCreateUpdate(actorID, update.Value)
			if err != nil {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_actor_create", update.Path, "actor create object", stateUpdateActual(update.Value), err)
			}
			if system.AdventureRules != nil {
				for field := range patch.State {
					if AdventureOwnsField(field) {
						return CompiledTurnStateUpdates{}, stateUpdateError(index, "adventure_owned_state_update", update.Path, "人物历练初态来自正式目录", field, fmt.Errorf("adventure state cannot be supplied by actor creation"))
					}
				}
			}
			configuredInitialActor := actorStateInitialActorIndex(system.InitialActors, actorID) >= 0
			if !configuredInitialActor && (patch.ActorName == "" || normalizeStatePanelActorID(patch.ActorName) != actorID) {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "actor_name_id_mismatch", update.Path, "actor_id identical to name", patch.ActorName, fmt.Errorf("新建 Actor 的 actor_id 必须与 name 完全相同，并直接使用故事语言中的角色名称: actor_id=%q name=%q", actorID, patch.ActorName))
			}
			if !configuredInitialActor {
				patch.ActorName = actorID
			}
			if template := actorStateTemplateByID(system, patch.TemplateID); template.ID != "" {
				patch.State = normalizeTurnSubmissionActorStateValues(actorID, template, patch.State)
				if validationErrors := validateTurnSubmissionActorInitialState(index, update.Path, template, patch.State, system.CultivationRules); len(validationErrors) > 0 {
					return CompiledTurnStateUpdates{}, &StateUpdateValidationErrors{Items: validationErrors}
				}
			}
			// The audit value has to record the submission, not the compiler's own
			// expansion of it. buildNewActorStateOps materializes a template default
			// for every field that has one, and each cultivation attribute carries a
			// default of 0, so a create whose initial_state named only the realm would
			// otherwise be audited as if the model had written 攻击/防御/身法/神识/
			// 生命上限/灵力上限 itself. The commit re-compiles this value, which made
			// the second pass refuse the first pass's own output. The reducer
			// operations below still carry the full expansion; only the audit input,
			// which the model's next attempt is compared against, is narrowed.
			submittedState := cloneActorStateRoot(patch.State)
			patch.SourceTurnID = options.SourceTurnID
			normalized, ops, actorOps, _, _, err := validateActorStatePatch(system, workingState, patch)
			if err != nil {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "actor_create_invalid", update.Path, "valid template and state", stateUpdateActual(update.Value), err)
			}
			for _, op := range ops {
				applyStateOp(workingState, op)
			}
			for _, op := range actorOps {
				applyActorStateOp(workingState, op)
			}
			canonicalPaths = append(canonicalPaths, canonical)
			auditValue := actorCreateAuditValue(normalized)
			if len(submittedState) == 0 {
				delete(auditValue, "state")
			} else {
				auditValue["state"] = submittedState
			}
			compiled.Updates = append(compiled.Updates, interactivestate.Update{Op: interactivestate.Create, Path: interactivestate.FormatPath(canonical), Value: auditValue})
			compiled.Ops = append(compiled.Ops, ops...)
			compiled.ActorOps = append(compiled.ActorOps, actorOps...)
			continue
		}

		if len(segments) < 2 {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "missing_state_field", update.Path, "/<actor_id>/<field_id>", update.Path, fmt.Errorf("replace 和 delta 必须指定状态字段"))
		}
		templateID, found := actorTemplateIDFromStateOrSystem(workingState, system, actorID)
		if !found {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "actor_not_found", update.Path, "existing actor_id", actorID, fmt.Errorf("状态路径中的 Actor 不存在: %s", actorID))
		}
		template := actorStateTemplateByID(system, templateID)
		field, found := actorStateFieldByID(template, segments[1])
		if !found {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "state_field_not_found", update.Path, strings.Join(turnSubmissionAllowedFields(template), ", "), segments[1], fmt.Errorf("Actor 状态字段不在模板中: actor=%s field=%s", actorID, segments[1]))
		}
		fieldID := actorStateFieldID(field)
		if deltaNormalized {
			slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-turn-submission] normalized lossless delta actor_id=%q field_id=%q from=string to=number location=internal/interactive/turn_state_updates.go", actorID, fieldID))
		}
		canonical := append([]string{actorID, fieldID}, segments[2:]...)
		if conflict := interactivestate.OverlappingPath(canonicalPaths, canonical); conflict != "" {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "overlapping_state_path", update.Path, "non-overlapping paths", conflict, fmt.Errorf("同一次提交不能包含重复或相互覆盖的状态路径: %s", conflict))
		}
		if stateUpdateConflictsWithRuleResolution(options, actorID, fieldID) {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "duplicate_rule_state_update", update.Path, "a field not consumed by RuleResolution", fieldID, fmt.Errorf("该字段已由本轮 RuleResolution 自动消费，不能在 state_updates 中重复修改"))
		}
		if stateUpdateConflictsWithWorldModule(options, actorID, fieldID) {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "world_owned_state_update", update.Path, "a scene field not written by this turn's world action", fieldID, fmt.Errorf("该字段由本轮行路或秘境结算写入，不能由叙述改写: %s", fieldID))
		}
		if stateUpdateConflictsWithRestModule(options, actorID, fieldID) {
			// The rest module is the one path that restores a pool this turn, and
			// how much it restores is the rules' answer. Accepting the model's own
			// write beside it would add a second, arbitrary number: the observed
			// failure was a day's rest that the rules priced at +5 while the model
			// also wrote 150, and the save landed on the model's number plus the
			// rules'. The other pool stays writable, so a wound the prose describes
			// is still recordable.
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "rest_owned_state_update", update.Path, "a pool not restored by this turn's rest module", fieldID, fmt.Errorf("该字段由本轮休养结算写入，不能由叙述改写: %s", fieldID))
		}
		if system.AdventureRules != nil && (actorID == DefaultActorID || getPath(currentState, actorStateFieldPath(actorID, AdventureStateField)) != nil) && AdventureOwnsField(fieldID) {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "adventure_owned_state_update", update.Path, "world.adventure", fieldID, fmt.Errorf("该字段由题材规则结算，请提交world.adventure"))
		}
		if IsXiuxianSettledField(system, fieldID) {
			// The cultivation settlement owns this field. Refusing the write is
			// what makes the design's "the model may not reword the character's
			// cultivation" hold structurally rather than by instruction: the model
			// can describe a breakthrough in prose, but the stage key and the
			// numbers derived from it only move through the rules.
			//
			// A character's first materialisation is exempt, and the exemption is
			// the create branch above rather than a check here: where someone
			// stands when they enter the story is a story fact, and changing it
			// afterwards is the rules' job. The two current pools are deliberately
			// not in this set, because a wound or an expenditure is a real fact the
			// narrative does record and the settlement's job there is to clamp.
			//
			// Deliberately narrow: this batch implements the rules paths for an
			// authorised training interval and an authorised breakthrough. A
			// story-driven loss of cultivation — a fall in realm, a seal — has no
			// rules path yet, so such an event cannot be committed through
			// state_changes. That is a known remaining difference rather than an
			// oversight: the alternative, allowing the narrative to write the stage,
			// is exactly what the design forbids.
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "cultivation_owned_state_update", update.Path, "a field the cultivation rules do not own", fieldID, fmt.Errorf("该字段由修炼规则结算写入，不能由叙述改写: %s", fieldID))
		}
		if actorStateFieldIsSettledFavorability(field) {
			// The settled affinity is computed from character_facts.affinities:
			// the projector derives the value, bands it and records the basis.
			// A direct write here would be a second, unbanded number with no
			// record behind it, and the two would disagree in front of the
			// player — which is exactly what the acceptance run caught.
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "affinity_owned_state_update", update.Path, "a field the affinity settlement writes", fieldID, fmt.Errorf("该字段由好感结算写入；会改变好感的互动请走 character_facts.affinities，不要在 state_changes 中直接改写: %s", fieldID))
		}
		if len(segments) == 2 && update.Op == interactivestate.Replace {
			if converted, changed := normalizeTurnSubmissionFieldValue(field, update.Value); changed {
				slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-turn-submission] normalized lossless field value actor_id=%q field_id=%q from=string to=%s location=internal/interactive/turn_state_updates.go", actorID, fieldID, field.Type))
				update.Value = converted
			}
		}

		currentValue := actorStateFieldValue(workingState, actorID, fieldID)
		if len(segments) == 2 && update.Op == interactivestate.Replace &&
			normalizeActorStateFieldName(fieldID) == xiuxianFieldImportantItems &&
			ActorStateSystemIsXiuxian(system) {
			// A whole-object replace cannot be taken literally for the pack: the
			// rules own the copies that have a registered definition, and a prose
			// rewrite of one of them would double-count against the item and trade
			// modules. See xiuxianProseCarriedItems.
			update.Value = xiuxianProseCarriedItems(system.CultivationRules,
				currentValue, update.Value, workingState)
		}
		nextValue, auditValue, err := applyStateUpdateValue(field, currentValue, segments[2:], update)
		if err != nil {
			code := "state_value_invalid"
			actualValue := update.Value
			if update.Op == interactivestate.Delta {
				code = "delta_target_not_number"
				actualValue = currentValue
				if len(segments) > 2 {
					if currentObject, ok := currentValue.(map[string]any); ok {
						if leaf, found := interactivestate.NestedValue(currentObject, segments[2:]); found {
							actualValue = leaf
						} else {
							actualValue = nil
						}
					}
				}
			} else if len(segments) > 2 {
				actualValue = currentValue
			}
			return CompiledTurnStateUpdates{}, stateUpdateError(index, code, update.Path, stateUpdateExpected(field, segments[2:], update.Op), stateUpdateActual(actualValue), err)
		}
		if proseOwnedPoolRecoveryRefused(system, fieldID, currentValue, nextValue) {
			// The pools may be lowered by prose and never raised by it. A real run
			// answered "躺满一整天养伤" by omitting the rest module and writing 生命
			// +60 itself, so the day cost no clock and no rules interval ran while
			// the save claimed a recovery. Refusing the write leaves the model one
			// way to make the prose true: submit the rest interval and let the rules
			// price it.
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "pool_restore_not_prose_owned", update.Path, "a pool that only falls, or one this turn's rest module restores", fieldID, fmt.Errorf("生命与灵力只能由休养或调息结算回升，不能由叙述直接加回: %s", fieldID))
		}
		if sceneLocationMoveRefused(options, actorID, fieldID, currentValue, nextValue) {
			// Where the character stands is a world fact, and the rules that read it
			// — a road's origin, a realm's entrance, an event's region — only see
			// registered places. A live run narrated walking into the cave, wrote
			// the cave's own wording into 当前详细地点 with no world module, and
			// committed: the scene then named no place the rules could resolve, so
			// the realm and the only road vanished from the entry and could not be
			// reached again on that branch. The write is refused here so the model
			// either submits the module that really moves the character or rewrites
			// the prose to show the attempt failing.
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "scene_location_not_world_settled", update.Path, "a place a registered road or realm settles, or a local move inside the current place", fieldID, fmt.Errorf("叙述不能把角色移到未经行路或秘境结算的地点: %s", fieldID))
		}
		if reflect.DeepEqual(currentValue, nextValue) {
			continue
		}

		patch := ActorStatePatch{
			ActorID:      actorID,
			State:        map[string]any{fieldID: nextValue},
			SourceTurnID: options.SourceTurnID,
		}
		if getPath(workingState, actorStateRoot+"."+actorID) == nil {
			patch.TemplateID = templateID
			for _, initialActor := range system.InitialActors {
				if initialActor.ID != actorID {
					continue
				}
				patch.ActorName = initialActor.Name
				patch.Role = initialActor.Role
				patch.Description = initialActor.Description
				patch.Gender = initialActor.Gender
				patch.SpeechStyle = initialActor.SpeechStyle
				patch.Tags = initialActor.Tags
				break
			}
		}
		_, ops, actorOps, _, _, err := validateActorStatePatch(system, workingState, patch)
		if err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "actor_state_invalid", update.Path, field.Type, stateUpdateActual(nextValue), err)
		}
		for _, op := range ops {
			applyStateOp(workingState, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(workingState, op)
		}
		canonicalPaths = append(canonicalPaths, canonical)
		compiled.Updates = append(compiled.Updates, interactivestate.Update{Op: update.Op, Path: interactivestate.FormatPath(canonical), Value: auditValue})
		compiled.Ops = append(compiled.Ops, ops...)
		compiled.ActorOps = append(compiled.ActorOps, actorOps...)
	}
	for _, intent := range sortedActorLifecycleIntents(lifecycleIntents, interactivestate.Archive) {
		ops, actorOps := compileActorArchivePresenceCleanup(system, workingState, intent, options.SourceTurnID)
		compiled.Ops = append(compiled.Ops, ops...)
		compiled.ActorOps = append(compiled.ActorOps, actorOps...)
	}

	compiled.Ops = normalizeStateOpsUnbounded(compiled.Ops)
	compiled.ActorOps = normalizeActorStateOps(compiled.ActorOps)
	return compiled, nil
}

func applyStateUpdateValue(field ActorStateField, current any, nestedPath []string, update interactivestate.Update) (any, any, error) {
	if len(nestedPath) == 0 {
		switch update.Op {
		case interactivestate.Replace:
			next, err := normalizeActorStateValue(field, update.Value)
			return next, next, err
		case interactivestate.Delta:
			if field.Type != "number" {
				return nil, nil, fmt.Errorf("delta 只能用于 number 字段")
			}
			currentNumber, ok := actorStateNumber(current)
			if !ok {
				return nil, nil, fmt.Errorf("delta 目标必须是已有数值，不能把缺失值当作 0")
			}
			delta, _ := actorStateNumber(update.Value)
			next, err := normalizeActorStateValue(field, currentNumber+delta)
			return next, delta, err
		}
	}
	if field.Type != "object" {
		return nil, nil, fmt.Errorf("只有 object 字段允许继续访问子路径")
	}
	root, ok := interactivestate.CloneObject(current)
	if !ok {
		if current != nil {
			return nil, nil, fmt.Errorf("目标 object 字段的当前值无效")
		}
		root = map[string]any{}
	}
	switch update.Op {
	case interactivestate.Replace:
		if err := interactivestate.SetNestedValue(root, nestedPath, update.Value, false); err != nil {
			return nil, nil, err
		}
		return root, update.Value, nil
	case interactivestate.Delta:
		currentLeaf, found := interactivestate.NestedValue(root, nestedPath)
		if !found {
			return nil, nil, fmt.Errorf("delta 目标叶子不存在，不能把缺失值当作 0")
		}
		currentNumber, ok := actorStateNumber(currentLeaf)
		if !ok {
			return nil, nil, fmt.Errorf("delta 目标叶子必须是已有数值")
		}
		delta, _ := actorStateNumber(update.Value)
		if err := interactivestate.SetNestedValue(root, nestedPath, currentNumber+delta, true); err != nil {
			return nil, nil, err
		}
		return root, delta, nil
	default:
		return nil, nil, fmt.Errorf("不支持的状态操作: %s", update.Op)
	}
}

func actorPatchFromCreateUpdate(actorID string, value any) (ActorStatePatch, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return ActorStatePatch{}, fmt.Errorf("create 的 value 必须是 object")
	}
	allowed := map[string]bool{"template_id": true, "name": true, "role": true, "description": true, "gender": true, "speech_style": true, "tags": true, "state": true}
	for key := range object {
		if !allowed[key] {
			return ActorStatePatch{}, fmt.Errorf("create 包含未知字段: %s", key)
		}
	}
	state := map[string]any(nil)
	if rawState, exists := object["state"]; exists {
		var stateOK bool
		state, stateOK = rawState.(map[string]any)
		if !stateOK {
			return ActorStatePatch{}, fmt.Errorf("create.state 必须是 object")
		}
	}
	actorName, err := actorCreateStringField(object, "name")
	if err != nil {
		return ActorStatePatch{}, err
	}
	templateID, err := actorCreateStringField(object, "template_id")
	if err != nil {
		return ActorStatePatch{}, err
	}
	role, err := actorCreateStringField(object, "role")
	if err != nil {
		return ActorStatePatch{}, err
	}
	description, err := actorCreateStringField(object, "description")
	if err != nil {
		return ActorStatePatch{}, err
	}
	gender, err := actorCreateStringField(object, "gender")
	if err != nil {
		return ActorStatePatch{}, err
	}
	speechStyle, err := actorCreateStringField(object, "speech_style")
	if err != nil {
		return ActorStatePatch{}, err
	}
	tags, err := actorCreateStringListField(object, "tags")
	if err != nil {
		return ActorStatePatch{}, err
	}
	return ActorStatePatch{
		ActorID:     actorID,
		ActorName:   actorName,
		TemplateID:  templateID,
		Role:        role,
		Description: description,
		Gender:      gender,
		SpeechStyle: speechStyle,
		Tags:        tags,
		State:       state,
	}, nil
}

func actorCreateStringField(object map[string]any, key string) (string, error) {
	value, exists := object[key]
	if !exists {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("create.%s 必须是 string", key)
	}
	return strings.TrimSpace(text), nil
}

func actorCreateStringListField(object map[string]any, key string) ([]string, error) {
	value, exists := object[key]
	if !exists {
		return nil, nil
	}
	switch list := value.(type) {
	case []string:
		return list, nil
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("create.%s 必须是 string 列表", key)
			}
			out = append(out, text)
		}
		return out, nil
	}
	return nil, fmt.Errorf("create.%s 必须是 string 列表", key)
}

func actorCreateAuditValue(patch ActorStatePatch) map[string]any {
	value := map[string]any{"template_id": patch.TemplateID}
	if patch.ActorName != "" {
		value["name"] = patch.ActorName
	}
	if patch.Role != "" {
		value["role"] = patch.Role
	}
	if patch.Description != "" {
		value["description"] = patch.Description
	}
	if gender := character.NormalizeGender(patch.Gender); gender != "" {
		value["gender"] = gender
	}
	if patch.SpeechStyle != "" {
		value["speech_style"] = patch.SpeechStyle
	}
	if tags := normalizeIdentityTags(patch.Tags); len(tags) > 0 {
		value["tags"] = tags
	}
	if len(patch.State) > 0 {
		value["state"] = patch.State
	}
	return value
}

func stateUpdateConflictsWithRuleResolution(options TurnStateUpdateCompileOptions, actorID, fieldID string) bool {
	if options.RuleResolution == nil || normalizeRuleStateConsumptionMode(options.RuleStateConsumptionMode) == RuleStateConsumptionModeSuggestionsOnly {
		return false
	}
	for _, change := range normalizeTurnStateChanges(options.RuleResolution.Result.StateChanges) {
		if normalizeStatePanelActorID(change.ActorID) == actorID && actorStateFieldNameKey(change.FieldID) == actorStateFieldNameKey(fieldID) {
			return true
		}
	}
	return false
}

// proseOwnedPoolRecoveryRefused reports whether a write raises a current pool.
//
// 生命 and 灵力 may be lowered by the narrative, because a wound or an
// expenditure is a fact the prose records and the settlement's job there is to
// clamp rather than to author. They may not be raised by it: the design's
// recovery rates belong to the rules, and a model that states its own number is
// deciding the character's resources. The failure this closes is concrete — a
// real run answered "躺满一整天养伤" without the rest module and wrote 生命 +60,
// so the day cost no clock and no rules interval ran while the save claimed a
// recovery. With the write refused, the only way to make the prose true is to
// submit the interval and let the rules price it.
//
// Only the two current pools are covered. A maximum or a derived attribute is
// already refused outright as rules-owned, and a story-driven fall in cultivation
// has no rules path yet, so neither belongs here.
func proseOwnedPoolRecoveryRefused(system StoryDirectorActorStateSystem, fieldID string, current, next any) bool {
	if !ActorStateSystemIsXiuxian(system) {
		return false
	}
	key := actorStateFieldNameKey(fieldID)
	if key != actorStateFieldNameKey(xiuxianFieldHealth) && key != actorStateFieldNameKey(xiuxianFieldMana) {
		return false
	}
	currentNumber, currentOK := actorStateNumber(current)
	nextNumber, nextOK := actorStateNumber(next)
	if !currentOK || !nextOK {
		// A pool whose stored value is not a number cannot be compared, and the
		// settlement will clamp it on this turn anyway. Refusing here would turn a
		// repair the rules already own into a submission error.
		return false
	}
	return nextNumber > currentNumber
}

// stateUpdateConflictsWithRestModule reports whether a write targets the pool a
// rest module in this same turn is about to restore.
//
// The comparison is on the resolved field ID, not the raw path, so 生命 and the
// template's own reference spelling are the same field here. A turn whose rest
// module was rejected carries no owner and therefore does not block the write:
// the prose's own record of a wound stays available whenever no rules interval
// is settling that pool.
func stateUpdateConflictsWithRestModule(options TurnStateUpdateCompileOptions, actorID, fieldID string) bool {
	if strings.TrimSpace(options.RestPoolField) == "" {
		return false
	}
	if normalizeStatePanelActorID(options.RestActorID) != actorID {
		return false
	}
	return actorStateFieldNameKey(options.RestPoolField) == actorStateFieldNameKey(fieldID)
}

// stateUpdateConflictsWithWorldModule reports whether a write targets the scene
// location a world action in this same turn is about to write.
//
// The comparison is on the resolved field ID and the story actor, which is who
// the place write belongs to. A turn whose world module was rejected carries no
// owner and therefore does not block the write: the prose's own record of a
// location change stays available whenever no rules action is settling it.
func stateUpdateConflictsWithWorldModule(options TurnStateUpdateCompileOptions, actorID, fieldID string) bool {
	if !options.WorldOwnsSceneLocation {
		return false
	}
	if actorID != normalizeStatePanelActorID(DefaultStoryContextActorID) {
		return false
	}
	return actorStateFieldNameKey(storySceneLocationField) == actorStateFieldNameKey(fieldID)
}

// sceneLocationMoveRefused reports whether a write to 当前详细地点 moves the
// character to a place the world did not settle.
//
// The field is written from prose every turn, so the guard cannot be "only a
// world module may write it": a scene legitimately refines where inside the
// current place the character stands, and the field is documented to carry the
// place together with its parent scope. What prose may not do is leave the
// registered map on its own. A live run narrated walking into the cave, wrote the
// cave's own wording into this field with no world module, and committed: the
// scene then named no place any rule could resolve, so the realm's entrance and
// the only road both vanished from the entry and could not be reached again on
// that branch.
//
// Three cases pass:
//
//   - This turn's world module really moves the character (a road, or a realm
//     action that changes the area they stand in). The module is the settlement,
//     and a realm entry leaves this field to the prose because the cave's own
//     wording lives only there.
//   - The committed value already names no registered place. The character is
//     inside a realm, or somewhere the authored map does not cover; the rules
//     cannot resolve it either way, and refusing every later refinement would
//     strand the scene rather than protect it.
//   - The write resolves to the same registered place. A refinement, and a scene
//     that keeps naming the place, are what the field is for.
//
// Everything else — a different registered place, or no registered place at all
// — is a move the prose is making on the world's behalf. It is refused, and the
// model's way to make the prose true is to submit the module that walks or enters
// the place, or to rewrite the turn so the attempt fails.
func sceneLocationMoveRefused(options TurnStateUpdateCompileOptions, actorID, fieldID string, current, next any) bool {
	if actorID != normalizeStatePanelActorID(DefaultStoryContextActorID) {
		return false
	}
	if actorStateFieldNameKey(storySceneLocationField) != actorStateFieldNameKey(fieldID) {
		return false
	}
	// A road and a realm exit own the field outright, and the compiler refuses
	// the write for that reason before reaching this check.
	if options.WorldOwnsSceneLocation {
		return false
	}
	currentPlace, currentOK := registeredPlaceInText(sceneLocationText(current))
	if !currentOK {
		// The character already stands somewhere the authored map does not cover:
		// inside a realm, or in a scene the prose has refined. The rules cannot
		// resolve it either way, and refusing every later refinement would strand
		// the scene rather than protect it.
		return false
	}
	nextPlace, nextOK := registeredPlaceInText(sceneLocationText(next))
	if nextOK && nextPlace == currentPlace {
		return false
	}
	// Leaving the registered place. A realm action that really moves the
	// character may take the scene off the map — the cave's own wording lives
	// only here — but it may not put them at a different registered place, which
	// is a move no module settled.
	if options.WorldSettlesMovement {
		return nextOK
	}
	return true
}

// sceneLocationText reads a stored 当前详细地点 value as text. A non-text value is
// not a place at all, and the schema check refuses it before this point; treating
// it as empty keeps the guard from inventing a move.
func sceneLocationText(value any) string {
	text, _ := value.(string)
	return text
}

func validateStateUpdateValueSize(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("状态值无法编码为 JSON: %w", err)
	}
	if len(data) > maxTurnStateUpdateValueBytes {
		return fmt.Errorf("状态值超过 %d bytes", maxTurnStateUpdateValueBytes)
	}
	return nil
}

func stateUpdateError(index int, code, path, expected, actual string, cause error) *StateUpdateValidationError {
	return &StateUpdateValidationError{Index: index, Code: code, Path: path, Expected: expected, Actual: actual, Cause: cause}
}

func stateUpdateExpected(field ActorStateField, nested []string, op string) string {
	if op == interactivestate.Delta {
		return "existing number"
	}
	if len(nested) > 0 {
		return "value inside object field"
	}
	return field.Type
}

func stateUpdateActual(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		if _, ok := actorStateNumber(value); ok {
			return "number"
		}
		return fmt.Sprintf("%T", value)
	}
}
