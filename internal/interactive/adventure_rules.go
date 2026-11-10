package interactive

import (
	"encoding/json"
	"fmt"
	"ta/internal/cultivation"
)

// AdventureRules is a bounded, immutable world catalog frozen in the existing
// actor schema. Only the Story reducer persists its results; it owns no store.
type AdventureRules struct {
	// Runtime-only link to the same immutable world snapshot, never a second serialized catalog.
	world *WorldContent
	// Only the validated Story-local resolver may mark a runtime overlay. Never serialized.
	generatedCatalog bool
	StartingLoadouts []AdventureStartingLoadout `json:"starting_loadouts,omitempty"`
	Progression      *AdventureProgression      `json:"progression,omitempty"`
	FogAreas         []AdventureFogArea         `json:"fog_areas,omitempty"`
	NPCs             []AdventureNPC             `json:"npcs,omitempty"`
	Markets          []AdventureMarket          `json:"markets,omitempty"`
	RepairMaterialID string                     `json:"repair_material_id,omitempty"`
	Family           string                     `json:"family"`
	Currency         string                     `json:"currency"`
	Skills           []AdventureSkill           `json:"skills"`
	Items            []AdventureItem            `json:"items"`
	Recipes          []AdventureRecipe          `json:"recipes,omitempty"`
	Opponents        []AdventureOpponent        `json:"opponents,omitempty"`
	StartingSkills   []string                   `json:"starting_skills,omitempty"`
	StartingItems    map[string]int             `json:"starting_items,omitempty"`
	StartingCurrency int                        `json:"starting_currency"`
}
type AdventureDockService struct {
	Scale         string `json:"scale"`
	RequiredSkill string `json:"required_skill"`
	PricePerHour  int    `json:"price_per_hour"`
	RepairPerHour int    `json:"repair_per_hour"`
}

type AdventureMarket struct {
	WorkService     *AdventureWorkService `json:"work_service,omitempty"`
	DockService     *AdventureDockService `json:"dock_service,omitempty"`
	LocationAliases []string              `json:"location_aliases,omitempty"`
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Location        string                `json:"location"`
	Items           []string              `json:"items"`
}

type AdventureEffect struct {
	Kind  string `json:"kind"`
	Power int    `json:"power"`
}

type AdventureConditionalEffects struct {
	RequiredItem string            `json:"required_item"`
	Effects      []AdventureEffect `json:"effects"`
}

