package interactive

import (
	"testing"

	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

// actorBridgeTestSystem is the smallest actor schema that can express the two
// creation paths: a story context that names who is present, and an important
// character who can be created by either path.
func actorBridgeTestSystem() StoryDirectorActorStateSystem {
	return normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{
				ID:   ActorStateStoryContextTemplateID,
				Name: "故事状态表",
				Fields: []ActorStateField{{
					Name: "在场角色", LegacyPath: "scene.present_actors", Type: "list",
				}},
			},
			{
				ID:   ActorStateImportantCharacterTemplateID,
				Name: "重要角色状态表",
				Fields: []ActorStateField{
					{Name: "基本身份", Type: "string"},
					{Name: "出场与位置", Type: "string"},
				},
			},
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
		},
	})
}

func actorBridgeTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	store := NewStore(t.TempDir())
	system := actorBridgeTestSystem()
	story, err := store.CreateStory(CreateStoryRequest{Title: "人物桥接", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// createActorUpdate is the state_changes shape the model submits for a first
// appearance: the actor id must equal the name, and the template must be one of
// the story's own.
func createActorUpdate(name, templateID string, state map[string]any) interactivestate.Update {
	return interactivestate.Update{
		Op:    interactivestate.Create,
		Path:  "/" + name,
		Value: map[string]any{"template_id": templateID, "name": name, "state": state},
	}
}

func appendActorBridgeTurn(t *testing.T, store *Store, storyID string, updates []interactivestate.Update, facts *TurnCharacterFactsInput) {
	t.Helper()
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "继续",
		Narrative: "夜色沉下来，街上的人各自散去。",
		TurnResult: &TurnResult{
			Choices:          testTurnChoices(),
			StateUpdates:     updates,
			CharacterFacts:   facts,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestActorStateCreateWithoutCharacterFactsRegistersThePerson is the first half
// of the desync: a turn that creates an actor through state_changes but submits
// no character_facts currently commits a person the character window, the cast
// block and the discovery exclusion list cannot see. The same turn's state must
// be enough to register them once.
func TestActorStateCreateWithoutCharacterFactsRegistersThePerson(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, []interactivestate.Update{
		createActorUpdate("柳掌柜", ActorStateImportantCharacterTemplateID, map[string]any{
			"基本身份":   "东市药铺掌柜",
			"出场与位置": "在场，东市药铺",
		}),
	}, nil)

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(state.CharacterIDs(), "柳掌柜") {
		t.Fatalf("an actor created by state_changes must be findable from the normal character entry; registered ids = %v", state.CharacterIDs())
	}
	profile, ok := state.Profile("柳掌柜")
	if !ok || profile.Name != "柳掌柜" {
		t.Fatalf("profile for the created actor = %+v ok=%v", profile, ok)
	}
}

// TestAnActorCreatedWithoutARoleDoesNotGetTheTemplateIDAsTheirIdentity guards
// the card the player actually reads. A create that only fills the template's
// own fields says nothing about the person's role in the story, and the schema
// writes the template id into that slot when it is empty. Printing
// "important_character" under someone's name is a schema label leaking onto the
// person card; the honest answer is that no role was stated.
//
// The introduction has the same problem in the other direction: the create said
// who this person is, in 基本身份, and the card showed an empty description
// because only the top-level `description` key was read.
func TestAnActorCreatedWithoutARoleDoesNotGetTheTemplateIDAsTheirIdentity(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, []interactivestate.Update{
		createActorUpdate("柳掌柜", ActorStateImportantCharacterTemplateID, map[string]any{
			"基本身份": "东市药铺掌柜，五十来岁，算盘打得极快。",
		}),
	}, nil)

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := state.Profile("柳掌柜")
	if !ok {
		t.Fatalf("the created actor must be registered; ids = %v", state.CharacterIDs())
	}
	if profile.Role == ActorStateImportantCharacterTemplateID {
		t.Fatalf("the template id is not a role: profile = %+v", profile)
	}
	if profile.Role != "" {
		t.Fatalf("a create that stated no role must register none, got %q", profile.Role)
	}
	if profile.Description != "东市药铺掌柜，五十来岁，算盘打得极快。" {
		t.Fatalf("the introduction must come from what the create actually said; description = %q", profile.Description)
	}
}

// TestAnActorBackedPersonKeepsTheirIntroductionAcrossALaterTurn is the same rule
// the submitted-profile path already follows, applied to the actor-backed one: a
// turn that only reports a change of attitude must not blank the card.
func TestAnActorBackedPersonKeepsTheirIntroductionAcrossALaterTurn(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, []interactivestate.Update{
		createActorUpdate("柳掌柜", ActorStateImportantCharacterTemplateID, map[string]any{
			"基本身份": "东市药铺掌柜，五十来岁，算盘打得极快。",
		}),
	}, nil)
	appendActorBridgeTurn(t, store, storyID, nil, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "柳掌柜", Name: "柳掌柜", Attitude: "客气了些",
		}},
	})

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := state.Profile("柳掌柜")
	if !ok {
		t.Fatalf("the person must still be registered; ids = %v", state.CharacterIDs())
	}
	if profile.Description != "东市药铺掌柜，五十来岁，算盘打得极快。" {
		t.Fatalf("a turn that only changed the attitude blanked the introduction: %+v", profile)
	}
	if profile.Role != "" {
		t.Fatalf("the template id must not reappear as a role: %q", profile.Role)
	}
}

