package interactive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ta/internal/character"
)

func localAffairFixture(pack string) (LocalAffairDefinition, string) {
	d := LocalAffairDefinition{ID: "local-evidence", Structure: LocalAffairStructure{Goal: "understand", Obstacle: "broken_process", Decision: "verify_or_trust", Consequence: "corrected_belief"}}
	route := ""
	switch pack {
	case "fanren-jie":
		d.Title = "同批药材的两种颜色"
		d.LocationID = "yue.market"
		d.Hook = "同一批药材，两只篮中的叶色不同。"
		d.Cause = "其中一篮曾移到潮湿墙脚；潮气使外叶变色，并非同批药材突然变种。"
		d.Methods = []LocalAffairMethod{{ID: "basket", Kind: "observation", LocationID: "yue.market", Label: "检查篮底与外叶", Evidence: "变色只在贴着湿篮底的外叶。", Finding: "受潮位置与变色位置一致。"}, {ID: "shelter", Kind: "observation", LocationID: "yue.market.outskirts", Label: "核对药棚的存放痕迹", Evidence: "药棚墙角的湿痕与一只篮底相合。", Finding: "临时存放处能解释为什么只有一篮变色。"}}
		route = "route_036:forward"
	case "lanchuan-qunxia":
		d.Title = "错了一寸的潮位记号"
		d.LocationID = "lanchuan-l04"
		d.Hook = "两艘小船照同一潮位记号出发，却都在浅处擦了底。"
		d.Cause = "记号木尺的拴扣磨损下滑，使水位读数偏高；船工并未暗改发船约定。"
		d.Structure = LocalAffairStructure{Goal: "protect", Obstacle: "conflicting_accounts", Decision: "negotiate_or_refuse", Consequence: "bounded_obligation"}
		d.Methods = []LocalAffairMethod{{ID: "gauge", Kind: "observation", LocationID: "lanchuan-l04", Label: "检查潮位木尺与拴扣", Evidence: "拴扣旧磨痕比现在的位置高一寸。", Finding: "记号自身移动足以造成错误读数。"}, {ID: "tally", Kind: "observation", LocationID: "lanchuan-l01", Label: "比对码头留存的同潮记录", Evidence: "同潮的固定石刻读数比村口木尺低一寸。", Finding: "固定基准说明误差在木尺，不在发船人的约定。"}}
		route = "lanchuan-route-l04-l01"
	case "xinghai-689":
		d.Title = "少了一半的供能读数"
		d.LocationID = "xinghai-l21"
		d.Hook = "维修前后的供能单看起来少了一半，但设备没有停机。"
		d.Cause = "维修台把双模块总量与单模块读数写进了同一栏，实际消耗没有翻倍。"
		d.Structure = LocalAffairStructure{Goal: "recover", Obstacle: "missing_evidence", Decision: "preserve_or_use", Consequence: "preserved_evidence"}
		d.Methods = []LocalAffairMethod{{ID: "units", Kind: "observation", LocationID: "xinghai-l21", Label: "检查维修台的单位栏", Evidence: "两张读数分别标了每模块与双模块总计。", Finding: "统一计量范围后两张记录相符。"}, {ID: "raw", Kind: "observation", LocationID: "xinghai-l22", Label: "核对哨站留存的原始数据", Evidence: "原始数据的两个模块各记录了一半总量。", Finding: "独立原始记录证实没有额外漏电。"}}
		route = "xinghai-route-l21-l22"
	}
	return d, route
}

