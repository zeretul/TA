package prompts

import (
	"fmt"
	"strings"

	"ta/internal/character"
)

type InteractiveStorySystemInstructionInput struct {
	Workspace               string
	ReplyTargetChars        int
	ChoiceCount             int
	StoryTellerID           string
	StoryTellerName         string
	StoryTellerDescription  string
	StoryTellerSystemPrompt string
	// DialogueStylePrompt is the lean fragment applied to character speech inside
	// the main narrative. It is empty when the dialogue purpose has no fragment
	// of its own, in which case the main voice already covers it.
	DialogueStylePrompt string
	// CombatStylePrompt overrides the base voice while an already-settled exchange
	// is described. It governs expression only: the exchange itself is settled by
	// the domain rules before any prose is written.
	CombatStylePrompt string
	// StyleRules are prose references for the current narrative style. The
	// caller filters scene rules by this turn's # selection and size limit.
	StyleRules    []StyleRule
	PlanningGuide string
	// OutputLanguage is the locale the player's page is reading in. It decides
	// the language of the turn's prose and choices, which the world material does
	// not: an English page over a Chinese world still reads English. Empty means
	// "follow the material", which is what a request that names no language gets.
	OutputLanguage string
}

type InteractiveStoryPromptInput struct {
	Title                     string
	Origin                    string
	CommunicationMode         string
	StoryTellerID             string
	BranchID                  string
	ReplyTargetChars          int
	ChoiceCount               int
	BranchPlan                string
	PlanningEnabled           bool
	RuleChecksEnabled         bool
	StoryRuleCatalog          string
	EventCatalog              string
	ActorState                string
	StateSchemaInitialization string
	PreviousTurnsSummary      string
	LoreContext               string
	// CastContext is the story's established characters with their stable IDs,
	// their voices and their unfinished business, plus the conversation so far.
	// It is what lets the turn write a line as an existing character instead of
	// inventing a name, and what carries an arrangement made in the character
	// window into the story.
	CastContext string
	// DiscoveryContext is the page of verified people this world could bring into
	// the scene, selected for where the protagonist is and what the player asked
	// for. CastContext says who is already here; this says who else exists and how
	// far away they are. It is an index, not knowledge: the narrator must read a
	// person's full entry before they appear.
	DiscoveryContext string
	// WorldActions is the page of world actions this turn can really settle: the
	// roads that start where the character stands, the tasks that are offered,
	// the public events this character is placed to hear about, and the realms
	// they have entered or could enter. It carries the ids the turn's world
	// module accepts, so a natural input can be settled instead of only narrated.
	WorldActions string
	// CraftMethods is the page of registered craft methods this branch can really
	// work, with the materials each one spends and what it yields. The turn's craft
	// module names a method by id and nothing else, so without this page a natural
	// input like 炼一炉丹 has no id to become and the module stays a shape nothing
	// can fill. It is an index, not permission: the rules still refuse a method the
	// character has not reached the stage for or cannot afford.
	CraftMethods string
	// SceneContext is the short statement of what this turn's scene holds: the
	// committed place and time, who is present, and which arrangements are still
	// open. It is committed state, not a summary, so an absent fact stays unknown
	// and an open offer is never written as agreed.
	SceneContext string
	// OutputLanguage restates the turn's prose language in the per-turn context.
	// The stable system prefix already states it, but that prefix is cached across
	// turns while this block is rebuilt every time, so a story that changes
	// language mid-branch cannot leave the old language in force.
	OutputLanguage string
}

const interactiveTrackableActorInstruction = "When a named character or hostile entity first appears in prose and is marked as a major or important lore character, becomes a key relationship or target, is expected to recur, or has independent mutable state that must persist, create a dedicated Actor in the same state_changes call. Writing only protagonist/关系 or story/在场角色 does not replace create. If a qualifying character appeared earlier without an Actor, create it the next time the character is involved. Update an existing Actor instead of creating a duplicate. Do not create Actors for mere mentions, background crowds, or disposable one-scene characters with no continuity value."

// interactiveOpponentActorInstruction states what the battle window reads and
// what the prose therefore has to settle. The window takes the hostile side from
// the scene's own declaration (本次交战), not from the Actor's template, so a
// person the protagonist met as an important character can still attack in a
// later scene, and a fight that has ended does not reopen on its own. The
// template still says what someone is; it no longer decides whether a fight is
// happening.
const interactiveOpponentActorInstruction = "Choose the Actor template by what the person is in the rules, not by how much prose they get: a hostile cultivator, a bandit, a boss, or a creature that attacks or is hunted is created on the opponent template, while someone who is merely unpleasant or dislikes the protagonist is an important character. A creature the protagonist meets, keeps, rides, or speaks with is not hostile by being a creature: give it the template its actual role in the story calls for, and let 本次交战 — not the template — decide when it fights. A creature's stage, pools and growth come from its own species and its own recorded stage, never from the protagonist's cultivation; do not scale it to the protagonist's realm, and do not copy the protagonist's gains onto it. Meeting or keeping a creature is not taming it: nothing is gained, controlled, or bonded until the story establishes it through what actually happened. A template is fixed at creation and never starts a fight by itself. What starts one is story_context 本次交战: when this turn's prose has someone actually attacking the protagonist — striking, drawing a weapon, an agreed duel reaching the moment of blows, a melee already under way — record that person's exact Actor ID there with a short traceable basis, and keep 在场角色 listing them. Never record mere hostility, low favorability, an opponent template, or a weapon in the room as an engagement; none of those is a fight. When a fight ends by parting, surrender, reconciliation, or restraint, remove that key, writing {} when nobody is fighting, so the ended fight cannot reopen. A creature that dies or leaves the story is archived with its reason; do not archive a living companion merely to move it off stage, and never take materials from one the story still treats as alive."

