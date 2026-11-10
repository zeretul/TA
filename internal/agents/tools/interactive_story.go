package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agent "ta/agent"

	"ta/internal/interactive"
)

const submitInteractiveTurnToolName = "submit_interactive_turn"
const SubmitInteractiveTurnToolName = submitInteractiveTurnToolName

type searchStoryHistoryInput struct {
	Keywords     []string `json:"keywords,omitempty" jsonschema:"description=Character, location, item, clue, or event keywords to search. Every keyword participates in matching. Omit to browse recent turns."`
	Match        string   `json:"match,omitempty" jsonschema:"description=Keyword matching: any matches at least one keyword; all requires every keyword. Default any."`
	BeforeTurnID string   `json:"before_turn_id,omitempty" jsonschema:"description=Search only current-branch history before this turn_id, preventing the current turn from being treated as an earlier fact."`
	Limit        int      `json:"limit,omitempty" jsonschema:"minimum=1,description=Desired page size, default 8. The shared tool-result byte budget may reduce the actual result count."`
	Cursor       string   `json:"cursor,omitempty" jsonschema:"description=Opaque next_cursor from the previous page; valid only for an identical search."`
}

// interactiveTurnCheckToolInput deliberately omits model-authored
// outcomes.state_changes. Deterministic State Bindings produce rule state
// changes; all remaining state mutations are submitted after the narrative.
type interactiveTurnCheckToolInput struct {
	Action       string                            `json:"action" jsonschema_description:"The action the player actually attempts this turn."`
	Intent       string                            `json:"intent" jsonschema_description:"The goal the player intends to achieve through this action."`
	Challenge    string                            `json:"challenge" jsonschema_description:"The risk, obstacle, or conflict requiring a fixed d20 ruling."`
	Cost         string                            `json:"cost" jsonschema_description:"Potential consequences such as failure, exposure, resource use, or relationship loss."`
	State        string                            `json:"state" jsonschema_description:"Only visible state, resources, position, relationships, or constraints directly relevant to this check."`
	Adjudication interactive.TurnCheckAdjudication `json:"adjudication,omitempty"`
	Rule         interactive.TurnCheckRule         `json:"rule,omitempty"`
	Bonuses      []interactive.TurnCheckBonus      `json:"bonuses,omitempty"`
	Difficulty   string                            `json:"difficulty" jsonschema:"enum=very_easy,enum=easy,enum=normal,enum=hard,enum=very_hard"`
	Outcomes     interactiveTurnCheckToolOutcomes  `json:"outcomes"`
}

type interactiveTurnCheckToolOutcomes struct {
	CriticalSuccess interactiveTurnCheckToolOutcome `json:"critical_success"`
	Success         interactiveTurnCheckToolOutcome `json:"success"`
	Failure         interactiveTurnCheckToolOutcome `json:"failure"`
	CriticalFailure interactiveTurnCheckToolOutcome `json:"critical_failure"`
}

type interactiveTurnCheckToolOutcome struct {
	Result string `json:"result" jsonschema_description:"Final consequence that prose must follow when this tier is selected."`
}

func (input interactiveTurnCheckToolInput) request() interactive.TurnCheckRequest {
	return interactive.TurnCheckRequest{
		Action: input.Action, Intent: input.Intent, Challenge: input.Challenge, Cost: input.Cost, State: input.State,
		Adjudication: input.Adjudication, Rule: input.Rule, Bonuses: input.Bonuses, Difficulty: input.Difficulty,
		Outcomes: interactive.TurnCheckOutcomes{
			CriticalSuccess: interactive.TurnCheckOutcome{Result: input.Outcomes.CriticalSuccess.Result},
			Success:         interactive.TurnCheckOutcome{Result: input.Outcomes.Success.Result},
			Failure:         interactive.TurnCheckOutcome{Result: input.Outcomes.Failure.Result},
			CriticalFailure: interactive.TurnCheckOutcome{Result: input.Outcomes.CriticalFailure.Result},
		},
	}
}

