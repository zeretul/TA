package interactive

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

const (
	// TurnWorldActionJourney walks a registered road. It is the formal time of a
	// turn that travels.
	TurnWorldActionJourney = "journey"
	// TurnWorldActionQuest offers, accepts, abandons, completes or settles a task.
	TurnWorldActionQuest = "quest"
	// TurnWorldActionPublicEvent joins or steps back from an authored event.
	TurnWorldActionPublicEvent = "public_event"
	// TurnWorldActionRealm enters, moves through, takes from or leaves a realm.
	TurnWorldActionRealm = "realm"
	// TurnWorldActionWait advances the shared StoryClock for an explicit player
	// request to let time pass without another timed activity.
	TurnWorldActionWait = "wait"
	// TurnWorldActionNone declares that this turn settles no world action. It is
	// a decision, not an action: the module is submitted with only this value so
	// an omitted module can only mean a missing submission, never an implicit
	// "nothing happened". Ordinary dialogue submits it on most turns.
	TurnWorldActionNone = "none"
)

// TurnWorldActionInput is the model-facing world module of one story turn.
//
// 行路、差事、公共事件和秘境 are facts about where the character stands and what
// the world has already offered them, so the main story has to be able to settle
// them. Without this module the four operation windows would be the only formal
// path, which the interface boundary forbids: the player expresses travel,
// taking a task, joining an event and entering a realm in their own narration,
// and this module is how that narration reaches the same definitions,
// eligibility and formal clock the entries use.
//
// One turn carries one world action. Each action publishes its own side record,
// and the two that move the formal clock — a stretch of road and a public event
// joined at a minute — would each advance from the same starting instant if both
// were carried, so one interval would be spent twice and the other not at all.
// The module also cannot share a turn with rest or training for the same reason.
type TurnWorldActionInput struct {
	LocalAffair       *LocalAffairActionInput `json:"local_affair,omitempty" jsonschema_description:"仅action=local_affair：地方小事的首次固定、依据真实已提交见闻核实、暂放与自愿继续；不是新地图、副本或奖励入口。"`
	Faction           *FactionActionInput     `json:"faction,omitempty" jsonschema_description:"仅action=faction：玩家自愿参与的一件共同制作事务，不能调动组织公库或控制NPC意愿。"`
	JourneyCompanions []JourneyCompanionInput `json:"journey_companions,omitempty" jsonschema_description:"仅journey新旅途：实际决定同行者及本人本回合scene回应索引。须出发前已同场，首次真实相遇可同轮登记本人资料并回应；普通兽可用真实非语言回应。本人回应须有participation_decision的kind=journey、target_id=本路线、decision=accept；不能以好感、主角命令或凭空搬来远处角色代替本人意愿；续程沿原名单，不重填。"`
	Cooperation       *CooperationActionInput `json:"cooperation,omitempty" jsonschema_description:"仅action=cooperation。单件事情的同场合作，不是队伍、全局授权或远程代劳。本人回应放在character_facts.exchanges.cooperation_decision。"`
	Adventure         *AdventureAction        `json:"adventure,omitempty" jsonschema_description:"武侠或星际的正式行动，仅action=adventure时填写；目录与资格以冻结规则为准。"`
	// Action names which world fact this turn settles, or none.
	Action string `json:"action" jsonschema:"enum=journey,enum=quest,enum=public_event,enum=realm,enum=wait,enum=adventure,enum=cooperation,enum=faction,enum=local_affair,enum=none" jsonschema_description:"每回合必须提交：journey行路、quest任务、public_event公共事件、realm秘境、wait按明确请求等待、adventure武侠或星际正式行动。faction是本人同意、真实出资与分段耗时的共同制作事务，填写faction与source_text。cooperation是本人同意的局部协作，填写cooperation与source_text。local_affair围绕现地自然遇见的具体小事展开，填写local_affair；后续核实、暂放、续查与收束都要source_text。无行动填none。adventure填写adventure对象与source_text；不得混入其他行动字段。"`
	// DurationSeconds is the interval the model proposes for an explicit wait.
	// Commit checks positivity, overflow, timed-action exclusivity and source.
	DurationSeconds int64 `json:"duration_seconds,omitempty" jsonschema:"minimum=0" jsonschema_description:"wait only. Positive in-fiction seconds the player's current request explicitly asked to pass. Propose the duration from that request; never infer it from narration or the scene's written time, and never combine it with another timed action."`
	// SourceText quotes the player's current request that authorizes this wait.
	SourceText string `json:"source_text,omitempty" jsonschema_description:"wait、adventure、cooperation、faction以及local_affair后续行动必填：逐字引用玩家本轮授权该行动的原话。不能引用模型自己写的叙述来证明授权。"`

	// RouteID names the road to walk. Empty continues the branch's single
	// unfinished journey, which is what "继续赶路" means without repeating the
	// destination.
	RouteID string `json:"route_id,omitempty" jsonschema_description:"journey only. The route id exactly as listed in the World Actions block. Omit to continue the one unfinished journey this branch already has."`
	// Arrive walks the whole road to its destination, which is the ordinary
	// "去坊市" case: one turn completes the journey rather than stopping at each
	// half-day.
	Arrive bool `json:"arrive,omitempty" jsonschema_description:"journey only. True when the character walks to the route's destination this turn. Set either arrive or target_minutes."`
	// TargetMinutes is the absolute elapsed minute to reach, counted from the
	// route's origin. Use it only for an explicit partial stop.
	TargetMinutes int64 `json:"target_minutes,omitempty" jsonschema_description:"journey only. The absolute minute from the route's origin to stop at, used only when the player asks to stop short of the destination. Never invent a duration the player did not ask for."`

	// TaskID names the authored task.
	TaskID        string `json:"task_id,omitempty" jsonschema_description:"quest only. The task id exactly as listed in the World Actions block."`
	QuestChoiceID string `json:"quest_choice_id,omitempty" jsonschema_description:"quest.choose使用的已登记处理方式ID；仅玩家明确选择该行动时提交，不能代替玩家做决定。"`
	// QuestAction is an explicit task action.
	QuestAction string `json:"quest_action,omitempty" jsonschema:"enum=offer,enum=accept,enum=abandon,enum=complete,enum=settle,enum=choose,enum=follow_up" jsonschema_description:"quest任务行动：offer提出见闻，accept由玩家接下，choose执行已登记处理方式并由规则判定结果与耗时，abandon放弃，complete核对目标，settle领取一次报酬，follow_up在时间成熟后向本场本人核实旧事（复制已列出的quest_choice_id；普通完成回响无需choice）。回访不代表接下新任务，也不会额外领奖。"`

	// EventID names the authored public event.
	EventID string `json:"event_id,omitempty" jsonschema_description:"public_event only. The event id exactly as listed in the World Actions block."`
	// EventAction is join or withdraw.
	EventAction string `json:"event_action,omitempty" jsonschema:"enum=join,enum=withdraw" jsonschema_description:"public_event only. join takes part in the event at the current formal minute; withdraw steps back from a participation already recorded."`
	// ParticipationAction is how the character takes part when joining:
	// observe, help or investigate.
	ParticipationAction string `json:"participation_action,omitempty" jsonschema:"enum=observe,enum=help,enum=investigate" jsonschema_description:"public_event join only. observe watches and asks around without committing, help actively assists the people at the centre, investigate looks for what is really going on. Omit to observe."`

	// RealmID names the authored realm. Required to enter.
	RealmID string `json:"realm_id,omitempty" jsonschema_description:"realm enter only. The realm id exactly as listed in the World Actions block."`
	// InstanceID names the instance a later action acts on. Empty uses the
	// branch's single instance, which is what "继续探索" means without repeating
	// it.
	InstanceID string `json:"instance_id,omitempty" jsonschema_description:"realm only. The instance id from the Known Realms block. Omit to act on the branch's single existing instance."`
	// RealmAction is enter, reenter, move, reward or exit.
	RealmAction string `json:"realm_action,omitempty" jsonschema:"enum=enter,enum=reenter,enum=move,enum=reward,enum=exit,enum=survey,enum=prepare,enum=approach,enum=resolve" jsonschema_description:"秘境行动。enter/reenter必须在真实入口，move仅已知通路，exit离开；新场地survey调查、prepare调整、approach选路、resolve处置发现，均填写页面实际列出的approach_id，不得自造奖励或资格。"`
	// ConnectionID and DestinationAreaID move the character through one declared
	// directed connection.
	ConnectionID      string `json:"connection_id,omitempty" jsonschema_description:"realm move only. The connection id exactly as listed in the Known Realms block."`
	DestinationAreaID string `json:"destination_area_id,omitempty" jsonschema_description:"realm move only. The destination area id exactly as listed in the Known Realms block."`
	// ContentRefID names the declared reward to take.
	ApproachID   string `json:"approach_id,omitempty" jsonschema_description:"调查、准备、选路或处置的已知方法标识，严格复制探索页。实际物品、技能、精力和耗时由规则核验，不能由正文取得资格。"`
	ContentRefID string `json:"content_ref_id,omitempty" jsonschema_description:"realm reward only. The reward id exactly as listed in the Known Realms block. The rules decide what it is and record it once."`
}

