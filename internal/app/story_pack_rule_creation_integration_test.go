package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"ta/config"
	"ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

var ruleCreationPackIDs = []string{
	"doupo-continent-ta07-full-gm-candidate",
	"harry-potter-1991-ta07-full-gm-candidate",
	"harry-potter-1991-student-ta07-full-gm-candidate",
	"lord-of-mysteries-ta07-full-gm-candidate",
	"sanguo-190-ta07-full-gm-candidate",
	"zichuan-xichuan-ta07-full-gm-candidate",
}

// Use the existing HTTP fake-model facility, persist its isolated settings, and
// construct the real App. This is product integration, never a live-model claim.
func ruleCreationTestApp(t *testing.T, answer func(enemyDecisionTestRequest) string) *App {
	t.Helper()
	root := t.TempDir()
	cfg := enemyDecisionTestConfig(t, root, answer)
	cfg.DenovaDir, cfg.Workspace = root, ""
	if err := config.WriteSettingsFile(config.UserConfigPath(root), config.Settings{
		OpenAIBaseURL: cfg.OpenAIBaseURL, OpenAIModel: cfg.OpenAIModel, OpenAIAPIKey: cfg.OpenAIAPIKey,
		ModelEndpoints: cfg.ModelEndpoints, ModelProfiles: cfg.ModelProfiles, AgentModels: cfg.AgentModels,
	}); err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	return application
}

func ruleCreationEqual(t *testing.T, label string, got, want any) {
	t.Helper()
	left, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(left) != string(right) {
		t.Fatalf("%s differs: got %s; want %s", label, left, right)
	}
}

func ruleCreationAssertState(t *testing.T, store *interactive.Store, storyID string, pack storypack.Pack, template storypack.ProtagonistTemplate, overlay map[string]any) interactive.StoryContext {
	t.Helper()
	cold := interactive.NewStore(store.Root())
	defer cold.Close()
	view, err := cold.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	state := actorStateOf(t, view.Snapshot.State, interactive.DefaultActorID)
	for key, want := range template.State {
		if value, ok := overlay[key]; ok {
			want = value
		}
		got, exists := state[key]
		if !exists {
			t.Fatalf("authored field disappeared: %s", key)
		}
		ruleCreationEqual(t, "authored field "+key, got, want)
	}
	for key, want := range overlay {
		ruleCreationEqual(t, "confirmed field "+key, state[key], want)
	}
	if view.Meta.PackBinding == nil || view.Meta.PackBinding.PackID != pack.ID || view.Meta.PackBinding.ContentHash != pack.ContentHash() {
		t.Fatal("product start did not freeze the selected revision")
	}
	if view.Snapshot.ActorStateSchema == nil || view.Snapshot.ActorStateSchema.System.AdventureRules != nil || view.Snapshot.ActorStateSchema.System.CultivationRules != nil {
		t.Fatal("opaque authored facts activated a dedicated rules engine")
	}
	return view
}