func newInteractiveHistoryTools(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	ctx.BranchID = strings.TrimSpace(ctx.BranchID)
	if ctx.MaxResultBytes <= 0 {
		ctx.MaxResultBytes = defaultToolResultMaxBytes
	}
	if ctx.Store == nil || ctx.StoryID == "" {
		return nil, nil
	}
	searchTool, err := agent.InferTool("search_story_history", "Search committed historical turns on the current branch. Turn events are the source of historical truth. Results contain only bounded player actions, narrative excerpts, state changes, and exact turn_id values and can be rebuilt from the event log. Use this to continue earlier characters, locations, clues, promises, or causality. Never treat results as current Actor State or future Director planning.", func(callCtx context.Context, input searchStoryHistoryInput) (string, error) {
		_ = callCtx
		result, err := ctx.Store.SearchStoryHistory(ctx.StoryID, ctx.BranchID, interactive.StoryHistorySearchRequest{
			Keywords:     input.Keywords,
			Match:        input.Match,
			BeforeTurnID: input.BeforeTurnID,
			Limit:        input.Limit,
			Cursor:       input.Cursor,
			MaxBytes:     ctx.MaxResultBytes,
		})
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(result)
		return string(data), err
	})
	if err != nil {
		return nil, err
	}
	descriptor := boundedReadDescriptor(ToolSourceHistory, "", agent.ToolResultRecoveryRerun)
	if ctx.MaxResultBytes > 0 {
		descriptor.MaxResultBytes = ctx.MaxResultBytes
	}
	definedSearchTool, err := defineTool(searchTool, descriptor)
	if err != nil {
		return nil, err
	}
	generated, err := newInteractiveGeneratedDefinitionTool(ctx)
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{definedSearchTool, generated}, nil
}