// Empty reports whether the module carries nothing at all. A module declaring
// action=none is not empty: it is the turn's explicit decision that no world
// action is settled, and only that declaration can satisfy the required world
// module.
func (input TurnWorldActionInput) Empty() bool {
	return input.LocalAffair == nil && input.Faction == nil && len(input.JourneyCompanions) == 0 && input.Cooperation == nil && input.Adventure == nil && strings.TrimSpace(input.Action) == "" &&
		strings.TrimSpace(input.RouteID) == "" && !input.Arrive && input.TargetMinutes == 0 &&
		strings.TrimSpace(input.TaskID) == "" && strings.TrimSpace(input.QuestAction) == "" &&
		strings.TrimSpace(input.EventID) == "" && strings.TrimSpace(input.EventAction) == "" &&
		strings.TrimSpace(input.ParticipationAction) == "" &&
		strings.TrimSpace(input.RealmID) == "" && strings.TrimSpace(input.InstanceID) == "" &&
		strings.TrimSpace(input.RealmAction) == "" && strings.TrimSpace(input.ConnectionID) == "" &&
		strings.TrimSpace(input.DestinationAreaID) == "" && strings.TrimSpace(input.ContentRefID) == "" && strings.TrimSpace(input.ApproachID) == "" &&
		input.DurationSeconds == 0 && strings.TrimSpace(input.SourceText) == ""
}

// Settles reports whether the module names a real world action. A none
// declaration and an empty module both settle nothing; only a none declaration
// counts as the turn's decision.
func (input TurnWorldActionInput) Settles() bool {
	action := strings.TrimSpace(input.Action)
	return action != "" && action != TurnWorldActionNone
}

// DeclaresNone reports whether this is the turn's explicit no-action decision.
func (input TurnWorldActionInput) DeclaresNone() bool {
	return strings.TrimSpace(input.Action) == TurnWorldActionNone
}

// cloneTurnWorldActionInput copies a world module so an accepted one can be
// retained in a draft without the caller's value being aliased.
func cloneTurnWorldActionInput(input *TurnWorldActionInput) *TurnWorldActionInput {
	if input == nil {
		return nil
	}
	cloned := *input
	if input.LocalAffair != nil {
		v := cloneLocalAffairInput(*input.LocalAffair)
		cloned.LocalAffair = &v
	}
	if input.Faction != nil {
		f := *input.Faction
		cloned.Faction = &f
	}
	cloned.JourneyCompanions = append([]JourneyCompanionInput(nil), input.JourneyCompanions...)
	if input.Cooperation != nil {
		c := *input.Cooperation
		c.ReportSourceRefs = append([]string(nil), c.ReportSourceRefs...)
		cloned.Cooperation = &c
	}
	if input.Adventure != nil {
		v := *input.Adventure
		if v.Generate != nil {
			definition := cloneAdventureGeneratedDefinition(*v.Generate)
			v.Generate = &definition
		}
		cloned.Adventure = &v
	}
	return &cloned
}

