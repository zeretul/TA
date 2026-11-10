package interactive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/exploration"
)

// These are read-projection fixtures over genuine committed realm actions, not
// evidence of natural dialogue or of normally acquired NPC cooperation.
func cooperationViewReadFixture(t *testing.T, s *Store, storyID string) (*storyBranchProjection, StoryMeta) {
	t.Helper()
	context, err := s.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	base := combatTestProjection(t, s, storyID)
	copy := *base
	copy.State = cloneStoryState(base.State)
	copy.Characters = cloneCharacterState(base.Characters)
	copy.Realms = cloneRealmInstances(base.Realms)
	return &copy, context.Meta
}

func TestCooperationViewRequiresActualClueAndKnownPartner(t *testing.T) {
	for _, project := range cooperationProjects {
		t.Run(project.ID, func(t *testing.T) {
			def, _ := exploration.RealmDefinition(project.RealmID)
			s, id, _ := realmExploreStory(t, def.PackageID, def.EntryLocationIDs[0], nil, nil)
			entered := realmTestApply(t, s, id, realmExploreRequest("", project.RealmID, RealmActionEnter, "", "enter"))
			p, meta := cooperationViewReadFixture(t, s, id)
			known := character.Profile{ID: "known-partner", StoryID: id, SourceKey: project.PartnerSourceKey, Name: "Known partner", Revision: 1}
			p.Characters = &character.State{Profiles: []character.Profile{known}}
			if views := cooperationProjectViews(p, meta); len(views) != 0 {
				t.Fatal("known partner revealed an unobserved project")
			}
			realmExploreDo(t, s, id, entered.InstanceID, project.RealmID, RealmActionSurvey, "survey")
			realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, project.RealmID, RealmActionExit, "", "exit"))
			p, meta = cooperationViewReadFixture(t, s, id)
			if views := cooperationProjectViews(p, meta); len(views) != 0 {
				t.Fatal("survey revealed an unencountered partner")
			}
			p.Characters = &character.State{Profiles: []character.Profile{known}}
			before, _ := json.Marshal(p)
			views := cooperationProjectViews(p, meta)
			if len(views) != 1 || views[0].PartnerID != known.ID || views[0].CanAsk || views[0].CanWork || views[0].CanWorkIfAgreed || views[0].CanReport {
				t.Fatalf("author location or profile was treated as actual presence: %+v", views)
			}
			for range 3 {
				_ = cooperationProjectViews(p, meta)
			}
			after, _ := json.Marshal(p)
			if string(before) != string(after) {
				t.Fatal("reading cooperation advanced state or time")
			}
			p.Characters.Profiles = append(p.Characters.Profiles, character.Profile{ID: "duplicate", StoryID: id, SourceKey: project.PartnerSourceKey, Name: known.Name})
			if len(cooperationProjectViews(p, meta)) != 0 {
				t.Fatal("ambiguous source identity exposed cooperation")
			}
			p.Characters.Profiles = p.Characters.Profiles[:1]
			inst := p.Realms[entered.InstanceID]
			delete(inst.ExploreSources, "clue")
			p.Realms[entered.InstanceID] = inst
			if len(cooperationProjectViews(p, meta)) != 0 {
				t.Fatal("discovery without actual survey event authorized a project")
			}
		})
	}
}