// The first two systems reuse frozen adventure rules. Fanren uses its own
// cultivation state. These are deterministic main-turn tests, not live AI runs.
func localAffairStory(t *testing.T, pack string) (*Store, string, string, LocalAffairDefinition, string) {
	t.Helper()
	d, route := localAffairFixture(pack)
	if pack != "fanren-jie" {
		s, id, dir := worldContentPreparedStory(t, pack, d.LocationID, nil, nil)
		return s, id, dir, d, route
	}
	system := journeyTestSystem(journeyPlaceLabel(d.LocationID))
	system.WorldContent = authoredWorldContent(t, "fanren-jie")
	dir := t.TempDir()
	s := NewStore(dir)
	t.Cleanup(func() { _ = s.Close() })
	story, err := s.CreateStory(CreateStoryRequest{Title: "本地见闻", ActorState: &system, PackBinding: &StoryPackBinding{PackID: pack, ContentVersion: 30, Scope: pack}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "看看周围", Narrative: "来到当地。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	return s, story.ID, dir, d, route
}
func localAffairCommit(t *testing.T, s *Store, id, branch, action, affair, method string, d *LocalAffairDefinition) TurnEvent {
	t.Helper()
	source := "我选择" + action
	w := &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: source, LocalAffair: &LocalAffairActionInput{Action: action, ID: affair, MethodID: method, Definition: d}}
	turn, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: branch, User: source, Narrative: "在现有事实范围内处理这件小事。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: w}})
	if err != nil {
		t.Fatal(action, err)
	}
	return turn
}
func localAffairPublic(t *testing.T, s *Store, id, branch string) []LocalAffairView {
	t.Helper()
	v, err := s.WorldStateFor(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	return v.LocalAffairs
}
func localAffairReject(t *testing.T, s *Store, id, branch string, w *TurnWorldActionInput, user string) {
	t.Helper()
	before, err := s.Snapshot(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: branch, User: user, Narrative: "此不合法断言不得落盘。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: w}})
	if err == nil {
		t.Fatal("invalid local affair accepted")
	}
	after, err := s.Snapshot(id, branch)
	if err != nil || before.TurnCount != after.TurnCount || !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) {
		t.Fatal("refusal changed canonical state", err)
	}
}

func TestLocalAffairThreeWorldMainTurnSupplyAndAlternativePaths(t *testing.T) {
	for _, pack := range []string{"fanren-jie", "lanchuan-qunxia", "xinghai-689"} {
		t.Run(pack, func(t *testing.T) {
			s, id, dir, d, route := localAffairStory(t, pack)
			opening := localAffairCommit(t, s, id, "main", "encounter", d.ID, "", &d)
			shown := localAffairPublic(t, s, id, "main")
			raw, _ := json.Marshal(shown)
			if len(shown) != 1 || shown[0].Status != "noticed" || strings.Contains(string(raw), d.Cause) || strings.Contains(string(raw), d.Methods[0].Evidence) {
				t.Fatal("opening exposed private answer or accepted an obligation", string(raw))
			}
			snapshot, _ := s.Snapshot(id, "main")
			serialized, _ := json.Marshal(snapshot)
			if strings.Contains(string(serialized), d.Cause) {
				t.Fatal("snapshot leaked hidden cause")
			}
			page, err := s.ReadHistoryPage(id, "main", "", 20)
			if err != nil {
				t.Fatal(err)
			}
			publicHistory, _ := json.Marshal(page)
			if strings.Contains(string(publicHistory), d.Cause) || strings.Contains(string(publicHistory), d.Methods[0].Evidence) {
				t.Fatal("history page leaked private affair")
			}
			ctx, err := s.LocalAffairsNarratorContext(id, "main", "")
			if err != nil || !strings.Contains(ctx, d.Cause) {
				t.Fatal("narrator lost fixed answer", err)
			}
			localAffairReject(t, s, id, "main", &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: "猜到了", LocalAffair: &LocalAffairActionInput{Action: "resolve", ID: d.ID}}, "猜到了")
			fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening.ID, Title: "另一条证据路径"})
			if err != nil {
				t.Fatal(err)
			}
			before, _, _ := s.JourneyState(id, "main")
			localAffairCommit(t, s, id, "main", "defer", d.ID, "", nil)
			localAffairReject(t, s, id, "main", &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: "调查", LocalAffair: &LocalAffairActionInput{Action: "verify", ID: d.ID, MethodID: d.Methods[0].ID}}, "调查")
			after, _, _ := s.JourneyState(id, "main")
			if before != after {
				t.Fatal("putting aside spent time or imposed a deadline")
			}
			localAffairCommit(t, s, id, "main", "resume", d.ID, "", nil)
			localAffairCommit(t, s, id, "main", "verify", d.ID, d.Methods[0].ID, nil)
			now, _, _ := s.JourneyState(id, "main")
			if now.Seconds-before.Seconds != 600 {
				t.Fatal("actual inspection did not pay one interval", now.Seconds-before.Seconds)
			}
			localAffairCommit(t, s, id, "main", "resolve", d.ID, "", nil)
			if got := localAffairPublic(t, s, id, "main")[0]; got.Cause != d.Cause || len(got.Findings) != 1 || got.Status != "resolved" {
				t.Fatal("one independent method did not pay off immediately", got)
			}
			localAffairReject(t, s, id, fork.ID, &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: "去另一处调查", LocalAffair: &LocalAffairActionInput{Action: "verify", ID: d.ID, MethodID: d.Methods[1].ID}}, "去另一处调查")
			if _, err := s.ApplyJourney(id, JourneyRequest{BranchID: fork.ID, RouteID: route, Arrive: true}); err != nil {
				t.Fatal("alternative route is not actually traversable", route, err)
			}
			localAffairCommit(t, s, id, fork.ID, "verify", d.ID, d.Methods[1].ID, nil)
			localAffairCommit(t, s, id, fork.ID, "resolve", d.ID, "", nil)
			other := localAffairPublic(t, s, id, fork.ID)[0]
			if other.Cause != d.Cause || len(other.Findings) != 1 || other.Findings[0] == shown[0].Hook {
				t.Fatal("alternative path changed answer or required both routes")
			}
			// A resolved local answer is not a forced ending. A genuinely different
			// next affair can be encountered without opening a new world or save.
			next := d
			next.ID = "another-local-situation"
			next.Title = "是否保留受托人名的原始记录"
			next.Hook = "原始记录含有一位无关受托人的名字，对外说明时是否遮去尚未决定。"
			next.Cause = "记录中的人名只是代收签名，与此前故障并无因果关系。"
			next.Methods = append([]LocalAffairMethod(nil), d.Methods...)
			next.Methods[0].Evidence = "签名栏注明仅代收，不是操作记录。"
			next.Methods[0].Finding = "名字属于代收者，并不能支持指认操作责任。"
			next.Methods[1].Evidence = "异地存根同样将签名记为代收。"
			next.Methods[1].Finding = "独立存根证实签名的职责范围。"
			next.Structure.Decision = "share_or_withhold"
			next.Structure.Consequence = "relationship_boundary"
			if next.Structure.Decision == d.Structure.Decision || next.Structure.Consequence == d.Structure.Consequence {
				t.Fatal("bad distinct fixture")
			}
			localAffairCommit(t, s, id, "main", "encounter", next.ID, "", &next)
			mainBefore := localAffairPublic(t, s, id, "main")
			otherBefore := localAffairPublic(t, s, id, fork.ID)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			cold := NewStore(dir)
			defer cold.Close()
			if !reflect.DeepEqual(mainBefore, localAffairPublic(t, cold, id, "main")) || !reflect.DeepEqual(otherBefore, localAffairPublic(t, cold, id, fork.ID)) {
				t.Fatal("cold replay lost branch-local cause, evidence or continuation")
			}
			t.Logf("%s: encounter -> optional defer/resume -> one real 600s inspection OR real route + independent inspection -> same fixed cause -> new structurally distinct affair; cold replay unchanged", pack)
		})
	}
}