// turnWorldOwnsSceneLocation reports whether this turn's world module is the
// path that writes 当前详细地点.
//
// A road writes where the character now stands, and leaving a realm writes them
// back to the place they entered from. When the rules are about to author that
// field, a state_changes write to it is refused for the same reason a rest's
// pool write is: two writers would leave the scene claiming one place and the
// record another.
func turnWorldOwnsSceneLocation(input *TurnWorldActionInput) bool {
	if input == nil || input.Empty() {
		return false
	}
	switch strings.TrimSpace(input.Action) {
	case TurnWorldActionJourney:
		return true
	case TurnWorldActionRealm:
		return strings.TrimSpace(input.RealmAction) == RealmActionExit
	}
	return false
}

// turnWorldSettlesMovement reports whether this turn's world module really moves
// the character: a road, or a realm action that changes the area they stand in.
//
// It is the permission for a state_changes write to 当前详细地点. A realm entry is
// the one realm action that leaves the field to the prose — the instance records
// the area, and the cave's own wording lives nowhere else — so it counts here.
// A task decision, an event participation, or a reward taken from the room the
// character is already standing in does not, and a write to the field in those
// turns is the prose moving the character on the world's behalf.
func turnWorldSettlesMovement(input *TurnWorldActionInput) bool {
	if input == nil || input.Empty() {
		return false
	}
	switch strings.TrimSpace(input.Action) {
	case TurnWorldActionJourney:
		return true
	case TurnWorldActionRealm:
		switch strings.TrimSpace(input.RealmAction) {
		case RealmActionEnter, RealmActionReenter, RealmActionMove, RealmActionExit:
			return true
		}
	}
	return false
}