// TestAnActorBackedPersonSurvivesAReopen is what the player does after closing
// the game. The profile is a story record, not a runtime cache, so a new Store
// on the same root must still know who this person is.
func TestAnActorBackedPersonSurvivesAReopen(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	system := actorBridgeTestSystem()
	story, err := store.CreateStory(CreateStoryRequest{Title: "人物桥接", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	appendActorBridgeTurn(t, store, story.ID, []interactivestate.Update{
		createActorUpdate("柳掌柜", ActorStateImportantCharacterTemplateID, map[string]any{
			"基本身份": "东市药铺掌柜，五十来岁，算盘打得极快。",
		}),
	}, nil)

	reopened := NewStore(root)
	state, err := reopened.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := state.Profile("柳掌柜")
	if !ok {
		t.Fatalf("the profile did not survive the reopen; ids = %v", state.CharacterIDs())
	}
	if profile.Description != "东市药铺掌柜，五十来岁，算盘打得极快。" {
		t.Fatalf("the introduction did not survive the reopen: %+v", profile)
	}
}

// TestCharacterFactsProfileWithoutActorStillRegisters is the mirror case: the
// character path already worked and must keep working, with no actor invented
// for a person the state never staged.
func TestCharacterFactsProfileWithoutActorStillRegisters(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, nil, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{CharacterID: "周砚", Name: "周砚", Role: "账房"}},
	})
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(state.CharacterIDs(), "周砚") {
		t.Fatalf("a profile submitted through character_facts must be registered; ids = %v", state.CharacterIDs())
	}
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	actors, _ := snapshot.State[actorStateRoot].(map[string]any)
	if _, exists := actors["周砚"]; exists {
		t.Fatal("registering a profile must not fabricate an actor the state never staged")
	}
}

// TestActorAndCharacterFactsInOneTurnShareOneIdentity is the case the two
// registries must agree on: a turn that both stages an actor and submits its
// profile must end with exactly one person, not two spellings of one person.
func TestActorAndCharacterFactsInOneTurnShareOneIdentity(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, []interactivestate.Update{
		createActorUpdate("钱守信", ActorStateImportantCharacterTemplateID, map[string]any{"基本身份": "南街牙人"}),
	}, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "钱守信", Name: "钱守信", SourceKey: "npc_qian_shouxin",
			Role: "南街牙人", Personality: []string{"嘴甜"}, SpeechStyle: "话密",
		}},
		Exchanges: []TurnCharacterExchangeInput{{
			CharacterID: "钱守信", Speaker: "character", Channel: "scene", Text: "客官要打听什么？",
		}},
	})
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if ids := state.CharacterIDs(); len(ids) != 1 || ids[0] != "钱守信" {
		t.Fatalf("one person appearing through both paths must be registered once; ids = %v", ids)
	}
	profile, ok := state.Profile("钱守信")
	if !ok || profile.SourceKey != "npc_qian_shouxin" || profile.Role != "南街牙人" {
		t.Fatalf("the actor-backed profile must keep the submitted identity; got %+v ok=%v", profile, ok)
	}
	if len(state.Exchanges) != 1 {
		t.Fatalf("the line must be recorded against the same person; exchanges = %+v", state.Exchanges)
	}
}