func TestLocalAffairStructureRejectsRenameAndRequiresBothMaterialAxes(t *testing.T) {
	d, _ := localAffairFixture("lanchuan-qunxia")
	prior := []LocalAffairState{{Definition: d, Status: "resolved"}}
	for _, kind := range []string{"rename", "decision-only", "consequence-only", "both"} {
		candidate := d
		candidate.ID = "different-name"
		candidate.Title = "完全换了人名地名的标题"
		candidate.Cause = "换个称呼和写法包装了相同的结构"
		if kind == "decision-only" || kind == "both" {
			candidate.Structure.Decision = "share_or_withhold"
		}
		if kind == "consequence-only" || kind == "both" {
			candidate.Structure.Consequence = "relationship_boundary"
		}
		if localAffairRepeated(prior, candidate) != (kind != "both") {
			t.Fatal("wrong structural repetition decision", kind)
		}
	}
	sameCause := d
	sameCause.Structure.Decision = "share_or_withhold"
	sameCause.Structure.Consequence = "relationship_boundary"
	if !localAffairRepeated(prior, sameCause) {
		t.Fatal("changing category labels laundered an identical cause")
	}
	// The scope is deliberately a recent window, not a false never-repeat claim.
	for i := 0; i < 12; i++ {
		other := d
		other.ID = fmt.Sprint(i)
		other.Structure.Goal = "gain_access"
		other.Cause = fmt.Sprintf("independent source %d", i)
		prior = append(prior, LocalAffairState{Definition: other})
	}
	if localAffairRepeated(prior, d) {
		t.Fatal("recent-window policy silently became an indefinite ban")
	}
	invalid := d
	invalid.Structure.Goal = "understand|recover"
	if validateLocalAffairDefinition(invalid) == nil {
		t.Fatal("joined enum values bypassed structural validation")
	}
	d.Methods[1] = d.Methods[0]
	d.Methods[1].ID = "renamed-method"
	if validateLocalAffairDefinition(d) == nil {
		t.Fatal("two names for the same inspection counted as independent")
	}
}

