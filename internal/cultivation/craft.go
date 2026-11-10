package cultivation

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// This file implements the rules half of 《自制-修仙技艺》V0.1.
//
// §3 (revised 2026-10-18) is the governing constraint: 炼丹、炼器、制符、阵法
// 优先共用规则和现有物品/效果模板，不按技艺分别建引擎、服务和操作窗. So there is
// one craft-method table, not four, and a method is a registered recipe rather
// than a per-art service. §5 adds that the same batch settles once and that a
// craft which has not formally begun has not yet spent anything.
//
// What the rules own here is exactly §2's list: a real method, a stage
// qualification, the materials the batch consumes, the in-fiction time it takes,
// the product it yields and the loss it may cost. What the AI owns is the
// process, the dialogue and the failure's shape — none of which can change the
// numbers.
//
// The table is deliberately short. §6 says to pick 少量代表方法 first and to prove
// 能学、能做、能使用、能交易及能恢复 before adding more, so the first version
// ships one method and one material to prove the whole path.

// ErrCraftUnknownMethod is returned for a method the table does not hold.
//
// ART-01 is the acceptance line behind this error: 有职业名但没有方法或材料时，不能
// 凭一句话直接取得产物. A caller that names no registered method cannot reach any
// output at all, however the prose describes them as a 丹师.
var ErrCraftUnknownMethod = errors.New("craft: unknown method")

// ErrCraftUnknownMaterial is returned for an input the catalogue cannot price.
var ErrCraftUnknownMaterial = errors.New("craft: unknown material")

// ErrCraftMissingMaterial is returned when the character does not hold a whole
// batch's worth of an input.
var ErrCraftMissingMaterial = errors.New("craft: missing material")

// ErrCraftStageTooLow is returned when the method's stage floor is not met.
var ErrCraftStageTooLow = errors.New("craft: stage too low")

// ErrCraftDurationTooShort is returned when the turn claims a shorter interval
// than the method needs for one batch.
var ErrCraftDurationTooShort = errors.New("craft: duration shorter than one batch")

// ErrCraftUnknownOutput is returned when the declared product is not a
// registered definition the method is allowed to yield.
var ErrCraftUnknownOutput = errors.New("craft: output not registered for this method")

// CraftInput names one material and how much of it one batch consumes.
type CraftInput struct {
	// DefID is a registered catalogue definition of kind material.
	DefID string `json:"def_id"`
	// Quantity is how many units one batch consumes. It is a whole number
	// because a batch consumes whole units: half a herb is not a fact the
	// inventory can record.
	Quantity int `json:"quantity"`
}

// CraftOutput is one product a method can yield.
//
// §2 requires 完成产物有稳定定义、品级、稀有度和数量, and §4.1's budget rules
// already fix those on the catalogue entry. The method therefore names a
// definition rather than describing a result, which is what makes
// 不得先叙述出神器再临时注册效果 structural.
type CraftOutput struct {
	// DefID is a registered catalogue definition the method may produce.
	DefID string `json:"def_id"`
	// Quantity is how many copies one successful batch yields.
	Quantity int `json:"quantity"`
	// Chance is the fraction of batches that yield this output, for a method
	// with more than one possible product. It sums to 1 across a method's
	// outputs. A single-output method states 1.
	Chance float64 `json:"chance"`
}

// CraftMethodDef is one registered craft recipe.
//
// A method is world content and a gameplay version, not story state: §3 says
// 方法参数属于世界内容和玩法版本，本局材料、进度和产物属于Story状态. So the
// definition is a table entry and the batch is a story event.
type CraftMethodDef struct {
	Source           string   `json:"source,omitempty"`
	RequiredTools    []string `json:"required_tools,omitempty"`
	RequiredLocation string   `json:"required_location,omitempty"`
	FailureCost      string   `json:"failure_cost,omitempty"`
	// ID is the stable identity, and NameKey the localisation key.
	ID      string `json:"id"`
	NameKey string `json:"name_key"`
	Version int    `json:"version"`
	// Art is which of §1's four arts the method belongs to. It is a label for
	// the reader and for grouping, not a second engine: the arithmetic below is
	// the same for every art.
	Art CraftArt `json:"art"`
	// MinimumStage is the lowest stage that may work the method. §2 requires a
	// real 境界／能力 condition, and 丹修 as a self-description is explicitly not
	// a substitute for it.
	MinimumStage string `json:"minimum_stage"`
	// MinutesPerBatch is the real in-fiction time one batch takes. §2 requires
	// 耗时 to be fixed before the work starts.
	MinutesPerBatch int64 `json:"minutes_per_batch"`
	// Inputs is what one batch consumes.
	Inputs []CraftInput `json:"inputs"`
	// Outputs is what one successful batch yields.
	Outputs []CraftOutput `json:"outputs"`
	// LossChance is the fraction of batches that fail outright, yielding no
	// output and consuming the inputs. §2 requires 失败代价 to be decided before
	// the work begins and forbids 默认每次炸炉、死亡或全毁, so a method states
	// its own rate rather than the narration choosing one.
	LossChance float64 `json:"loss_chance"`
	// RequiresLearning marks methods granted through the formal skill-learning module.
	RequiresLearning bool `json:"requires_learning"`
}