// TestCharacterFactsSourceKeyRebindsTheTurnsActorOps guards the numbers half of
// the same rule: when a preset person returns under a minted id, the profile and
// the actor ops must land on the established actor together. Prose on one id and
// numbers on another is the split this test forbids.
func TestCharacterFactsSourceKeyRebindsTheTurnsActorOps(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	// First appearance: the preset person is staged and registered under the
	// author's key.
	appendActorBridgeTurn(t, store, storyID, []interactivestate.Update{
		createActorUpdate("苏管事", ActorStateImportantCharacterTemplateID, map[string]any{"基本身份": "驿馆管事"}),
	}, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "苏管事", Name: "苏管事", SourceKey: "npc_su",
		}},
	})
	// Second turn: the model mints a new id from a different spelling and stages
	// the actor under that id as well.
	appendActorBridgeTurn(t, store, storyID, []interactivestate.Update{
		createActorUpdate("苏驿丞", ActorStateImportantCharacterTemplateID, map[string]any{"基本身份": "驿馆管事"}),
	}, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "苏驿丞", Name: "苏驿丞", SourceKey: "npc_su",
		}},
		Exchanges: []TurnCharacterExchangeInput{{
			CharacterID: "苏驿丞", Speaker: "character", Channel: "scene", Text: "路引呢？",
		}},
	})

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if ids := state.CharacterIDs(); len(ids) != 1 {
		t.Fatalf("a source-keyed person must not be registered twice; ids = %v", ids)
	}
	if len(state.Exchanges) != 1 || state.Exchanges[0].CharacterID != "苏管事" {
		t.Fatalf("the second turn's line must be rebound onto the established person; exchanges = %+v", state.Exchanges)
	}

	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	actors, _ := snapshot.State[actorStateRoot].(map[string]any)
	if _, exists := actors["苏驿丞"]; exists {
		t.Fatalf("the turn's actor creation must be rebound onto the established actor, not left as a second one; actors = %v", mapKeys(actors))
	}
	if _, exists := actors["苏管事"]; !exists {
		t.Fatalf("the established actor must survive the rebound; actors = %v", mapKeys(actors))
	}
}