func TestLocalAffairRegenerationAndRewindRestoreAnswerAndTime(t *testing.T) {
	s, id, _, d, _ := localAffairStory(t, "lanchuan-qunxia")
	opening := localAffairCommit(t, s, id, "main", "encounter", d.ID, "", &d)
	old, _ := s.TurnWorldView(id, "main", opening.ID)
	if len(old.LocalAffairs) != 0 {
		t.Fatal("regenerate context inherited target's future answer")
	}
	verify := localAffairCommit(t, s, id, "main", "verify", d.ID, d.Methods[0].ID, nil)
	parent, _ := s.TurnWorldView(id, "main", verify.ID)
	if len(parent.LocalAffairs) != 1 || len(parent.LocalAffairs[0].Evidence) != 0 {
		t.Fatal("regenerate context saw discarded evidence")
	}
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", ReplaceTurnID: verify.ID, User: "这回先不查", Narrative: "暂把问题记下。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: "这回先不查", LocalAffair: &LocalAffairActionInput{Action: "defer", ID: d.ID}}}})
	if err != nil {
		t.Fatal(err)
	}
	view := localAffairPublic(t, s, id, "main")[0]
	if view.Status != "deferred" || len(view.Findings) != 0 || view.Cause != "" {
		t.Fatal("replacement kept an investigation", view)
	}
	clock, _, _ := s.JourneyState(id, "main")
	if clock.Seconds != 0 {
		t.Fatal("replacement kept inspection time", clock)
	}
	resumed := localAffairCommit(t, s, id, "main", "resume", d.ID, "", nil)
	if err := s.RewindToTurnParent(id, RewindTurnRequest{BranchID: "main", TurnID: resumed.ID}); err != nil {
		t.Fatal(err)
	}
	if localAffairPublic(t, s, id, "main")[0].Status != "deferred" {
		t.Fatal("rewind kept discarded resume")
	}
}