// CraftArt is which of the four arts a method belongs to.
type CraftArt string

const (
	CraftArtAlchemy   CraftArt = "alchemy"
	CraftArtSmithing  CraftArt = "smithing"
	CraftArtTalisman  CraftArt = "talisman"
	CraftArtFormation CraftArt = "formation"
)

// Validate refuses a method the arithmetic could not honestly run.
//
// The checks mirror the item catalogue's: a method that cannot be priced, timed
// or resolved is refused at registration rather than at the first batch, so an
// editing mistake is caught where it is written.
func (m CraftMethodDef) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("%w: missing id", ErrCraftUnknownMethod)
	}
	switch m.Art {
	case CraftArtAlchemy, CraftArtSmithing, CraftArtTalisman, CraftArtFormation:
	default:
		return fmt.Errorf("craft method %q: unknown art", m.ID)
	}
	if m.FailureCost != "" && m.FailureCost != "consume_inputs" {
		return fmt.Errorf("craft method %q: unsupported failure cost", m.ID)
	}
	if m.Version < 1 {
		return fmt.Errorf("craft method %q: version must be at least 1", m.ID)
	}
	if _, ok := ParseStageKey(m.MinimumStage); !ok {
		return fmt.Errorf("craft method %q: unknown minimum stage %q", m.ID, m.MinimumStage)
	}
	if m.MinutesPerBatch <= 0 {
		return fmt.Errorf("craft method %q: one batch must take real time", m.ID)
	}
	if len(m.Inputs) == 0 {
		return fmt.Errorf("craft method %q: a batch must consume something", m.ID)
	}
	seenInputs := map[string]bool{}
	for _, input := range m.Inputs {
		if seenInputs[input.DefID] {
			return fmt.Errorf("craft method %q: duplicate input", m.ID)
		}
		seenInputs[input.DefID] = true
		if strings.TrimSpace(input.DefID) == "" || input.Quantity <= 0 {
			return fmt.Errorf("craft method %q: every input needs a definition and a positive count", m.ID)
		}
	}
	if len(m.Outputs) == 0 {
		return fmt.Errorf("craft method %q: a batch must yield something", m.ID)
	}
	total := 0.0
	for _, output := range m.Outputs {
		if strings.TrimSpace(output.DefID) == "" || output.Quantity <= 0 {
			return fmt.Errorf("craft method %q: every output needs a definition and a positive count", m.ID)
		}
		if math.IsNaN(output.Chance) || math.IsInf(output.Chance, 0) || output.Chance <= 0 || output.Chance > 1 {
			return fmt.Errorf("craft method %q: an output's chance must be inside (0,1]", m.ID)
		}
		total += output.Chance
	}
	// §2's 可产出范围 has to be a real partition: the chances are the whole of
	// what a batch can produce, so they add to one. A method whose chances fell
	// short would silently drop batches; one that overshot would let a single
	// batch yield two of its listed products.
	if math.Abs(total-1) > 1e-9 {
		return fmt.Errorf("craft method %q: output chances add to %v, want 1", m.ID, total)
	}
	if math.IsNaN(m.LossChance) || math.IsInf(m.LossChance, 0) || m.LossChance < 0 || m.LossChance > 1 {
		return fmt.Errorf("craft method %q: loss chance must be inside [0,1]", m.ID)
	}
	// A method that always fails is not a method, and one that can never fail
	// still has to say so explicitly rather than by omission.
	if m.LossChance >= 1 {
		return fmt.Errorf("craft method %q: a method that always fails produces nothing", m.ID)
	}
	return nil
}

// CraftRegistry holds the registered craft methods of one story branch.
//
// It mirrors SkillRegistry and the technique table: definitions are the rules'
// own content, a character's progress is story state, and a branch inherits only
// what was registered before the fork.
type CraftRegistry struct {
	lookup    func(string) (CraftMethodDef, bool)
	lookupIDs func() []string
	defs      map[string]CraftMethodDef
	order     []string
}