func newInteractiveTurnTools(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	if ctx.PrepareTurn == nil && ctx.SubmitTurnResult == nil {
		return nil, nil
	}
	tools := make([]agent.ToolDefinition, 0, 2)
	if ctx.PrepareTurn != nil {
		desc := strings.Join([]string{
			"Execute one fixed d20 rule check for this turn. The Interactive Agent provides the action, intent, challenge, cost, relevant current state, pre-roll adjudication, runtime bonus sources and values, difficulty, and critical-success/success/failure/critical-failure consequences. This tool rolls, applies advantage or disadvantage, computes the target, resolves the tier, and returns the selected final consequence.",
			"Protocol: difficulty is very_easy/easy/normal/hard/very_hard; use normal for ordinary difficulty, never medium/moderate. adjudication explains why a check is required, the stakes, the difficulty basis, and the advantage/disadvantage basis. State references use actor_id + field_id in state_refs. rule is optional; when present, template is dice_check and roll_mode is normal/advantage/disadvantage. When a TRPG rule catalog is available, provide its exact template_id; the backend resolves fixed template values and audit metadata.",
			"When context provides a TRPG check configuration, first use trigger, must_check_examples, and skip_check_examples to decide whether to check, then use difficulty_guidance for difficulty/bonuses. The four outcomes describe narrative consequences only and do not include state operations.",
			"When state_bindings are available, choose binding_id and provide actor_id plus target_actor_id when needed. The tool reads Actor State to calculate binding modifiers and outcome_state_changes; do not calculate them again. narrative_state_refs only help write the four outcomes.*.result values before the roll.",
			`Minimal example: {"action":"pick the lock","intent":"enter the warehouse","challenge":"open it before the patrol arrives","cost":"failure reveals the intrusion","state":"The protagonist has simple tools.","adjudication":{"reason":"Time pressure and failure would change the alert state.","stakes":"Failure brings the patrol closer.","difficulty_reason":"The old lock is simple but a patrol is nearby, so use normal difficulty.","roll_mode_reason":"The tools fit but the environment is tense, so roll normally.","state_refs":[{"actor_id":"protagonist","field_id":"stamina"}]},"rule":{"template_id":"dm-osr-player-skill","label":"OSR player-skill priority","failure_policy":"blocked"},"bonuses":[{"kind":"equipment","reason":"Simple lock-picking tools","value":2}],"difficulty":"normal","outcomes":{"critical_success":{"result":"Open it silently and find an extra clue."},"success":{"result":"Open it, but lose time."},"failure":{"result":"The lock stays shut and the patrol draws closer."},"critical_failure":{"result":"The tool breaks and alerts the patrol."}}}`,
		}, "\n")
		prepareTool, err := agent.InferTool("prepare_interactive_turn", desc, func(callCtx context.Context, input interactiveTurnCheckToolInput) (agent.ToolResult, error) {
			resolution, err := ctx.PrepareTurn(callCtx, input.request())
			if err != nil {
				return agent.ToolResult{}, err
			}
			modelData, err := json.MarshalIndent(resolution.ModelToolOutput(), "", "  ")
			if err != nil {
				return agent.ToolResult{}, err
			}
			displayData, err := json.MarshalIndent(resolution.ToolOutput(), "", "  ")
			if err != nil {
				return agent.ToolResult{}, err
			}
			return agent.ToolResult{
				ModelContent:   string(modelData),
				DisplayContent: string(displayData),
				Details:        json.RawMessage(displayData),
				Status:         agent.ToolResultSuccess,
			}, nil
		})
		if err != nil {
			return nil, err
		}
		definedPrepareTool, err := defineTool(prepareTool, interactiveStoryWorkflowDescriptor())
		if err != nil {
			return nil, err
		}
		tools = append(tools, definedPrepareTool)
	}
	if ctx.SubmitTurnResult != nil {
		desc := strings.Join([]string{
			"After outputting all player-visible prose, submit this turn's state_changes and choices through one entry point. Include both in the first call. The current runtime context states whether Game Agent planning is enabled. When ready=false, resubmit the modules named by retry_modules; a module that is still unsubmitted but that the locked prose settles — world above all — belongs in the same call, because the prose cannot settle what the turn never submits. Accepted modules are retained. End immediately when ready=true without repeating or rewriting prose.",
			"When initialize_story_state_schema is available, finalize the schema draft before prose. The first state_changes must fill every field in initialization_guide.required_state_changes together with other major state established by prose. Do not use empty, unset, unknown, or pending placeholders. This tool does not replace schema initialization.",
			fmt.Sprintf("Submit state_changes as a native JSON array, never as a JSON.stringify string. A normal turn should usually stay within %d items; this is not a validation limit, and a complex opening or genuinely larger fact set may exceed it. Never drop important state established by prose merely to reduce the count. Each item uses exactly one operation: replace={op,actor_id,field_id,value,optional subpath}, delta={op,actor_id,field_id,value,optional subpath}, create={op,actor_id,template_id,name,optional role/description/initial_state}, or archive/restore={op,actor_id,reason}. create has no field_id, subpath, or value. Put every initial field for a new Actor in one create.initial_state instead of replacing fields before creation. archive is only for an Actor confirmed dead or permanently gone whose history must remain; restore is only for returning an archived Actor. Both require established factual reasons and must not be inferred automatically from health or narrative wording. Include only fields changed in prose, use exact IDs from the Actor State Handbook, and do not repeat fields consumed by RuleResolution.", recommendedTurnStateChangesPerSubmission),
			"Creating an Actor requires name, and actor_id must exactly equal name in the story language. Do not invent an English, romanized, or slug ID. For an existing Actor, copy actor_id exactly from the state handbook.",
			"Every turn must replace story_context actor_id=story, field_id=当前事件. Also replace 当前详细地点 when it is uninitialized or prose establishes a location change. Do not write empty values to unchanged fields.",
			"choices must match the prose ending and contain exactly the number of distinct suggestions configured for the current story. Submit an empty array only for a terminal turn whose prepare_interactive_turn result has terminal_candidate.",
			"When a module is rejected, repair the same intended state facts. Do not bypass validation by deleting an important character, ability, item, location, or situation already established in prose. You may merge a new Actor's initial_state or compress redundant descriptions.",
			fmt.Sprintf("When planning is enabled, plan_update maintains private future intent as Markdown up to %d bytes. If no plan exists, initialize it with mode=replace_document and follow the injected planning template. For routine changes to an existing modular plan, prefer mode=replace_sections and send only changed section bodies; headings must copy existing unique H2 text exactly. Use replace_document for major replans or any heading, order, or module change. Otherwise omit plan_update while the plan remains useful. Valid sibling section edits are retained when another section is rejected, so retry only retry_sections. When planning is disabled, omit plan_update.", 64*1024),
			"Use the current turn's Actor State Handbook as the authority for the complete parameter template, available IDs, field types, and the number of choices placeholders matching this story's choice_count.",
			"recap is the turn's own recollection for the player's history list, written in the story's language. Submit one short paragraph of about 40–100 characters describing what the protagonist actually did, what they learned and how the situation changed. A quiet turn may be shorter. Do not retell the scenery, do not preview what might happen next, and do not write a guess as if it were established. It is optional and never blocks the turn: an absent recap is not a missing module, and the history simply shows an excerpt of the prose instead.",
			"character_facts records what this turn established about people, and it is what lets a conversation continue between the story and the character window. Submit profiles only for a character appearing formally for the first time, or whose identity, role, attitude or circumstances changed; copy character_id exactly from the Established Characters block for anyone already listed, and mint a short stable id in the story's language for a new arrival. Submit exchanges for lines the prose actually gave a named character, in order, with the channel they happened on: scene for speech inside the narrative, private for a one-to-one conversation, message for something the character sent on their own initiative. Set addressed_to on a line the prose aimed at one particular person, using that person's exact character id or protagonist; leave it out for a line said to the room. It is what records who the words were directed at, since character_id names the speaker for that character's own line. Submit knowledge for what a character came to know and how they came to know it, and only for characters who could actually perceive it. Submit matters for unfinished business the turn created or settled: a permission actually granted, a promise made, information disclosed, a misunderstanding formed, or a durable change of stance. A permission is not the act it permits and a plan is not a completed trade, so do not record an arrangement as carried out until the prose carries it out. Submit messages for a character who wrote to the protagonist on their own initiative this turn, each with the reason they wrote; this is not a per-turn routine, so send one only when the turn's events, an existing appointment, an unfinished matter or their own situation gives a concrete reason, and never let a message decide or accept anything for the protagonist. Omit character_facts entirely when the turn introduced nobody, settled nothing, disclosed nothing and nobody wrote; that is the common case. Submit affinities only when this turn's events actually changed how a character feels about the protagonist: they helped, kept a word, slighted them, deceived them, or did something that speaks to what that person actually values. One entry per complete interaction, never one per line: do not split a rescue into arrival, talk, fight and farewell, and do not score the same fact again because a task reward or a private conversation thanks them for it a second time. Kind names the size of the interaction and delta must lie inside that size's own range; the backend computes the value, so never state a total. Ordinary courtesy, repeated praise, a gift already given and a retelling of something already counted are worth nothing, and a character who did not perceive it cannot be moved by it. memberships只记录主角本人实际知情的势力身份，pending不等于加入；formal/temporary须有已冻结接纳权的在场本人定向回应及玩家本轮真实申请原话，left撤回资格，不授指挥权或服从。 relationships只记录实际成立或解除的正式主角关系。建立关系必须引用本回合先提出、后明确接受的两条不同话语，由主角和本人各说一句；scene的addressed_to须明确指向对方，不能引用旁人或全场发言。合理请求、高好感、沉默、拒绝和附条件回答不等于接受，不能替玩家补写同意。 Affinity is a reference for how someone behaves, never a permission: it never produces an item, a technique, an authorization or a consent the world has not otherwise established.",
			"When the story is a cultivation story and the turn covers time a character actually spends training, set cultivation.days to that real elapsed time; omit the module entirely when the turn is not a training interval. Set attempt_breakthrough only when the player explicitly attempts to enter a realm and the rules' preconditions are met, and never to make a dramatic moment land. Omit cultivation entirely in the common case.",
			"rest records time the character really spends resting or regulating, and it is the only way that time is spent. Set duration_seconds to the in-fiction seconds the player actually asked for — 在客栈休养一天 is 86400, 就地调息半个时辰 is 3600 — and set mode to health_rest for ordinary rest that restores health or safe_meditation for safe regulation that restores mana. The two pools are separate: rest never also restores mana and meditation never heals health, so one turn carries one mode. Never invent a duration the player did not ask for, and never let a turn in which the prose says the character rested or meditated go out without this module, or the prose will promise a recovery the rules never applied and the world will not have moved. The rules decide whether the character may rest at all — in battle, in danger, dead or needing treatment are refused — and how much the interval restores; do not state a restored amount, a new maximum or a number of hours in state_changes. When this module is set, the pool it restores is written by the rules and a state_changes write to that same pool is refused, so a wound or an expenditure recorded in the same turn belongs on the other pool. More generally, 生命 and 灵力 may be lowered in state_changes — a wound or an expenditure is a fact the prose records — but never raised: only this module's interval restores a pool, and a turn that heals in prose without submitting it is refused. A rest that a real boundary interrupts stays owed and the next rest of the same mode continues it. Do not train and rest in the same turn: a rest is not a training interval. Omit rest entirely when the turn spent no time resting; that is the common case.",
			"craft records time the character really spends working at a furnace, bench, brush or array, and it is the only way a craft's product comes into being. Set method_id to a registered craft method exactly as the Craft Methods block lists it, and duration_seconds to the in-fiction seconds the player actually asked to spend — 炼一炉丹 is the method's own batch length times the batches the interval pays for. A profession such as 丹修 or 器修, a recipe that only appeared in conversation, and a product the player wants are none of them a method, so none of them may be named here. Everything else is the rules' answer and must never be stated: how many whole batches the interval pays for, which material copies it spends, whether the method's loss rate takes a batch, and what the product is. Never name a product, a quantity or a quality, never write 重要物品 in the same turn, and never let prose say a pill was produced without this module — a batch that could not be afforded or a stage the character has not reached is refused and writes nothing at all. Do not craft and train, rest or take a world action in the same turn: each of those spends the same interval of in-fiction time. Omit craft entirely when the turn worked no craft; that is the common case.",
			"techniques records cultivation methods (功法) — the lasting practice that produces a character's attributes, not the skills it is cast through. Set learn with the real minutes the character spends studying, and only when they genuinely study; a method completes only when the accumulated time reaches its standard, so a manual read or a claim that someone learned it grants nothing. Do not write study into the prose without setting learn for it: when the narration covers a stretch of time spent on a method, the minutes that stretch covers are the minutes to submit, and omitting them leaves the character where they started however the prose reads. Set circulate or stop only for methods the character has already mastered. A method that completes its study this turn may be circulated in the same turn: the rules apply the study before the switch, so set both. Stopping keeps the mastery and only ends the circulation. A method's percentages, growth and quality belong to the rules and must never be restated in state_changes. Omit techniques entirely when the turn neither studied nor switched a method; that is the common case.",
			"items records what a character really obtained and what they took into hand. Set acquire with a registered def_id and the name the story calls it; the name becomes the key the copy is recorded under, so a second copy of the same name is more of the same stack. Do not also acquire a copy this turn's trade module buys: a purchase is recorded by trade, which prices it and deducts the total, so listing it in both would record goods the purse never paid for. Set equip with the instance_id that copy is recorded under, and set unequip with the slot to empty (主法器, 护具, 布阵, 攻击法宝, 防御法宝 or 辅助法宝). Equipping moves a copy the character already holds: it never creates one, so a sword the prose says they picked up must be acquired before it can be equipped. The rules check that the definition is registered and that the character has reached the item's applicable stage — a 顶品 treasure written into a 炼气 character's hands is refused, because 适用阶段不是点击颜色就能越过. Deploying a 阵盘 costs 10% of the operator's stage base mana to raise and 2% per exchange to keep, so a formation the character cannot pay for is refused rather than recorded; dismantle it with unequip when upkeep fails, when the character leaves its area, or when the 阵盘 is broken. Grades, rarity, budgets and bonuses belong to the registered definition and must never be written into the slot by hand: 主法器, 护具, 布阵, 攻击法宝, 防御法宝 and 辅助法宝 are rules-owned fields now, so write them through this module rather than through state_changes. The three 法宝 slots are one each and do not replace one another, and one real object occupies one slot only: a copy already worn somewhere else must be unequipped before it can be put in another slot. Equipping is not activating — a treasure's own registered capability, its conditions and its mana cost still decide what it does, and three equipped treasures never become three extra major actions in an exchange. A pill is not equipped; it is carried, and it is taken either in battle or, outside one, through use. Set use with the instance_id a carried pill is recorded under when the character really swallows it outside a fight — the case that matters after one, when 生命 will not rise any other way: the rules spend one dose and restore the pool by the grade's own amount, and they refuse the dose on a full pool rather than charging for nothing. An attack pill lasts three exchanges and a support pill is spent by a training interval, so neither is settled here. Omit items entirely when the turn obtained, equipped, took off and swallowed nothing; that is the common case.",
			"trade 仅用于凡人修仙交易。武侠、星海、西幻报价后使用 world.action=adventure 及 buy/sell，沿用报价返回的 market_id、item_id 和数量。trade records goods and coin that actually changed hands this turn. Set merchant to the shop or trader's name in the story's words, and reuse exactly the same name for every later deal with them: it is the key their prepared cash position is recorded under. Set buys with a registered def_id, the name the story calls the copy, and a quantity; set sells with the instance_id the copy is recorded under in 重要物品 and a quantity. Never write a price: retail is the definition's registered base price adjusted by the market factor, buyback is that price times the merchant's own buyback factor, and both are calculated by the rules. A price merely heard, asked about or negotiated is not a purchase, and the story may quote a number only after calling quote_trade. Set payments when the character hands coin to a person rather than buying from a shop — a toll, a bribe, a share, a debt paid, a price agreed with someone standing on the road: name the recipient's Actor id in to and the whole number of 灵石 in amount, with no merchant. The rules check the definition, the character's real balance, whether the copy is really held, whether the merchant's real cash covers the buyback and whether a payment's recipient is an Actor whose own 灵石 the rules can write; a line that fails writes nothing at all, so there is never a partial deduction. An agreement is not a payment: only coin the prose really counted out this turn belongs in payments, and 欠账、承诺、待结款 stay in the narrative as promises. The purse (灵石) and the merchant record are rules-owned, so never write either through state_changes — a payment that only moves coin between two characters is recorded here and nowhere else. Omit trade entirely when nothing changed hands; that is the common case.",
			"world每回合必交：只结算玩家本轮实际选择的一种世界行动；没有则action=none，不可遗漏。journey复制World Actions的route_id（省略仅继续唯一未完旅程），通常arrive=true一次走到；仅玩家要求中途停留时用target_minutes。行路耗时与地点由规则结算，不再用state_changes写当前详细地点。quest复制task_id与实际quest_action：接下差事是accept，不是settle；只有真正完成并领取才发一次报酬。public_event复制event_id，join时按实际意图选observe/help/investigate；由规则核地区和时刻。realm进入时复制realm_id；其余用已知instance_id，move只走当前ways给出的connection_id或destination_area_id，reward仅用于仍列为可领取的旧场地物品。新场地调查、准备、选路或处置分别为survey/prepare/approach/resolve，复制当前可见方法的approach_id；服务核验已持有工具、已学技能、真实耗材和固定耗时，所得自动入库存，不能再提交reward或Acquire重复领取。方法触及风险线会留下受挫而非死亡，已有所得保留；不可替玩家强选深入，调查、取部分就走与暂不进入同样合法。退出先走已知退路到出口，重进仍须在真实外部入口，退出不会重置压力/选择/所得。回访只看正式StoryClock，现实等待不推进。wait仅用于玩家明确要求经过时间，duration_seconds为正且source_text逐字引用本轮请求；不得从正文时间标签推算。任何正式耗时行动都不与rest/cultivation/craft并用。所有资格和可行性以服务返回为准，拒绝不能在正文冒充已经成功；未建模创意可以演绎，但不能凭正文自造资源、通路或资格。",
			"skills records techniques a character genuinely obtained this turn. Learn an already published technique with def_id and its version. To invent a technique the world does not already have, set template_id to an approved template and name to the technique's name in the story's words, and leave version empty: every number — power, mana, burden, targets, preparation, focus requirement, shield, residual — comes from the template and must never be restated or adjusted, and a quality/rarity that disagrees with the template's published pair is refused rather than raised. " + "可选模板及数值只能取自本局世界规则与当前可用来源；未列出的模板不得猜测或借用其他世界。" + " The definition ids, their versions and every number that follows from them belong to the rules, not to the prose: never invent a definition id, never describe a technique as learned when the character only read about it or watched it, and never restate its power, cost or burden in state_changes. Omit skills entirely when the turn taught nobody a technique; that is the common case.",
		}, "\n")
		submitTool, err := newSubmitInteractiveTurnTool(desc, ctx.SubmitTurnResult, ctx.RequestTurnCompletion, ctx.ResetNarrativeCandidate)
		if err != nil {
			return nil, err
		}
		definedSubmitTool, err := defineTool(submitTool, interactiveStoryWorkflowDescriptor())
		if err != nil {
			return nil, err
		}
		tools = append(tools, definedSubmitTool)
	}
	return tools, nil
}