type AdventureSkill struct {
	SpaceCombat *AdventureSpaceCombatProfile `json:"space_combat,omitempty"`
	Mobility    int                          `json:"mobility,omitempty"`
	// GeneratedFrom is the frozen foundation of a Story-local adaptation.
	GeneratedFrom string `json:"generated_from,omitempty"`
	cultivation.DefinitionDetails
	RangeMeters         int                           `json:"range_meters,omitempty"`
	Backlash            *AdventureSkillBacklash       `json:"backlash,omitempty"`
	DrainCounterSkills  []string                      `json:"drain_counter_skills,omitempty"`
	WeaponStyle         bool                          `json:"weapon_style,omitempty"`
	Rush                bool                          `json:"rush,omitempty"`
	EnemyEffects        []AdventureEffect             `json:"enemy_effects,omitempty"`
	OncePerTarget       bool                          `json:"once_per_target,omitempty"`
	Materials           map[string]int                `json:"materials,omitempty"`
	RequiredTool        string                        `json:"required_tool,omitempty"`
	RequiredWeapon      string                        `json:"required_weapon,omitempty"`
	RequiredWeaponClass string                        `json:"required_weapon_class,omitempty"`
	ConditionalEffects  []AdventureConditionalEffects `json:"conditional_effects,omitempty"`
	Effects             []AdventureEffect             `json:"effects,omitempty"`
	RequiredItem        string                        `json:"required_item,omitempty"`
	ID                  string                        `json:"id"`
	Name                string                        `json:"name"`
	Kind                string                        `json:"kind"`
	Scale               string                        `json:"scale"`
	Power               int                           `json:"power"`
	Cost                int                           `json:"cost"`
	RequiredRank        int                           `json:"required_rank"`
	TrainingHours       int                           `json:"training_hours"`
	Prerequisite        string                        `json:"prerequisite,omitempty"`
}
type AdventureItem struct {
	WeaponClass string `json:"weapon_class,omitempty"`
	Mobility    int    `json:"mobility,omitempty"`
	// GeneratedFrom preserves the frozen foundation of an owned adaptation.
	GeneratedFrom string `json:"generated_from,omitempty"`
	cultivation.DefinitionDetails
	AttackKind      string `json:"attack_kind,omitempty"`
	CooldownSeconds int64  `json:"cooldown_seconds,omitempty"`
	MaxCharges      int    `json:"max_charges,omitempty"`
	RadioDevice     bool   `json:"radio_device,omitempty"`
	// SupportOnly assets can power authored work, never qualify as combat craft.
	SupportOnly   bool           `json:"support_only,omitempty"`
	Materials     map[string]int `json:"materials,omitempty"`
	Acquisition   string         `json:"acquisition"`
	Unique        bool           `json:"unique,omitempty"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Scale         string         `json:"scale"`
	Price         int            `json:"price"`
	Power         int            `json:"power"`
	Durability    int            `json:"durability,omitempty"`
	Effect        string         `json:"effect,omitempty"`
	RequiredSkill string         `json:"required_skill,omitempty"`
	Chance        int            `json:"chance,omitempty"`
	Multiplier    int            `json:"multiplier,omitempty"`
}
type AdventureRecipe struct {
	// RequiredTool is a reusable owned tool, not a consumed ingredient.
	RequiredTool string `json:"required_tool,omitempty"`
	cultivation.DefinitionDetails
	RequiredAsset string         `json:"required_asset,omitempty"`
	FuelCost      int            `json:"fuel_cost,omitempty"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	SkillID       string         `json:"skill_id"`
	SkillRank     int            `json:"skill_rank"`
	Hours         int            `json:"hours"`
	Materials     map[string]int `json:"materials"`
	OutputID      string         `json:"output_id"`
	OutputCount   int            `json:"output_count"`
}
type AdventureOpponent struct {
	Mindless   bool                        `json:"mindless,omitempty"`
	ShipCombat *AdventureShipCombatProfile `json:"ship_combat,omitempty"`
	// New named encounters require a committed, present character. Omitted in old frozen packs.
	RequiresPresence bool `json:"requires_presence,omitempty"`
	// Authored mastery for anonymous opponents; a bound NPC uses its actual learned record.
	SkillRanks               map[string]int `json:"skill_ranks,omitempty"`
	ElectronicDistanceMeters int            `json:"electronic_distance_meters,omitempty"`
	AttackKind               string         `json:"attack_kind,omitempty"`
	MentalAttack             bool           `json:"mental_attack,omitempty"`
	MentalCorruptionPower    int            `json:"mental_corruption_power,omitempty"`
	WeaponID                 string         `json:"weapon_id,omitempty"`
	Inventory                map[string]int `json:"inventory,omitempty"`
	CharacterSourceKey       string         `json:"character_source_key,omitempty"`
	Lethal                   bool           `json:"lethal,omitempty"`
	Rank                     int            `json:"rank,omitempty"`
	Energy                   int            `json:"energy,omitempty"`
	Location                 string         `json:"location"`
	LocationAliases          []string       `json:"location_aliases,omitempty"`
	SkillIDs                 []string       `json:"skill_ids,omitempty"`
	ID                       string         `json:"id"`
	Name                     string         `json:"name"`
	Scale                    string         `json:"scale"`
	Health                   int            `json:"health"`
	Attack                   int            `json:"attack"`
	Defense                  int            `json:"defense"`
	RewardCurrency           int            `json:"reward_currency"`
	RewardExperience         int            `json:"reward_experience"`
}

const AdventureStateField = "历练状态"

type AdventureCraftWork struct {
	Quantity    int `json:"quantity"`
	WorkedHours int `json:"worked_hours"`
}

