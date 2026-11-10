package interactive

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/storytime"
)

// This file makes 《自制-修仙技艺》V0.1 playable through the main story.
//
// §3's revised constraint is the shape everything here follows: 炼丹、炼器、制符、
// 阵法优先共用规则和现有物品/效果模板，不按技艺分别建引擎、服务和操作窗. So a
// craft is one turn module — the same module for every art — and it reuses the
// item catalogue, the pack's 重要物品 record, the formal clock and the journal.
// There is no workshop screen, no second inventory and no per-art service.
//
// It is modelled on the rest module for one reason: a craft is the other kind of
// turn that really spends in-fiction time. A Turn carries no clock of its own, so
// a turn that works a batch has to publish the interval as a side record, exactly
// as a rest or a journey does. Everything else — the inputs, the product, the
// loss — is written into the Turn's own StateDelta, because unlike a rest those
// facts belong to the turn rather than to a record parented after it.
//
// §5's 同一批次只结算一次 is the reason the module resolves the whole run in one
// call: the batch count, the materials it spends and the products it yields are
// one answer, so a retry re-reads the same command rather than spending twice.

// StateOpSourceCraft marks an Actor State operation the craft rules produced.
//
// It is its own source kind for the same reason the trade's is: a batch is a rule
// outcome rather than prose, and the reroll path has to be able to drop one and
// re-derive it without touching anything the narrative wrote.
const StateOpSourceCraft = "craft"

// TurnCraftInput is the model-facing craft module of one story turn.
//
// It carries the method the character really knows, the interval the player
// really asked to spend, and nothing else. It carries no product, no count and
// no quality: those are the rules' answer, read from the registered method and
// the character's committed position. That is the same split TurnRestInput makes,
// and it is what makes ART-01 structural — there is nowhere in this module for a
// profession name to turn into an item.
type TurnCraftInput struct {
	// ActorID is who works the batch. Omit for the protagonist.
	ActorID string `json:"actor_id,omitempty" jsonschema_description:"实际制作的角色，省略则为主角。"`
	// MethodID names a registered craft method. A trade name, a profession or a
	// recipe heard in conversation is not one.
	MethodID string `json:"method_id,omitempty" jsonschema_description:"已登记制作方法的稳定标识，如 craft.alchemy.spring_return 或 craft.talisman.ward。要求学习的方法须先由 skills 正式授予，职业称号和口头提及不算学会。"`
	// DurationSeconds is the real in-fiction interval the player asked for.
	DurationSeconds int64 `json:"duration_seconds,omitempty" jsonschema_description:"玩家本次实际要求投入的故事内秒数；不足一批也可开始并保存进度，开工即扣本批材料；以后用同一method_id续作，仅完成后取得成品。不能擅自扩大玩家要求的时长。"`
}

// Empty reports whether the module carries nothing to apply.
func (input TurnCraftInput) Empty() bool {
	return strings.TrimSpace(input.MethodID) == "" && input.DurationSeconds <= 0
}

// cloneTurnCraftInput copies a craft module so an accepted one can be retained
// in a draft without the caller's value being aliased.
func cloneTurnCraftInput(input *TurnCraftInput) *TurnCraftInput {
	if input == nil {
		return nil
	}
	cloned := *input
	return &cloned
}

// validateTurnCraftModule rejects a craft module the rules could not act on.
//
// It checks shape, not outcome. Whether the method exists, whether the character
// qualifies and whether they hold the materials are the rules' answers, read
// against the committed branch while the turn commits; they are reported as a
// refusal rather than as a malformed submission, so the player reads a real
// reason. What must not pass here is a module that names no method while asking
// to spend time, or one that names a method without spending any.
func validateTurnCraftModule(input *TurnCraftInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	methodID := strings.TrimSpace(input.MethodID)
	if methodID == "" {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleCraft, nil, TurnSubmissionDiagnosticInvalidModule,
			"/craft/method_id", "a registered craft method identifier", "",
			"craft.method_id must name a registered craft method; a profession or an improvised recipe is not one.",
		)
	}
	if input.DurationSeconds <= 0 {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleCraft, nil, TurnSubmissionDiagnosticInvalidModule,
			"/craft/duration_seconds", "a positive number of in-fiction seconds", fmt.Sprint(input.DurationSeconds),
			"craft.duration_seconds must be the positive interval the player actually asked to spend at the craft.",
		)
	}
	return nil
}