// NewInteractiveTurn builds the rule-resolution and turn-submission tools for
// one story-scoped Agent run.
func NewInteractiveTurn(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	return newInteractiveTurnTools(ctx)
}

type submitInteractiveTurnToolSchema struct {
	StateChanges   []interactive.TurnStateChangeInput   `json:"state_changes,omitempty" jsonschema_description:"Incremental Actor state changes established by this turn's prose. Submit a native JSON array, never a serialized string. Submit an empty array when nothing changed."`
	Choices        []string                             `json:"choices,omitempty" jsonschema_description:"The configured number of distinct next-action suggestions. Use an empty array only when RuleResolution declared terminal_candidate."`
	CharacterFacts *interactive.TurnCharacterFactsInput `json:"character_facts,omitempty" jsonschema_description:"Character-side facts this turn's prose established: who appeared formally for the first time, what a named character said, learned or was promised, granted or left unresolved, and whether the turn changed how anyone feels about the protagonist or a formal bond. For a first formal appearance, submit profiles and mint a short stable character_id in the story's language; this includes a continuing nonhuman companion when the turn establishes one identity worth remembering. For anyone already in the Established Characters block, copy that exact character_id. A first appearance is established by the turn's actual scene, not by the player's unaccepted wish. Omit when the turn established no new identity or character fact."`
	Cultivation    *interactive.TurnCultivationInput    `json:"cultivation,omitempty" jsonschema_description:"The authorised cultivation interval this turn covers, and any authorised major breakthrough. Set days to the real elapsed time the prose covers when the character actually spends it training; omit when the turn is not a training interval. Set attempt_breakthrough only when the player explicitly attempts to enter a realm and the conditions are met. Do not describe training in prose without setting days, and do not set days for time the character spent doing something else."`
	Techniques     *interactive.TurnTechniqueInput      `json:"techniques,omitempty" jsonschema_description:"Cultivation methods (功法) this turn studied, started or stopped. Set learn with the real minutes actually spent studying; a method completes only when the accumulated time reaches its standard. Set circulate or stop for methods already mastered, including one that completes its study this turn. Omit when the turn neither studied nor switched a method."`
	Trade          *interactive.TurnTradeInput          `json:"trade,omitempty" jsonschema_description:"Goods and coin that actually changed hands this turn. merchant names the shop and must be spelled the same way on every deal with them; buys name a registered def_id plus the story's name for the copy; sells name the copy recorded under 重要物品; payments name a person's Actor id and the whole 灵石 really handed to them outside a shop deal, such as a toll, a bribe, a share or a repayment. No price is supplied here: retail and buyback are calculated from the registered definition and the merchant's prepared cash, and the rules refuse a line the character cannot pay for, a copy they do not hold, a merchant whose cash cannot cover the buyback, or a payment to someone the rules cannot write a purse for. A refused line writes nothing, and an agreed price that has not been paid is a promise, not a payment. Omit when nothing changed hands; never write 灵石 or the merchant record through state_changes."`
	Items          *interactive.TurnItemsInput          `json:"items,omitempty" jsonschema_description:"仅凡人修仙的物品、法器和丹药使用本模块。obtain须有已登记定义及真实来源，equip使用实际持有的副本，unequip卸下实际装备，use服用实际持有丹药。归属、资格与恢复由规则判定。武侠、星海和原创西幻的装备、使用必须省略本模块，使用world.action=adventure中的equip/unequip/use及已持有item_id；机械部署使用deploy。"`
	Skills         *interactive.TurnSkillLearningInput  `json:"skills,omitempty" jsonschema_description:"Techniques a character genuinely obtained this turn. Learn a published technique with def_id and version. Invent one with template_id plus name, leaving version empty: every number comes from the approved template and cannot be supplied or adjusted here. Omit when the turn taught nobody a technique; do not list a technique the prose merely mentioned, quoted or had someone read about. The rules decide whether the definition exists and whether the character may hold it, and report a refusal with the reason."`
	Rest           *interactive.TurnRestInput           `json:"rest,omitempty" jsonschema_description:"仅凡人修仙休养使用本模块：mode取health_rest或safe_meditation，duration_seconds为玩家明确授权的秒数。武侠、星海和原创西幻必须省略本模块，改用world.action=adventure、world.adventure.action=rest及hours。恢复量由规则结算，不能自行给出恢复数值或重复计算同一时段。"`
	Craft          *interactive.TurnCraftInput          `json:"craft,omitempty" jsonschema_description:"仅凡人修仙的craft_methods制作使用本模块，method_id复制Craft Methods目录，duration_seconds为玩家实际授权的秒数。批次、耗材、损耗和成品由规则决定，不能手写重要物品。武侠、星海、原创西幻的recipes制作须省略本模块，改用world.action=adventure、world.adventure.action=craft并填写recipe_id和hours。与修炼、休养及其他world行动互斥。"`
	World          *interactive.TurnWorldActionInput    `json:"world,omitempty" jsonschema_description:"每回合的正式世界行动。journey/quest/public_event/realm使用世界行动目录；武侠、星海及原创西幻采用adventure并填写adventure与source_text原话，制作使用adventure.action=craft、recipe_id、hours而非顶层craft；技能和物品资格以冻结目录为准；战斗回合仅由独立窗口提交。wait需正数duration_seconds与玩家原话。无行动填none；同回合不能同时修炼、休养或制作。"`
	PlanUpdate     *interactive.TurnPlanUpdateInput     `json:"plan_update,omitempty" jsonschema_description:"Only when Game Agent planning is enabled. Initialize or restructure with replace_document; routinely update existing unique H2 bodies with replace_sections. Omit while the current plan remains useful."`
	Recap          *string                              `json:"recap,omitempty" jsonschema_description:"One short paragraph in the story's language recalling what this turn actually published, shown in the player's history list. Reference length is 40–100 characters, and a quiet turn may be shorter. Say what the protagonist actually did, what they learned, and how the situation changed — do not retell scenery, do not preview the next turn, and do not state a guess as a fact. It carries no acceptance state: the turn commits with or without it, and an absent recap is not a module awaiting repair."`
}