type AdventureActor struct {
	Crafting             map[string]AdventureCraftWork  `json:"crafting,omitempty"`
	GeneratedDefinitions []AdventureGeneratedDefinition `json:"generated_definitions,omitempty"`
	ItemReadyAt          map[string]int64               `json:"item_ready_at,omitempty"`
	ItemCharges          map[string]int                 `json:"item_charges,omitempty"`
	MentalCorruption     int                            `json:"mental_corruption,omitempty"`
	MentalExposureID     string                         `json:"mental_exposure_id,omitempty"`
	MentalShock          int                            `json:"mental_shock,omitempty"`
	RushBlocked          int                            `json:"rush_blocked,omitempty"`
	SoftLimbs            bool                           `json:"soft_limbs,omitempty"`
	LastSealTarget       string                         `json:"last_seal_target,omitempty"`
	FogAreaID            string                         `json:"fog_area_id,omitempty"`
	FogTreated           bool                           `json:"fog_treated,omitempty"`
	SourceKey            string                         `json:"source_key,omitempty"`
	UsedTargetSkills     map[string]bool                `json:"used_target_skills,omitempty"`
	OpeningHazards       []AdventureOpeningHazard       `json:"opening_hazards,omitempty"`
	Growth               *AdventureNPCProgress          `json:"growth,omitempty"`
	Family               string                         `json:"family"`
	Poison               int                            `json:"poison,omitempty"`
	ControlRecovery      int                            `json:"control_recovery,omitempty"`
	Control              int                            `json:"control,omitempty"`
	Acquired             map[string]bool                `json:"acquired"`
	Rank                 int                            `json:"rank"`
	Experience           int                            `json:"experience"`
	Health               int                            `json:"health"`
	MaxHealth            int                            `json:"max_health"`
	Energy               int                            `json:"energy"`
	MaxEnergy            int                            `json:"max_energy"`
	Injury               int                            `json:"injury"`
	Currency             int                            `json:"currency"`
	Skills               map[string]int                 `json:"skills"`
	Training             map[string]int                 `json:"training"`
	Inventory            map[string]int                 `json:"inventory"`
	Equipment            map[string]string              `json:"equipment"`
	Assets               map[string]AdventureAsset      `json:"assets"`
	Defeated             map[string]bool                `json:"defeated"`
	Battle               *AdventureBattle               `json:"battle,omitempty"`
}
type AdventureAsset struct {
	PermanentDamage int   `json:"permanent_damage,omitempty"`
	DisabledUntil   int64 `json:"disabled_until,omitempty"`
	Health          int   `json:"health"`
	MaxHealth       int   `json:"max_health"`
	Shield          int   `json:"shield"`
	Fuel            int   `json:"fuel"`
}
type AdventureBattle struct {
	Mindless              bool                        `json:"mindless,omitempty"`
	NoProgressRounds      int                         `json:"no_progress_rounds,omitempty"`
	PlayerRetreatProgress int                         `json:"player_retreat_progress,omitempty"`
	EnemyRetreatProgress  int                         `json:"enemy_retreat_progress,omitempty"`
	PlayerCeasefireOffer  bool                        `json:"player_ceasefire_offer,omitempty"`
	EnemyCeasefireOffer   bool                        `json:"enemy_ceasefire_offer,omitempty"`
	EnemyLastAction       string                      `json:"enemy_last_action,omitempty"`
	PlayerScale           string                      `json:"player_scale,omitempty"`
	PersonalCombatSkillID string                      `json:"personal_combat_skill_id,omitempty"`
	ShipCombat            *AdventureShipCombatProfile `json:"ship_combat,omitempty"`
	// Only a skill actually resolved in the last exchange may be disclosed.
	EnemyLastSkillID         string            `json:"enemy_last_skill_id,omitempty"`
	ElectronicDistanceMeters int               `json:"electronic_distance_meters,omitempty"`
	AttackKind               string            `json:"attack_kind,omitempty"`
	EnemyEquipment           map[string]string `json:"enemy_equipment,omitempty"`
	EnemyItemCharges         map[string]int    `json:"enemy_item_charges,omitempty"`
	EnemyItemReadyAt         map[string]int64  `json:"enemy_item_ready_at,omitempty"`
	MentalAttack             bool              `json:"mental_attack,omitempty"`
	MentalCorruptionPower    int               `json:"mental_corruption_power,omitempty"`
	EnemyInjury              int               `json:"enemy_injury,omitempty"`
	EnemyLastSeal            bool              `json:"enemy_last_seal,omitempty"`
	EnemyRushBlocked         int               `json:"enemy_rush_blocked,omitempty"`
	EnemySoftLimbs           bool              `json:"enemy_soft_limbs,omitempty"`
	EnemyRank                int               `json:"enemy_rank"`
	EnemyMaxEnergy           int               `json:"enemy_max_energy"`
	EnemyActorID             string            `json:"enemy_actor_id,omitempty"`
	EnemySkillRanks          map[string]int    `json:"enemy_skill_ranks,omitempty"`
	EnemyInventory           map[string]int    `json:"enemy_inventory"`
	CharacterSourceKey       string            `json:"character_source_key,omitempty"`
	Lethal                   bool              `json:"lethal,omitempty"`
	EnemyEnergy              int               `json:"enemy_energy"`
	ID                       string            `json:"id"`
	Scale                    string            `json:"scale"`
	OpponentID               string            `json:"opponent_id"`
	OpponentName             string            `json:"opponent_name"`
	Health                   int               `json:"health"`
	MaxHealth                int               `json:"max_health"`
	Attack                   int               `json:"attack"`
	Defense                  int               `json:"defense"`
	Round                    int               `json:"round"`
	Status                   string            `json:"status"`
	EnemyControlRecovery     int               `json:"enemy_control_recovery,omitempty"`
	EnemyControl             int               `json:"enemy_control"`
	EnemyPoison              int               `json:"enemy_poison"`
	AssetID                  string            `json:"asset_id,omitempty"`
}