// craftHeldFromState reads registered stock and the tools an actor really holds.
//
// Frozen methods distinguish consumed material inputs from reusable tools.
// Both need actual ownership, but filtering out every non-material here made a
// legal tool requirement impossible. Unknown prose entries are still excluded;
// only a method's registered material inputs can enter the consumption list.
func craftHeldFromState(cultivationRules *cultivation.Rules, state map[string]any, actorID string) []cultivation.CraftHeld {
	record, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	catalog := storyItemCatalog(cultivationRules,
		state)
	held := make([]cultivation.CraftHeld, 0, len(record))
	for key, value := range record {
		entry, ok := value.(map[string]any)
		if !ok || len(entry) == 0 {
			continue
		}
		defID := xiuxianEntryText(entry, "定义")
		if defID == "" {
			continue
		}
		definition, ok := catalog.Definition(defID)
		if !ok {
			continue
		}
		held = append(held, cultivation.CraftHeld{
			InstanceID: strings.TrimSpace(key),
			DefID:      definition.ID,
			Quantity:   xiuxianCarriedQuantity(entry),
		})
	}
	// Equipping moves a real copy out of the pack. It remains available as a
	// tool, never as a consumable input; verify the definition's actual slot.
	for _, slot := range xiuxianSlotOrder {
		entry, ok := xiuxianEquipmentEntry(state, actorID, slot)
		if !ok || len(entry) == 0 || xiuxianCarriedQuantity(entry) < 1 {
			continue
		}
		definition, ok := catalog.Definition(xiuxianEntryText(entry, "定义"))
		if !ok || definition.Kind == cultivation.KindMaterial {
			continue
		}
		actualSlot, err := xiuxianItemSlotForKind(definition.Kind)
		if err != nil || actualSlot != slot {
			continue
		}
		held = append(held, cultivation.CraftHeld{InstanceID: "slot:" + slot, DefID: definition.ID, Quantity: 1})
	}
	return held
}

// turnCraftOutcome is what one committed craft produced.
type turnCraftOutcome struct {
	input           TurnCraftInput
	method          cultivation.CraftMethodDef
	batch           cultivation.CraftBatch
	elapsed         int64
	progressMinutes int64
	ops             []ActorStateOp
}

// craftDraw derives the method's own chance roll from the command identity.
//
// §7's ART-02 requires 失败和不确定性不被反复重抽, and a craft's loss rate is
// exactly that kind of uncertainty. The identity is the durable command rather
// than the turn: a turn ID is minted fresh on every append, so binding the die to
// it would hand a retried or refreshed request a different draw — the very
// re-roll the rule forbids. This mirrors cultivationBreakthroughRoll, which binds
// its die the same way for the same reason.
//
// A commit that carries no command identity still needs a stable die, so the turn
// ID is the fallback. It is only reached where no retry can occur.
func craftDraw(identity DomainCommitIdentity, fallbackTurnID, methodID string) float64 {
	seed := strings.TrimSpace(identity.CommandID) + "\x00" +
		strings.TrimSpace(identity.OperationID) + "\x00" + fmt.Sprint(identity.Cycle)
	if strings.TrimSpace(identity.CommandID) == "" {
		seed = strings.TrimSpace(fallbackTurnID)
	}
	sum := sha256.Sum256([]byte(seed + "\x00" + strings.TrimSpace(methodID) + "\x00craft"))
	return float64(binary.BigEndian.Uint64(sum[:8])) / float64(1<<64)
}

