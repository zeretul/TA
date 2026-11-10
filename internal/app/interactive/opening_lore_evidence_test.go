package interactiveapp

import (
	"context"
	"encoding/json"
	"ta/config"
	agentcontext "ta/internal/agents/context"
	agentinteractive "ta/internal/agents/interactive"
	"ta/internal/agents/toolruntime"
	"ta/internal/agents/tools"
	"ta/internal/book/lore"
	"ta/internal/interactive"
	"testing"
)

func TestOpeningSchemaCreditsOnlyDeliveredLoreBodies(t *testing.T) {
	for _, mode := range []string{"read-id", "read-name", "list-full", "list-index", "resident"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			store := interactive.NewStore(root)
			defer store.Close()
			core := interactive.GeneratedStoryActorStateCore()
			story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "灯印岛", Origin: "你来到台前。", ActorState: &core, StateSchemaPolicy: &interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeAdaptTemplate}, PackBinding: &interactive.StoryPackBinding{PackID: "island", Scope: "island"}})
			if err != nil {
				t.Fatal(err)
			}
			loadMode := "manual"
			if mode == "resident" {
				loadMode = "resident"
			}
			for _, entry := range []lore.ItemInput{{ID: "own-rule", Scope: "island", Name: "灯印条件", Type: "rule", LoadMode: loadMode, Keywords: []string{"灯印"}, Content: "灯亮且自愿并有见证人时才成立。"}, {ID: "foreign-rule", Scope: "other", Name: "别处规则", Type: "rule", LoadMode: "resident", Content: "不得读到此项。"}} {
				if _, err := lore.NewStore(root).Create(entry); err != nil {
					t.Fatal(err)
				}
			}
			sc, err := store.StoryContext(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{Workspace: root}
			c := NewConversation(store, "", root, story.ID, "main", "开场", 600, cfg).WithOpeningStateSchema(sc)
			batch := interactive.ActorStateSchemaBatch{Finalize: true, Items: []interactive.ActorStateSchemaBatchItem{{ItemID: "review-rule", Requirements: []interactive.ActorStateSchemaRequirementReview{{Source: interactive.ActorStateSchemaRequirementSource{Kind: "lore", ID: "own-rule"}, Requirement: "当前事件记录现场的事实。", ValuePolicy: "schema_only", Decision: "covered", TemplateID: "story_context", FieldID: "当前事件", ExpectedType: "string"}}}}}
			blocked, err := c.SubmitOpeningStateSchemaBatch(context.Background(), batch)
			if err != nil || len(blocked.Blocked) != 1 {
				t.Fatalf("unread lore must be blocked: %+v %v", blocked, err)
			}
			if mode == "resident" {
				_, err = c.AssembleModelContext(context.Background(), "开场", agentcontext.ModelContextInput{UserMessage: "开场", Budget: c.ModelContextBudget()})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				projected := toolruntime.ProjectInteractiveContext(agentinteractive.InteractiveStoryToolContext{Store: store, StoryID: story.ID, BranchID: "main", LoreScope: "island", ObserveLoreRead: c.ObserveLoreRead})
				definitions, err := tools.NewCatalog(cfg, nil, tools.RuntimeExecutables{}).InteractiveStory(projected)(config.ResolvedAgentToolSettings{config.AgentToolLoreRead: true})
				if err != nil {
					t.Fatal(err)
				}
				toolName, input := "read_lore_items", `{"ids":["own-rule","foreign-rule"]}`
				if mode == "read-name" {
					input = `{"names":["灯印条件","别处规则"]}`
				}
				if mode == "list-full" || mode == "list-index" {
					toolName = "list_lore_items"
					input = `{"keywords":["灯印"],"detail":"full"}`
					if mode == "list-index" {
						input = `{"keywords":["灯印"],"detail":"index"}`
					}
				}
				ran := false
				for _, def := range definitions {
					info, err := def.Tool.Info(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if info.Name == toolName {
						if _, err := def.Tool.Run(context.Background(), input); err != nil {
							t.Fatal(err)
						}
						ran = true
					}
				}
				if !ran {
					t.Fatal("lore tool absent")
				}
			}
			c.mu.Lock()
			ids := append([]string(nil), c.openingStateSchemaAudit.ReviewedLoreIDs...)
			c.mu.Unlock()
			for _, id := range ids {
				if id == "foreign-rule" {
					t.Fatal("foreign lore credited")
				}
			}
			result, err := c.SubmitOpeningStateSchemaBatch(context.Background(), batch)
			if mode == "list-index" {
				if err != nil || result.Finalized || len(result.Blocked) != 1 {
					t.Fatalf("index credited as a body read: %+v %v", result, err)
				}
				return
			}
			if err != nil || !result.Finalized {
				b, _ := json.Marshal(result)
				t.Fatalf("successful read not credited: %s %v", b, err)
			}
		})
	}
}