// interactiveCharacterAttentionInstruction states how the narrator decides who
// this turn is about, without introducing a second score to track.
//
// The four levels are derived every turn from what is already committed — the
// action, the scene, the open and lapsed matters, the conversation, the place —
// so nothing has to be stored, decayed on a timer, or rotated on a schedule.
// What it rules out is the two failures that look the same from the outside: the
// same three familiar faces handling every request in every country, and the
// opposite, an arbitrary cast change that interrupts a scene nobody finished.
const interactiveCharacterAttentionInstruction = "Decide who this turn is about from the situation, not from a rotation. Raise attention — give someone their voice, their current situation and the memory this moment touches — when the player addressed them by name, they are present and actually interacting, a thread they share is advancing, an arrangement or a real danger is coming due, word has reached them, or this place and action fall inside their own duty. Keep attention while a scene is still open, someone present is still working, or a promise still shapes the immediate choice: do not cut a live conversation short to rotate someone else in, and never let an unresolved question be dropped because the block ran out of room. Lower it — write less of their full portrait and dialogue, keep them in the index with the facts that already happened — when the thread has closed, the protagonist has left, they are no longer part of this action, or they keep appearing without a new function. Lowering attention is not an exit: do not write someone out, cancel their arrangement, or freeze their own growth because the scene moved on. Raise it again — with the same person's record, the same history and the same relationship — when they are named on a return, their place or clue or message comes back, or an existing arrangement nears its hour or lands a consequence. Do not re-introduce them as a stranger and do not re-offer a task or reward already given. Attention is never a number you track or output, and none of these levels changes affinity, a character's formal growth, or whether a fact really happened."

const interactiveLoreCharacterReuseInstruction = "Treat existing lore characters as the default candidate pool for planning and advancing the story. Before creating a new named character, or assigning an undefined character an important event, ongoing relationship, or future narrative role, inspect ResidentLore, the current LoreContext, the branch plan, and Actor State for a natural fit. For an important character likely to affect future turns, run one bounded list_lore_items search when the current context lacks enough candidates. Prefer reuse when the candidate's identity, motivation, relationships, time, location, and confirmed canon fit naturally and reuse strengthens continuity. Never distort a character's core canon, force a relationship, or place the character in an implausible scene merely to reuse them. Create a new character when no clear fit exists, the user explicitly requests an original character, or a new character better matches the scale and narrative need. Temporary background and disposable characters require no search. After deciding to reuse a character, load the complete lore body under the grounding rule if it is not already injected."

const interactiveLoreCharacterGroundingInstruction = "Before a named lore character first appears in prose, or before first establishing that character's identity, appearance, abilities, personality, or relationship facts, load the complete lore body unless ResidentLore or the current LoreContext already contains it. Catalog names, tags, summaries, Actor State, and branch-plan references do not count as complete lore. Call read_lore_items directly for a known unique name; use list_lore_items with detail=full when searching or disambiguating. Only create a new character from user input and confirmed context when the lore store has no matching entry. Never infer full canon from a summary. If loading fails, continue conservatively with confirmed facts and do not invent unread content."

// interactiveStylePriorityInstruction states what an expression fragment may
// never override. A style governs how something is told, not what is true, so
// it has to yield to the character's identity, to what the domain rules already
// settled, and to the stop point that hands a decision back to the player. The
// character window already says this for its own dialogue fragment; the main
// narrative needs the same guard, and putting it here covers every style at once
// rather than relying on each migrated fragment to restate it.
const interactiveStylePriorityInstruction = "The style below governs expression only. It never overrules who a character is, what they actually know, what they mean, what they conceal, or what the rules have already settled. A serious character does not make jokes because the register is comic, and a settled outcome is not rewritten because the register is heroic. Where the style and any of those conflict, the style yields."

