package app

import (
	"reflect"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// TestAConfirmedIdentityStartsTheTemplateItResolvedAgainst drives a created
// character through the real start path and checks what the game will show.
//
// The design's rule is that creation changes who the character is, never what
// they are worth: the realm, the purse, the carried things and the techniques
// come from the state basis template, while the persona fields come from the
// player's confirmed draft. A start that let the draft supply any of the former
// would be the "AI grants the realm" defect.
func TestAConfirmedIdentityStartsTheTemplateItResolvedAgainst(t *testing.T) {
	dataRoot := t.TempDir()
	service := &InteractiveAppService{app: &App{
		workspace: dataRoot,
		cfg:       &config.Config{DenovaDir: dataRoot},
	}}
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	template, ok := pack.ProtagonistTemplate("sect-disciple")
	if !ok {
		t.Fatal("the pack has no sect-disciple template")
	}

	req, templateID, err := withConfirmedIdentity(pack, pack.StoryRequest(), storypack.IdentityChoice{
		Name:                 "林照",
		Gender:               "female",
		Age:                  23,
		OriginID:             "custom",
		OriginText:           "世家出身的外门弟子",
		StateBasisTemplateID: "sect-disciple",
		AffiliationID:        "yue.huangfeng",
		LocationID:           "yue.huangfeng.approach",
		RootCategoryID:       "dual",
		RootAttributes:       []string{"wood", "water"},
		AptitudeID:           "good",
		WeaponID:             "sword",
		TraitIDs:             []string{"steady"},
		Identity:             "外门里没人记得她的姓。",
		Appearance:           "青衫，指节有薄茧。",
		Background:           "世家旁支，自幼被送进山门。",
		Situation:            "刚领了今日的差事。",
		Predicament:          "月底前凑不齐那笔灵石。",
	})
	if err != nil {
		t.Fatalf("fold the confirmed identity into the request: %v", err)
	}
	if templateID != "sect-disciple" {
		t.Fatalf("state basis = %q, want sect-disciple", templateID)
	}
	if req.PackTemplate == nil || req.PackTemplate.TemplateID != "sect-disciple" {
		t.Fatalf("request does not bind the resolved template: %+v", req.PackTemplate)
	}
	// The opening must be written from the confirmed premise. A preset here would
	// open the story as the template's own person.
	if req.Opening.Mode != interactive.StoryOpeningModeAI {
		t.Fatalf("opening mode = %q, want ai", req.Opening.Mode)
	}
	if !strings.Contains(req.Origin, "林照") {
		t.Fatalf("opening premise does not name the created character: %q", req.Origin)
	}
	if req.Protagonist.Mode != interactive.StoryProtagonistModeCustom || req.Protagonist.Name != "林照" {
		t.Fatalf("protagonist = %+v, want a custom 林照", req.Protagonist)
	}

	req, err = service.withStoryCreationDefaults(req)
	if err != nil {
		t.Fatalf("apply creation defaults: %v", err)
	}
	req, err = service.withPackTemplate(req)
	if err != nil {
		t.Fatalf("seed the state basis: %v", err)
	}

	store := interactive.NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatalf("create story: %v", err)
	}
	context, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatalf("reopen story: %v", err)
	}
	protagonist := actorStateOf(t, context.Snapshot.State, "protagonist")

	// What the character is worth is the template's, unchanged.
	if got := protagonist["当前境界"]; got != template.State["当前境界"] {
		t.Fatalf("realm = %v, want the template's %v", got, template.State["当前境界"])
	}
	if got := protagonist["灵石"]; !sameActorStateValue(got, template.State["灵石"]) {
		t.Fatalf("spirit stones = %v, want the template's %v", got, template.State["灵石"])
	}
	for _, key := range []string{"已学功法", "主法器", "护具", "重要物品"} {
		if got, want := protagonist[key], template.State[key]; !sameActorStateValue(got, want) {
			t.Fatalf("%s = %v, want the template's %v", key, got, want)
		}
	}

	// What the character is is the player's confirmed draft.
	if got := protagonist["姓名"]; got != "林照" {
		t.Fatalf("name field = %v, want 林照", got)
	}
	for key, want := range map[string]string{
		"基本身份":  "外门里没人记得她的姓。",
		"外貌与装扮": "青衫，指节有薄茧。",
		"性格与背景": "世家旁支，自幼被送进山门。",
		"当前处境":  "刚领了今日的差事。",
	} {
		if got := protagonist[key]; got != want {
			t.Fatalf("%s = %v, want %q", key, got, want)
		}
	}
	// The aptitude the rules normalize is the one the page offered, and the
	// composed text carries exactly one aptitude word.
	foundation, _ := protagonist["灵根与特殊体质"].(string)
	if !strings.Contains(foundation, "良好") {
		t.Fatalf("灵根与特殊体质 = %q, does not carry the chosen 良好 aptitude", foundation)
	}
	if story.Protagonist.Name != "林照" {
		t.Fatalf("story protagonist = %q, want 林照", story.Protagonist.Name)
	}
	// The stage decides whether the player still owes a choice of protagonist
	// from this flag. It must be set on the creation path, and it must survive a
	// reopen: the handoff that skips the setup screen is in memory only, so a
	// reload would otherwise show the second character picker again.
	if !story.IdentityConfirmed {
		t.Fatal("a confirmed creation did not mark the story's identity as confirmed")
	}
	reopened, err := store.Index()
	if err != nil {
		t.Fatalf("reopen the story index: %v", err)
	}
	found := false
	for _, entry := range reopened.Stories {
		if entry.ID != story.ID {
			continue
		}
		found = true
		if !entry.IdentityConfirmed {
			t.Fatal("the confirmed-identity flag did not survive reopening the story")
		}
	}
	if !found {
		t.Fatalf("story %q is missing from the reopened index", story.ID)
	}
}