// AdventureAction names intentions, never arbitrary deltas or opponent stats.
type AdventureAction struct {
	// Only the server may supply an already checked enemy intent.
	enemyChoice string
	Generate    *AdventureGeneratedDefinition `json:"generate,omitempty" jsonschema_description:"action=generate时提交本局新内容。技能须有已学模板基础；装备须有一件未装备、未部署的同模板实物用于改制，不额外创造库存；可用design提出实际效果，规则验证后结算材料、专业与工时。"`
	MarketID    string                        `json:"market_id,omitempty" jsonschema_description:"当前实际所在市场ID；买卖仅限其登记商品，船坞维修或日常劳作仅限登记服务。"`
	Action      string                        `json:"action" jsonschema:"enum=generate,enum=learn,enum=train,enum=refine,enum=rest,enum=work,enum=buy,enum=sell,enum=equip,enum=unequip,enum=withdraw,enum=use,enum=craft,enum=battle_start,enum=battle_round,enum=repair,enum=deploy" jsonschema_description:"题材正式行动；战斗回合由独立战斗窗口提交，不能用正文假定胜利。"`
	SkillID     string                        `json:"skill_id,omitempty" jsonschema_description:"已冻结技能目录中的ID。"`
	ItemID      string                        `json:"item_id,omitempty" jsonschema_description:"已冻结物品目录中的ID。"`
	RecipeID    string                        `json:"recipe_id,omitempty" jsonschema_description:"已冻结制作方法ID。"`
	TargetID    string                        `json:"target_id,omitempty" jsonschema_description:"机械或舰船的物品ID；针灸技能对敌封穴时填opponent，默认仍为自身疗伤。"`
	OpponentID  string                        `json:"opponent_id,omitempty" jsonschema_description:"已冻结对手ID，不接受自报敌人属性。"`
	Quantity    int                           `json:"quantity,omitempty" jsonschema_description:"本次交易数量，1至100。"`
	Hours       int                           `json:"hours,omitempty" jsonschema_description:"玩家明确授权的训练、学习、休养、劳作或制作小时数。"`
	Choice      string                        `json:"choice,omitempty" jsonschema:"enum=attack,enum=guard,enum=escape,enum=skill,enum=item,enum=talk,enum=listen,enum=surrender,enum=pursue,enum=ceasefire,enum=accept_ceasefire" jsonschema_description:"独立战斗窗口的一轮选择；surrender仅用于武侠或西幻明确非致命交锋，结算为无奖励败退；talk仅用于中软筋散后的言语对峙，不算胜利；listen是在已登记精神遭遇中主动倾听，可能加深侵蚀并使抵御失效。"`
	Scale       string                        `json:"scale,omitempty" jsonschema:"enum=person,enum=machine,enum=ship" jsonschema_description:"battle_start时为玩家参战尺度；省略沿用敌方尺度。个人对舰须person并用skill_id选择实际学会的space_combat能力，敌方须有ship_combat登记，不自动部署或消耗停泊资产。"`
}
type AdventureOutcome struct {
	DurationSeconds int64  `json:"duration_seconds"`
	Message         string `json:"message"`
}

