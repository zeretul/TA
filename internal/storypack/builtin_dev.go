//go:build !embedweb

package storypack

// devBuiltinPacks are the packs a development build seeds in addition to the
// delivered ones. The detective pack is a test world: its setting, catalog and
// opening are not part of what a player receives, so it is compiled in only
// when the frontend is not embedded — that is, in `go test` and a plain
// `go build`. A delivered build passes `-tags embedweb` and does not carry it
// at all, which is what keeps it out of the builtin directory and the default
// resources rather than merely off the page.
//
// The detective rules modules themselves (dm-mystery-investigation-stats-v1,
// detective-investigation-v1) stay in internal/interactive: they are general
// investigation capability and a user-imported mystery world uses them.
func devBuiltinPacks() []string {
	return []string{builtinPackJSONDetective}
}

const builtinPackJSONDetective = `{
  "version": 1,
  "id": "wumian-zhentan",
  "name": "雾夜侦探",
  "description": "民国风格的都市推理世界：一起看似寻常的失踪案，牵出码头、报馆与租界巡捕房之间的暗线。玩家以私家侦探身份在有限线索中做出判断。",
  "tags": ["推理", "民国", "都市"],
  "story": {
    "title": "雾夜侦探",
    "origin": "民国二十三年，沪上入秋的第一场大雾。你的事务所开在法租界霞飞路尽头，招牌上的漆已经掉了大半。今早七点，一位穿墨绿旗袍的女士敲开了门。",
    "protagonist_name": "陆知白",
    "protagonist": "一名从巡捕房退下来的私家侦探，观察力强，熟悉沪上各方势力，不轻易动武，习惯用提问逼出对方的破绽。",
    "reply_target_chars": 2500,
    "choice_count": 3,
    "opening_text": "雾从黄浦江上漫过来，把霞飞路的梧桐泡得发黑。你刚给自己倒上第二杯茶，门就被敲响了——三下，短促，带着克制的急。门外站着一位穿墨绿旗袍的女士，手里攥着一只男式皮手套，指节因为用力而发白。她说她丈夫昨夜没有回家，而巡捕房告诉她，成年男子失踪不足四十八小时不予立案。她顿了顿，补了一句：可是昨天晚上，有人往我家门缝里塞了这张照片。",
    "module_refs": {
      "event_package_ids": ["default"],
      "rule_system_id": "dm-mystery-investigation-stats-v1",
      "actor_state_id": "detective-investigation-v1",
      "image_preset_id": "realistic"
    }
  },
  "world": [
    {
      "id": "wumian-shanghai",
      "name": "沪上租界",
      "type": "location",
      "importance": "high",
      "tags": ["地点", "世界设定"],
      "keywords": ["法租界", "公共租界", "巡捕房", "霞飞路"],
      "load_mode": "resident",
      "brief_description": "民国二十三年沪上租界的基本格局。",
      "content": "民国二十三年的沪上被租界切割成几块彼此交错的辖区。法租界与公共租界各有自己的巡捕房、法庭与监狱，华界则由市警察局管辖。案子一旦跨过租界线，证据移交、人员协查都会变得极其缓慢。租界内帮会、报馆、洋行与烟馆并存，消息流通极快，但每一条消息背后都有出价的人。"
    },
    {
      "id": "wumian-yuanming",
      "name": "袁明远",
      "type": "character",
      "importance": "high",
      "tags": ["关键人物", "失踪者"],
      "keywords": ["袁明远", "丈夫", "失踪"],
      "load_mode": "auto",
      "brief_description": "失踪的丈夫，某洋行账房先生。",
      "content": "袁明远，三十四岁，法租界某英资洋行的账房先生，负责三处码头货栈的进出账目。为人谨慎，不赌不嫖，同事评价是『无趣得让人放心』。最后一次被人看见是昨夜十点，在十六铺码头附近的电车终点站。他随身带走的只有一只皮手套——另一只留在了家里。"
    },
    {
      "id": "wumian-photo",
      "name": "门缝里的照片",
      "type": "item",
      "importance": "high",
      "tags": ["关键物品", "线索"],
      "keywords": ["照片", "线索", "威胁"],
      "load_mode": "auto",
      "brief_description": "被塞进门缝的照片，是本案第一条实物线索。",
      "content": "一张三寸大小的黑白照片，边缘有裁切过的痕迹。画面拍的是十六铺码头的一处货栈门口，夜里，有人正把几只木箱搬上板车。照片上没有日期，没有署名，但角落处露出半截袖口——那是洋行账房先生常穿的那种深灰呢料。照片背面用铅笔写着一行小字：『别再问了』。"
    },
    {
      "id": "wumian-shen",
      "name": "沈曼青",
      "type": "character",
      "importance": "high",
      "tags": ["关键人物", "委托人"],
      "keywords": ["沈曼青", "女士", "委托人"],
      "load_mode": "auto",
      "brief_description": "前来委托的女士，袁明远的妻子。",
      "content": "沈曼青，二十九岁，出身沪上没落书香门第，在圣玛利亚女中做过三年国文教员。举止克制，说话条理清楚，很少流露情绪。委托时她只说丈夫失踪，没有提任何怀疑对象，也没有提钱——她先把一枚翡翠镯子放在桌上，说这是定金。她左手无名指上戴着一枚素圈婚戒，戒圈内侧磨损得很厉害。"
    }
  ]
}`