// planTurnCraftCommit resolves one craft module against the position the turn
// left the character in.
//
// It returns the reason the rules refused instead of failing the turn, so the
// prose the model already wrote can still be published with the refusal named in
// the result. A refusal that silently produced an item would be the exact fake
// success ART-01 exists to prevent, so the caller must not treat an empty
// outcome as "apply nothing and carry on as though it worked".
//
// An empty refusal means the batch really ran. It may still have lost its last
// batch to the method's loss rate — that is a real outcome, not a refusal, and
// the caller reads batch.FailedBatches for it.
func planTurnCraftCommit(
	system StoryDirectorActorStateSystem,
	actorID string,
	state map[string]any,
	input *TurnCraftInput,
	identity DomainCommitIdentity,
	turnID string,
) (turnCraftOutcome, string) {
	if input == nil || input.Empty() {
		return turnCraftOutcome{}, ""
	}
	registry := storyCraftRegistryFromState(system, state)
	methodID := strings.TrimSpace(input.MethodID)
	method, ok := registry.Method(methodID)
	if !ok {
		slog.WarnContext(context.Background(), "craft method is not registered", "turn", turnID, "method", methodID)
		return turnCraftOutcome{}, craftRefusalWords(cultivation.ErrCraftUnknownMethod, cultivation.CraftMethodDef{ID: methodID}, *input)
	}
	if method.RequiresLearning && !craftMethodKnown(state, actorID, method) {
		slog.WarnContext(context.Background(), "craft refused: method not learned", "turn", turnID, "method", method.ID)
		return turnCraftOutcome{}, "尚未学会这个制作方法，材料与时间均未扣除，也没有产物。"
	}
	stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), system.CultivationRules)
	if !ok {
		slog.WarnContext(context.Background(), "craft refused: unreadable stage", "turn", turnID, "actor", actorID)
		return turnCraftOutcome{}, "这个角色的境界读不出来，这次制作没有开始。"
	}
	batch, work, err := planCraftWork(registry, state, actorID, method, cultivation.CraftRequest{
		Location: storySceneText(state, storySceneLocationField),
		MethodID: methodID,
		Minutes:  input.DurationSeconds / 60,
		Stage:    stage,
		Held: craftHeldFromState(system.CultivationRules,
			state, actorID),
		Draw: craftDraw(identity, turnID, methodID),
	})
	if err != nil {
		slog.WarnContext(context.Background(), "craft batch was refused", "turn", turnID, "method", methodID, "reason", err)
		return turnCraftOutcome{}, craftRefusalWords(err, method, *input)
	}
	ops, ok := craftActorOps(system.CultivationRules,
		state, actorID, batch, method, turnID)
	if !ok {
		return turnCraftOutcome{}, "这次制作的账目对不上，没有结算。"
	}
	var progressMinutes int64
	if pending, ok := work[methodID].(craftWork); ok {
		progressMinutes = pending.WorkedMinutes
	}
	previous, _ := xiuxianEquipmentEntry(state, actorID, craftWorkField)
	if len(previous) > 0 || len(work) > 0 {
		ops = append(ops, ActorStateOp{Op: "set", ActorID: actorID, FieldID: craftWorkField, Value: work, Reason: "保存实际制作进度与已投入材料，不提前领取成品。", SourceTurnID: turnID, SourceKind: StateOpSourceCraft, SourceID: methodID})
	}
	return turnCraftOutcome{
		progressMinutes: progressMinutes,
		input:           *input,
		method:          method,
		batch:           batch,
		elapsed:         batch.ElapsedMinutes * 60,
		ops:             ops,
	}, ""
}

// craftRefusalWords states a refused batch as the outcome the prose has to
// narrate instead of the success it just wrote.
//
// The live run's failure was not that the batch was refused — a 15% loss rate is
// supposed to refuse sometimes — but that the refusal never reached the model.
// It wrote 剑成了 and then, on the following turn, used items.acquire to put a
// blade in the pack with a source it made up. Naming what really happened is
// what lets it write 这次制作废了 instead.
func craftRefusalWords(err error, method cultivation.CraftMethodDef, input TurnCraftInput) string {
	methodID := strings.TrimSpace(method.ID)
	label := methodID
	if label == "" {
		label = strings.TrimSpace(input.MethodID)
	}
	switch {
	case errors.Is(err, cultivation.ErrCraftUnknownMethod):
		return "`" + label + "` 不是本故事登记的制作方法，这次制作没有开始，也没有产物。"
	case errors.Is(err, cultivation.ErrCraftStageTooLow):
		return "境界不到 `" + label + "` 要求的 `" + string(method.MinimumStage) + "`，这次制作没有开始，也没有产物。"
	case errors.Is(err, cultivation.ErrCraftDurationTooShort):
		return fmt.Sprintf("`%s` 一批要 %d 分钟，本次投入不足一分钟，这次制作没有开始，材料也没有动。", label, method.MinutesPerBatch)
	case errors.Is(err, cultivation.ErrCraftMissingMaterial):
		return "手上没有 `" + label + "` 需要的材料，这次制作没有开始，也没有产物。"
	}
	return "这次制作被规则拒绝了，没有开始，也没有产物。"
}

