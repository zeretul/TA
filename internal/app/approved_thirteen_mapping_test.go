package app

import (
	"fmt"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"testing"
)

// These are synthetic committed-source engineering cases, not real meetings.
// Profile facts here are explicit fixtures. Real unknown profiles stay locked.
type approvedMappingCase struct {
	assetID, source, name, role, gender, form, timeSemantics, roleSemantics, roleRecognition, recognition, worldTime, location string
	year                                                                                                                       int
	adult                                                                                                                      bool
}

func approvedMappingFixture(t *testing.T, spec approvedMappingCase) (media.StoryAsset, interactive.StoryContext, character.State) {
	t.Helper()
	p := character.Profile{ID: "sinistra", StoryID: "st", Name: spec.name, KnownAs: spec.name, SourceKey: spec.source, Role: spec.role, Lifeform: "human", Gender: spec.gender, Revision: 1}
	appearance := spec.form + "，正常衣着"
	location := spec.location
	if location == "" {
		location = "已提交的实际会面地点"
	}
	scene := character.Scene{NodeID: "turn", WorldTime: spec.worldTime, Location: location, Present: []string{"protagonist", "sinistra"}}
	n := 0
	facts := character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: "晚上好。"}, {CharacterID: p.ID, Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: "protagonist", Text: spec.recognition}}, Knowledge: []character.KnowledgeDraft{{CharacterID: "protagonist", Statement: "亲眼看见并认识此人。", Nature: character.NatureExperienced, Channel: character.ChannelScene, Visual: &character.VisualObservationDraft{SubjectID: p.ID, Form: spec.form, ContactExchangeIndex: 1, RecognitionExchangeIndex: 2}}}}
	projected, err := character.Project(character.ProjectionContext{StoryID: "st", BranchID: "main", TurnID: "turn", CommitID: "commit", WorldTime: scene.WorldTime, Scene: scene, KnownCharacters: []string{p.ID}, VisualContext: &character.VisualObservationContext{ObserverID: "protagonist", Subjects: map[string]character.VisualSubject{p.ID: {Profile: p, Appearance: appearance}}}, NextID: func(prefix string) string { n++; return fmt.Sprintf("%s%d", prefix, n) }}, facts)
	if err != nil {
		t.Fatal(err)
	}
	state := character.State{Profiles: []character.Profile{p}, Exchanges: projected.Exchanges, Knowledge: projected.Knowledge}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{SourceKey: p.SourceKey, Name: p.Name}}}}, Snapshot: interactive.Snapshot{StoryID: "st", BranchID: "main", CurrentTurn: &interactive.TurnEvent{ID: "turn"}, State: map[string]any{"actors": map[string]any{"story": map[string]any{"state": map[string]any{"当前时间": scene.WorldTime, "当前详细地点": scene.Location, "在场角色": []any{"protagonist", p.ID}}}, p.ID: map[string]any{"state": map[string]any{"外貌与装扮": appearance}}}}}}
	asset := media.StoryAsset{ID: spec.assetID, SourceKey: p.SourceKey, Reviewed: true, Rule: &media.StoryRule{Kind: "observed_named_portrait", Year: spec.year, TimeSemantics: spec.timeSemantics, RoleSemantics: spec.roleSemantics, RoleRecognition: spec.roleRecognition, Form: spec.form, Lifeform: "human", Gender: spec.gender, KnownAs: p.KnownAs, CanonicalName: p.Name, Role: p.Role, AdultIdentity: spec.adult}}
	return asset, ctx, state
}