func TestCooperationViewReportsOnlyRealUnsharedOutcomes(t *testing.T) {
	const realm = exploration.RealmMountainHerbCave
	s, id, _ := realmExploreStory(t, "fanren-jie", realmTestOrigin, nil, nil)
	entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
	realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionSurvey, "survey")
	ingress := realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionApproach, "slow")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "core")
	preserved := realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionResolve, "preserve")
	p, _ := cooperationViewReadFixture(t, s, id)
	facts := cooperationReportFacts(p, entered.InstanceID)
	want := []CooperationReportFactView{
		{Description: "主角实际完成了“沿旧槽慢行”。", Kind: "ingress", SourceRef: "ingress:" + ingress.EventID, DescriptionKey: "game.cooperation.reportFacts.ingress", ValueKey: "game.realm.explore.herbCave.slow.name"},
		{Description: "主角实际完成了“保住旧阵源”。", Kind: "disposition", SourceRef: "disposition:" + preserved.EventID, DescriptionKey: "game.cooperation.reportFacts.disposition", ValueKey: "game.realm.explore.herbCave.preserve.name"},
	}
	if !reflect.DeepEqual(facts, want) {
		t.Fatalf("reports lost real journal sources: %+v", facts)
	}
	due := preserved.Instance.ExploreState.ReturnDueSeconds
	questWait(t, s, id, "main", due-worldFlowClock(t, s, id).Seconds)
	realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionApproach, "return")
	p, _ = cooperationViewReadFixture(t, s, id)
	if got := cooperationReportFacts(p, entered.InstanceID); !reflect.DeepEqual(got, want) {
		t.Fatal("opening a connection was reported as a side-area visit")
	}
	side := realmExploreMove(t, s, id, entered.InstanceID, realm, "side", "side")
	want = append(want, CooperationReportFactView{Kind: "side", SourceRef: "side:" + side.EventID, DescriptionKey: "game.cooperation.reportFacts.side"})
	p, meta := cooperationViewReadFixture(t, s, id)
	if got := cooperationReportFacts(p, entered.InstanceID); !reflect.DeepEqual(got, want) {
		t.Fatalf("actual side visit not reportable: %+v", got)
	}
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "side-to-core")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "entry", "core-to-entry")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "exit-after-side"))
	p, meta = cooperationViewReadFixture(t, s, id)
	project := cooperationProjects[0]
	partner := character.Profile{ID: "known-partner", StoryID: id, SourceKey: project.PartnerSourceKey, Name: "Known partner"}
	p.Characters = &character.State{Profiles: []character.Profile{partner}, Matters: []character.Matter{{
		ID: "worked-matter", StoryID: id, BranchID: "main", CharacterID: partner.ID, Kind: character.MatterAction, Status: character.MatterOpen, Statement: "Review the observed clue.", WorldTime: "day 1", SourceEventID: "work-event",
		Cooperation: &character.CooperationTerms{ProjectID: project.ID, ProjectVersion: project.Version, RealmInstanceID: entered.InstanceID, ConsentExchangeID: "consent-event", SharedDiscoverySourceRef: p.Realms[entered.InstanceID].ExploreSources["clue"], Phase: character.CooperationReported, WorkSourceRef: "work-event", ReportSourceRefs: []string{want[0].SourceRef}},
	}}}
	before, _ := json.Marshal(p)
	views := cooperationProjectViews(p, meta)
	if len(views) != 1 || !reflect.DeepEqual(views[0].ReportFacts, want[1:]) || views[0].CanReport || views[0].CanWork || views[0].Phase != "reported" {
		t.Fatalf("report projection disclosed already shared facts or ignored presence: %+v", views)
	}
	views[0].ReportFacts[0].SourceRef = "mutated-view"
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("report view aliases or mutates committed matters")
	}
	storyContextActorState(p.State)[normalizeActorStateFieldName(storyScenePresentField)] = []any{partner.ID}
	if present := cooperationProjectViews(p, meta); len(present) != 1 || !present[0].CanReport || present[0].CanWork {
		t.Fatalf("available reporting was blocked by the completed-work restriction: %+v", present)
	}
	if len(cooperationReportFacts(p, "another-instance")) != 0 {
		t.Fatal("report crossed the instance boundary")
	}
	// Even if two result classes cite one event, sharing one class does not
	// silently share the other. This fixture changes only the derived source map.
	inst := p.Realms[entered.InstanceID]
	inst.ExploreSources["disposition"] = ingress.EventID
	p.Realms[entered.InstanceID] = inst
	views = cooperationProjectViews(p, meta)
	if len(views) != 1 || len(views[0].ReportFacts) != 2 || views[0].ReportFacts[0].SourceRef != "disposition:"+ingress.EventID {
		t.Fatal("report deduplication collapsed different outcome classes")
	}
}