// NewCraftRegistry returns an empty registry.
func NewCraftRegistry() *CraftRegistry {
	return &CraftRegistry{defs: map[string]CraftMethodDef{}}
}

// Register validates and stores one method.
func (r *CraftRegistry) Register(method CraftMethodDef) error {
	if err := method.Validate(); err != nil {
		return err
	}
	if r.lookup != nil {
		if existing, ok := r.Method(method.ID); ok && !reflect.DeepEqual(existing, method) {
			return fmt.Errorf("craft: archived definition cannot be overwritten: %s", method.ID)
		}
	}
	if _, exists := r.defs[method.ID]; !exists {
		r.order = append(r.order, method.ID)
	}
	r.defs[method.ID] = cloneCraftMethod(method)
	return nil
}

// Method resolves one registered method.
func (r *CraftRegistry) Method(id string) (CraftMethodDef, bool) {
	if r == nil {
		return CraftMethodDef{}, false
	}
	id = strings.TrimSpace(id)
	method, ok := r.defs[id]
	if !ok && r.lookup != nil {
		method, ok = r.lookup(id)
		if !ok || method.ID != id || method.Validate() != nil {
			return CraftMethodDef{}, false
		}
		r.defs[id] = cloneCraftMethod(method)
	}
	return cloneCraftMethod(method), ok
}

// IDs lists the registered method identities in registration order.
func (r *CraftRegistry) IDs() []string {
	ids := lookupCatalogIDs(r.order, r.lookupIDs)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := r.Method(id); ok {
			out = append(out, id)
		}
	}
	return out
}

// Methods lists the registered methods in registration order.
func (r *CraftRegistry) Methods() []CraftMethodDef {
	methods := make([]CraftMethodDef, 0, len(r.order))
	for _, id := range r.IDs() {
		if method, ok := r.Method(id); ok {
			methods = append(methods, cloneCraftMethod(method))
		}
	}
	return methods
}

func cloneCraftMethod(method CraftMethodDef) CraftMethodDef {
	method.Inputs = append([]CraftInput(nil), method.Inputs...)
	method.Outputs = append([]CraftOutput(nil), method.Outputs...)
	method.RequiredTools = append([]string(nil), method.RequiredTools...)
	return method
}

// CraftBatch is the resolution of one batch against a real position.
//
// It is the answer the rules give, not a request: the caller supplies the method
// and the interval, and this is what the position and the method together make
// of it.
type CraftBatch struct {
	// Batches is how many whole batches the interval really paid for. §2 forbids
	// charging for work that has not formally started, so a partial batch is not
	// started at all and is not charged.
	Batches int
	// ElapsedMinutes is the real time the batches consumed. It is at most the
	// interval offered and is the number the clock advances by.
	ElapsedMinutes int64
	// Consumed is the material each batch really spent, batched together.
	Consumed []CraftConsumed
	// Products is what the batches really yielded. It is empty for a failed
	// batch, which still consumed its inputs.
	Products []CraftProduct
	// FailedBatches is how many batches were lost to the method's loss rate.
	FailedBatches int
}

// CraftConsumed is one material the batch spent.
type CraftConsumed struct {
	DefID string
	// InstanceID is the pack key the copy was recorded under, so the write can
	// deduct by name.
	InstanceID string
	Quantity   int
}

// CraftProduct is one product the batch yielded.
type CraftProduct struct {
	DefID    string
	Quantity int
}

// CraftRequest is one batch request against a real position.
type CraftRequest struct {
	Location string
	MethodID string
	// Minutes is the real in-fiction interval the turn offers.
	Minutes int64
	// Stage is the character's committed stage.
	Stage Cultivation
	// Held is what the character really carries, keyed by pack key, each entry
	// resolved to its definition and count.
	Held []CraftHeld
	// Draw is the method's own chance roll in [0,1), supplied by the caller so a
	// retried command re-reads the same die rather than rolling again. §7's
	// ART-02 requires 失败和不确定性不被反复重抽.
	Draw float64
}

// CraftHeld is one carried copy a batch may spend.
type CraftHeld struct {
	InstanceID string
	DefID      string
	Quantity   int
}

