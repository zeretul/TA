package interactiveapp

import (
	"context"
	"strings"
	agentcontext "ta/internal/agents/context"
	"ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

// Uses the real main-GM assembler and all builtin data, not a reconstructed prompt.
func TestBuiltinRuleContractReachesMainGMModelContext(t *testing.T) {
	for _, variant := range []string{"doupo-continent", "harry-potter-1991", "harry-potter-1991-student", "lord-of-mysteries", "zichuan-xichuan", "sanguo-190"} {
		t.Run(variant, func(t *testing.T) {
			pack, err := storypack.NewLibrary(t.TempDir()).GetContent(variant + "-ta07-full-gm-candidate")
			if err != nil {
				t.Fatal(err)
			}
			var contract string
			for _, entry := range pack.World {
				if entry.ID == "adapter-"+variant+"-gm-runtime-contract" {
					if entry.Audience != "gm" || entry.LoadMode != "resident" {
						t.Fatal("contract is not GM resident")
					}
					contract = entry.Content
				}
			}
			if contract == "" {
				t.Fatal("missing contract")
			}
			root := t.TempDir()
			book := lore.NewStore(root)
			var ops []lore.Operation
			for _, input := range pack.WorldInputs() {
				ops = append(ops, lore.Operation{Op: "create", Item: input})
			}
			if _, err = book.ApplyOperations("Seed full builtin contract fixture", ops); err != nil {
				t.Fatal(err)
			}
			store := interactive.NewStore(root)
			story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "GM contract context", StoryTellerID: "classic", ReplyTargetChars: 800, PackBinding: pack.Binding(), PackCatalog: catalogSnapshot(pack.FreezeCatalog()), NarrativeRequirements: "RETIRED-CONTRACT-MUST-NOT-BE-INJECTED"})
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			store = interactive.NewStore(root)
			defer store.Close()
			conversation := NewConversation(store, t.TempDir(), root, story.ID, "", "继续", 800, nil)
			assembled, err := conversation.AssembleModelContext(context.Background(), "继续", agentcontext.ModelContextInput{UserMessage: "继续", Budget: conversation.ModelContextBudget()})
			if err != nil {
				t.Fatal(err)
			}
			fragmentFound, messageFound := false, false
			for _, fragment := range assembled.Context.Fragments {
				if fragment.Source == "interactive.resident_lore" && strings.Contains(fragment.Content, contract) {
					fragmentFound = true
				}
			}
			for _, message := range assembled.Messages {
				if strings.Contains(message.Content, contract) {
					messageFound = true
				}
				if strings.Contains(message.Content, "RETIRED-CONTRACT-MUST-NOT-BE-INJECTED") {
					t.Fatal("retired requirements resurrected")
				}
			}
			if !fragmentFound || !messageFound {
				t.Fatal("full contract missing from actual GM model messages")
			}
			public, err := lore.NewScopedStore(root, pack.WorldScope()).WithVisibility(lore.IsPublic).ResidentContextMarkdown()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(public, contract) {
				t.Fatal("GM contract leaked into public resident view")
			}
		})
	}
}