// TestAPackNamedProtagonistIsNotAConfirmedIdentity pins the other half: a pack
// that declares its own protagonist produces a non-empty custom name without
// ever asking the player anything, so the flag must stay false for it.
func TestAPackNamedProtagonistIsNotAConfirmedIdentity(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:       "从世界包直接开始",
		Protagonist: interactive.StoryProtagonist{Mode: interactive.StoryProtagonistModeCustom, Name: "韩立"},
	})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}
	if story.Protagonist.Name != "韩立" {
		t.Fatalf("story protagonist = %q, want 韩立", story.Protagonist.Name)
	}
	if story.IdentityConfirmed {
		t.Fatal("a pack-named protagonist was treated as a confirmed creation")
	}
}

// TestCreationOverlayCannotReachTheRules pins the closed whitelist: the overlay a
// confirmed character contributes carries persona prose and nothing a rule reads.
func TestCreationOverlayCannotReachTheRules(t *testing.T) {
	dataRoot := t.TempDir()
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	resolved, err := pack.ResolveIdentity(storypack.IdentityChoice{
		Name:           "沈砚",
		Age:            21,
		OriginID:       "loose",
		AffiliationID:  "none",
		LocationID:     "yue.market",
		RootCategoryID: "single",
		RootAttributes: []string{"fire"},
		AptitudeID:     "normal",
		Identity:       "散修，靠替人抄书换灵石。",
		Appearance:     "旧袍洗得发白。",
		Background:     "无门无派。",
		Situation:      "在集市上等一个消息。",
	})
	if err != nil {
		t.Fatalf("resolve identity: %v", err)
	}
	overlay, err := identityStateOverlay(resolved)
	if err != nil {
		t.Fatalf("build overlay: %v", err)
	}
	allowed := map[string]bool{
		"基本身份": true, "外貌与装扮": true, "性格与背景": true,
		"当前处境": true, "灵根与特殊体质": true,
	}
	for key := range overlay {
		if !allowed[key] {
			t.Fatalf("overlay wrote %q, which a rule reads", key)
		}
	}
	for _, key := range []string{"当前境界", "灵石", "已学功法", "主法器", "护具", "重要物品", "功法与专精"} {
		if _, ok := overlay[key]; ok {
			t.Fatalf("overlay reached the mechanical field %q", key)
		}
	}
}

// TestAConfirmedIdentityDoesNotReplayTheTemplateOpening keeps a created
// character from being opened as the ready-made one.
func TestAConfirmedIdentityDoesNotReplayTheTemplateOpening(t *testing.T) {
	dataRoot := t.TempDir()
	service := &InteractiveAppService{app: &App{
		workspace: dataRoot,
		cfg:       &config.Config{DenovaDir: dataRoot},
	}}
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	template, _ := pack.ProtagonistTemplate("sect-disciple")
	if strings.TrimSpace(template.OpeningText) == "" {
		t.Skip("this template ships no fixed opening")
	}
	req, _, err := withConfirmedIdentity(pack, pack.StoryRequest(), storypack.IdentityChoice{
		Name: "林照", Age: 23, OriginID: "custom", OriginText: "世家旁支",
		StateBasisTemplateID: "sect-disciple", AffiliationID: "yue.huangfeng",
		LocationID: "yue.huangfeng.approach", RootCategoryID: "dual",
		RootAttributes: []string{"wood", "water"}, AptitudeID: "good",
	})
	if err != nil {
		t.Fatalf("fold identity: %v", err)
	}
	req, err = service.withStoryCreationDefaults(req)
	if err != nil {
		t.Fatalf("creation defaults: %v", err)
	}
	req, err = service.withPackTemplate(req)
	if err != nil {
		t.Fatalf("seed state basis: %v", err)
	}
	if req.Opening.Mode != interactive.StoryOpeningModeAI || req.Opening.CustomText != "" {
		t.Fatalf("the template's fixed opening was replayed: %+v", req.Opening)
	}
}

func sameActorStateValue(a, b any) bool {
	if na, ok := actorStateNumberForTest(a); ok {
		nb, okb := actorStateNumberForTest(b)
		return okb && na == nb
	}
	return reflect.DeepEqual(a, b)
}