func TestApprovedThirteenPortraitMappings(t *testing.T) {
	cases := []approvedMappingCase{
		{assetID: "legacy-f9102706c724bffbb4aff26e2ea4394e123292eaf90dda13bc832a5d8d035f39", source: "sanguo-person-caocao", name: "曹操", role: "汉末人物", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "background_classification", roleRecognition: "", recognition: "我叫曹操。", worldTime: "190年9月1日", year: 190, adult: true},
		{assetID: "legacy-1416b431fd833f35c7e0a369d05517cc5d7fadc2d31381d08dfd014ca6cdff06", source: "sanguo-person-liubei", name: "刘备", role: "汉末人物", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "background_classification", roleRecognition: "", recognition: "我叫刘备。", worldTime: "190年9月1日", year: 190, adult: true},
		{assetID: "legacy-a7afe8cfb8c49a9c175f4c676772fd67a74a048a987d38717f38209d07d685b3", source: "sanguo-person-sun_jian", name: "孙坚", role: "汉末人物", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "background_classification", roleRecognition: "", recognition: "我叫孙坚。", worldTime: "190年9月1日", year: 190, adult: true},
		{assetID: "legacy-b33eb271b4d42b4871db682430ced00aef9c9bf21ba8e786b24ed1638abe0dd2", source: "sanguo-person-yuanshao", name: "袁绍", role: "汉末人物", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "background_classification", roleRecognition: "", recognition: "我叫袁绍。", worldTime: "190年9月1日", year: 190, adult: true},
		{assetID: "legacy-49fe780b3f123ac6c292d8b7daf399983867aa518b4c4fec97dbc14887b0bf30", source: "sanguo-person-dongzhuo", name: "董卓", role: "汉末人物", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "background_classification", roleRecognition: "", recognition: "我叫董卓。", worldTime: "190年9月1日", year: 190, adult: true},
		{assetID: "legacy-c9dd34a4a3f461a00f2cde92ade00a86fe00d60829e397d36c3dbfa9be441574", source: "sanguo-person-lvbu", name: "吕布", role: "汉末人物", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "background_classification", roleRecognition: "", recognition: "我叫吕布。", worldTime: "190年9月1日", year: 190, adult: true},
		{assetID: "legacy-c1f1c6f771ca38b99dc52fc8a91683ea70c0b766f66a3deef26da16b17e2f263", source: "sanguo-person-guanyu", name: "关羽", role: "汉末人物", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "background_classification", roleRecognition: "", recognition: "我叫关羽。", worldTime: "190年9月1日", year: 190, adult: true},
		{assetID: "legacy-d4d1b24ca69eb6238b8a140b65572c7d3574f67ec1a5af2b5c6c79a94e628aef", source: "sanguo-person-zhangfei", name: "张飞", role: "汉末人物", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "background_classification", roleRecognition: "", recognition: "我叫张飞。", worldTime: "190年9月1日", year: 190, adult: true},
		{assetID: "legacy-2d7522afdacb48167b67d3a5d62bd579d66aeeff9fe35a17ae1036f00eeadb21", source: "zc-person-sterling", name: "斯特林", role: "参与禁卫防务的军官", gender: "male", form: "人类男性", timeSemantics: "", roleSemantics: "current_role", roleRecognition: "参与禁卫防务的军官", recognition: "我是斯特林，参与禁卫防务的军官。", worldTime: "778年9月1日", year: 778, adult: false},
		{assetID: "p10-art-zc-person-liqing-v1", source: "zc-person-liqing", name: "李清", role: "军官，与斯特林有婚约", gender: "female", form: "人类女性", timeSemantics: "", roleSemantics: "current_role", roleRecognition: "军官", recognition: "我是李清，军官。", worldTime: "778年9月1日", year: 778, adult: false},
		{assetID: "legacy-615d9791155bf8973f58c5ff0e974de36ced48f233c3489575de57ebbd8f6471", source: "dp-fa-ma", name: "法犸", role: "加玛帝国炼药师公会会长，关心本国炼药人才和公会事务。", gender: "male", form: "老年人类男性", timeSemantics: "current_observation", roleSemantics: "current_role", roleRecognition: "加玛帝国炼药师公会会长", recognition: "我是法犸，加玛帝国炼药师公会会长。", worldTime: "本局第1日清晨", year: 0, adult: false},
		{assetID: "p10-art-dp-person-su-qian-v1", source: "dp-su-qian", name: "苏千", role: "迦南学院内院大长老，负责学院安全及内院事务。", gender: "male", form: "老年人类男性", timeSemantics: "current_observation", roleSemantics: "current_role", roleRecognition: "迦南学院内院大长老", recognition: "我是苏千，迦南学院内院大长老。", worldTime: "本局第1日清晨", year: 0, adult: false},
		{assetID: "p10-art-lotm-npc-xio-derecha-v1", source: "lotm-xio-derecha", name: "休·迪尔查", role: "赏金猎人", gender: "female", form: "人类女性", timeSemantics: "current_observation", roleSemantics: "current_role", roleRecognition: "赏金猎人", recognition: "我是休·迪尔查，赏金猎人。", worldTime: "本局第1日清晨", year: 0, adult: false},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			a, c, s := approvedMappingFixture(t, spec)
			if !storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("source-backed fixture denied")
			}
			s.Profiles[0].Gender = "unspecified"
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("unknown profile gender authorized from image rule")
			}
			a, c, s = approvedMappingFixture(t, spec)
			s.Knowledge[0].Visual.Form = "未知形态"
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("unknown form authorized")
			}
			a, c, s = approvedMappingFixture(t, spec)
			s.Knowledge[0].RetractedBy = "later"
			if storyAssetAllowed(a, c, s, "sinistra") {
				t.Fatal("retracted observation authorized")
			}
			if spec.roleRecognition != "" {
				nameOnly := spec
				nameOnly.recognition = "我叫" + spec.name + "。"
				a, c, s = approvedMappingFixture(t, nameOnly)
				if storyAssetAllowed(a, c, s, "sinistra") {
					t.Fatal("name-only source established actual role")
				}
			}
		})
	}
}