const recommendedTurnStateChangesPerSubmission = 24

type submitInteractiveTurnTool struct {
	info              *agent.ToolInfo
	submit            func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)
	requestCompletion func(context.Context) bool
	resetNarrative    func(context.Context)
}

func newSubmitInteractiveTurnTool(
	description string,
	submit func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error),
	requestCompletion func(context.Context) bool,
	resetNarrative func(context.Context),
) (agent.Tool, error) {
	info, err := agent.GoStruct2ToolInfo[submitInteractiveTurnToolSchema](submitInteractiveTurnToolName, description)
	if err != nil {
		return nil, err
	}
	parameters, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		return nil, err
	}
	if parameters == nil || parameters.Properties == nil {
		return nil, fmt.Errorf("submit_interactive_turn schema missing root properties")
	}
	stateChanges, ok := parameters.Properties.Get("state_changes")
	if !ok || stateChanges == nil {
		return nil, fmt.Errorf("submit_interactive_turn schema missing state_changes")
	}
	stateChanges.Description = fmt.Sprintf(
		"%s A normal turn should usually stay within %d items; this is not a validation limit, and a complex opening or genuinely larger fact set may exceed it.",
		strings.TrimSpace(stateChanges.Description),
		recommendedTurnStateChangesPerSubmission,
	)
	info.ParamsOneOf = agent.NewParamsOneOfByJSONSchema(parameters)
	return &submitInteractiveTurnTool{info: info, submit: submit, requestCompletion: requestCompletion, resetNarrative: resetNarrative}, nil
}