// validateTurnWorldActionModule rejects a world module the rules could not act
// on.
//
// It checks shape, not outcome. Whether the character may walk this road from
// here, take this task, join this event or enter this realm is a domain answer
// read from committed facts, and it is answered against the branch while the
// turn commits rather than guessed here.
func validateTurnWorldActionModule(input *TurnWorldActionInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	action := strings.TrimSpace(input.Action)
	invalid := func(path, expected, actual, message string) *TurnSubmissionDiagnostic {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleWorld, nil, TurnSubmissionDiagnosticInvalidModule,
			path, expected, actual, message,
		)
	}
	if len(input.JourneyCompanions) > 0 && action != TurnWorldActionJourney {
		return invalid("/world/journey_companions", "action=journey", action, "同行只属于这一次行路。")
	}
	if input.LocalAffair != nil && action != TurnWorldActionLocalAffair {
		return invalid("/world/local_affair", "action=local_affair", action, "地方事务不能混入另一项行动。")
	}
	if input.Faction != nil && action != TurnWorldActionFaction {
		return invalid("/world/faction", "action=faction", action, "共同制作不能混入其他行动。")
	}
	if input.Cooperation != nil && action != TurnWorldActionCooperation {
		return invalid("/world/cooperation", "action=cooperation", action, "协作不能混入其他行动。")
	}
	if input.Adventure != nil && action != "adventure" {
		return invalid("/world/adventure", "action=adventure", action, "题材行动不能和其他世界行动混用。")
	}
	if input.ApproachID != "" && (action != TurnWorldActionRealm || !realmExploreAction(input.RealmAction)) {
		return invalid("/world/approach_id", "探索方法行动", action, "approach_id只能用于秘境调查、准备、选路或处置。")
	}
	switch action {
	case TurnWorldActionLocalAffair:
		if reason := validateLocalAffairInput(input); reason != "" {
			return invalid("/world/local_affair", "one local affair action", reason, reason)
		}
	case TurnWorldActionFaction:
		if reason := validateFactionInput(input); reason != "" {
			return invalid("/world/faction", "one faction affair action", reason, reason)
		}
	case TurnWorldActionCooperation:
		if reason := validateCooperationInput(input); reason != "" {
			return invalid("/world/cooperation", "one scoped cooperation action", reason, reason)
		}
	case "adventure":
		if strings.TrimSpace(input.SourceText) == "" {
			return invalid("/world/source_text", "current player request", "empty", "题材行动需引用玩家本轮行动原话。")
		}
		if input.Adventure == nil || input.Adventure.Action == "" || input.Adventure.Action == "battle_round" {
			return invalid("/world/adventure", "a supported adventure action", "missing or battle_round", "请填写题材行动；交战回合只能在战斗窗口提交。")
		}
		if input.RouteID != "" || input.TaskID != "" || input.EventID != "" || input.RealmID != "" || input.DurationSeconds != 0 {
			return invalid("/world", "one adventure action", "mixed actions", "每回合仅结算一种正式行动。")
		}
	case TurnWorldActionJourney:
		if strings.TrimSpace(input.TaskID) != "" || strings.TrimSpace(input.EventID) != "" || strings.TrimSpace(input.RealmID) != "" {
			return invalid("/world", "only journey fields", "fields of another world action", "A world module settles one action; omit the other actions' fields.")
		}
		if !input.Arrive && input.TargetMinutes <= 0 {
			return invalid("/world/arrive", "arrive=true or a positive target_minutes", fmt.Sprintf("arrive=%t target_minutes=%d", input.Arrive, input.TargetMinutes),
				"A journey must reach the destination or a later minute; a request that covers no ground would show a road nobody walked.")
		}
		if input.TargetMinutes < 0 {
			return invalid("/world/target_minutes", "a non-negative minute from the route origin", fmt.Sprint(input.TargetMinutes), "journey.target_minutes cannot be negative.")
		}
	case TurnWorldActionQuest:
		if strings.TrimSpace(input.RouteID) != "" || input.Arrive || input.TargetMinutes != 0 ||
			strings.TrimSpace(input.EventID) != "" || strings.TrimSpace(input.RealmID) != "" {
			return invalid("/world", "only quest fields", "fields of another world action", "A world module settles one action; omit the other actions' fields.")
		}
		if strings.TrimSpace(input.TaskID) == "" {
			return invalid("/world/task_id", "a task id from the World Actions block", "", "quest requires task_id; copy it exactly from the World Actions block.")
		}
		switch strings.TrimSpace(input.QuestAction) {
		case QuestActionChoose:
			if strings.TrimSpace(input.QuestChoiceID) == "" {
				return invalid("/world/quest_choice_id", "registered choice id", "", "处理事务需要选择已登记的处理方式。")
			}
		case QuestActionOffer, QuestActionAccept, QuestActionAbandon, QuestActionComplete, QuestActionSettle, QuestActionFollowUp:
		default:
			return invalid("/world/quest_action", "offer, accept, abandon, complete, settle, choose or follow_up", strings.TrimSpace(input.QuestAction),
				"quest.quest_action must name one action the rules settle.")
		}
	case TurnWorldActionPublicEvent:
		if strings.TrimSpace(input.RouteID) != "" || input.Arrive || input.TargetMinutes != 0 ||
			strings.TrimSpace(input.TaskID) != "" || strings.TrimSpace(input.RealmID) != "" {
			return invalid("/world", "only public_event fields", "fields of another world action", "A world module settles one action; omit the other actions' fields.")
		}
		if strings.TrimSpace(input.EventID) == "" {
			return invalid("/world/event_id", "an event id from the World Actions block", "", "public_event requires event_id; copy it exactly from the World Actions block.")
		}
		switch strings.TrimSpace(input.EventAction) {
		case PublicEventActionJoin:
			switch strings.TrimSpace(input.ParticipationAction) {
			case "", "observe", "help", "investigate":
			default:
				return invalid("/world/participation_action", "observe, help or investigate", strings.TrimSpace(input.ParticipationAction),
					"participation_action must name how the character takes part.")
			}
		case PublicEventActionWithdraw:
		default:
			return invalid("/world/event_action", "join or withdraw", strings.TrimSpace(input.EventAction),
				"public_event.event_action must name one action the rules settle.")
		}
	case TurnWorldActionRealm:
		if strings.TrimSpace(input.RouteID) != "" || input.Arrive || input.TargetMinutes != 0 ||
			strings.TrimSpace(input.TaskID) != "" || strings.TrimSpace(input.EventID) != "" {
			return invalid("/world", "only realm fields", "fields of another world action", "A world module settles one action; omit the other actions' fields.")
		}
		switch strings.TrimSpace(input.RealmAction) {
		case RealmActionEnter, RealmActionReenter:
			if strings.TrimSpace(input.RealmID) == "" {
				return invalid("/world/realm_id", "a realm id from the World Actions block", "", "Entering a realm requires realm_id; copy it exactly from the World Actions block.")
			}
		case RealmActionMove:
			if strings.TrimSpace(input.ConnectionID) == "" && strings.TrimSpace(input.DestinationAreaID) == "" {
				return invalid("/world/connection_id", "a destination_area_id from the Known Realms block, or its connection_id", "",
					"Moving inside a realm names one declared connection or destination.")
			}
		case RealmActionReward:
			if strings.TrimSpace(input.ContentRefID) == "" {
				return invalid("/world/content_ref_id", "a reward id from the Known Realms block", "",
					"Taking a realm reward requires content_ref_id.")
			}
		case RealmActionSurvey, RealmActionPrepare, RealmActionApproach, RealmActionResolve:
			if strings.TrimSpace(input.ApproachID) == "" {
				return invalid("/world/approach_id", "已知方法标识", "", "请复制探索页列出的approach_id。")
			}
			if input.ContentRefID != "" || input.ConnectionID != "" || input.DestinationAreaID != "" {
				return invalid("/world", "一种探索方法", "混合移动或领取", "方法的消耗和所得由规则原子结算，不能同时移动或领取。")
			}
		case RealmActionExit:
		default:
			return invalid("/world/realm_action", "enter, reenter, move, reward or exit", strings.TrimSpace(input.RealmAction),
				"realm.realm_action must name one action the rules settle.")
		}
	case TurnWorldActionWait:
		if strings.TrimSpace(input.RouteID) != "" || input.Arrive || input.TargetMinutes != 0 ||
			strings.TrimSpace(input.TaskID) != "" || strings.TrimSpace(input.QuestAction) != "" ||
			strings.TrimSpace(input.EventID) != "" || strings.TrimSpace(input.EventAction) != "" ||
			strings.TrimSpace(input.ParticipationAction) != "" || strings.TrimSpace(input.RealmID) != "" ||
			strings.TrimSpace(input.InstanceID) != "" || strings.TrimSpace(input.RealmAction) != "" ||
			strings.TrimSpace(input.ConnectionID) != "" || strings.TrimSpace(input.DestinationAreaID) != "" ||
			strings.TrimSpace(input.ContentRefID) != "" {
			return invalid("/world", "only wait fields", "fields of another world action", "A world module settles one action; omit the other actions' fields.")
		}
		if input.DurationSeconds <= 0 {
			return invalid("/world/duration_seconds", "a positive duration in seconds", fmt.Sprint(input.DurationSeconds), "A wait must advance a positive amount of in-fiction time.")
		}
		if strings.TrimSpace(input.SourceText) == "" {
			return invalid("/world/source_text", "an exact quote from the player's current request", "", "A wait needs source text from the player's explicit request.")
		}
	case TurnWorldActionNone:
		if strings.TrimSpace(input.RouteID) != "" || input.Arrive || input.TargetMinutes != 0 ||
			strings.TrimSpace(input.TaskID) != "" || strings.TrimSpace(input.QuestAction) != "" ||
			strings.TrimSpace(input.EventID) != "" || strings.TrimSpace(input.EventAction) != "" ||
			strings.TrimSpace(input.ParticipationAction) != "" ||
			strings.TrimSpace(input.RealmID) != "" || strings.TrimSpace(input.InstanceID) != "" ||
			strings.TrimSpace(input.RealmAction) != "" || strings.TrimSpace(input.ConnectionID) != "" ||
			strings.TrimSpace(input.DestinationAreaID) != "" || strings.TrimSpace(input.ContentRefID) != "" ||
			input.DurationSeconds != 0 || strings.TrimSpace(input.SourceText) != "" {
			return invalid("/world", "action=none alone", "action=none with another action's fields", "A world module settles one action; action=none carries no other fields.")
		}
	default:
		return invalid("/world/action", "journey, quest, public_event, realm, wait or none", action,
			"world.action must name one world fact this build settles, or none.")
	}
	return nil
}