// craftLossRewriteReason states a lost batch as the outcome the prose has to
// narrate.
//
// Every performed batch paid its materials and time. Only the failed batch's
// product is missing; earlier successful products remain real. Neither a
// total-success story nor a total-loss story may overwrite that mixed outcome.
func craftLossRewriteReason(cultivationRules *cultivation.Rules, outcome turnCraftOutcome) string {
	methodID := strings.TrimSpace(outcome.method.ID)
	spent := make([]string, 0, len(outcome.batch.Consumed))
	catalog := storyItemCatalog(cultivationRules)
	for _, consumed := range outcome.batch.Consumed {
		if consumed.Quantity <= 0 {
			continue
		}
		spent = append(spent, fmt.Sprintf("%d 份 %s", consumed.Quantity, craftDefLabel(catalog, consumed.DefID)))
	}
	if len(outcome.batch.Products) > 0 {
		products := make([]string, 0, len(outcome.batch.Products))
		for _, product := range outcome.batch.Products {
			if product.Quantity > 0 {
				products = append(products, fmt.Sprintf("%d 份 %s", product.Quantity, craftDefLabel(catalog, product.DefID)))
			}
		}
		return fmt.Sprintf("这次制作 `%s` 共完成%d批，其中%d批成功、%d批报废；材料（%s）和时间已按全部实际制作批次结算，保留成品：%s。正文要写真实的部分成功与损失，不能写成全部成功或全无成品，也不能用 items 再领取一次成品。",
			methodID, outcome.batch.Batches, outcome.batch.Batches-outcome.batch.FailedBatches, outcome.batch.FailedBatches, strings.Join(spent, "、"), strings.Join(products, "、"))
	}
	reason := fmt.Sprintf("这次制作 `%s` 的成品失败了：本批按 %.0f%% 的失败率报废，材料（%s）已经耗掉，没有产出任何成品，时间也已经过去。",
		methodID, outcome.method.LossChance*100, strings.Join(spent, "、"))
	return reason + "正文要写真实结果（材料报废、重来或改做别的），不能写成制作成功，也不能自己把成品写进行囊或写成已取得。"
}