func (t *submitInteractiveTurnTool) Info(context.Context) (*agent.ToolInfo, error) {
	return t.info, nil
}

func (t *submitInteractiveTurnTool) Run(ctx context.Context, argumentsInJSON string, _ ...agent.ToolOption) (agent.ToolResult, error) {
	input := interactive.DecodeInteractiveTurnSubmissionInput(argumentsInJSON)
	receipt, err := t.submit(ctx, input)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if receipt.NarrativeRewriteRequired {
		// The prose this run retained narrates a success the rules just denied.
		// Dropping it is what lets the next model output become the narrative;
		// otherwise the protocol would keep treating the refused text as the
		// locked first candidate and the turn could never finish.
		if t.resetNarrative != nil {
			t.resetNarrative(ctx)
		}
		slog.InfoContext(ctx, fmt.Sprintf("[interactive-turn] narrative rewrite required reason=%q", receipt.NarrativeRewriteReason))
	}
	if receipt.Ready {
		requested := false
		if t.requestCompletion != nil {
			requested = t.requestCompletion(ctx)
		}
		slog.InfoContext(ctx, fmt.Sprintf("[interactive-turn] accepted all result modules completion_requested=%t", requested))
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return agent.ToolResult{}, err
	}
	result := agent.TextToolResult(string(data))
	result.Details = data
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
