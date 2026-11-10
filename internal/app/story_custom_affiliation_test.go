package app

import (
	"strings"
	"testing"

	"ta/config"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

func TestCustomIdentityOpeningAndEmptyPersonaReplacePreset(t *testing.T) {
	root := t.TempDir()
	service := &InteractiveAppService{app: &App{workspace: root, cfg: &config.Config{DenovaDir: root}}}
	pack, err := storypack.NewLibrary(root).Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	choice := storypack.IdentityChoice{Name: "叶流云", Gender: "female", Age: 24, OriginID: "custom", OriginText: "听潮小筑的外门游学者", StateBasisTemplateID: "sect-disciple", AffiliationText: "听潮小筑", LocationText: "海岬旧灯塔", OpeningMode: "custom", Opening: "落潮时，灯塔下有人敲门。", RootCategoryID: "dual", RootAttributes: []string{"metal", "water"}, AptitudeID: "ordinary"}
	req, _, err := withConfirmedIdentity(pack, pack.StoryRequest(), choice)
	if err != nil {
		t.Fatal(err)
	}
	if req.Opening.Mode != interactive.StoryOpeningModeCustom || req.Opening.CustomText != choice.Opening {
		t.Fatal("custom opening not preserved")
	}
	req, err = service.withStoryCreationDefaults(req)
	if err != nil {
		t.Fatal(err)
	}
	req, err = service.withPackTemplate(req)
	if err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatal(err)
	}
	context, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	state := actorStateOf(t, context.Snapshot.State, interactive.DefaultActorID)
	for _, key := range []string{"基本身份", "外貌与装扮", "性格与背景", "当前处境", "功法与专精"} {
		text, _ := state[key].(string)
		if strings.Contains(text, "黄枫谷") || strings.Contains(text, "燕家") {
			t.Fatalf("%s inherited preset: %s", key, text)
		}
	}
	if state["姓名"] != "叶流云" || !strings.Contains(state["基本身份"].(string), "听潮小筑") {
		t.Fatal("confirmed custom identity did not reach actor")
	}
	instruction, err := interactive.StoryOpeningInstruction(context.Meta)
	if err != nil || !strings.Contains(instruction, choice.Opening) || !strings.Contains(instruction, "海岬旧灯塔") {
		t.Fatalf("first-turn consumer lost custom choices: %v", err)
	}
}