// turnWorldPlan is one world action the rules have accepted, together with the
// operations the turn's own state delta will carry.
//
// Nothing here has been written yet: the side record is parented after whatever
// the turn leaves at the branch head, so it can only be built once the commit
// knows that position.
type turnWorldPlan struct {
	localAffair     *LocalAffairState
	action          string
	cooperation     *cooperationPlannedAction
	faction         *factionPlannedAction
	journey         *journeyPlannedStretch
	quest           *questPlannedAction
	event           *publicEventPlannedAction
	realm           *realmPlannedAction
	wait            *storyClockAdvance
	ops             []ActorStateOp
	narrativeResult string
}

type storyClockAdvance struct {
	Before StoryClock
	After  StoryClock
}

// turnWorldRequests turns the module into the four domain requests.
func turnWorldRequests(branchID, turnID string, input *TurnWorldActionInput) (JourneyRequest, QuestRequest, PublicEventRequest, RealmRequest) {
	return JourneyRequest{
		BranchID:      branchID,
		TurnID:        turnID,
		RouteID:       strings.TrimSpace(input.RouteID),
		TargetMinutes: input.TargetMinutes,
		Arrive:        input.Arrive,
	}, QuestRequest{
		BranchID: branchID,
		TurnID:   turnID,
		TaskID:   strings.TrimSpace(input.TaskID),
		Action:   strings.TrimSpace(input.QuestAction),
		ChoiceID: strings.TrimSpace(input.QuestChoiceID),
	}, PublicEventRequest{
		BranchID:            branchID,
		TurnID:              turnID,
		EventID:             strings.TrimSpace(input.EventID),
		Action:              strings.TrimSpace(input.EventAction),
		ParticipationAction: strings.TrimSpace(input.ParticipationAction),
	}, RealmRequest{
		BranchID:          branchID,
		TurnID:            turnID,
		RealmID:           strings.TrimSpace(input.RealmID),
		InstanceID:        strings.TrimSpace(input.InstanceID),
		Action:            strings.TrimSpace(input.RealmAction),
		ConnectionID:      strings.TrimSpace(input.ConnectionID),
		DestinationAreaID: strings.TrimSpace(input.DestinationAreaID),
		ContentRefID:      strings.TrimSpace(input.ContentRefID),
		ApproachID:        strings.TrimSpace(input.ApproachID),
	}
}

// planTurnWorldCommit settles this turn's world action.
//
// It reads two different states on purpose, and the difference is the whole
// reason this signature takes both:
//
//   - state is the state the turn's own operations produced. It is where the
//     arithmetic happens: a reward adds to the balance this turn really left,
//     and an item joins the pack as this turn really left it.
//   - The projection's own committed state is where the turn found the
//     character. It answers the preconditions — which road starts here, which
//     realm's entrance this is, which region an event happens in. A narrator who
//     writes 主角走进洞府 has described the turn's result; it has not moved the
//     character, and the turn's own prose must not be the evidence for its own
//     permission. Judging an entrance against the sentence that describes being
//     inside is exactly the false refusal this split removes.
//
// It reports the refusal as an error rather than swallowing it. A refused world
// action cannot be a warning: the turn's prose already narrates the walk, the
// task, the participation or the entrance, and the other modules may already
// carry the location or the item that success would have produced. Committing
// the turn anyway would publish a success the world never settled, so the
// caller fails the turn and the model rewrites it. Submission runs the same
// planner first, which is where an ordinary refusal is caught while the prose
// can still change.
func planTurnWorldCommit(
	projection *storyBranchProjection,
	meta StoryMeta,
	system StoryDirectorActorStateSystem,
	state map[string]any,
	branchID, storyID, turnID string,
	input *TurnWorldActionInput,
	facts ...*TurnCharacterFactsInput,
) (turnWorldPlan, error) {
	// The preconditions read the committed branch; the settlement arithmetic
	// reads the turn's own writes.
	view := recoveryViewAt(projection, state)
	plan, err := planTurnWorldAction(view, storyCommittedState(projection), meta, system, branchID, turnID, input, facts...)
	if err != nil {
		return turnWorldPlan{}, fmt.Errorf("world action refused: %w", err)
	}
	for _, op := range plan.ops {
		if err := validateActorStateOp(op); err != nil {
			return turnWorldPlan{}, fmt.Errorf("world action refused: invalid operation: %w", err)
		}
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-story] world action accepted story_id=%s branch=%s action=%s", storyID, branchID, plan.action))
	return plan, nil
}