func TestCooperationViewSetbackCannotClaimSuccessfulDisposition(t *testing.T) {
	const realm = exploration.RealmMountainHerbCave
	s, id, _ := realmExploreStory(t, "fanren-jie", realmTestOrigin, nil, nil)
	entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
	realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionSurvey, "survey")
	realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionResolve, "outer")
	ingress := realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionApproach, "slow")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "core")
	setback := realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionResolve, "preserve")
	p, _ := cooperationViewReadFixture(t, s, id)
	facts := cooperationReportFacts(p, entered.InstanceID)
	if len(facts) != 2 || facts[0].SourceRef != "ingress:"+ingress.EventID || facts[1].SourceRef != "setback:"+setback.EventID {
		t.Fatalf("failed preservation was projected as successful: %+v", facts)
	}
	inst := p.Realms[entered.InstanceID]
	inst.ExploreSources["disposition"] = setback.EventID
	p.Realms[entered.InstanceID] = inst
	for _, fact := range cooperationReportFacts(p, entered.InstanceID) {
		if strings.Contains(fact.SourceRef, "disposition:") {
			t.Fatal("source cache alone invented successful disposition")
		}
	}
	inst.Snapshot.ExploreState.Pressure = 0
	p.Realms[entered.InstanceID] = inst
	if len(cooperationReportFacts(p, entered.InstanceID)) != 0 {
		t.Fatal("invalid snapshot was exposed as a reportable fact")
	}
}