// PlanCraftBatch resolves one batch request.
//
// Ordering is deliberate: qualification first, then time, then materials. A
// character who does not qualify is told that rather than that they are short of
// herbs, and a turn that offered too little time is told that rather than losing
// materials it never got to use. §2's 未正式开始不先扣成本 is the reason
// materials are checked last and charged only once the batch really runs.
func PlanCraftBatch(registry *CraftRegistry, request CraftRequest) (CraftBatch, error) {
	if registry == nil {
		return CraftBatch{}, ErrCraftUnknownMethod
	}
	method, ok := registry.Method(request.MethodID)
	if !ok {
		return CraftBatch{}, ErrCraftUnknownMethod
	}
	if method.RequiredLocation != "" && strings.TrimSpace(request.Location) != method.RequiredLocation {
		return CraftBatch{}, fmt.Errorf("craft: required location unavailable")
	}
	for _, tool := range method.RequiredTools {
		found := false
		for _, held := range request.Held {
			if held.DefID == tool && held.Quantity > 0 {
				found = true
				break
			}
		}
		if !found {
			return CraftBatch{}, fmt.Errorf("craft: required tool unavailable: %s", tool)
		}
	}
	if !stageAtLeast(request.Stage, method.MinimumStage) {
		return CraftBatch{}, ErrCraftStageTooLow
	}
	if request.Minutes < method.MinutesPerBatch {
		return CraftBatch{}, ErrCraftDurationTooShort
	}
	batches := int(request.Minutes / method.MinutesPerBatch)
	if batches < 1 {
		return CraftBatch{}, ErrCraftDurationTooShort
	}
	// How many batches the materials really pay for. §5's 同一批次只结算一次 and
	// §2's 同一批材料不能同时用于另一件物品或交易 both follow from charging each
	// batch's inputs against the stock the character actually holds.
	affordable := batches
	heldByDef := make(map[string]int, len(request.Held))
	for _, held := range request.Held {
		defID := strings.TrimSpace(held.DefID)
		if defID == "" || held.Quantity <= 0 {
			continue
		}
		heldByDef[defID] += held.Quantity
	}
	for _, input := range method.Inputs {
		available := heldByDef[input.DefID]
		payable := available / input.Quantity
		if payable < affordable {
			affordable = payable
		}
	}
	if affordable < 1 {
		return CraftBatch{}, ErrCraftMissingMaterial
	}

	batch := CraftBatch{Batches: affordable, ElapsedMinutes: int64(affordable) * method.MinutesPerBatch}
	// Deduct each real stack independently. Totalling several copies into the
	// first key can overdraw it even when aggregate inventory is sufficient.
	held := append([]CraftHeld(nil), request.Held...)
	sort.Slice(held, func(i, j int) bool { return held[i].InstanceID < held[j].InstanceID })
	for _, input := range method.Inputs {
		remaining := input.Quantity * affordable
		for _, copy := range held {
			if strings.TrimSpace(copy.DefID) != input.DefID || copy.Quantity <= 0 || remaining <= 0 {
				continue
			}
			spent := minInt(copy.Quantity, remaining)
			batch.Consumed = append(batch.Consumed, CraftConsumed{DefID: input.DefID, InstanceID: copy.InstanceID, Quantity: spent})
			remaining -= spent
		}
	}

	// The loss rate is read once for the whole run rather than per batch, so a
	// ten-batch commission cannot roll ten times and turn a 10% method into a
	// 65% chance of at least one disaster. A draw inside the loss band loses one
	// batch — the last one — which is the honest reading of "this attempt failed"
	// for a run the player asked to complete.
	draw := request.Draw
	if math.IsNaN(draw) || math.IsInf(draw, 0) || draw < 0 {
		draw = 0
	}
	draw = math.Mod(draw, 1)
	if method.LossChance > 0 && draw < method.LossChance {
		batch.FailedBatches = 1
	}
	successful := affordable - batch.FailedBatches
	if successful == 0 {
		return batch, nil
	}

	// Which product the run yields. A single-output method has one entry with
	// chance 1 and always yields it; a method with alternatives reads the same
	// draw against the cumulative chances, so the choice is fixed by the command
	// rather than re-rolled on a retry.
	cumulative := 0.0
	for _, output := range method.Outputs {
		cumulative += output.Chance
		if draw < cumulative || cumulative >= 1-1e-9 {
			batch.Products = append(batch.Products, CraftProduct{
				DefID:    output.DefID,
				Quantity: output.Quantity * successful,
			})
			break
		}
	}
	return batch, nil
}

