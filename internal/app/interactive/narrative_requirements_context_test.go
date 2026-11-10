package interactiveapp

import (
	"context"
	"strings"
	"testing"

	agentcontext "ta/internal/agents/context"
	"ta/internal/interactive"
)

// The per-story narrative-requirements layer is retired. Writing direction now
// comes from the shared writing preferences plus the chosen style, so a story
// that still carries stored text from an older workspace must not receive a
// third direction fragment, and the text must not reach the model at all.
func TestStoredNarrativeRequirementsNeverReachTheModel(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:                 "旧故事仍存有叙事要求",
		StoryTellerID:         "classic",
		ReplyTargetChars:      800,
		NarrativeRequirements: "全篇克制，不写心理独白。",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "", "我推门进去", story.ReplyTargetChars, nil)
	assembled, err := conversation.AssembleModelContext(context.Background(), "我推门进去", agentcontext.ModelContextInput{
		UserMessage: "我推门进去",
		Budget:      conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, fragment := range assembled.Context.Fragments {
		if strings.Contains(fragment.Source, "narrative_requirements") {
			t.Fatalf("the retired requirements layer must not produce a fragment: %#v", fragment)
		}
	}
	for _, message := range assembled.Messages {
		if strings.Contains(message.Content, "全篇克制，不写心理独白。") {
			t.Fatal("stored narrative requirements are still visible to the model")
		}
	}
}

// A story that never had its own direction must stay identical: no fragment,
// and nothing in the messages either.
func TestStoryWithoutOwnRequirementsAddsNoFragment(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "使用工作区默认", StoryTellerID: "classic", ReplyTargetChars: 800,
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "", "继续", story.ReplyTargetChars, nil)
	assembled, err := conversation.AssembleModelContext(context.Background(), "继续", agentcontext.ModelContextInput{
		UserMessage: "继续",
		Budget:      conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range assembled.Context.Fragments {
		if strings.Contains(fragment.Source, "narrative_requirements") {
			t.Fatalf("a story without its own direction must not add a requirements fragment: %#v", fragment)
		}
	}
}