func TestCooperationViewUsesRealCostPlansWithoutApplyingThem(t *testing.T) {
	for _, project := range cooperationProjects {
		t.Run(project.ID, func(t *testing.T) {
			def, _ := exploration.RealmDefinition(project.RealmID)
			items := map[string]int{}
			if project.PlayerItemID != "" {
				items[project.PlayerItemID] = project.PlayerItemQuantity
			}
			s, id, _ := realmExploreStory(t, def.PackageID, def.EntryLocationIDs[0], items, nil)
			entered := realmTestApply(t, s, id, realmExploreRequest("", project.RealmID, RealmActionEnter, "", "enter"))
			realmExploreDo(t, s, id, entered.InstanceID, project.RealmID, RealmActionSurvey, "survey")
			realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, project.RealmID, RealmActionExit, "", "exit"))
			p, meta := cooperationViewReadFixture(t, s, id)
			partner := character.Profile{ID: "known-partner", StoryID: id, SourceKey: project.PartnerSourceKey, KnownAs: "Only the known name", Name: "Known partner"}
			p.Characters = &character.State{Profiles: []character.Profile{partner}}
			// Qualified projection fixtures use the same frozen native NPC actor;
			// this is not an acquisition or first-appearance integration test.
			var npc AdventureActor
			if project.PartnerSkillID != "" {
				system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
				for _, definition := range system.AdventureRules.NPCs {
					if definition.CharacterSourceKey == project.PartnerSourceKey {
						npc = definition.Actor
						break
					}
				}
				if npc.Rank == 0 {
					t.Fatal("frozen native partner is missing")
				}
				npc.SourceKey = partner.SourceKey
				cooperationViewSetActorFixture(p, partner.ID, npc)
			}
			context := storyContextActorState(p.State)
			context[normalizeActorStateFieldName(storySceneLocationField)] = project.LocationID
			context[normalizeActorStateFieldName(storyScenePresentField)] = []any{partner.ID}
			before, _ := json.Marshal(p)
			views := cooperationProjectViews(p, meta)
			if len(views) != 1 || !views[0].CanAsk || !views[0].CanWorkIfAgreed || views[0].CanWork || views[0].BlockedReason != "not_agreed" || views[0].PartnerName != partner.KnownAs {
				t.Fatalf("valid current preparation or known alias not projected: %+v", views)
			}
			wantEnergy := 0
			if project.PartnerSkillID != "" {
				wantEnergy = 6
			}
			if views[0].PartnerEnergyCost != wantEnergy || views[0].ClueText == "" || views[0].ClueSourceRef == "" {
				t.Fatalf("real cost or shareable observation omitted: %+v", views[0])
			}
			after, _ := json.Marshal(p)
			if string(before) != string(after) {
				t.Fatal("eligibility projection applied planned resource deductions")
			}
			p.Characters.Matters = []character.Matter{{ID: "ordinary", StoryID: id, BranchID: "main", CharacterID: partner.ID, Statement: "We already researched everything.", Kind: character.MatterPromise, Status: character.MatterOpen}}
			if view := cooperationProjectViews(p, meta)[0]; view.Phase != "none" || view.CanWork {
				t.Fatal("ordinary promise became cooperation consent")
			}
			p.Characters.Matters[0].Cooperation = &character.CooperationTerms{ProjectID: project.ID, ProjectVersion: project.Version, RealmInstanceID: entered.InstanceID, ConsentExchangeID: "actual-consent", SharedDiscoverySourceRef: p.Realms[entered.InstanceID].ExploreSources["clue"], Phase: character.CooperationAgreed}
			if view := cooperationProjectViews(p, meta)[0]; view.CanWork || view.BlockedReason != "not_agreed" {
				t.Fatal("agreed matter without a sourced personal exchange authorized work")
			}
			consent := character.Exchange{
				ID: "actual-consent", StoryID: id, BranchID: "main", CharacterID: partner.ID,
				Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: DefaultActorID,
				Text: "我愿意按这次说好的分工一起核对。", WorldTime: "当前", SourceEventID: "consent-source-event",
				Audience:            []string{partner.ID, DefaultActorID},
				CooperationDecision: &character.CooperationDecision{ProjectID: project.ID, RealmInstanceID: entered.InstanceID, Decision: character.CooperationAccept},
			}
			if err := consent.Validate(); err != nil {
				t.Fatalf("invalid consent exchange fixture: %v", err)
			}
			p.Characters.Exchanges = []character.Exchange{consent}
			if view := cooperationProjectViews(p, meta)[0]; view.Phase != "agreed" || !view.CanWork {
				t.Fatalf("actual agreed phase not reflected: %+v", view)
			}
			refusal := consent
			refusal.ID, refusal.SourceEventID, refusal.Text = "later-refusal", "refusal-source-event", "现在我不愿意做这件事。"
			refusal.CooperationDecision = &character.CooperationDecision{ProjectID: project.ID, RealmInstanceID: entered.InstanceID, Decision: character.CooperationRefuse}
			p.Characters.Exchanges = append(p.Characters.Exchanges, refusal)
			if view := cooperationProjectViews(p, meta)[0]; view.Phase != "agreed" || view.CanWork || view.BlockedReason != "not_agreed" {
				t.Fatalf("latest refusal did not block previously agreed work: %+v", view)
			}
			renewed := consent
			renewed.ID, renewed.SourceEventID = "renewed-consent", "renewed-source-event"
			p.Characters.Exchanges = append(p.Characters.Exchanges, renewed)
			before, _ = json.Marshal(p)
			if view := cooperationProjectViews(p, meta)[0]; !view.CanWork || view.BlockedReason != "" {
				t.Fatalf("new sourced acceptance did not restore work eligibility: %+v", view)
			}
			after, _ = json.Marshal(p)
			if string(before) != string(after) {
				t.Fatal("consent projection rewrote exchanges or cooperation terms")
			}
			if project.PartnerSkillID != "" {
				npc.Energy = 5
				cooperationViewSetActorFixture(p, partner.ID, npc)
				view := cooperationProjectViews(p, meta)[0]
				if view.PartnerEnergyCost != 6 || view.CanWork || view.BlockedReason != "partner_energy" {
					t.Fatalf("insufficient real partner energy not shown: %+v", view)
				}
			}
			if project.PlayerItemID != "" {
				npc.Energy = 100
				cooperationViewSetActorFixture(p, partner.ID, npc)
				player, err := AdventureActorFromState(p.State, DefaultActorID)
				if err != nil {
					t.Fatal(err)
				}
				player.Inventory[project.PlayerItemID] = 0
				cooperationViewSetActorFixture(p, DefaultActorID, player)
				view := cooperationProjectViews(p, meta)[0]
				if view.CanWork || view.BlockedReason != "missing_material" || len(view.PlayerCosts) != 1 || view.PlayerCosts[0].Quantity != 1 || view.PlayerCosts[0].Name == "" {
					t.Fatalf("missing player-owned wire or actual cost label not shown: %+v", view)
				}
			}
			p.Characters.Matters[0].Cooperation.Phase = character.CooperationWorked
			p.Characters.Matters[0].Cooperation.WorkSourceRef = "actual-work"
			if view := cooperationProjectViews(p, meta)[0]; view.CanWork || view.CanWorkIfAgreed || view.CanAsk || view.Phase != "worked" {
				t.Fatal("finished preparation was offered again")
			}
		})
	}
}

func cooperationViewSetActorFixture(p *storyBranchProjection, id string, actor AdventureActor) {
	values := map[string]any{}
	for _, op := range adventureActorOps(actor, "qualified-view-fixture") {
		values[op.FieldID] = op.Value
	}
	actors := p.State[actorStateRoot].(map[string]any)
	actors[id] = map[string]any{"id": id, "template_id": DefaultActorID, "state": values}
}