// craftActorOps writes the pack changes one batch produces.
//
// Both halves are one absolute write to 重要物品, because they are one fact about
// the pack: the herbs that went in are gone and the pill that came out is there.
// Writing them as two operations against a map the other does not see would let
// the second overwrite the first, and a delta would let a replay spend twice.
func craftActorOps(cultivationRules *cultivation.Rules, state map[string]any,
	actorID string,
	batch cultivation.CraftBatch,
	method cultivation.CraftMethodDef,
	turnID string,
) ([]ActorStateOp, bool) {
	record, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	if !ok {
		record = map[string]any{}
	}
	next := make(map[string]any, len(record)+len(batch.Products))
	for key, value := range record {
		next[key] = value
	}
	catalog := storyItemCatalog(cultivationRules,
		state)

	for _, consumed := range batch.Consumed {
		entry, ok := next[consumed.InstanceID].(map[string]any)
		if !ok {
			return nil, false
		}
		remaining := xiuxianCarriedQuantity(entry) - consumed.Quantity
		if remaining < 0 {
			return nil, false
		}
		updated := make(map[string]any, len(entry)+1)
		for key, value := range entry {
			updated[key] = value
		}
		// An exhausted copy stays at zero rather than disappearing: §2 requires
		// the batch's cost to stay readable after it is paid, and deleting the key
		// would erase the evidence of what was spent.
		updated["数量"] = float64(remaining)
		next[consumed.InstanceID] = updated
	}

	for _, product := range batch.Products {
		definition, ok := catalog.Definition(product.DefID)
		if !ok {
			return nil, false
		}
		// The key is the definition's identity rather than a per-batch name, so a
		// second batch stacks onto the first copy instead of creating a parallel
		// entry the trade module would price as a different good.
		name := strings.TrimSpace(definition.ID)
		if name == "" {
			name = strings.TrimSpace(product.DefID)
		}
		// §4.2 of 自制-装备与物品品级 requires an item's real source to be recorded
		// when it is obtained. A batch's output was made by this character, so the
		// source is that fact and nothing else: naming the method id here would put
		// an engineering identifier in front of the player, and the audit trail for
		// which method ran is already on the state op's Reason and SourceID.
		if existing, held := next[name].(map[string]any); held {
			merged := make(map[string]any, len(existing)+1)
			for key, value := range existing {
				merged[key] = value
			}
			merged["数量"] = float64(xiuxianCarriedQuantity(existing) + product.Quantity)
			// A copy the character already had keeps the source it was first given
			// by; a copy that somehow reached the pack without one is filled in now,
			// because this batch really did make some of what is in the stack.
			if xiuxianEntryText(existing, "来历") == "" {
				merged["来历"] = craftProductProvenance()
			}
			next[name] = merged
			continue
		}
		next[name] = xiuxianItemPackEntryWithFlavor(
			definition, name, product.Quantity,
			"",
			craftProductProvenance(),
		)
		if strings.HasPrefix(definition.ID, "gen.item.") {
			next[name].(map[string]any)["名称"] = definition.NameKey
		}
	}

	if len(next) == 0 {
		return nil, false
	}
	return []ActorStateOp{{
		Op:           "set",
		ActorID:      normalizeStatePanelActorID(actorID),
		FieldID:      normalizeActorStateFieldName(xiuxianFieldImportantItems),
		Value:        next,
		Reason:       fmt.Sprintf("按已登记制作方法 %s 实际投入与产出结算，不由叙述改写。", method.ID),
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceCraft,
		SourceID:     method.ID,
	}}, true
}

// craftProductProvenance says where a crafted product came from, in the story's
// own words. The character made it, and that is the whole of its origin: a
// product is never a find, a purchase or an award.
func craftProductProvenance() string {
	return "自行炼制所得。"
}

// removeCraftActorOps drops a previous craft's writes.
//
// It mirrors the trade module's remover so the reroll path can discard a batch
// along with the rule outcome it was derived from, without touching anything the
// narrative wrote.
func removeCraftActorOps(ops []ActorStateOp) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceCraft {
			continue
		}
		out = append(out, op)
	}
	return out
}

