package publiquevent

// 太岳灵潮 is the first public event this build really runs.
//
// It is original rather than a retelling of a scene from 凡人修仙传, which is what
// the queue's "选一条原著关联或原创公共事件" allows: the region, the market and the
// kind of trouble a spirit tide brings are consistent with the world, and no
// authored plot is forced on the player. Nobody is told that a named character
// must be met or that a particular thing must happen.
//
// The shape of it is the ordinary shape of a public situation: a rumour that
// turns into a rush, a peak that hurts people who were not ready, and a
// settling that pays out according to what one actually did. It advances on the
// story's formal clock alone, so a character who spends three days cultivating
// comes back to a different stage of it without anybody scheduling anything.
//
// The times are deliberately coarse — hours and days, not turns — because §4.8
// asks for a real deadline. Six hours is long enough that an ordinary errand does
// not miss the rush, and five days is short enough that a character who goes into
// seclusion for a fortnight finds it over.

// PublicEventIDTaiyueTide names the event as it is stored on the journal.
const PublicEventIDTaiyueTide = "evt.taiyue_spirit_tide"

// taiyueTide is the authored definition.
var taiyueTide = Definition{
	SchemaVersion:  SchemaVersion,
	ID:             PublicEventIDTaiyueTide,
	Version:        "1.0",
	PackageID:      "fanren-jie",
	PackageVersion: "3",
	Title:          "太岳灵潮",
	Brief:          "太岳山的灵气这几年最厚的一回，采药人、行商和散修都往山里去。坊市里人人都在说这件事，可谁也说不好山里到底出了什么。",
	Region:         "越国坊市",
	// RegionLocationIDs are the registered places this event can be heard of
	// from. The event exists whether or not anyone is standing there, but §6
	// means a character somewhere else has not been told about it, so the panel
	// must not show it to them. Naming the places makes that a fact rather than a
	// guess about the scene's free text.
	RegionLocationIDs: []string{"yue.market"},
	Participants: []Participant{
		{
			ID:         "npc_herb_clerk",
			Name:       "收药的伙计",
			PublicRole: "在坊市口摆着秤收太岳药材，价钱一天一变。",
			// §6: this is never handed to a player view.
			SecretMotive: "东家让他压着价收，等灵潮过了再抬价卖出去，所以他嘴上说山里没什么好东西。",
		},
		{
			ID:           "npc_old_gatherer",
			Name:         "老采药人",
			PublicRole:   "在太岳山里走了几十年，认得哪条道能走、哪条道不能走。",
			SecretMotive: "他知道今年塌了两条近路，却不愿说，怕别人抢在他前头进山。",
		},
		{
			ID:           "npc_market_steward",
			Name:         "坊市执事",
			PublicRole:   "管着临时集市的摊位和过路钱，说要给进山的人立个规矩。",
			SecretMotive: "他想借这场灵潮抽一笔过路钱，账目只报给上头一半。",
		},
	},
	Stages: []Stage{
		{
			ID:      "taiyue.rumor",
			Title:   "风声",
			Summary: "坊市里都在说太岳山的灵气比往年厚，采药人开始结伴往山里走。没人说得清山里究竟有什么，只知道今年进山的人比去年多。",
			ParticipantLines: []string{
				"收药的伙计说山里没什么好东西，可他收药材的价一天比一天高。",
				"老采药人在摊子前停了很久，最后只买了一捆绳子和两天的干粮。",
			},
			OpenAfterSeconds: 0,
		},
		{
			ID:      "taiyue.rush",
			Title:   "上山的人",
			Summary: "进山的道被踩宽了，坊市口的药材价一天一变。有几家进山的人没按时回来，家里人开始在坊市口等。",
			ParticipantLines: []string{
				"老采药人承认山里塌了两条近路，绕路要多走一天半。",
				"收药的伙计把价钱压得很低，说山上下来的货都是急着出手的。",
				"坊市执事在道口立了块木牌，进山要交十枚灵石。",
			},
			SecretLines: []string{
				"伙计压价是东家的意思，货都囤在西边仓里等灵潮过去。",
			},
			OpenAfterSeconds: 6 * 3600,
		},
		{
			ID:      "taiyue.surge",
			Title:   "灵气最盛",
			Summary: "山里灵气到了最盛的时候，兽群从深处被赶出来，进山的路断了两处。坊市口有人抬着伤者回来，也有人说山里的东西好得不像话。",
			ParticipantLines: []string{
				"受伤的人说兽群是被灵气逼出来的，不是冲着人来的。",
				"老采药人带着一队人绕远路回来了，队伍里少了两个。",
				"执事临时加派了人守道口，可他自己的人也在往山里搬东西。",
			},
			SecretLines: []string{
				"少的那两个人是走了塌掉的那条近路，老采药人没有拦。",
				"执事往山里搬的是空箱子，他要收的是别人带不出来的东西。",
			},
			OpenAfterSeconds: 48 * 3600,
		},
		{
			ID:      "taiyue.settle",
			Title:   "潮落",
			Summary: "灵气退了，进山的人陆续回来。坊市把这次的账算清：谁带回来了什么，谁没有回来，过路钱怎么处置，都在这个时候有个说法。",
			ParticipantLines: []string{
				"坊市执事把过路钱的账摊开给众人看，数目和他先前说的对不上。",
				"收药的伙计在潮落那天关了仓，仓里的东西被执事扣下了一半。",
				"老采药人把塌掉的那条近路指给了坊市，说以后该立个牌。",
			},
			SecretLines: []string{
				"执事扣下的一半货物，最后有一半进了他自己的库。",
			},
			OpenAfterSeconds: 5 * 24 * 3600,
			Final:            true,
		},
	},
	Actions: []Action{
		{
			Kind:  ActionObserve,
			Label: "在坊市里打听",
			Brief: "不进山，只在坊市里看、听、问。能知道别人愿意说的，也知道别人不愿意说的。",
			Reward: &Reward{
				Kind: RewardCurrency, RefID: "灵石", Quantity: 5,
			},
		},
		{
			Kind:  ActionHelp,
			Label: "跟着采药人进山",
			Brief: "帮着背东西、认路、把伤者抬回来。做的是实活，分的是实钱。",
			Reward: &Reward{
				Kind: RewardCurrency, RefID: "灵石", Quantity: 15,
			},
		},
		{
			Kind:  ActionInvestigate,
			Label: "查清山里的实情",
			Brief: "为什么今年灵气这么厚、塌掉的路是谁的事、过路钱去了哪里。查到底的人拿到的比干活的人多，也更招人记恨。",
			Reward: &Reward{
				Kind: RewardCurrency, RefID: "灵石", Quantity: 25,
			},
		},
		{
			Kind:  ActionWithdraw,
			Label: "不掺和",
			Brief: "这场灵潮与你无关。你没有从里面拿到什么，也没有欠下什么。",
			// No reward: standing aside really pays nothing. It is still a
			// recorded decision, so a later stage knows the character chose out
			// rather than simply never hearing about it.
		},
	},
}

