package app

import (
	"encoding/json"
	"strings"
	"ta/config"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/quest"
	"ta/internal/storypack"
	"testing"
)

func TestAdventureQuestEnglishViewUsesActualStoryAndPreservesJournal(t *testing.T) {
	for _, packID := range []string{"lanchuan-qunxia", "xinghai-689"} {
		t.Run(packID, func(t *testing.T) {
			root := t.TempDir()
			application := &App{workspace: root, cfg: &config.Config{DenovaDir: root}}
			service := &InteractiveAppService{app: application}
			pack, err := storypack.NewLibrary(root).Get(packID)
			if err != nil {
				t.Fatal(err)
			}
			req := pack.StoryRequest()
			req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: packID, TemplateID: pack.ProtagonistTemplates[0].ID}
			req, err = service.withStoryCreationDefaults(req)
			if err != nil {
				t.Fatal(err)
			}
			req, err = service.withPackTemplate(req)
			if err != nil {
				t.Fatal(err)
			}
			store := interactive.NewStore(root)
			defer store.Close()
			application.interactive = store
			story, err := store.CreateStory(req)
			if err != nil {
				t.Fatal(err)
			}
			// Public opportunities require the opening's committed scene, not merely
			// a created save. Only descriptive location is established here; no
			// inventory, skill, success or task result is injected.
			location := "潮信渡"
			if packID == "xinghai-689" {
				location = "朱伯利外环47号减压长廊"
			}
			_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "看看周围", Narrative: "你留意身边的公开告示和往来交谈。", TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Op: "replace", Path: "/story/当前详细地点", Value: location}}, Choices: []string{"继续", "观察", "询问"}}})
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.StoryContext(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			zh, err := application.QuestStateFor(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			en, err := application.QuestStateForLocale(story.ID, "main", "en-US")
			if err != nil {
				t.Fatal(err)
			}
			if en.Location == zh.Location {
				t.Fatal("authored current location was not localized")
			}
			if len(en.Available) == 0 || len(en.Available) != len(zh.Available) {
				t.Fatal("actual initial opportunities missing")
			}
			for i, v := range en.Available {
				if v.TaskID != zh.Available[i].TaskID || v.Title == zh.Available[i].Title {
					t.Fatal("opportunity identity or locale mismatch")
				}
			}
			id := en.Available[0].TaskID
			accepted, err := application.ApplyQuest(QuestAppRequest{StoryID: story.ID, BranchID: "main", TaskID: id, Action: "accept", CommandID: "accept", Locale: "en-US"})
			if err != nil {
				t.Fatal(err)
			}
			d, _ := quest.DefinitionByID(id)
			english := quest.LocalizedDefinition(d, "en-US")
			if accepted.Objectives[0].Description != english.Objectives[0].Description {
				t.Fatal("action response not localized")
			}
			saved, err := store.StoryContext(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			_, err = application.QuestStateForLocale(story.ID, "main", "en-US")
			if err != nil {
				t.Fatal(err)
			}
			after, err := store.StoryContext(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(saved.Snapshot)
			b, _ := json.Marshal(after.Snapshot)
			if string(a) != string(b) {
				t.Fatal("language read mutated journal")
			}
			if before.Snapshot.BranchID != after.Snapshot.BranchID {
				t.Fatal("language read switched branch")
			}
			state := interactive.QuestState{Tasks: []interactive.QuestStateView{{TaskID: d.ID, Resolutions: map[string]quest.ChoiceResult{d.Choices[0].ObjectiveID: {ChoiceID: d.Choices[0].ID, Outcome: "failure", EchoResolved: true, EchoAcknowledgedBy: "known-source"}}}}}
			localizeQuestViews(&state, "en-US")
			if len(state.Tasks[0].Consequences) != 2 || !strings.Contains(state.Tasks[0].Consequences[0], "does not succeed") {
				t.Fatal("failed consequence used success prose")
			}
		})
	}
}

func TestPersonalEchoEnglishProjectionRetainsKnowledgeBoundary(t *testing.T) {
	d, _ := quest.DefinitionByID("xinghai-a01")
	c := d.Choices[0]
	if c.SuccessReaction == nil {
		t.Fatal("authored reaction missing")
	}
	result := quest.ChoiceResult{ChoiceID: c.ID, Outcome: "success", EchoResolved: true, EchoEncounterRequired: true}
	state := interactive.QuestState{Available: []interactive.QuestOption{{TaskID: d.ID}}, Tasks: []interactive.QuestStateView{{TaskID: d.ID, Resolutions: map[string]quest.ChoiceResult{c.ObjectiveID: result}}}}
	localizeQuestViews(&state, "en-US")
	if len(state.Tasks[0].Consequences) != 1 || state.Tasks[0].Choices[0].SuccessEcho != "" || state.Tasks[0].Choices[0].SuccessReaction != nil || state.Available[0].Choices[0].SuccessEcho != "" {
		t.Fatal("English read disclosed an unacknowledged reaction")
	}
	result.EchoAcknowledgedBy = "montuo"
	state.Tasks[0].Resolutions[c.ObjectiveID] = result
	localizeQuestViews(&state, "en-US")
	if len(state.Tasks[0].Consequences) != 2 || state.Tasks[0].Choices[0].SuccessEcho == "" {
		t.Fatal("known English response disappeared")
	}
	translated := quest.LocalizedDefinition(d, "en-US")
	if translated.Choices[0].SuccessReaction.Statement == c.SuccessReaction.Statement {
		t.Fatal("authored English reaction missing")
	}
	if got, _ := quest.DefinitionByID(d.ID); got.Choices[0].SuccessReaction.Statement != c.SuccessReaction.Statement {
		t.Fatal("localization rewrote canonical facts")
	}
}