// AdventureRulesContext combines frozen rules with a bounded generated working
// set. The archive remains queryable; complete definitions are never cut in half.
func AdventureRulesContext(r AdventureRules) string {
	projected, omitted := adventureGeneratedContextRules(r)
	data, err := json.Marshal(adventureMechanicalRules(projected))
	if err != nil {
		return ""
	}
	return "# 题材正式规则\n原创西幻family=fantasy以精力同时表示体能与专注，不另造法力或修仙资源；种族与职业称呼不授予目录外能力。只有当前family和已登记定义适用的下列机制才可使用。\n个人对舰不能按等级直接判胜：仅已学space_combat能力可在battle_start以scale=person、skill_id明确选择，之后每轮付energy_per_round，攻击另按实际技能、气力与弹药；Scale描述敌方，player_scale描述玩家。ship_combat记录单舰或有限聚合编队的舰壳门槛、火控与封锁，不代表完整舰队指挥。低阶同名斩舰招式、手雷和短暂闭气不自动破舰；无足够真实破坏力可以不破防。只有参战资产受损，不能把个人所受炮火写给停泊舰艇。\n本题材首次建立可持续参与的NPC时，state_changes的create使用adventure-npc模板，并以人物资料的source_key关联作者人物；不要使用important_character通用模板或生命10/10文本，人物等级、生命、技能与物品由冻结的题材来源初始化。场景在场名单使用同一actor_id。NPC只按已建立的来源身份、本人实际库存和冻结计划随Story时钟成长；不在state_changes捏造等级、技能或资源。既有当前安排可暂停原计划，不能凭空添加师承。\n来源：本局冻结的adventure_rules，完整规则目录上限128 KiB，玩家详情简介与背景单独保存，不进入此规则片段。\n武侠、星海及原创西幻的休息、装备、使用、制作与部署全部走本题材行动，不填凡人的顶层rest/items/craft/cultivation模块。装备填写equip与item_id，休息填写rest与hours，部署机械填写deploy与已持有成品item_id，不能把recipe_id当成物品。登记规则已决定的资格、装备、部署和维修不再另掷骰判定成败或追加故障；正式成功的成品不能因叙事说材料破旧就被写坏。规则拒绝时只解释真实缺失条件，不自编损伤。使用world.action=adventure与adventure对象提交generate/learn/train/refine/rest/work/buy/sell/equip/use/craft/deploy/repair/battle_start；普通回合提交world.action=none。不得在state_changes写历练状态及其数值投影。工时hours按小时计，一小时是3600秒，一时辰是两小时；不能把六小时写成六个时辰。仅正式时钟推进才经过时间，暂停、读档或重新打开游戏本身不代表跨夜或次日，过去正文里的误用不能覆盖正式时间。个人craft可分段：开工即投入该批quantity的材料和燃料，crafting记录recipe_id、quantity和worked_hours；未完成不发成品或经验。续作仍填同一recipe_id，hours仅为本次新增投入，quantity沿用未完数量；工时不足可以暂停，以后接着做，不能重复扣料、虚构成品或用休息/等待代替制作。只有玩家明确授权的时长才可填hours；每回合一种行动，耗时由规则推进Story时钟。work在当前市场的work_service下提交market_id与玩家授权hours，工时、报酬与体力按冻结服务同比结算；只能描述已结算报酬，不在正文或state_changes自行给钱。不授予任务完成、职业资格或组织关系。买卖必须有当前场景真实商人和实际交付；不是商品的孤品price=0。玩家明确买入或卖出时，使用 world.action=adventure 提交 buy 或 sell，填写当前市场的 market_id、登记 item_id、quantity，source_text 引用玩家原话。只看价时不提交交易；未提交或被拒绝时，正文停在问价、备料或条件不足，不写已经收钱、交货或制成。独特物已有NPC主人时，只有已登记任务的同源发放人依约在场交付才能转移，结算同时扣除对方实物与佩戴；胜利本身不是夺物许可，无约定来源的奖励或制作不能再造一件。已知技能和物品不代表已学会或已持有；learn需要正式取得的传授或书册依据，禁止凭背景资料授予。craft的required_tool须实际持有且不会消耗；普通制作与改制都检查。craft的required_asset必须是实际持有、已部署且完好的指定作业设备，fuel_cost按批次扣除燃料；support_only载具不参加战斗。监听配方仅处理登记的普通跳频录音，残句不是完整明文，不授予通信身份或频道；量子纠缠保密通信无法据此破译。seal仅限制本轮突进，同一目标不能连续封穴；针灸默认疗伤，对敌封穴须明确target_id=opponent。fog_areas只限制登记的急进身法，特定辟瘴药或真实离区才能解除。soften实际服药后只能言语对峙或恢复，不是扣血毒；talk只结束为无战利的对峙。reflect借还真实来袭，不凭空伤敌；drain只转移已有内力并可能被禅功克制；backlash是全力招式另付的生命与伤势代价。mental_attack仅对实际参战人物造成独立精神伤害，按真实火候结算，不叠武器、弹药、破舰力或控制/侵蚀；普通护甲、借力反射与激光盾不能抵消，实际施展的mental_guard按比例防护。mental_guard不攻击或无条件停敌；精神侵蚀必须来自登记遭遇。mental_restore仅可用真实持有的有限次物件缓解已发生侵蚀，不能当作满血药；主动listen会放弃防护并加深侵蚀。electronic_disrupt只瘫痪登记在技能射程内的近处电子节点一轮，不伤害普通人物或毁掉整艘舰船；必须付气力与热反噬。overclock是舰船逃逸超频，不是维修：气力与燃料抽干、动力套筒永久损耗、存在停机融毁风险，之后至少三小时动力瘫痪；不能以休养、收回或维修抹去永久损伤和未到期锁定。repair可指定当前真实船坞的market_id支付登记服务费与材料，不能远程维修。generate配合generate对象可创作本局新技能、装备或制作方法：选同类template_id，填写category、name、source、description。技能从已学基础发展，装备改制一件真实持有且未装备/部署的模板实物；独特物品、充能/冷却物品与每目标限次技能不能复制。省略design时保留模板机制；design可推导实际不同的技能效果、装备威力或新制作产物。技能组合只支持已学同尺度基础的attack/guard/heal/control/poison/repair，最多三种且不重复；复杂条件技能仍按原模板。效果增强自动提高消耗、学习工时和所需火候；generate时须填写玩家已授权hours。装备改制须指定method_id及实物，方法产物种类与尺度须匹配；按强度增加核心材料、专业火候和工时，可补充材料但不能省去核心材料或降低价值。craft类的design用product_name及可选product_template_id与power定义新产物，登记不发物品，随后仍须craft实际扣料制作。六色quality_grade只作展示，简介与背景自由创作，不额外加数值；新定义只存本局档案，按稳定ID取用；上下文仅显示有限工作集，未显示不代表被删除，可用search_generated_definitions按名称、ID或分页读取。档案安全上限16384项及4MiB，超限保留旧内容并拒绝新登记。原始资料给来源与人物动机，以下目录给资格和正式效果。\n开始交战必须是当前场景真实遭遇；requires_presence的实名入口须绑定已建立且在场的本人，location只提示其作者出场去处，不能从目录召人；不得仅因目录有对手就让远处强敌出现。battle_round仅由独立战斗窗口提交；胜负、伤势和资源以窗口结算为准。招式按玩家实际参战尺度使用；目标尺度须通过已登记能力，不能仅因名字或等级跨尺度。可撤离、追击、提议停战或接受已提交的停战；只有双方同意才无胜负结束，撤离须经真实机动与拦截结算。技能火候由训练时长累计，武侠四级，星际十级；主角等级与阶段按本局progression表累计，未配置时最高十级。progression是本游戏自制数值适配，不等同原作等级。train只结算技能练满前的有效工时；精通后可用refine深修本阶段基础，须达到practice_mastery并支付逐小时物资费用。refine在阶段边界或资金耗尽时停止，只推进实际工时；下一阶段须先取得并学会对应practice_skill或practice_alternatives中的一项。无指定基础的第一阶段须填写一个已学技能ID。深修不赠送装备、师承、身份或跨尺度能力，也不触发强制结局。\n" + string(data) + omitted
}

func NewAdventureActor(r AdventureRules) AdventureActor {
	a := AdventureActor{Family: r.Family, Rank: 1, Health: 100, MaxHealth: 100, Energy: 100, MaxEnergy: 100, Currency: r.StartingCurrency, Skills: map[string]int{}, Training: map[string]int{}, Inventory: map[string]int{}, Equipment: map[string]string{}, Assets: map[string]AdventureAsset{}, Defeated: map[string]bool{}}
	for _, id := range r.StartingSkills {
		a.Skills[id] = 1
	}
	for id, n := range r.StartingItems {
		a.Inventory[id] = n
	}
	return a
}

// AdventureActorFromState reads the canonical actor value from a Story projection.
func AdventureActorFromState(state map[string]any, actorID string) (AdventureActor, error) {
	var a AdventureActor
	v := getPath(state, actorStateFieldPath(actorID, AdventureStateField))
	if v == nil {
		return a, fmt.Errorf("adventure actor state is missing")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return a, err
	}
	err = json.Unmarshal(b, &a)
	return a, err
}