// CraftRecordEvent publishes the interval a crafting turn really spent.
//
// It is a side record because a Turn carries no clock: its projection cannot move
// one, so without this the character would work a whole day at the furnace while
// the world stood still. It carries no operations — the Turn's own StateDelta
// already holds the material deduction and the product, and writing them twice
// would let a replay apply them twice.
//
// It carries the resolved batch as well as the interval, because the Turn's
// module inputs are not persisted: a reroll has to be able to re-derive the craft
// from this record, and it can only do that if the record says which method was
// worked and for how long the player asked.
type CraftRecordEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	// TurnID is the main-story turn this batch belongs to.
	TurnID string `json:"turn_id,omitempty"`
	// Cursor is the canonical commit position this record was projected at.
	Cursor string `json:"cursor,omitempty"`
	// WorldTime is the story's own display text for this moment. It is a label;
	// every calculation uses ClockAfter, which is the formal axis.
	WorldTime string `json:"world_time,omitempty"`
	// ActorID is who worked the batch.
	ActorID string `json:"actor_id,omitempty"`
	// MethodID and MethodVersion name the registered method, and NameKey is its
	// localisation key so a reader can show the name without the rules table.
	MethodID      string `json:"method_id"`
	MethodVersion int    `json:"method_version"`
	MethodNameKey string `json:"method_name_key,omitempty"`
	// Art is which of §1's four arts the method belongs to.
	Art string `json:"art,omitempty"`
	// DurationSeconds is the interval the player offered. It is kept because it
	// is the input a reroll re-derives from; the resolved batch below is what the
	// rules made of it.
	DurationSeconds int64 `json:"duration_seconds"`
	// Batches counts completed batches. ElapsedMinutes includes actual work on
	// an unfinished batch whose inputs and progress are saved in the turn delta.
	Batches        int   `json:"batches"`
	ElapsedMinutes int64 `json:"elapsed_minutes"`
	// Consumed and Products are the run's real cost and yield. Failed batches
	// consume inputs without yielding a product; successful batches still yield.
	Consumed []cultivation.CraftConsumed `json:"consumed,omitempty"`
	Products []cultivation.CraftProduct  `json:"products,omitempty"`
	// FailedBatches is how many batches the method's loss rate took.
	FailedBatches int `json:"failed_batches,omitempty"`
	// ClockBefore and ClockAfter are the branch's formal time axis around this
	// batch. ClockAfter - ClockBefore.Seconds is the in-fiction time the craft
	// really consumed, and the craft owns no other clock.
	ClockBefore StoryClock `json:"clock_before"`
	ClockAfter  StoryClock `json:"clock_after"`
}

// turnCraftRecord publishes the interval a crafting turn really spent.
//
// It is parented after whatever the turn left at the branch head, so it folds
// exactly once and a regenerated turn's record no longer matches the path.
func turnCraftRecord(
	projection *storyBranchProjection,
	parentID, branchID string,
	turn TurnEvent,
	outcome turnCraftOutcome,
	system StoryDirectorActorStateSystem,
	now string,
) *CraftRecordEvent {
	if outcome.elapsed <= 0 {
		return nil
	}
	actorID := normalizeStatePanelActorID(outcome.input.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	clock, err := branchStoryClock(projection, actorID, system)
	if err != nil {
		return nil
	}
	clockAfter, err := AdvanceStoryClock(clock, outcome.elapsed)
	if err != nil {
		return nil
	}
	return &CraftRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCraftRecord, ID: newID("cfr"),
		ParentID: parentID, BranchID: branchID, Ts: now,
		TurnID: turn.ID, Cursor: "",
		ActorID:         actorID,
		MethodID:        outcome.method.ID,
		MethodVersion:   outcome.method.Version,
		MethodNameKey:   outcome.method.NameKey,
		Art:             string(outcome.method.Art),
		DurationSeconds: outcome.input.DurationSeconds,
		Batches:         outcome.batch.Batches,
		ElapsedMinutes:  outcome.batch.ElapsedMinutes,
		Consumed:        outcome.batch.Consumed,
		Products:        outcome.batch.Products,
		FailedBatches:   outcome.batch.FailedBatches,
		ClockBefore:     clock,
		ClockAfter:      clockAfter,
	}
}

// validateCraftRecordEvent refuses a record the arithmetic could not fold.
func validateCraftRecordEvent(event CraftRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("制作记录缺少 branch_id / a craft record requires a branch")
	}
	if strings.TrimSpace(event.MethodID) == "" {
		return fmt.Errorf("制作记录缺少方法 / a craft record requires a method")
	}
	return event.ClockAfter.Validate()
}

// applyCraftRecord folds one crafting interval into a branch projection.
//
// It moves the formal clock and nothing else. The pack changes belong to the
// Turn's own StateDelta, which the projection applies from the Turn; repeating
// them here would spend the materials twice on a replay.
func applyCraftRecord(branch *storyBranchProjection, event CraftRecordEvent) error {
	if err := event.ClockAfter.Validate(); err != nil {
		return err
	}
	if event.ClockBefore.Validate() != nil {
		return fmt.Errorf("制作记录缺少合法的起始时刻 / a craft record needs a valid clock before it")
	}
	if event.ClockAfter.Seconds < branch.clockSeconds() {
		return fmt.Errorf("%w: craft record moves the story clock backwards", storytime.ErrTimeReversal)
	}
	if event.ClockAfter.Seconds-event.ClockBefore.Seconds != event.ElapsedMinutes*60 {
		return fmt.Errorf("制作记录的时间与批次不一致 / craft clock and batches disagree")
	}
	clock := event.ClockAfter
	branch.Clock = &clock
	return nil
}