// planTurnWorldAction is the one judgement both the commit and the submission
// dry run make. Keeping a single function is what stops the two from drifting:
// a module the submission accepted can never be refused by the commit for a
// reason the submission did not see, and vice versa.
//
// projection carries the arithmetic and the side records (where the character
// stands after this turn's own writes, which instances exist). scene is the
// state the turn found the branch in, which is where the preconditions are read
// from. Callers that have no turn writes yet pass the same state for both.
func planTurnWorldAction(
	projection *storyBranchProjection,
	scene map[string]any,
	meta StoryMeta,
	system StoryDirectorActorStateSystem,
	branchID, turnID string,
	input *TurnWorldActionInput,
	facts ...*TurnCharacterFactsInput,
) (turnWorldPlan, error) {
	if projection != nil {
		projection.bindWorld(meta)
	}
	journeyReq, questReq, eventReq, realmReq := turnWorldRequests(branchID, turnID, input)
	plan := turnWorldPlan{action: strings.TrimSpace(input.Action)}
	switch plan.action {
	case TurnWorldActionLocalAffair:
		next, err := planLocalAffairAction(projection, scene, meta, *input)
		if err != nil {
			return turnWorldPlan{}, err
		}
		plan.localAffair = &next
		if input.LocalAffair.Action == "verify" {
			method, _ := localAffairMethod(next.Definition, input.LocalAffair.MethodID)
			if method.Kind == "observation" {
				clock, err := branchStoryClock(projection, DefaultStoryContextActorID, system)
				if err != nil {
					return turnWorldPlan{}, err
				}
				if adventureOpeningBoundary(scene, clock.Seconds, clock.Seconds+600, projection.worldContent()) < clock.Seconds+600 {
					return turnWorldPlan{}, fmt.Errorf("an opening hazard interrupts this investigation")
				}
				after, err := AdvanceStoryClock(clock, 600)
				if err != nil {
					return turnWorldPlan{}, err
				}
				plan.wait = &storyClockAdvance{Before: clock, After: after}
			}
		}
	case TurnWorldActionFaction:
		if reason := validateFactionInput(input); reason != "" {
			return turnWorldPlan{}, fmt.Errorf("%s", reason)
		}
		var current *TurnCharacterFactsInput
		if len(facts) > 0 {
			current = facts[0]
		}
		faction, err := planFactionAction(projection, scene, meta, system, branchID, turnID, *input.Faction, current)
		if err != nil {
			return turnWorldPlan{}, err
		}
		plan.faction = &faction
		plan.ops = faction.Ops
		plan.wait = faction.Clock
	case TurnWorldActionCooperation:
		if reason := validateCooperationInput(input); reason != "" {
			return turnWorldPlan{}, fmt.Errorf("%s", reason)
		}
		var current *TurnCharacterFactsInput
		if len(facts) > 0 {
			current = facts[0]
		}
		cooperation, err := planCooperationAction(projection, scene, meta, branchID, turnID, input.Cooperation, current)
		if err != nil {
			return turnWorldPlan{}, err
		}
		plan.cooperation = &cooperation
		plan.ops = cooperation.Ops
	case "adventure":
		if system.AdventureRules == nil || input.Adventure == nil {
			return turnWorldPlan{}, fmt.Errorf("adventure rules or action missing")
		}
		if err := validateAdventureScene(*system.AdventureRules, scene, *input.Adventure); err != nil {
			return turnWorldPlan{}, err
		}
		a, err := AdventureActorFromState(scene, DefaultActorID)
		if err != nil {
			return turnWorldPlan{}, err
		}
		refreshAdventureFogExposure(*system.AdventureRules, &a, scene)
		nowSeconds := int64(0)
		if projection != nil {
			nowSeconds = projection.clockSeconds()
		}
		next, outcome, err := ResolveAdventureActionAt(*system.AdventureRules, a, *input.Adventure, turnID, nowSeconds)
		if err != nil {
			return turnWorldPlan{}, err
		}
		if outcome.DurationSeconds > 0 && projection != nil && adventureOpeningBoundary(scene, projection.clockSeconds(), projection.clockSeconds()+outcome.DurationSeconds, projection.worldContent()) < projection.clockSeconds()+outcome.DurationSeconds {
			return turnWorldPlan{}, fmt.Errorf("pending opening hazard interrupts this productive interval; leave the danger area or shorten the action")
		}
		enemyActorID, enemyActor, err := adventureEncounterActor(*system.AdventureRules, projection, *input.Adventure)
		if err != nil {
			return turnWorldPlan{}, err
		}
		if enemyActorID != "" {
			if enemyActor.Health <= 0 {
				return turnWorldPlan{}, fmt.Errorf("opponent is incapacitated")
			}
			bindAdventureEncounter(next.Battle, enemyActorID, enemyActor, *system.AdventureRules)
		}
		plan.ops = adventureActorOps(next, turnID)
		action := input.Adventure
		switch action.Action {
		case "learn", "train":
			plan.narrativeResult = "研习预结算结果：" + outcome.Message
			switch level := next.Skills[action.SkillID]; {
			case level == 0:
				plan.narrativeResult += "目前只累积研习进度，尚未学会，不能施展。"
			case a.Skills[action.SkillID] == 0:
				plan.narrativeResult += "本次已学会该技能，无需再通过考核才算学会；可将后续考核写成应用或检验。"
			default:
				plan.narrativeResult += "按实际掌握程度叙述，不另设生效条件。"
			}
		case "work", "buy", "sell", "craft":
			plan.narrativeResult = fmt.Sprintf("本轮正式行动预结算：%s 钱财从%d到%d%s，消耗%d秒（1小时3600秒，1时辰7200秒）。正文只写此次实际交付、收支和经过的时间，旧约定仍以正式记录为准。",
				outcome.Message, a.Currency, next.Currency, system.AdventureRules.Currency, outcome.DurationSeconds)
		}
		if action.Action == "craft" {
			if system.AdventureRules.Family == "space" {
				previous := a.Crafting[action.RecipeID].WorkedHours
				plan.narrativeResult += fmt.Sprintf(" 星海采用现代计时，旁白与人物对白均用小时、分钟、秒，不使用时辰或刻；不要重复旧对白里的古代计时说法。本次制作%d小时，这批累计%d小时；若提及时长，只引用这两个正式值，不必逐段报时。", outcome.DurationSeconds/3600, int64(previous)+outcome.DurationSeconds/3600)
			}
			if work, continuing := a.Crafting[action.RecipeID]; continuing {
				plan.narrativeResult += fmt.Sprintf(" 本次接续已保存的%d小时工时，只新增%d小时；当前起点仍是%s。暂停、读档或重开不等于过夜，本轮没有另行结算等待或睡眠，开头直接接上原现场，不写第二天才回来。过程写接续操作即可，不必逐时辰报时；旧正文的额外工时或过夜说法不能覆盖本次正式结果。", work.WorkedHours, outcome.DurationSeconds/3600, storySceneText(scene, storySceneTimeField))
			}
		}
		if outcome.DurationSeconds > 0 {
			clock := NewStoryClock(storySceneText(scene, storySceneTimeField))
			if projection != nil && projection.Clock != nil {
				clock = *projection.Clock
			}
			after, err := AdvanceStoryClock(clock, outcome.DurationSeconds)
			if err != nil {
				return turnWorldPlan{}, err
			}
			plan.wait = &storyClockAdvance{Before: clock, After: after}
		}
	case TurnWorldActionJourney:
		// The lifespan edge is read from scene rather than from the turn's writes,
		// and that is correct here even though the edge is arithmetic: a turn's
		// world module is exclusive with its cultivation module, so no breakthrough
		// can happen in the same turn that walks. The realm in scene is therefore
		// the realm the road starts in and the one the crossing check will judge.
		var currentFacts *TurnCharacterFactsInput
		if len(facts) > 0 {
			currentFacts = facts[0]
		}
		var known []string
		if projection.Characters != nil {
			known = projection.Characters.CharacterIDs()
		}
		companions, err := validateJourneyCompanions(scene, projection.State, strings.TrimSpace(input.RouteID), input.JourneyCompanions, currentFacts, known)
		if err != nil {
			return turnWorldPlan{}, err
		}
		journeyReq.companionSources = companions
		stretch, err := planJourneyStretch(projection, scene, branchID, system, journeyReq, newID("jnyrec"), lifespanBoundary(meta, scene, DefaultActorID))
		if err != nil {
			return turnWorldPlan{}, err
		}
		plan.journey = &stretch
		plan.ops = stretch.Ops
	case TurnWorldActionQuest:
		action, err := planQuestAction(projection, system, questReq)
		if err != nil {
			return turnWorldPlan{}, err
		}
		// §4.10: a task is raised in a world and by a person this story really
		// has. The entry asks the same question before offering it, so a task the
		// player was never shown cannot be accepted by naming its id.
		if !questDefinitionEligible(meta, action.Definition) || !questActionDiscovered(projection, action.Definition, action.Action) {
			return turnWorldPlan{}, fmt.Errorf("%w: %s is not part of this story's world", ErrQuestNotHere, action.Definition.ID)
		}
		plan.quest = &action
		plan.ops = action.Ops
	case TurnWorldActionPublicEvent:
		action, err := planPublicEventAction(projection, scene, meta, system, eventReq)
		if err != nil {
			return turnWorldPlan{}, err
		}
		plan.event = &action
		plan.ops = action.Ops
	case TurnWorldActionRealm:
		action, err := planRealmAction(projection, scene, meta, branchID, realmReq)
		if err != nil {
			return turnWorldPlan{}, err
		}
		plan.realm = &action
		plan.ops = action.Ops
	case TurnWorldActionWait:
		var clock StoryClock
		if projection != nil && projection.Clock != nil {
			clock = *projection.Clock
		} else {
			// The projection carries this turn's proposed state writes, but a
			// first wait must anchor to the scene at turn start. The reducer
			// replays the clock advance before applying those writes.
			clock = NewStoryClock(storySceneText(scene, storySceneTimeField))
		}
		if err := clock.Validate(); err != nil {
			return turnWorldPlan{}, err
		}
		after, err := AdvanceStoryClock(clock, adventureOpeningBoundary(scene, clock.Seconds, clock.Seconds+input.DurationSeconds, projection.worldContent())-clock.Seconds)
		if err != nil {
			return turnWorldPlan{}, fmt.Errorf("wait interval refused: %w", err)
		}
		plan.wait = &storyClockAdvance{Before: clock, After: after}
	default:
		return turnWorldPlan{}, fmt.Errorf("unknown world action %q", plan.action)
	}
	return plan, nil
}