func TestLocalAffairDryRunUsesSameFrozenFactsAndRefusesForgedSource(t *testing.T) {
	s, id, _, d, _ := localAffairStory(t, "xinghai-689")
	localAffairCommit(t, s, id, "main", "encounter", d.ID, "", &d)
	validation := worldFlowSubmissionContext(t, s, id)
	w := &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: "我检查记录", LocalAffair: &LocalAffairActionInput{Action: "verify", ID: d.ID, MethodID: d.Methods[0].ID}}
	plan, err := dryRunTurnWorldPlan(validation, w, nil)
	if err != nil || plan.clockSeconds() != 600 {
		t.Fatal("submission cannot reach actual local planner", err)
	}
	validation.CurrentPlayerInput = w.SourceText
	prepared, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{World: w})
	if prepared.TurnResult().World == nil || len(receipt.Diagnostics) != 0 {
		t.Fatal("actual submission refused valid local action", receipt)
	}
	validation.CurrentPlayerInput = "我现在只想闲聊"
	prepared, receipt = PrepareTurnSubmission(validation, nil, TurnSubmissionInput{World: w})
	if prepared.TurnResult().World != nil || len(receipt.Diagnostics) == 0 {
		t.Fatal("actual submission accepted fabricated player choice", receipt)
	}
	localAffairReject(t, s, id, "main", w, "我现在只想闲聊")
	copy := d
	copy.Cause = "临时换一个幕后黑手"
	localAffairReject(t, s, id, "main", &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, LocalAffair: &LocalAffairActionInput{Action: "encounter", ID: d.ID, Definition: &copy}}, "再看看")
	cross := d
	cross.ID = "cross-world"
	cross.Methods = append([]LocalAffairMethod(nil), d.Methods...)
	cross.Methods[1].LocationID = "yue.market"
	localAffairReject(t, s, id, "main", &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, LocalAffair: &LocalAffairActionInput{Action: "encounter", ID: cross.ID, Definition: &cross}}, "看异地消息")
}

// This adversarial witness fixture intentionally constructs a known present
// person and committed exchange. It tests evidence attribution, not acquisition.
func TestLocalAffairWitnessCannotUseOldPrivateRemoteOrOtherSpeakers(t *testing.T) {
	d, _ := localAffairFixture("lanchuan-qunxia")
	d.Methods[1] = LocalAffairMethod{ID: "witness", Kind: "witness", LocationID: d.LocationID, CharacterSourceKey: "lanchuan-p06", Label: "请蓝阿鲤核对原尺", Evidence: "我量过那道旧磨痕，拴扣确实滑下一寸。", Finding: "本人比对旧痕，确认读数偏差来源。"}
	p := &storyBranchProjection{State: map[string]any{}, Characters: &character.State{Profiles: []character.Profile{{ID: "witness", StoryID: "story", Name: "蓝阿鲤", SourceKey: "lanchuan-p06"}}}}
	applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Value: d.LocationID})
	applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{"witness"}})
	a := LocalAffairState{Definition: d, Status: "noticed", EarlierExchangeCount: 1}
	m := d.Methods[1]
	old := character.Exchange{ID: "old", SourceEventID: "event-old", CharacterID: "witness", Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, Text: m.Evidence, Audience: []string{DefaultActorID, "witness"}}
	current := old
	current.ID = "current"
	current.SourceEventID = "event-current"
	p.Characters.Exchanges = []character.Exchange{old, current}
	if _, ok := localAffairWitness(p, a, m, "old"); ok {
		t.Fatal("old testimony counted as a new check")
	}
	if _, ok := localAffairWitness(p, a, m, "current"); !ok {
		t.Fatal("actual personally heard witness was refused")
	}
	for _, variant := range []string{"private", "other-speaker", "wrong-claim", "negated-claim", "remote"} {
		q := *p
		q.State = cloneStoryState(p.State)
		q.Characters = cloneCharacterState(p.Characters)
		e := &q.Characters.Exchanges[1]
		switch variant {
		case "private":
			e.Audience = []string{"witness"}
		case "other-speaker":
			e.Speaker = character.SpeakerPlayer
		case "wrong-claim":
			e.Text = "我也只是听说而已。"
		case "negated-claim":
			e.Text = "我否认这句话：" + m.Evidence
		case "remote":
			applyActorStateOp(q.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{}})
		}
		if _, ok := localAffairWitness(&q, a, m, "current"); ok {
			t.Fatal("invalid witness accepted", variant)
		}
	}
}