// craftRecordForTurn finds the craft record a turn published.
//
// A rerolled turn is re-read from the journal, where TurnResult's modules are
// deliberately absent, so the side record is the only place the craft's inputs
// survive. This is the same reason the recovery record is read back.
func craftRecordForTurn(path []StoryEventRecord, branchID, turnID string) (CraftRecordEvent, string, bool) {
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeCraftRecord {
			continue
		}
		var event CraftRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		if event.BranchID != branchID || strings.TrimSpace(event.TurnID) != turnID {
			continue
		}
		return event, record.Envelope.ID, true
	}
	return CraftRecordEvent{}, "", false
}

// craftRerollRecord republishes a rerolled turn's craft against the replacement
// position.
//
// The interval is fixed by the record's own window rather than re-derived from a
// fresh reading of the materials: the world already spent those hours, and a
// reroll is a second reading of the same events rather than a rewind of the
// calendar. What is re-derived is the batch — which materials it really spent and
// what it really yielded — because the rule outcome the craft was settled against
// has been replaced.
//
// The caller passes the position the replacement turn ends at, not the live
// branch position: the materials this rerolled turn's own items and trade
// modules just obtained are materials the furnace can really consume, and
// reading the live map would refuse a batch the turn itself paid for.
func craftRerollRecord(
	system StoryDirectorActorStateSystem,
	state map[string]any,
	target TurnEvent,
	source CraftRecordEvent,
	sourceID string,
	now string,
) *CraftRecordEvent {
	if source.ClockBefore.Validate() != nil || source.ClockAfter.Validate() != nil {
		return nil
	}
	actorID := normalizeStatePanelActorID(source.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	method, found := storyCraftRegistryFromState(system, state).Method(source.MethodID)
	if !found || method.Version != source.MethodVersion {
		return nil
	}
	outcome, refusal := planTurnCraftCommit(system, actorID, state, &TurnCraftInput{ActorID: actorID, MethodID: source.MethodID, DurationSeconds: source.DurationSeconds}, turnIdentity(target), target.ID)
	if refusal != "" || outcome.elapsed != source.ClockAfter.Seconds-source.ClockBefore.Seconds {
		return nil
	}
	batch := outcome.batch
	// The window is the superseded record's own, kept verbatim. Spending it again
	// would move the calendar for work the world already paid for.
	return &CraftRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCraftRecord, ID: newID("cfr"),
		ParentID: sourceID, BranchID: target.BranchID, Ts: now,
		TurnID: target.ID, Cursor: "",
		ActorID:         actorID,
		MethodID:        source.MethodID,
		MethodVersion:   source.MethodVersion,
		MethodNameKey:   source.MethodNameKey,
		Art:             source.Art,
		DurationSeconds: source.DurationSeconds,
		Batches:         batch.Batches,
		ElapsedMinutes:  batch.ElapsedMinutes,
		Consumed:        batch.Consumed,
		Products:        batch.Products,
		FailedBatches:   batch.FailedBatches,
		ClockBefore:     source.ClockBefore,
		ClockAfter:      source.ClockAfter,
	}
}

// craftRerollActorOps re-derives a rerolled turn's craft writes against the
// replacement position.
func craftRerollActorOps(system StoryDirectorActorStateSystem, state map[string]any, target TurnEvent, source CraftRecordEvent) []ActorStateOp {
	actorID := normalizeStatePanelActorID(source.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	method, found := storyCraftRegistryFromState(system, state).Method(source.MethodID)
	if !found || method.Version != source.MethodVersion {
		return nil
	}
	outcome, refusal := planTurnCraftCommit(system, actorID, state, &TurnCraftInput{ActorID: actorID, MethodID: source.MethodID, DurationSeconds: source.DurationSeconds}, turnIdentity(target), target.ID)
	if refusal != "" {
		return nil
	}
	return outcome.ops
}
