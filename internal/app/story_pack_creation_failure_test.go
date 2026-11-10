package app

import (
	"context"
	"fmt"
	"net"
	"strings"
	"syscall"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestCharacterDraftConnectionFailureIsLocalized(t *testing.T) {
	cause := fmt.Errorf("generate character draft: %w", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED})
	err := characterDraftError(context.Background(), cause)
	for _, sample := range []struct{ locale, want string }{{"zh-CN", "无法连接当前游戏模型服务"}, {"en-US", "Cannot connect"}} {
		text := storypack.IdentityErrorText(err, sample.locale)
		if !strings.Contains(text, sample.want) || strings.Contains(text, "dial tcp") {
			t.Fatalf("unexpected failure: %s", text)
		}
	}
}

func TestCharacterDraftPreservesInferredLocation(t *testing.T) {
	draft, err := parseIdentityDraftPayload(`{"identity":"叶氏少主","location":"叶氏山庄内院","situation":"在家中接待来访者"}`)
	if err != nil || draft.Location != "叶氏山庄内院" {
		t.Fatalf("draft location lost: %+v %v", draft, err)
	}
}

// The model proposes prose and a place; the committed journal retains the authored purse and realm.
func TestCharacterDraftLocationAndStrongOriginSurviveConfirmation(t *testing.T) {
	root := t.TempDir()
	cfg := enemyDecisionTestConfig(t, root, func(request enemyDecisionTestRequest) string {
		var prompt string
		for _, message := range request.Messages {
			prompt += message.Content
		}
		for _, fact := range []string{"世家少主", "叶氏", "三千灵石", "location", "帅气，玉树临风", "润色"} {
			if !strings.Contains(prompt, fact) {
				t.Errorf("missing creation fact %q", fact)
			}
		}
		return `{"identity":"天赋顶尖，在越国都属于顶尖","appearance":"身量挺拔，眉目清朗。","background":"自幼在叶氏剑坪随长辈练剑。","location":"越国叶氏山庄内院","situation":"在越国叶氏山庄内院接待来访者","opening":"晨光照进叶氏山庄，你在内院翻开来帖。"}`
	})
	application := &App{cfg: cfg}
	choice := storypack.IdentityChoice{Name: "叶行舟", Age: 24, Gender: "male", OriginID: "clan-young-master", AffiliationID: "own", LocationID: "auto", RootCategoryID: "single", RootAttributes: []string{"metal"}, AptitudeID: "ordinary", Identity: "天赋顶尖", Appearance: "帅气，玉树临风", Background: "叶氏，自幼习剑"}
	result, err := application.StoryPacks().DraftStoryPackIdentity(context.Background(), "fanren-jie", StoryPackIdentityDraftRequest{Choice: choice, Locale: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity.LocationName != "越国叶氏山庄内院" || result.Identity.AffiliationName != "叶氏" {
		t.Fatalf("unexpected identity: %+v", result.Identity)
	}
	choice.LocationText = result.Identity.LocationName
	choice.Identity = result.Identity.IdentityText
	choice.Appearance = result.Identity.Appearance
	choice.Background = result.Identity.Background
	choice.Situation = result.Identity.Situation
	pack, err := application.StoryPacks().Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	req, _, err := withConfirmedIdentity(pack, pack.StoryRequest(), choice)
	if err != nil {
		t.Fatal(err)
	}
	service := &InteractiveAppService{app: application}
	req, err = service.withStoryCreationDefaults(req)
	if err != nil {
		t.Fatal(err)
	}
	req, err = service.withPackTemplate(req)
	if err != nil {
		t.Fatal(err)
	}
	journal := t.TempDir()
	store := interactive.NewStore(journal)
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	store = interactive.NewStore(journal)
	defer store.Close()
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	actor := actorStateOf(t, snapshot.State, "protagonist")
	if actor["当前境界"] != "foundation_early" || fmt.Sprint(actor["灵石"]) != "3000" || actor["当前处境"] != choice.Situation {
		t.Fatalf("confirmed state differs: %v", actor)
	}
	if actor["基本身份"] != "世家少主" || actor["外貌与装扮"] != "身量挺拔，眉目清朗。" || actor["性格与背景"] != "自幼在叶氏剑坪随长辈练剑。" {
		t.Fatalf("confirmed profile lost its selected role or polished prose: %v", actor)
	}
}