const (
	PublicEventIDTideGauge = "evt.lanchuan_tide_gauge"
	TideGaugeBeforeRise    = "tide_gauge.before_rise"
	TideGaugeAfterRise     = "tide_gauge.after_rise"
)

var tideGauge = Definition{
	SchemaVersion:     SchemaVersion,
	ID:                PublicEventIDTideGauge,
	Version:           "1.0",
	PackageID:         "lanchuan-qunxia",
	PackageVersion:    "1",
	Title:             "潮信渡的松扣",
	Brief:             "潮信渡到白鹭渔村仍可走十二小时旱路。想随船走，先看看渡头木牌。",
	Region:            "潮信渡",
	RegionLocationIDs: []string{"lanchuan-l02"},
	Participants: []Participant{
		{ID: "lanchuan-p01", Name: "宋老橹", PublicRole: "老渡公，在渡头贴出当天能走的水路。"},
	},
	Stages: []Stage{
		{
			ID:               TideGaugeBeforeRise,
			Title:            "涨水前",
			Summary:          "渡头木牌写着：下次涨水前，短渡须重新校准。宋老橹站在潮尺旁，正等人来帮忙对照石碑水线。",
			ParticipantLines: []string{"你和宋老橹对上石碑水线，压紧滑落的拴扣。他记下短渡水线，答应凭这次校准带你走十小时短渡，涨水以后也认这道记号。"},
		},
		{
			ID:               TideGaugeAfterRise,
			Title:            "涨水后",
			OpenAfterSeconds: 36 * 3600,
			Summary:          "渡头换了木牌：短渡只走涨水前校过的水线。错过校准的客人可找宋老橹复核外汊，或走原来的旱路。木牌上没有写潮尺出了什么问题。",
			ParticipantLines: []string{"宋老橹指给你看滑落的拴扣。旧刻痕已没在水下，你们到外桩复核水线，只能确认十一小时的外汊水路。"},
		},
	},
	Actions: []Action{
		{Kind: ActionHelp, StageID: TideGaugeBeforeRise, Label: "找宋老橹校准潮尺", Brief: "对照石碑与潮尺，确认十小时短渡。涨水前校准后，涨水前后都能出发。"},
		{Kind: ActionInvestigate, StageID: TideGaugeAfterRise, Label: "找宋老橹复核外汊", Brief: "错过了短渡校准，复核后可走十一小时外汊水路。"},
	},
}

// Definitions returns every authored public event.
func Definitions() []Definition {
	return []Definition{taiyueTide, tideGauge}
}

// DefinitionByID finds an authored event.
func DefinitionByID(id string) (Definition, bool) {
	for _, definition := range Definitions() {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

// No seeding is needed, and that is the point.
//
// A public event is not a row that has to be created when a story starts: its
// situation is a pure function of the branch's committed clock, so every branch
// has it as soon as the clock is past the first stage. Only the character's own
// participation is a fact worth storing, and that is written when they choose.
//
// The consequence is the one the queue asks for: no background scheduler, no
// world tick, and no way for a story to be told an event advanced that the
// elapsed time does not support.