// All 55 ready-made templates enter CreateInteractiveStoryContext and complete
// the production opening tool/atomic commit protocol with controlled inputs.
// Each of the six configurations additionally drives PrepareStartTemplate, CreateBook and
// Apply; other templates reuse that same imported Project, avoiding 55 duplicate
// world-book imports without bypassing the product story-creation service.
func TestRuleCandidatesProductStartsPreserveEveryAuthoredTemplate(t *testing.T) {
	expectedCounts := ruleCreationTemplateCounts(t)
	for _, packID := range ruleCreationPackIDs {
		t.Run(packID, func(t *testing.T) {
			var calls atomic.Int32
			var secret atomic.Value
			secret.Store("")
			application := ruleCreationTestApp(t, func(request enemyDecisionTestRequest) string {
				calls.Add(1)
				for _, message := range request.Messages {
					if marker := secret.Load().(string); marker != "" && (strings.Contains(message.Content, marker) || strings.Contains(oneLineText(message.Content), oneLineText(marker))) {
						t.Error("private-character model request contains imported GM-only content")
					}
				}
				return `{"reply":"我只知道眼前的事情。","affinities":[],"relationships":[],"matters":[]}`
			})
			pack, err := application.StoryPacks().Get(packID)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := len(pack.ProtagonistTemplates), expectedCounts[packID]; got != want {
				t.Fatalf("authored template count for %s = %d, want %d", packID, got, want)
			}
			first := pack.ProtagonistTemplates[0]
			started, err := application.StoryPacks().PrepareStartTemplate(context.Background(), packID, first.ID)
			if err != nil {
				t.Fatal(err)
			}
			index, err := application.InteractiveStories()
			if err != nil || len(index.Stories) != 1 {
				t.Fatalf("placeholder not removed: %+v %v", index, err)
			}
			for index, template := range pack.ProtagonistTemplates {
				t.Run(template.ID, func(t *testing.T) {
					story := started.Story
					if index > 0 {
						req, _, err := pack.StoryRequestForTemplate(template.ID)
						if err != nil {
							t.Fatal(err)
						}
						req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: pack.ID, TemplateID: template.ID}
						story, err = application.CreateInteractiveStoryContext(context.Background(), req)
						if err != nil {
							t.Fatal(err)
						}
					}
					ruleCreationCompleteOpening(t, application, story.ID, template, nil)
					view := ruleCreationAssertState(t, application.interactive, story.ID, pack, template, nil)
					if view.Meta.Protagonist.Name != template.CharacterName || view.Meta.Protagonist.Profile != template.Profile || view.Meta.Origin != template.Origin {
						t.Fatal("template personality, identity or opening premise was replaced")
					}
				})
			}
			if calls.Load() != 0 {
				t.Fatal("controlled opening tools unexpectedly invoked the HTTP model")
			}

			t.Run("imported_gm_private_boundary", func(t *testing.T) {
				// Compare the entire actual imported collection, not a hand-seeded store.
				view, err := application.interactive.StoryContext(started.Story.ID, "main")
				if err != nil {
					t.Fatal(err)
				}
				if view.Snapshot.TurnCount == 0 {
					ruleCreationCompleteOpening(t, application, started.Story.ID, first, nil)
					view = ruleCreationAssertState(t, application.interactive, started.Story.ID, pack, first, nil)
				}
				reader := lore.NewScopedStore(started.Workspace, view.Meta.PackBinding.Scope).WithVisibility(interactive.StoryLoreVisibility(view.Meta, view.Snapshot.State))
				items, err := reader.List()
				if err != nil {
					t.Fatal(err)
				}
				byID := map[string]lore.Item{}
				for _, item := range items {
					byID[item.ID] = item
				}
				inputs := pack.WorldInputs()
				if len(items) != len(inputs) {
					t.Fatalf("world import incomplete: got %d, want %d", len(items), len(inputs))
				}
				var gm lore.Item
				for _, input := range inputs {
					item, ok := byID[input.ID]
					if !ok || item.Content != input.Content || item.Audience != input.Audience {
						t.Fatalf("world import changed body or audience: %s", input.ID)
					}
					if item.Audience == "gm" && gm.ID == "" {
						gm = item
					}
				}
				if gm.ID == "" {
					t.Fatal("candidate has no GM item to exercise")
				}
				read, err := reader.Read(gm.ID)
				if err != nil || read.Content != gm.Content {
					t.Fatal("GM reader cannot read the imported revision", err)
				}
				// A same-name public fixture deliberately exercises the GM lore-name
				// lookup temptation; its synthetic identity grants no GM knowledge.
				profile := character.Profile{ID: gm.Name, Name: gm.Name, Role: "公开场景中的测试人物"}
				projected := strings.Join(characterLoreFor(started.Workspace, view.Meta.PackBinding.Scope, profile, character.Scene{}, len(items), view), "\n")
				if strings.Contains(projected, gm.Name+" (") {
					t.Fatal("GM-only item entered the character lore projection")
				}
				// The controlled presence and reply below are fixture facts, not a
				// claim that a real narrator completed this world's opening.
				if _, err := application.interactive.AppendCharacterFacts(started.Story.ID, interactive.AppendCharacterFactsRequest{BranchID: "main", WorldTime: characterWorldTime(view), Profiles: []character.Profile{profile}}); err != nil {
					t.Fatal(err)
				}
				if _, _, err := application.interactive.AppendTurnWithState(started.Story.ID, interactive.AppendTurnWithStateRequest{
					BranchID: "main", User: "查看眼前", Narrative: "一位测试人物站在眼前。",
					ActorOps:   []interactive.ActorStateOp{{Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色", Value: []any{interactive.DefaultActorID, profile.ID}}},
					TurnResult: &interactive.TurnResult{Choices: ruleCreationFixtureChoices(view.Meta.ChoiceCount)},
				}); err != nil {
					t.Fatal(err)
				}
				secret.Store(ruleCreationGMBodyMarker(t, gm.Content))
				if _, err := application.SendCharacterMessage(context.Background(), CharacterChatRequest{StoryID: started.Story.ID, BranchID: "main", CharacterID: profile.ID, Message: "你看到了什么？", CommandID: "creation-boundary-chat", Locale: "zh-CN"}); err != nil {
					t.Fatal(err)
				}
				if calls.Load() != 1 {
					t.Fatalf("private route model calls = %d, want 1", calls.Load())
				}
				ruleCreationAssertState(t, application.interactive, started.Story.ID, pack, first, nil)
			})
		})
	}
}

func TestRuleCandidatesCustomDraftConfirmationAndRejectedChoices(t *testing.T) {
	for _, packID := range ruleCreationPackIDs {
		t.Run(packID, func(t *testing.T) {
			var calls atomic.Int32
			var response atomic.Value
			response.Store(`{}`)
			var backgroundRule atomic.Value
			backgroundRule.Store("")
			application := ruleCreationTestApp(t, func(request enemyDecisionTestRequest) string {
				calls.Add(1)
				var prompt strings.Builder
				for _, message := range request.Messages {
					prompt.WriteString(message.Content)
				}
				if !strings.Contains(prompt.String(), backgroundRule.Load().(string)) || !strings.Contains(prompt.String(), "历练状态") {
					t.Error("draft prompt lost authored facts or custom background instructions")
				}
				return response.Load().(string)
			})
			pack, err := application.StoryPacks().Get(packID)
			if err != nil {
				t.Fatal(err)
			}
			backgroundRule.Store(pack.Creation.CustomBackgroundRule)
			base := storypack.IdentityChoice{Name: "自建测试者", Age: pack.Creation.AgeRange[0], Gender: "unspecified", OriginID: "custom", StateBasisTemplateID: pack.Creation.CustomStateBasisTemplates[0], AffiliationID: "none", LocationID: "auto", Background: "出生在玩家另述的远方，未获得额外资源。"}
			projectReady := false
			for _, basis := range pack.Creation.CustomStateBasisTemplates {
				t.Run(basis, func(t *testing.T) {
					choice := base
					choice.StateBasisTemplateID = basis
					resolved, err := pack.ResolveIdentity(choice)
					if err != nil {
						t.Fatal(err)
					}
					const polishedBackground = "自远方来，行囊里还是启程时那几样旧物。"
					payload, err := json.Marshal(map[string]any{"location": resolved.LocationName, "identity": "谨慎的旅人", "appearance": "朴素衣装", "background": polishedBackground, "situation": "整理眼前行李", "opening": "你停下脚步查看行李。", "state": map[string]any{"钱财": 999999, "历练等级": 99}, "rank": 99})
					if err != nil {
						t.Fatal(err)
					}
					response.Store(string(payload))
					drafted, err := application.StoryPacks().DraftStoryPackIdentity(context.Background(), packID, StoryPackIdentityDraftRequest{Choice: choice, Locale: "zh-CN"})
					if err != nil {
						t.Fatal(err)
					}
					identity := drafted.Identity
					if identity.Background != polishedBackground || identity.Age != choice.Age || identity.InitialState == nil || len(identity.InitialState.AuthoredFields) == 0 {
						t.Fatal("draft lost fixed identity or the authored preview")
					}
					choice.LocationID, choice.LocationText = identity.LocationID, ""
					choice.Identity, choice.Appearance, choice.Situation = identity.IdentityText, identity.Appearance, identity.Situation
					choice.Background = identity.Background
					choice.Opening, choice.OpeningMode = identity.Opening, "custom"
					var story interactive.StorySummary
					if !projectReady {
						started, err := application.StoryPacks().PrepareStartIdentity(context.Background(), packID, "", &choice)
						if err != nil {
							t.Fatal(err)
						}
						story = started.Story
						projectReady = true
					} else {
						req, _, err := withConfirmedIdentity(pack, pack.StoryRequest(), choice)
						if err != nil {
							t.Fatal(err)
						}
						story, err = application.CreateInteractiveStoryContext(context.Background(), req)
						if err != nil {
							t.Fatal(err)
						}
					}
					template, _ := pack.ProtagonistTemplate(basis)
					overlay := map[string]any{"姓名": choice.Name, "基本身份": choice.Identity, "外貌与装扮": choice.Appearance, "性格与背景": choice.Background, "当前处境": choice.Situation}
					ruleCreationCompleteOpening(t, application, story.ID, template, overlay)
					view := ruleCreationAssertState(t, application.interactive, story.ID, pack, template, overlay)
					if !view.Meta.IdentityConfirmed || view.Meta.Opening.CustomText != choice.Opening || !strings.Contains(view.Meta.NarrativeRequirements, pack.Creation.CustomBackgroundRule) {
						t.Fatal("confirmation lost opening, flag or author constraints")
					}
					state := actorStateOf(t, view.Snapshot.State, interactive.DefaultActorID)
					for _, field := range identity.InitialState.AuthoredFields {
						ruleCreationEqual(t, "preview "+field.Name, field.Value, state[field.Name])
					}
					if len(identity.InitialState.AuthoredFields) != len(template.State) {
						t.Fatal("preview silently omitted authored facts")
					}
				})
			}
			beforeCalls := calls.Load()
			beforeProjects, err := application.projectRegistry.List(false)
			if err != nil {
				t.Fatal(err)
			}
			bad := map[string]func(*storypack.IdentityChoice){
				"age":           func(c *storypack.IdentityChoice) { c.Age = pack.Creation.AgeRange[1] + 1 },
				"origin":        func(c *storypack.IdentityChoice) { c.OriginID = "missing-origin" },
				"basis":         func(c *storypack.IdentityChoice) { c.StateBasisTemplateID = "missing-basis" },
				"location_id":   func(c *storypack.IdentityChoice) { c.LocationID = "missing-location" },
				"location_text": func(c *storypack.IdentityChoice) { c.LocationText = "未声明的秘密起点" },
				"discipline":    func(c *storypack.IdentityChoice) { c.AdventureMethodID = "invented-growth" },
				"affiliation":   func(c *storypack.IdentityChoice) { c.AffiliationID = "invented-membership" },
			}
			for _, id := range pack.Creation.StateBasisTemplates {
				if !containsStringForRuleCreation(pack.Creation.CustomStateBasisTemplates, id) {
					forbidden := id
					bad["disallowed_custom_basis"] = func(c *storypack.IdentityChoice) { c.StateBasisTemplateID = forbidden }
					break
				}
			}
			for name, mutate := range bad {
				t.Run("reject_"+name, func(t *testing.T) {
					choice := base
					mutate(&choice)
					_, err := application.StoryPacks().DraftStoryPackIdentity(context.Background(), packID, StoryPackIdentityDraftRequest{Choice: choice})
					var identityErr *storypack.IdentityError
					if !errors.As(err, &identityErr) {
						t.Fatalf("draft did not reject before model: %v", err)
					}
					_, err = application.StoryPacks().PrepareStartIdentity(context.Background(), packID, "", &choice)
					if !errors.As(err, &identityErr) {
						t.Fatalf("start did not reject before Project creation: %v", err)
					}
				})
			}
			afterProjects, err := application.projectRegistry.List(false)
			if err != nil || !reflect.DeepEqual(beforeProjects, afterProjects) || calls.Load() != beforeCalls {
				t.Fatal("rejected choices created a Project or invoked the model", err)
			}
		})
	}
}

func containsStringForRuleCreation(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