func TestLocalAffairRealWitnessMainTurnAndImmediatePayoff(t *testing.T) {
	d, _ := localAffairFixture("fanren-jie")
	d.Methods[1] = LocalAffairMethod{ID: "holder", Kind: "witness", LocationID: d.LocationID, CharacterSourceKey: "npc_zhao_ling", Label: "向实际经手者核对移篮经过", Evidence: "我把那一篮移到潮湿墙脚，另外一篮没有挪。", Finding: "经手者说明只有一篮受潮，和外叶颜色相合。"}
	system := journeyTestSystem(journeyPlaceLabel(d.LocationID))
	system.WorldContent = authoredWorldContent(t, "fanren-jie")
	for i := range system.InitialActors {
		if system.InitialActors[i].ID == DefaultStoryContextActorID {
			system.InitialActors[i].State[storyScenePresentField] = []any{"holder"}
		}
	}
	system.InitialActors = append(system.InitialActors, ActorStateInitialActor{ID: "holder", Name: "赵灵", TemplateID: ActorStateImportantCharacterTemplateID, State: map[string]any{xiuxianFieldArrangement: "未定"}})
	store := NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(CreateStoryRequest{Title: "已认识经手人的规则样本", ActorState: &system, PackBinding: &StoryPackBinding{PackID: "fanren-jie", ContentVersion: 30, Scope: "fanren-jie"}, PackCatalog: &PackCatalogSnapshot{ContentVersion: 30, Characters: []PackCatalogPerson{{SourceKey: "npc_zhao_ling", Name: "赵灵", PlaceCandidate: d.LocationID}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "到坊市", Narrative: "经手人赵灵在此。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "holder", Name: "赵灵", SourceKey: "npc_zhao_ling"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	localAffairCommit(t, store, story.ID, "main", "encounter", d.ID, "", &d)
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "问问这两篮如何存放", Narrative: d.Methods[1].Evidence, TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "holder", Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: d.Methods[1].Evidence}}}}})
	if err != nil {
		t.Fatal(err)
	}
	chars, err := store.CharacterState(story.ID, "main")
	if err != nil || len(chars.Exchanges) != 1 {
		t.Fatal("actual witness record missing", err)
	}
	source := "这就把原因查清，留好证言，我不替别人答应任何修理。"
	w := &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: source, LocalAffair: &LocalAffairActionInput{Action: "verify", ID: d.ID, MethodID: "holder", ExchangeID: chars.Exchanges[0].ID, Conclude: true}}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: source, Narrative: "这条本人证言使原因清楚了，不必再查另一条。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: w}})
	if err != nil {
		view, _ := store.TurnWorldView(story.ID, "main", "")
		snapshot, _ := store.Snapshot(story.ID, "main")
		t.Fatalf("%v; characters=%+v; holder=%+v; scene=%+v", err, view.Characters, getPath(snapshot.State, actorStateRoot+".holder"), getPath(snapshot.State, actorStateRoot+".story"))
	}
	got := localAffairPublic(t, store, story.ID, "main")[0]
	if got.Cause != d.Cause || got.Status != "resolved" || got.PlayerDecision != source || len(got.Findings) != 1 {
		t.Fatal("actual testimony did not immediately pay off", got)
	}
	clock, _, _ := store.JourneyState(story.ID, "main")
	if clock.Seconds != 0 {
		t.Fatal("using already-heard testimony invented a paid inspection")
	}
}