// TestActorCreateDoesNotBecomeAContactWhenOnlyMentioned keeps the boundary: an
// actor the turn never staged and the prose only named must not appear through
// the bridge.
func TestActorCreateDoesNotBecomeAContactWhenOnlyMentioned(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, nil, nil)
	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if ids := state.CharacterIDs(); len(ids) != 0 {
		t.Fatalf("a turn that staged nobody must register nobody; ids = %v", ids)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

var _ = character.State{}

// createActorUpdateWithIdentity is the same create, carrying the three identity
// facts the model settles when it stages somebody: gender, voice and the labels
// a reader would search them by.
func createActorUpdateWithIdentity(name, templateID string, state map[string]any, identity map[string]any) interactivestate.Update {
	value := map[string]any{"template_id": templateID, "name": name, "state": state}
	for key, item := range identity {
		value[key] = item
	}
	return interactivestate.Update{Op: interactivestate.Create, Path: "/" + name, Value: value}
}

// TestStateChangesCreateSavesTheIdentityItStated covers the entry point that has
// no character_facts at all: a turn that stages someone through state_changes
// alone and settles their gender, voice and labels must leave all three where
// the person card reads them, and must leave them again after a reopen.
func TestStateChangesCreateSavesTheIdentityItStated(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	system := actorBridgeTestSystem()
	story, err := store.CreateStory(CreateStoryRequest{Title: "人物桥接", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	appendActorBridgeTurn(t, store, story.ID, []interactivestate.Update{
		createActorUpdateWithIdentity("柳掌柜", ActorStateImportantCharacterTemplateID,
			map[string]any{"基本身份": "东市药铺掌柜，五十来岁，算盘打得极快。"},
			map[string]any{
				"gender":       "male",
				"speech_style": "先问用途再报价，算账时嘴里念着数目",
				"tags":         []string{"东市药铺", "掌柜"},
			}),
	}, nil)

	for _, attempt := range []struct {
		name  string
		store *Store
	}{{"first read", store}, {"after reopen", NewStore(root)}} {
		state, err := attempt.store.CharacterState(story.ID, "main")
		if err != nil {
			t.Fatalf("%s: %v", attempt.name, err)
		}
		profile, ok := state.Profile("柳掌柜")
		if !ok {
			t.Fatalf("%s: the created actor must be registered; ids = %v", attempt.name, state.CharacterIDs())
		}
		if profile.Gender != "male" {
			t.Fatalf("%s: gender the create stated = %q", attempt.name, profile.Gender)
		}
		if profile.SpeechStyle != "先问用途再报价，算账时嘴里念着数目" {
			t.Fatalf("%s: voice the create stated = %q", attempt.name, profile.SpeechStyle)
		}
		if len(profile.Tags) != 2 || profile.Tags[0] != "东市药铺" || profile.Tags[1] != "掌柜" {
			t.Fatalf("%s: labels the create stated = %v", attempt.name, profile.Tags)
		}
	}
}

// TestStateChangesCreateWithoutIdentityLeavesItUnsettled is the other half of
// the same rule. A create that said nothing about gender must not get one, and
// nothing may be inferred from the name or the role: an empty value is the
// honest record of a fact nobody decided.
func TestStateChangesCreateWithoutIdentityLeavesItUnsettled(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, []interactivestate.Update{
		createActorUpdate("阿沅", ActorStateImportantCharacterTemplateID, map[string]any{
			"基本身份": "东市药铺的学徒",
		}),
	}, nil)

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := state.Profile("阿沅")
	if !ok {
		t.Fatalf("the created actor must be registered; ids = %v", state.CharacterIDs())
	}
	if profile.Gender != "" {
		t.Fatalf("an unsettled gender must stay empty, got %q", profile.Gender)
	}
	if profile.SpeechStyle != "" {
		t.Fatalf("a voice nobody wrote must stay empty, got %q", profile.SpeechStyle)
	}
	if len(profile.Tags) != 0 {
		t.Fatalf("labels nobody stated must stay empty, got %v", profile.Tags)
	}
}

// TestStateChangesCreateThatDescribedNobodyRegistersNoContact guards the empty
// card. A create with a template and nothing else would put a name-only row in
// the character window and in the discovery exclusion list, which hides the
// person from the turn that actually describes them.
func TestStateChangesCreateThatDescribedNobodyRegistersNoContact(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, []interactivestate.Update{
		createActorUpdate("无名氏", ActorStateImportantCharacterTemplateID, nil),
	}, nil)

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if containsString(state.CharacterIDs(), "无名氏") {
		t.Fatalf("a create that described nobody must not register a contact; ids = %v", state.CharacterIDs())
	}
}

// TestCharacterFactsProfileSavesItsLabelsAndVoice is the submitted-profile entry
// point. The three facts the turn settled there must reach the card and stay
// there across a reopen, and a later turn that states nothing about them must
// not clear them.
func TestCharacterFactsProfileSavesItsLabelsAndVoice(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	system := actorBridgeTestSystem()
	story, err := store.CreateStory(CreateStoryRequest{Title: "人物桥接", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	appendActorBridgeTurn(t, store, story.ID, nil, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "周砚", Name: "周砚", Role: "账房",
			Description: "城南绸缎庄的账房，袖口总沾着墨。",
			Gender:      "male", SpeechStyle: "话少，先听后答",
			Tags: []string{"城南绸缎庄", "账房"},
		}},
	})
	// A later turn reports one more label and nothing else.
	appendActorBridgeTurn(t, store, story.ID, nil, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "周砚", Name: "周砚", Attitude: "客气了些",
			Tags: []string{"识文断字"},
		}},
	})

	state, err := NewStore(root).CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := state.Profile("周砚")
	if !ok {
		t.Fatalf("the submitted profile must be registered; ids = %v", state.CharacterIDs())
	}
	if profile.Gender != "male" || profile.SpeechStyle != "话少，先听后答" {
		t.Fatalf("identity stated once must survive a later silent turn: %+v", profile)
	}
	for _, want := range []string{"城南绸缎庄", "账房", "识文断字"} {
		if !containsString(profile.Tags, want) {
			t.Fatalf("label %q must be kept; labels = %v", want, profile.Tags)
		}
	}
	if profile.Description != "城南绸缎庄的账房，袖口总沾着墨。" {
		t.Fatalf("a later turn must not blank the introduction: %q", profile.Description)
	}
}

// TestARepeatedLabelIsNotStoredTwice keeps the label list from growing a
// duplicate every time the same fact is restated, which would make the search
// result read as two different things.
func TestARepeatedLabelIsNotStoredTwice(t *testing.T) {
	store, storyID := actorBridgeTestStory(t)
	appendActorBridgeTurn(t, store, storyID, nil, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "周砚", Name: "周砚", Description: "城南绸缎庄的账房。",
			Tags: []string{"账房", "城南绸缎庄"},
		}},
	})
	appendActorBridgeTurn(t, store, storyID, nil, &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "周砚", Name: "周砚", Tags: []string{"账房"},
		}},
	})

	state, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	profile, _ := state.Profile("周砚")
	count := 0
	for _, tag := range profile.Tags {
		if tag == "账房" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("a restated label must appear once; labels = %v", profile.Tags)
	}
}