// storyCommittedState is the state a turn found the branch in.
//
// The world planners take the preconditions from it — where the character
// really stands, which realm they are outside of — while the turn's own writes
// answer only what the settlement owes. A nil projection reads as nil rather
// than as an empty map, so a caller that has no branch keeps the planner's own
// "nothing to check" path instead of silently answering "no place".
func storyCommittedState(projection *storyBranchProjection) map[string]any {
	if projection == nil {
		return nil
	}
	return projection.State
}

// clockSeconds reports the formal instant this world action leaves the branch
// on, or zero when the action spends no time.
//
// A journey and a participation both carry their own committed instant; taking a
// task and entering a realm do not move the clock at all. The commit reads this
// to know where the branch really stands after the turn, which is what a
// lifespan crossing is judged against.
func (plan turnWorldPlan) clockSeconds() int64 {
	if plan.cooperation != nil && plan.cooperation.ClockAfter != nil {
		return plan.cooperation.ClockAfter.Seconds
	}
	if plan.realm != nil && plan.realm.ClockAfter != nil {
		return plan.realm.ClockAfter.Seconds
	}
	if plan.quest != nil && plan.quest.ClockAfter != nil {
		return plan.quest.ClockAfter.Seconds
	}
	if plan.wait != nil {
		return plan.wait.After.Seconds
	}
	if plan.journey != nil && plan.journey.ClockAfter.Validate() == nil {
		return plan.journey.ClockAfter.Seconds
	}
	if plan.event != nil && plan.event.Clock.Validate() == nil {
		return plan.event.Clock.Seconds
	}
	return 0
}