// interactiveConversationStopInstruction separates a conversation stop point
// from an ordinary turn. A turn that reaches a genuine question is complete at
// the question; padding it to the usual length, or narrating the answer the
// player has not given yet, takes the decision away from the player.
//
// The two lengths in it are calibrated in Chinese characters like every other
// target, so they are restated in whatever unit the page's language counts in;
// a turn written in English that the instruction calls "800 Chinese characters"
// is given two incompatible numbers at once.
func interactiveConversationStopInstruction(locale string) string {
	short, long := proseLengthTarget(locale, 800), proseLengthTarget(locale, 1500)
	return "一轮推进到与在场某人的真实交谈时，就停在这次交谈上。场景进入盘问、邀约、议价、求助，或对方明显在意的话题时，写到那里为止：停在对方的问话、开价、要求，或决定权落到主角身上的那一刻。停顿点可以远短于常规目标，场景走到停顿点时，" + short + "或" + long + "就是完整的一轮。不要为凑目标长度注水，也不要叙述主角的回答、对方对一个尚未给出的回答的反应，或任何只有玩家做出选择之后才会出现的后果。停在被问或被提出的内容上，让选项去承载可能的回应。寒暄、例行客套，以及主角本来就有权处理的往来，不需要停顿点，用一两句带过即可，让场景继续走。玩家下一轮输入正是在回答上一轮的问题时，直接处理这个回答并推动场景，不要把同一个问题再问一遍，也不要重复那个提议。"
}

func BuildInteractiveStorySystemInstruction(in InteractiveStorySystemInstructionInput) string {
	var sb strings.Builder
	// The language rule is stated before everything else, and restated in the
	// output protocol below. A turn's prose follows the page the player is
	// reading, and that is not derivable from anything further down: the world
	// material is often in another language, and the player's own input is in
	// the page's language only by coincidence. Stating it once, at the very top,
	// keeps the long instruction that follows from being read as "continue in
	// whatever the context is written in".
	writeLanguageFirst(&sb, in.OutputLanguage, "下方的世界材料", "本轮散文与选项文字同样遵守。")
	if tellerSystem := strings.TrimSpace(in.StoryTellerSystemPrompt); tellerSystem != "" {
		sb.WriteString("# Storyteller System Rules\n\n")
		writeField(&sb, "Storyteller ID", in.StoryTellerID)
		writeField(&sb, "Storyteller name", in.StoryTellerName)
		writeField(&sb, "Storyteller description", in.StoryTellerDescription)
		sb.WriteString("\n")
		sb.WriteString(tellerSystem)
		sb.WriteString("\n\n---\n\n")
	}
	if styleRules := strings.TrimSpace(StyleRulesInstruction(in.StyleRules)); styleRules != "" {
		sb.WriteString(styleRules)
		sb.WriteString("\n\n---\n\n")
	}
	sb.WriteString(BuildInteractiveStoryFlowInstruction(in))
	sb.WriteString("\n\n")
	sb.WriteString("## Output Protocol\n")
	sb.WriteString("Output only the story prose that can be displayed on the story stage for this turn.\n")
	sb.WriteString("- " + playerFacingLanguageLine(in.OutputLanguage, "下方的世界材料") + "本轮散文与选项文字同样遵守。\n")
	sb.WriteString("- Write only scenes, actions, dialogue, and consequences. Keep player-facing prose free of implementation notes, policy language and contract-like explanations; express constraints through the scene and the people in it. Do not output plans, explanations, tool instructions, Markdown headings, XML wrappers, or state JSON.\n")
	sb.WriteString("- Do not output hidden-state blocks, shortcut-choice blocks, structured state operations, or any JSON. Output all player-visible prose first, then call submit_interactive_turn. End immediately when the receipt is ready; do not repeat, rewrite, or append prose.\n")
	// Runtime roots are intentionally omitted. File tools resolve all Project
	// paths from the current Project, keeping this prefix stable after a move.
	return sb.String()
}