func TestLocalAffairLongerHistoryResumesOldThreadWithinBoundedContext(t *testing.T) {
	s, id, _, d, _ := localAffairStory(t, "lanchuan-qunxia")
	goals := []string{"understand", "recover", "protect", "reconcile", "gain_access", "choose_commitment"}
	obstacles := []string{"missing_evidence", "scarcity", "competing_duties"}
	// These synthetic structure tuples exercise storage/context bounds, not the
	// prose quality or novelty of a live model's generated stories.
	for i := 0; i < 15; i++ {
		next := d
		next.ID = fmt.Sprintf("history-%02d", i)
		next.Title = fmt.Sprintf("记下的地方小事%02d", i)
		next.Cause = fmt.Sprintf("Fixture cause %02d: a distinct bounded source record.", i)
		next.Structure.Goal = goals[i%len(goals)]
		next.Structure.Obstacle = obstacles[i/len(goals)]
		localAffairCommit(t, s, id, "main", "encounter", next.ID, "", &next)
		localAffairCommit(t, s, id, "main", "defer", next.ID, "", nil)
	}
	before, err := s.LocalAffairsNarratorContext(id, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(before, "frozen ") > 6 || len(before) > 24000 || strings.Contains(before, "Fixture cause 00:") {
		t.Fatal("private context was not bounded by relevant recent affairs")
	}
	recalled, recallErr := s.LocalAffairsNarratorContext(id, "main", "", "我想回顾记下的地方小事00的已知经过")
	if recallErr != nil || !strings.Contains(recalled, "Fixture cause 00:") {
		t.Fatal("exact known title could not recall an older frozen affair", recallErr)
	}
	localAffairCommit(t, s, id, "main", "resume", "history-00", "", nil)
	after, err := s.LocalAffairsNarratorContext(id, "main", "")
	if err != nil || !strings.Contains(after, "Fixture cause 00:") || len(after) > 24000 {
		t.Fatal("explicitly resumed old thread stayed hidden behind newer entries", err)
	}
	if len(localAffairPublic(t, s, id, "main")) != 15 {
		t.Fatal("context budget deleted durable affairs")
	}
}

func TestLocalAffairHistoricalWorldPageCannotSeeDiscardedPayoff(t *testing.T) {
	s, id, _, d, _ := localAffairStory(t, "xinghai-689")
	localAffairCommit(t, s, id, "main", "encounter", d.ID, "", &d)
	source := "我核对单位，查清后就先放下"
	turn, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: source, Narrative: "单位差异已经核对。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: source, LocalAffair: &LocalAffairActionInput{Action: "verify", ID: d.ID, MethodID: d.Methods[0].ID, Conclude: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	live, _ := s.WorldStateFor(id, "main")
	old, err := s.WorldStateAtTurnParent(id, "main", turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if live.LocalAffairs[0].Cause != d.Cause || old.LocalAffairs[0].Cause != "" || len(old.LocalAffairs[0].Findings) != 0 || old.Clock.Seconds != 0 {
		t.Fatal("historical world actions leaked future payoff or time", old.LocalAffairs, old.Clock)
	}
	raw, _ := json.Marshal(old)
	if strings.Contains(string(raw), d.Cause) {
		t.Fatal("historical public JSON leaked fixed answer")
	}
}

func TestLocalAffairPartialJourneyCannotInvestigateOldOrigin(t *testing.T) {
	s, id, _, d, route := localAffairStory(t, "fanren-jie")
	localAffairCommit(t, s, id, "main", "encounter", d.ID, "", &d)
	if _, err := s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: route, TargetMinutes: 15}); err != nil {
		t.Fatal(err)
	}
	localAffairReject(t, s, id, "main", &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: "查看旧地的篮底", LocalAffair: &LocalAffairActionInput{Action: "verify", ID: d.ID, MethodID: d.Methods[0].ID}}, "查看旧地的篮底")
	other := d
	other.ID = "not-at-the-old-stop"
	localAffairReject(t, s, id, "main", &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, LocalAffair: &LocalAffairActionInput{Action: "encounter", ID: other.ID, Definition: &other}}, "途中看看")
	if _, err := s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: route, Arrive: true}); err != nil {
		t.Fatal(err)
	}
	localAffairCommit(t, s, id, "main", "verify", d.ID, d.Methods[1].ID, nil)
	localAffairCommit(t, s, id, "main", "resolve", d.ID, "", nil)
	if got := localAffairPublic(t, s, id, "main")[0]; got.Status != "resolved" || got.Cause != d.Cause {
		t.Fatal("arrival did not enable the actual alternative source", got)
	}
}