// turnWorldSettlementEvents turns a planned world action into the side records
// the commit writes, parented after whatever the turn left at the branch head.
//
// The action's own state writes are not repeated here: they are in the turn's
// StateDelta, and writing them a second time would let a replay apply them
// twice. What the records carry is the fact the world changed — the stretch of
// road walked, the task's new state, the participation, the realm instance — and
// any task objective that fact ticks.
func turnWorldSettlementEvents(
	projection *storyBranchProjection,
	system StoryDirectorActorStateSystem,
	branchID, parentID, now, turnID string,
	input *TurnWorldActionInput,
	plan turnWorldPlan,
) ([]any, string, error) {
	journeyReq, questReq, eventReq, realmReq := turnWorldRequests(branchID, "", input)
	switch plan.action {
	case TurnWorldActionLocalAffair:
		event := LocalAffairRecordEvent{V: schemaVersion, Type: StoryEventTypeLocalAffairRecord, ID: newID("affair"), ParentID: parentID, BranchID: branchID, Ts: now, TurnID: turnID, Action: input.LocalAffair.Action, State: cloneLocalAffair(*plan.localAffair)}
		event.State.LastEventID = event.ID
		if plan.wait != nil {
			event.ClockBefore = cloneStoryClock(&plan.wait.Before)
			event.ClockAfter = cloneStoryClock(&plan.wait.After)
			for i := range event.State.Evidence {
				if event.State.Evidence[i].SourceEventID == "pending-inspection" {
					event.State.Evidence[i].SourceEventID = event.ID
				}
			}
		}
		if event.State.OriginEventID == "" {
			event.State.OriginEventID = event.ID
		}
		if err := validateLocalAffairRecord(event); err != nil {
			return nil, "", err
		}
		events := []any{event}
		head := event.ID
		if plan.wait != nil {
			closed, next := storyIntervalEvents(projection, system, branchID, head, now, turnID, "", plan.wait.Before, plan.wait.After)
			events = append(events, closed...)
			head = next
		}
		return events, head, nil
	case TurnWorldActionFaction:
		return factionSettlementEvents(projection, system, branchID, parentID, now, turnID, *plan.faction)
	case TurnWorldActionCooperation:
		return cooperationSettlementEvents(projection, system, branchID, parentID, now, turnID, *plan.cooperation)
	case TurnWorldActionJourney:
		journeyReq.TurnID = turnID
		record := journeyRecordEvent(branchID, parentID, now, journeyReq, *plan.journey)
		headID := record.ID
		newEvents := []any{record}
		// An arrival is a fact, and a task objective may be waiting on it. The
		// tick is appended to this same transaction so the road and the task
		// cannot be separated by a crash.
		if plan.journey.Advanced.Done {
			ticks, _, err := questAdvanceEvents(projection, branchID, turnID, questFactFromJourneyArrival(plan.journey.Advanced.Position.LocationID), record.ID, headID, now, plan.journey.ClockAfter.Seconds)
			if err != nil {
				return nil, "", err
			}
			for _, tick := range ticks {
				newEvents = append(newEvents, tick)
				if questRecord, ok := tick.(QuestRecordEvent); ok {
					headID = questRecord.ID
				}
			}
		}
		// The stretch of road spent the interval between the two instants, so a
		// matter whose deadline fell inside it closes even though the turn was
		// about travelling.
		closed, headID := storyIntervalEvents(intervalProjectionAfterOps(projection, plan.journey.Ops), system, branchID, headID, now, turnID, "", plan.journey.Clock, plan.journey.ClockAfter)
		newEvents = append(newEvents, closed...)
		return newEvents, headID, nil
	case TurnWorldActionQuest:
		questReq.TurnID = turnID
		recordID := newID("qstrec")
		record := questRecordEvent(branchID, parentID, now, questReq, *plan.quest, recordID)
		events := []any{record}
		head := record.ID
		if echo := questEchoCharacterEvent(branchID, head, now, turnID, *plan.quest); echo != nil {
			events = append(events, *echo)
			head = echo.ID
		}
		if plan.quest.ClockAfter != nil {
			view := questProjectionAfter(projection, *plan.quest)
			closed, next := storyIntervalEvents(view, system, branchID, head, now, turnID, "", *plan.quest.ClockBefore, *plan.quest.ClockAfter)
			events = append(events, closed...)
			head = next
		}
		return events, head, nil
	case TurnWorldActionPublicEvent:
		recordID := newID("evtrec")
		record := publicEventRecordEvent(branchID, parentID, now, eventReq, *plan.event, recordID)
		return []any{record}, record.ID, nil
	case TurnWorldActionRealm:
		recordID := newID("rlmrec")
		realmReq.TurnID = turnID
		record := realmRecordEvent(branchID, parentID, now, realmReq, *plan.realm, recordID)
		headID := record.ID
		newEvents := []any{record}
		if used, err := cooperationUseEvent(projection, branchID, headID, now, turnID, record); err != nil {
			return nil, "", err
		} else if used != nil {
			newEvents = append(newEvents, *used)
			headID = used.ID
		}
		// Taking a declared reward is a fact, and a task objective may be waiting
		// on it. The tick lands here so a task cannot complete on a reward the
		// branch did not really take.
		if plan.realm.Reward != nil {
			ticks, _, err := questAdvanceEvents(projection, branchID, turnID, questFactFromRealmReward(*plan.realm.Reward), record.ID, headID, now, projection.clockSeconds())
			if err != nil {
				return nil, "", err
			}
			for _, tick := range ticks {
				newEvents = append(newEvents, tick)
				if questRecord, ok := tick.(QuestRecordEvent); ok {
					headID = questRecord.ID
				}
			}
		}
		for _, reward := range plan.realm.Rewards {
			ticks, _, err := questAdvanceEvents(projection, branchID, turnID, questFactFromRealmReward(reward), record.ID, headID, now, plan.realm.ClockAfter.Seconds)
			if err != nil {
				return nil, "", err
			}
			for _, tick := range ticks {
				newEvents = append(newEvents, tick)
				if r, ok := tick.(QuestRecordEvent); ok {
					headID = r.ID
				}
			}
		}
		if plan.realm.ClockBefore != nil && plan.realm.ClockAfter != nil {
			closed, next := storyIntervalEvents(intervalProjectionAfterOps(projection, plan.realm.Ops), system, branchID, headID, now, turnID, "", *plan.realm.ClockBefore, *plan.realm.ClockAfter)
			newEvents = append(newEvents, closed...)
			headID = next
		}
		return newEvents, headID, nil
	case "adventure", TurnWorldActionWait:
		if plan.wait == nil {
			return nil, parentID, nil
		}
		events, headID := storyIntervalEvents(intervalProjectionAfterOps(projection, plan.ops), system, branchID, parentID, now, turnID, "", plan.wait.Before, plan.wait.After)
		return events, headID, nil
	}
	return nil, parentID, nil
}