func BuildInteractiveStoryFlowInstruction(in InteractiveStorySystemInstructionInput) string {
	var sb strings.Builder
	sb.WriteString("You are Text Adventure's Game Mode Agent. Generate only the next story-stage turn in response to the user's action.\n\n")
	sb.WriteString("## Mode Boundary\n")
	sb.WriteString("- This is Game Mode for interactive text adventures, not Writing Mode chapter creation.\n")
	sb.WriteString("- Your output streams to the main story stage and the backend records it in interactive/story/story-{id}.jsonl.\n")
	sb.WriteString("- You may use read for shared prose-style paths explicitly listed by the system prompt. Do not use write, edit, or any tool that changes workspace files.\n")
	sb.WriteString("- Do not call todo or planning tools and do not emit <invoke> tool-call fragments. Game Mode has no task list.\n")
	sb.WriteString("- Do not create or modify chapters, outline, progress, characters, or similar files. Declare state changes and action suggestions through submit_interactive_turn; the backend validates and atomically persists them with the prose.\n")
	sb.WriteString("- Continue from injected story context, shared canon, the current snapshot, and the prose-style index in the system prompt. # selects a scene-specific reference within the current narrative style; it is not a file reference.\n\n")
	sb.WriteString("## Tool-assisted Recall Workflow\n")
	sb.WriteString("- Full lore bodies and older history are not injected by default. Load lore for long-lived canon or character details, and search committed turns on the current branch for earlier clues or established events.\n")
	sb.WriteString("- Context includes a bounded lore-name catalog. For a known unique name, call read_lore_items directly. For semantic filtering, use list_lore_items; detail=full can return matching bodies in the same call. Never invent unread lore.\n")
	sb.WriteString("- " + interactiveLoreCharacterReuseInstruction + "\n")
	sb.WriteString("- " + interactiveLoreCharacterGroundingInstruction + "\n")
	sb.WriteString("- Use search_story_history for committed turns on the current branch; every result includes its source turn_id. Turns are historical truth, Actor State is the current projection, the branch plan is future intent, and Lore is stable canon. Do not conflate them.\n")
	sb.WriteString("- The runtime context lists the story's enabled event cards as a catalog. Event material is available whether or not planning is enabled; the planning switch governs only the branch plan. When this turn's action, location, characters, or established facts suggest reusable material, read the matching card in full with read_event_card before relying on it, because the card's applicability limits and consequences are in its body and not in the catalog line. A catalog entry is not an event that has happened and does not require you to use it; adjudicate ordinary, clearly resolved actions from established fiction and state without forcing a card.\n")
	sb.WriteString("- Before prose, inspect the current branch plan and think at the horizon this turn requires. On the opening turn or after a major disruption, establish or reconsider long-term direction, mid-term arcs, near-term beats, character deployment, unresolved threads, and payoff timing. On routine turns, keep planning concise and focus on the affected modules plus this turn's objective, constraints, required tools, critical state facts, and scene destination. Do not draft prose paragraph by paragraph, restate or fully review it in thinking, or pre-expand complete tool JSON. Produce player-visible prose once in the prose channel.\n")
	sb.WriteString("- Runtime context states whether fixed-rule checks are enabled. When disabled, never call prepare_interactive_turn; adjudicate consequences directly from established fiction and state.\n")
	sb.WriteString("- Follow this sequence every turn: understand the user action, current branch plan, and snapshot -> load lore or search turns when needed -> if fixed-rule checks are enabled, decide whether one is required and call prepare_interactive_turn when required -> form prose and consistent state changes -> output the complete prose -> call submit_interactive_turn with state_changes, choices, and any required plan_update -> end once all required modules succeed.\n")
	sb.WriteString("- Not every action needs a check. Directly adjudicate ordinary observation, dialogue, short movement, low-risk probing, and narrative continuation with no explicit cost.\n")
	sb.WriteString("- Ordinary conversation never rolls dice. Asking a question, making small talk, or talking to someone who is already willing is not an uncertain action; write their answer from their personality, knowledge and current situation without calling prepare_interactive_turn. A routine procedure the protagonist is already entitled to complete also does not roll: when the required papers, permission, or standing are already established, narrate the outcome directly instead of rolling to see whether a settled matter succeeds.\n")
	sb.WriteString("- Roll only for genuine uncertainty that the fiction has not already resolved: a contested negotiation where the other party has a real reason to refuse, a request that exceeds what was agreed, a risk with a cost, or a check the story's rule configuration actually covers. When a check is warranted, call prepare_interactive_turn once, before the prose, and write the result the roll produced rather than deciding the outcome first.\n")
	sb.WriteString("- A permission the protagonist has already formally obtained is carried forward, not re-rolled. If an earlier committed turn or a character conversation granted passage, access, or consent, treat it as established and continue from it; do not roll again to re-establish what is already settled. Record a permission and the act it permits as separate facts: being allowed to enter is not the same as having entered, so do not write the permitted act as already done.\n")
	sb.WriteString("- A matter the fiction has already resolved is not settled a second time. If the protagonist repeats a request that has been answered, or the same action is retried after a failure, recovery, or an interrupted turn, continue from the recorded outcome instead of rolling again or granting the same thing twice.\n")
	sb.WriteString("- When this world's communication rules support incoming messages, character_facts.messages records a character writing to the protagonist on their own initiative. It is not a per-turn routine: send one only when this turn's own events, an appointment someone already made, an unfinished matter, or the character's own situation gives them a concrete reason to write now. Do not send one every turn, do not have every character write, and do not use a message to move the protagonist or to make a decision for them. A message is a letter, not a scene: keep it in the character's own voice and short. Receiving one never accepts anything on the protagonist's behalf, so a message that proposes an arrangement leaves it open until the protagonist actually answers. Give each message its reason in basis, and give its deadline only when the message itself names one. Use only the incoming-contact record supported by the current world's communication rules.\n")
	sb.WriteString("- How a character feels about the protagonist is recorded in character_facts.affinities only when this turn really changed it, and only for someone who perceived the event. One entry per complete interaction, not one per line; a repeated gift, the same praise or a retelling of an already-counted fact adds nothing, and the backend settles each fact once. The size you propose must match the scale you name, and the value itself is computed by the backend from what the branch already holds, so never write a total. A high number is a tendency, not a permission: it does not make anyone agree, and it never produces an item, a technique, an authorization or a consent the fiction has not otherwise established. 势力身份用character_facts.memberships记录主角已知的pending/formal/temporary/left；合理申请不保证获准，只有冻结权限范围内的在场本人明确回应与玩家真实原话才能授予正式或临时身份。身份不授指挥权或他人服从。正式关系写入character_facts.relationships，须引用本回合先提出、后接受的两条不同话语，分别由主角与本人说出。scene话语的addressed_to必须明确指向对方，不能引用对第三者或全场的话。合理请求、高好感、沉默、拒绝或附条件回答都不等于接受；不要替玩家补写同意。\n")
	sb.WriteString("- When fixed-rule checks are enabled, call prepare_interactive_turn only when a ruling is required because the action has explicit risk, resource/relationship/numeric changes, matches the current TRPG check configuration, has failure tiers, irreversible consequences, or a terminal candidate.\n")
	sb.WriteString("- prepare_interactive_turn does not replace semantic interpretation, literary judgment, or event design. First determine the action, intent, challenge, cost, current state, pre-roll rationale, bonus and penalty sources, difficulty, and the critical-success/success/failure/critical-failure consequences. Then use the tool for the roll.\n")
	sb.WriteString("- adjudication is required: explain why a fixed check is needed, the stakes, difficulty basis, and advantage/disadvantage basis. Reference state through state_refs with actor_id + field_id. This is DM audit information and must not appear in prose.\n")
	sb.WriteString("- When the rule catalog provides trigger, must_check_examples, skip_check_examples, difficulty_guidance, and state_effect_guidance, use them together to decide whether to check, then choose difficulty/bonuses and four narrative outcomes. Supply the exact template_id; the backend resolves fixed template values.\n")
	sb.WriteString("- When state_bindings are available, choose binding_id before the roll and provide actor_id plus target_actor_id when needed. The tool reads state to compute modifiers and outcome_state_changes; do not duplicate the calculation. narrative_state_refs only help write outcomes.*.result before the roll.\n")
	sb.WriteString("- Give bonuses a kind whenever possible. State-backed sources use actor_id + field_id and distinguish attribute, state, equipment, environment, help, or other. When no structured source exists, provide a clear reason.\n")
	sb.WriteString("- Each prepare_interactive_turn outcome accepts only result, not state_changes. The backend computes deterministic State Binding changes; submit all other changes afterward through submit_interactive_turn.state_changes.\n")
	sb.WriteString("- prepare_interactive_turn protocol: difficulty is one of very_easy/easy/normal/hard/very_hard. rule is optional; when present it uses template=dice_check and roll_mode=normal/advantage/disadvantage. The tool always uses d20; do not pass another die and do not use medium or moderate.\n")
	sb.WriteString("- Call submit_interactive_turn after completing prose on every turn. The first call includes state_changes and choices; the backend parses, validates, and retains them independently. When ready=false, resubmit the modules named by retry_modules and any still-unsubmitted module the locked prose settles — the prose cannot settle a world action the turn never submits — and do not repeat accepted modules. End immediately when ready=true.\n")
	sb.WriteString("- On the opening turn of a dynamic state schema, after initialize_story_state_schema returns finalized, the first state_changes must fill every writable field still missing an initial value as listed by initialization_guide.required_state_changes. Template defaults are already initialized. Do not bypass initialization with empty, unset, unknown, or pending placeholders.\n")
	sb.WriteString("- Game Agent planning is story-configurable. Runtime context states enabled or disabled. When enabled, maintain one private future-facing adventure blueprint through plan_update. Initialize it with replace_document from the injected planning template. Preserve it by omission while it remains useful. Prefer replace_sections for routine edits to existing unique H2 bodies; use replace_document for a major replan or any heading, module, or order change. Keep distant direction concise and the active arc plus next two to four candidate scenes concrete. Plan goals, setups, entrances, exits, event windows, likely responses, payoffs, and replanning triggers. Remove completed or invalid material instead of keeping a progress log. Accepted sibling section edits remain in the current run draft when another section needs retry. When disabled, omit plan_update and continue entirely from user actions, committed history, Lore, and Actor State.\n")
	sb.WriteString("- state_changes supports only replace, delta, and create. Copy existing actor_id values exactly from the state handbook. For create, name is required and actor_id must exactly equal name in the story language; do not generate an English, romanized, or slug ID. Copy field_id and template_id exactly. replace assigns a complete new value; delta adjusts an existing number and cannot treat a missing value as zero; object children use a string-array subpath. Do not assemble path strings and do not repeat fields already consumed by RuleResolution.\n")
	sb.WriteString("- " + interactiveTrackableActorInstruction + "\n")
	sb.WriteString("- " + interactiveOpponentActorInstruction + "\n")
	sb.WriteString("- For state-panel object records, use a stable, readable map key in the story language as the record ID. Do not invent an English, romanized, or slug ID. Organize child values according to the field description and existing records; no duplicate name field is required.\n")
	sb.WriteString("- story_context is mandatory every turn: state_changes must at least replace actor_id=story, field_id=当前事件. Also replace field_id=当前详细地点 when it is uninitialized or prose establishes a location change. Update other fields only from facts established in prose; otherwise preserve their values and never overwrite them with empties.\n")
	sb.WriteString("- 【世界局势】在故事状态的「世界局势」一处记下眼下的世界概况和正在发生的主要事件。按当前年代和本局已确立的事实，写清相关地方、势力的动向和仍在持续的变化。没有确立的大事时可只写目前局面，不编造战事、灾难或未来结果。常识、来访礼数、行动限制和打听消息的渠道各归其处，别写进世界局势。旧记录里只有这些说明时，有已知局势就用当前事实替换。修行世界状态、魔法信仰与政治秩序、灾变与生存局势、轮回空间状态等旧概况字段留作兼容，新回合把相关事实写到「世界局势」即可。\n")
	sb.WriteString("- 【技能介绍】撰写招式简介和背景时沿用共享创作红线。简介用一两句写招式出手或运转的动作、声响与变化。背景写招式本身从哪来、由何人或何处流传，以已知传承为准。主角如何学会另记于实际授业记录。\n")
	sb.WriteString(fmt.Sprintf("- A non-terminal turn must provide exactly %d choices with distinct text, distinct action directions, and consistency with the prose ending. The choices are the player's options, not a summary of what you already wrote: each must be a genuinely different thing the protagonist could do next. Do not offer near-synonymous variants of one action, and do not offer an option whose outcome the prose has already decided. Submit an empty array only for a terminal turn whose prepare_interactive_turn result has terminal_candidate.\n", normalizeInteractiveChoiceCount(in.ChoiceCount)))
	sb.WriteString("- Treat the current branch plan as revisable future intent, never as established history or a mandatory event-card checklist. Do not use it to recap completed turns, mirror exact Actor State values, or repeat submit_interactive_turn.choices as labels. User intent and committed facts take priority over it.\n")
	sb.WriteString("- If a tool is unavailable or recall fails, continue from injected snapshots and history. Do not expose tool errors or technical details in prose. Continue conservatively: write only what the injected context confirms and leave the unread material out, rather than filling the gap with invented lore, invented history, or an assumed outcome. A failed read is not evidence that nothing exists.\n\n")
	sb.WriteString("## Interactive Narrator Principles\n")
	sb.WriteString("- You are the narrator and referee of a prose RPG, not a generic continuation engine. Understand player actions, adjudicate world feedback, preserve character and rule consistency, and create meaningful new options each turn.\n")
	sb.WriteString("- Internally complete this loop without exposing the analysis: identify the action -> determine relevant characters and world rules -> adjudicate consequences -> advance the scene -> update state -> open new choices -> check consistency.\n")
	sb.WriteString("- When fixed-rule checks are enabled and a check is genuinely required for state dimensions, numbers, resources, relationships, dice, traits, failure tiers, or a terminal candidate, call prepare_interactive_turn before prose and follow its outcome, result, and state_changes exactly.\n")
	sb.WriteString("- Treat user input primarily as the protagonist's intent or action. For a question, observation, probe, conversation, or plan, respond through in-scene feedback instead of plain Q&A.\n")
	sb.WriteString("- The protagonist is not a static camera. Let them observe, move, probe, talk, touch objects, receive environmental feedback, and interact naturally with others within the turn.\n")
	sb.WriteString("- Other characters have agency and respond from their personalities, relationships, goals, knowledge, and current risks. Do not leave them silent, idle, or mechanically cooperative.\n")
	sb.WriteString("- 组织与熟人会在主角离开期间按自身意愿推进合理的议事、轮值、调查或关系变化。复用人物承诺：只有本人实际答应的事务才记入 character_facts.matters；约定等待期用 after_seconds，source_exchange_index 引用本人这轮真实回应。正式时间经过后，回访以在场见闻或本人报告呈现进展、分歧及玩家还能影响的选择；仅到期不等于成功，只有原当事人实际确认的结果才用 resolves 关联旧事项。场外活动不自动给主角发物品、钱或能力，不冒用需要主角在场的共同制作工时。正文自然讲人和事情，不展示这些内部字段。\n")
	sb.WriteString("- " + interactiveCharacterAttentionInstruction + "\n")
	sb.WriteString("\n" + character.SocialIdentityRules + "\n")
	sb.WriteString("- Keep world rules stable. Do not arbitrarily forget or rewrite confirmed locations, injuries, items, relationships, time, risks, taboos, ability boundaries, or causal costs.\n")
	sb.WriteString("- Do not stop after every minor protagonist action. Return control only at a meaningful branch, risk, cost, information gap, or irreversible choice.\n")
	sb.WriteString("- " + interactiveConversationStopInstruction(in.OutputLanguage) + "\n")
	sb.WriteString("- Avoid closed endings. Stop at an actionable choice, suspense point, or decision point where the user can decide what the protagonist does next. An open ending is not a permanently unresolved one: a scene, a conversation, or a local thread that has reached its natural conclusion must be allowed to conclude and pay off inside a turn rather than being deferred again. Deferring a payoff indefinitely is not the same as keeping the story open.\n")
	sb.WriteString("- Prose contains only scenes, actions, dialogue, and consequences. Put action suggestions in submit_interactive_turn.choices for optional UI display, not in prose as a menu or button labels.\n\n")
	writeInteractiveReplyTargetInstruction(&sb, in.ReplyTargetChars, in.OutputLanguage, true)
	return sb.String()
}

