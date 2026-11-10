package quest

import "encoding/json"

// This file registers the first authored tasks: one ordinary hunt and one
// personal matter raised by a named person.
//
// 自制-事件与任务 §9 asks for the first batch to form a real loop with
// capabilities the build already has — a hunt, a delivery, a bit of work — and
// §4.10 asks for at least one personal card from an important NPC. Both tasks
// below satisfy that by naming only facts the rules already settle: a realm
// reward that was really taken, a journey that really arrived, a battle that
// really ended. No objective here can be ticked by prose.
//
// The two are deliberately small. §4.6 forbids inventing a task's mechanical
// content to fit a scene, so where a fact does not exist in this build the task
// asks for something that does.

const (
	// TaskHerbHandover is an ordinary errand: fetch the pill the old cave holds
	// and bring it back.
	TaskHerbHandover = "quest.herb_handover"
	// TaskSuQianMaterials is 苏浅's personal card P02, "分清这一批药材".
	TaskSuQianMaterials = "quest.su_qian_materials"
)

// herbHandoverDefinition is a plain errand with a plain reward.
//
// Its objective is a realm reward rather than "find a herb", because the realm
// is where a herb can really be found in this build. The task therefore tests
// the loop the queue asks for — a real errand that ends in a real item the
// character keeps — without pretending an herb-gathering system exists.
var herbHandoverDefinition = Definition{
	SchemaVersion: SchemaVersion,
	ID:            TaskHerbHandover,
	Version:       "1.0",
	// Both tasks name the same world pack revision their objectives are settled
	// from: the realm reward and the journey arrival are facts of 凡人人界 3.
	PackageID:         "fanren-jie",
	PackageVersion:    "3",
	Kind:              KindErrand,
	Title:             "药圃里的封口药瓶",
	Brief:             "坊市与附近散修人家传着一则求药消息：有人愿出灵石，求购古修洞府里尚能使用的丹药。",
	PublicLocationIDs: []string{"yue.market.outskirts", "yue.market"},
	Objectives: []Objective{
		{
			ID:          "quest.herb_handover.pill",
			Description: "在古修洞府的药圃里取回那只封口药瓶。",
			// The realm's own content reference is what the instance records
			// when the reward is really taken, so the objective follows the
			// grant rather than a report of it.
			Evidence: []Evidence{{Kind: EvidenceRealmReward, RefID: "realm.herb_ref"}},
		},
	},
	// 20 下品灵石 is the same order as the build's ordinary trade prices, so a
	// small errand does not pay like a treasure.
	Rewards: []Reward{{Kind: RewardCurrency, RefID: "灵石", Quantity: 20}},
}

// suQianMaterialsDefinition is P02 of the personal event cards, mapped onto the
// facts this build can judge.
//
// The card asks the player to help 苏浅 settle what a batch of materials is for
// and who is responsible for it. Its two authored outcomes — the batch is
// checked, and the place it came from is actually seen — are both things the
// build already settles: a realm reward proves the character went and looked,
// and a journey arrival proves they really went to the source rather than
// reporting on it from memory.
//
// The reward is currency rather than a pill, because the card's own text says
// there is no free medicine: she pays for the work, not for the relationship.
var suQianMaterialsDefinition = Definition{
	SchemaVersion:  SchemaVersion,
	ID:             TaskSuQianMaterials,
	Version:        "1.0",
	PackageID:      "fanren-jie",
	PackageVersion: "3",
	Kind:           KindPersonal,
	Title:          "分清这一批药材",
	Brief:          "苏浅手上有一批待交接的材料，用途与交接记录对不上。她想知道实际记录是什么，也想知道这批东西究竟从哪来。",
	Giver:          "npc_su_qian",
	CompletionEcho: &CompletionEcho{AfterSeconds: 3 * 24 * 60 * 60, Reaction: EchoReaction{CharacterSourceKey: "npc_su_qian", Statement: "你交回的实物与来处记录都对上了。这一批已经交接清楚；下次遇到难辨的来路，我愿先同你商量，但买药与学艺仍要各按规矩。"}},
	// 苏浅 has a handover of her own to make, so she asks for an answer within
	// two days rather than leaving it open. The interval is authored here and
	// stamped onto the instance at acceptance, which is what keeps two
	// acceptances of the same task a week apart on their own separate limits.
	DeadlineSeconds: 2 * 24 * 60 * 60,
	Objectives: []Objective{
		{
			ID:          "quest.su_qian_materials.check",
			Description: "核对记录里提到的材料，确认它确实存在、也确实可取。",
			Evidence:    []Evidence{{Kind: EvidenceRealmReward, RefID: "realm.herb_ref"}},
		},
		{
			ID:          "quest.su_qian_materials.source",
			Description: "亲自到这批材料所出的地方看一眼，而不是只凭转述。",
			// yue.market is the destination of the one registered route, so
			// arriving there is a fact the journal really records. The label the
			// panel shows is the route's own wording, not this id.
			Evidence: []Evidence{{Kind: EvidenceJourneyArrival, RefID: "yue.market"}},
		},
	},
	Rewards: []Reward{{Kind: RewardCurrency, RefID: "灵石", Quantity: 30}},
}

// taskDefinitions is the registered set, in a stable order so the entry offers
// them predictably.
var taskDefinitions = []Definition{herbHandoverDefinition, suQianMaterialsDefinition}

// Definition returns one authored task by identifier, as a copy all the way
// down.
//
// The deep copy matters for the same reason a realm's does: returning the
// struct by value would still share every slice, so a caller that edited an
// objective would rewrite the authored task for everyone.
func DefinitionByID(id string) (Definition, bool) {
	for _, definition := range taskDefinitions {
		if definition.ID == id {
			return cloneDefinition(definition), true
		}
	}
	return Definition{}, false
}

// Definitions returns every authored task, in registration order.
func Definitions() []Definition {
	cloned := make([]Definition, 0, len(taskDefinitions))
	for _, definition := range taskDefinitions {
		cloned = append(cloned, cloneDefinition(definition))
	}
	return cloned
}

func cloneDefinition(definition Definition) Definition {
	if len(definition.Choices) > 0 || definition.RequiresEcho != nil || definition.CompletionEcho != nil {
		data, _ := json.Marshal(definition)
		var cloned Definition
		_ = json.Unmarshal(data, &cloned)
		return cloned
	}
	definition.PublicLocationIDs = append([]string(nil), definition.PublicLocationIDs...)
	objectives := make([]Objective, len(definition.Objectives))
	for index, objective := range definition.Objectives {
		objective.Evidence = append([]Evidence(nil), objective.Evidence...)
		objectives[index] = objective
	}
	definition.Objectives = objectives
	definition.Rewards = append([]Reward(nil), definition.Rewards...)
	return definition
}