// builtinCraftMethods is the first-version representative set.
//
// §6 asks for 少量代表方法 and says not to claim playability from a recipe list
// that never ran, so each method here reaches the whole path: learn it, work it
// with real materials over real time, obtain a registered product, and trade
// that product through the existing trade module.
//
// All four arts share the same inventory, clock and result settlement.
// The talisman uses the existing combat shield consumer.
//
// The arithmetic below is shared, which is §3's point: adding a method is a table
// entry, not an engine.
var builtinCraftMethods = []CraftMethodDef{
	{
		ID: "craft.talisman.ward", NameKey: "craft.talisman.ward.name", Version: 1,
		Art: CraftArtTalisman, MinimumStage: "qi_1", MinutesPerBatch: 120, RequiresLearning: true,
		Inputs:     []CraftInput{{DefID: "material.talisman.paper", Quantity: 1}, {DefID: "material.talisman.ink", Quantity: 1}},
		Outputs:    []CraftOutput{{DefID: "talisman.ward.lower", Quantity: 1, Chance: 1}},
		LossChance: 0.10,
	},
	{
		// 回春丹 from 青萝草. The product is the catalogue's restoring pill, so
		// ART-04's budget requirement is met by reusing a published entry rather
		// than by describing a new effect.
		ID: "craft.alchemy.spring_return", NameKey: "craft.alchemy.spring_return.name", Version: 1,
		Art: CraftArtAlchemy, RequiresLearning: true,
		MinimumStage: "qi_1",
		// Half a day at the furnace. Long enough to be a real interval, short
		// enough that a single main-story turn can honestly cover it.
		MinutesPerBatch: 360,
		Inputs: []CraftInput{
			{DefID: "material.herb.green_vine", Quantity: 2},
		},
		Outputs: []CraftOutput{
			{DefID: "pill.restore.health.lower", Quantity: 1, Chance: 1},
		},
		// One batch in ten is lost. §2 requires a stated loss rather than a
		// narrator choosing one, and forbids defaulting to 炸炉 or death.
		LossChance: 0.10,
	},
	{
		// 凡铁剑 from 铁矿石. The product is the catalogue's white implement, so a
		// forged blade is priced by the same budget table as one the story hands
		// over, and §4.1's 适用阶段 is the entry's own qi_1 rather than a second
		// number written here.
		//
		// Its stage floor matches the blade's because a smith who cannot operate
		// what they made has not reached the art's first rung — the same reading
		// 炼丹 uses, where the method's floor is the pill's registered stage.
		ID: "craft.smithing.iron_sword", NameKey: "craft.smithing.iron_sword.name", Version: 1,
		Art: CraftArtSmithing, RequiresLearning: true,
		MinimumStage: "qi_1",
		// Twelve hours at the forge: a full day's work split by the night, which
		// is the shortest interval a blade honestly takes and still fits inside
		// one main-story turn.
		MinutesPerBatch: 720,
		Inputs: []CraftInput{
			{DefID: "material.ore.iron", Quantity: 3},
		},
		Outputs: []CraftOutput{
			{DefID: "implement.lower.common", Quantity: 1, Chance: 1},
		},
		// One batch in seven is lost to a bad heat or a cracked billet. Higher
		// than the pill's rate because §2 asks each method to state its own
		// failure rather than borrow a neighbour's.
		LossChance: 0.15,
	},
	{
		// 聚锋阵 from 护阵石. The product is the catalogue's single-type attack
		// formation, whose 增益, 启动耗灵 and 每轮维持 the equip path already
		// writes; carving the disc does not add a second way for a formation to
		// take effect.
		//
		// The floor is the formation's own qi_4 rather than the smithing method's
		// qi_1: §2 requires 真实掌握适用技艺, and an array a 炼气三层 character
		// cannot deploy is not one they can honestly cut.
		ID: "craft.formation.gathered_edge", NameKey: "craft.formation.gathered_edge.name", Version: 1,
		Art: CraftArtFormation, RequiresLearning: true,
		MinimumStage: "qi_4",
		// A full day: cutting the disc, aligning the marks and letting the array
		// settle. §2 requires 耗时 to be fixed before the work starts, and a day
		// is long enough that the interval is a real cost rather than a formality.
		MinutesPerBatch: 1440,
		Inputs: []CraftInput{
			{DefID: "material.stone.ward", Quantity: 2},
		},
		Outputs: []CraftOutput{
			{DefID: "formation.attack.middle", Quantity: 1, Chance: 1},
		},
		// One batch in ten: a misaligned mark ruins the disc. §2 forbids defaulting
		// to 炸炉 or death, so the loss is the disc and nothing else.
		LossChance: 0.10,
	},
}

// NewCraftRegistryWithBuiltins returns the first-version registry.
func NewCraftRegistryWithBuiltins(rules ...*Rules) *CraftRegistry {
	if len(rules) > 0 && rules[0] != nil {
		registry := NewCraftRegistry()
		for _, def := range rules[0].CraftMethods {
			_ = registry.Register(def)
		}
		return registry
	}
	registry := NewCraftRegistry()
	for _, method := range builtinCraftMethods {
		_ = registry.Register(method)
	}
	return registry
}