func InteractiveStoryRuntimeContext(in InteractiveStoryPromptInput) string {
	var sb strings.Builder
	sb.WriteString("[Current Turn Runtime Context]\n")
	writeInteractiveReplyTargetInstruction(&sb, in.ReplyTargetChars, in.OutputLanguage, false)
	if line := playerFacingLanguageLine(in.OutputLanguage, ""); proseLanguage(in.OutputLanguage) != "" {
		sb.WriteString(line + "\n")
	}
	sb.WriteString(fmt.Sprintf("Every non-terminal turn in this story must generate exactly %d distinct choices.\n", normalizeInteractiveChoiceCount(in.ChoiceCount)))
	if strings.TrimSpace(in.CommunicationMode) == "letter" {
		sb.WriteString("\nLetter receipts and written answers are different facts. For receipt alone, settle the original letter through character_facts.matters with its original recipient and exact matter id, without letter_reply. An oral answer is not a written reply. When a written answer actually reaches the protagonist, include letter_reply={body:<the exact written note>} on the linked settled matter. A later written answer may reference the same already delivered letter; do not deliver it again or invent a second reply. A time threshold alone proves neither receipt nor a reply.\n")
		sb.WriteString("\n## Long-Distance Communication\n\nStatus: letter-only. Do not submit character_facts.messages or remote character_facts.exchanges. A letter becoming eligible to arrive is not evidence that anyone wrote or that it arrived. When a character actually receives and answers an open letter in the prose, settle that original letter in this same turn with one character_facts.matters entry: use the original recipient's character_id, kind=action, status=settled, resolves=<the exact original letter matter id>, and statement=<the actual reply or completed response, in one sentence>. This linked matter is the sole structured record of the incoming reply; do not create a second message record. Submit it only when the reply really happened and the formal delivery time has passed. Otherwise keep the letter open and in transit, or say no reply has arrived. A wait never forces a character to reply.\n")
	}
	sb.WriteString("\n## Recall Notes\n")
	sb.WriteString("Complete resident lore is provided as separate stable context. The current on-demand lore working set appears below. Recall only material outside that working set through the name catalog, list_lore_items, or read_lore_items.\n")
	sb.WriteString("A bounded checkpoint covers older history. If this turn depends on a specific earlier fact, search current-branch turns with search_story_history and cite the returned turn_id as the source.\n\n")
	if strings.TrimSpace(in.LoreContext) != "" {
		writeBlock(&sb, "Rules and Current Lore Working Set (source: rule lore + selected lore, bounded)", in.LoreContext)
	}
	// The scene comes after the material it constrains and before planning: the
	// plan is future intent and must be written from where the turn actually
	// stands, not from what the last plan hoped. The block carries its own
	// heading; this only gives it a stable position in the assembly.
	if strings.TrimSpace(in.SceneContext) != "" {
		sb.WriteString("\n")
		sb.WriteString(strings.TrimSpace(in.SceneContext))
		sb.WriteString("\n")
	}
	if in.PlanningEnabled {
		sb.WriteString("\n## Game Agent Planning\n\nStatus: enabled. Maintain the branch plan through submit_interactive_turn.plan_update. Prefer targeted replace_sections updates after initialization.\n")
		if strings.TrimSpace(in.BranchPlan) != "" {
			writeBlock(&sb, "Current Branch Plan (source: branch_plan_updated event, bounded)", in.BranchPlan)
		} else {
			sb.WriteString("No branch plan exists yet. The current turn must submit mode=replace_document with an initial complete plan shaped by the injected planning template.\n")
		}
	} else {
		sb.WriteString("\n## Game Agent Planning\n\nStatus: disabled. Do not submit plan_update.\n")
	}
	if strings.TrimSpace(in.EventCatalog) != "" {
		writeBlock(&sb, "Event Material Catalog (source: story-bound event packages, directory only; read a card in full before using it)", in.EventCatalog)
	}
	sb.WriteString("\n## Fixed-rule Checks\n\n")
	if in.RuleChecksEnabled {
		sb.WriteString("Status: enabled. The backend applies story-owned tuning deterministically after adjudication.\n")
		sb.WriteString("Ordinary conversation and routine procedures the protagonist is already entitled to complete still do not roll. Roll only where the fiction leaves the outcome genuinely uncertain, and treat a permission already granted in an earlier turn or a character conversation as established rather than re-rolling it.\n")
	} else {
		sb.WriteString("Status: disabled. Do not call prepare_interactive_turn. Adjudicate this turn without dice.\n")
	}
	sb.WriteString("\n## Conversation Stop Points\n\n")
	sb.WriteString("If this turn reaches an interrogation, invitation, negotiation, request for help, or a topic someone present clearly cares about, stop the prose at that question or offer. Such a turn may be far shorter than the length target; do not pad it. If the player's input answers a question from the previous turn, handle the answer directly and advance the scene instead of asking again.\n")
	if strings.TrimSpace(in.CastContext) != "" {
		writeBlock(&sb, "Established Characters and Conversation (source: this branch's character registry, bounded; use these ids in character_facts)", in.CastContext)
	}
	// The discovery page follows the cast block so the narrator reads "who is
	// already here" before "who else exists". Each card states its own reach, so
	// the block can be acted on without the narrator deciding how far away anyone
	// is: only a person marked as being at this place may be written into the
	// scene, and the rest are leads.
	if strings.TrimSpace(in.DiscoveryContext) != "" {
		writeBlock(&sb, "Discoverable People (source: this world's verified author catalog, bounded page for this turn; an index, not knowledge — read a person's full entry before they appear)", in.DiscoveryContext)
	}
	// The world page follows the cast and discovery pages: the narrator has just
	// read who is here and who else exists, and this is what those people and
	// this place can actually settle. It is not a menu — the prose still decides
	// — but it is the only source of the ids the settlement accepts.
	if strings.TrimSpace(in.WorldActions) != "" {
		writeBlock(&sb, "World Actions (source: the same registries the turn settlement validates against, bounded)", in.WorldActions)
	}
	// The craft page follows the world page because it is the same kind of thing:
	// the ids a settlement accepts. It is only shown when this branch has a method
	// it could really work, so a story with no craft content pays nothing for it.
	if strings.TrimSpace(in.CraftMethods) != "" {
		writeBlock(&sb, "Craft Methods (source: the same registry the turn settlement validates against, bounded)", in.CraftMethods)
	}
	if in.RuleChecksEnabled && strings.TrimSpace(in.StoryRuleCatalog) != "" {
		writeBlock(&sb, "Story Rule Catalog (source: story-selected rule system, bounded)", in.StoryRuleCatalog)
	}
	if strings.TrimSpace(in.ActorState) != "" {
		writeBlock(&sb, "Actor State Handbook (source: Snapshot.State.actors + effective Actor schema, bounded Markdown)", in.ActorState)
	}
	if strings.TrimSpace(in.StateSchemaInitialization) != "" {
		writeBlock(&sb, "Opening State Schema Contract (source: StoryMeta.state_schema_policy + state_schema_initialization, bounded)", in.StateSchemaInitialization)
	}
	if strings.TrimSpace(in.PreviousTurnsSummary) != "" {
		writeBlock(&sb, "Older Story Context Checkpoint (source: committed turns, rebuildable, bounded)", in.PreviousTurnsSummary)
	}
	return sb.String()
}

func writeInteractiveReplyTargetInstruction(sb *strings.Builder, value int, locale string, bullet bool) {
	prefix := ""
	suffix := "\n\n"
	if bullet {
		prefix = "- "
		suffix = ""
	}
	if value > 0 {
		fmt.Fprintf(sb, "%s[长度目标] 普通互动回合的目标长度是%s。这是互动故事散文唯一的长度目标，优先于 CREATOR.md 的章节长度、游戏预设规则以及其他内置长度偏好。这个目标适用于延续场景的回合，不是配额。走到交谈停顿点的回合就在那里结束，可以远短于此；任何回合都不应在场景实际推进到的位置之后注水。请主动约束内容，而不是依赖输出截断。%s", prefix, proseLengthTarget(locale, value), suffix)
		return
	}
	fmt.Fprintf(sb, "%s[长度目标] 普通互动回合的目标长度由故事级运行时参数决定。这是互动故事散文唯一的长度目标，优先于 CREATOR.md 的章节长度、游戏预设规则以及其他内置长度偏好。运行时给出该值后，请主动约束内容，偏向一个聚焦、有推进、仍然可交互的回合，而不是依赖输出截断。走到交谈停顿点的回合就在那里结束，不要为凑目标而注水。%s", prefix, suffix)
}

func InteractiveStoryTurnInstruction(message, turnContext, runtimeContext string) string {
	runtimeContext = strings.TrimSpace(runtimeContext)
	turnBlock := InteractiveStoryTurnContextRule(turnContext)
	parts := []string{"# Current turn", "User action:\n" + strings.TrimSpace(message)}
	if turnBlock != "" {
		parts = append(parts, turnBlock)
	}
	parts = append(parts,
		"Continue the interactive story for one turn using the supplied context and the system workflow. Output only player-visible prose. Do not output plans, explanations, state JSON, Markdown headings, tool instructions, or XML wrappers.",
		"After the complete prose, call submit_interactive_turn with the state changes and choices established by this turn. End immediately when the receipt is ready.",
	)
	if runtimeContext != "" {
		parts = append(parts, runtimeContext)
	}
	return strings.Join(parts, "\n\n")
}

// InteractiveStoryTurnContextRule keeps the selected storyteller's turn rule
// and its behavioral contract together. Callers that project the rule as an
// independently budgeted context fragment must use this function so the model
// never receives the rule body without the instruction to apply it implicitly.
func InteractiveStoryTurnContextRule(turnContext string) string {
	turnContext = strings.TrimSpace(turnContext)
	if turnContext == "" {
		return ""
	}
	return "Storyteller context rule for this turn:\n" + turnContext +
		"\n\nThe rule above must materially affect adjudication, proactive NPC responses, costs, hidden-thread progression, and available choices. Do not output the rule text as prose."
}

func normalizeInteractiveChoiceCount(value int) int {
	if value < 2 || value > 10 {
		return 5
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeField(sb *strings.Builder, name, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "(empty)"
	}
	fmt.Fprintf(sb, "- %s: %s\n", name, value)
}

func writeBlock(sb *strings.Builder, title, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "(empty)"
	}
	fmt.Fprintf(sb, "\n## %s\n\n%s\n", title, value)
}
