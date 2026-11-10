package app

import (
	"fmt"
	"strings"

	"ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/interactive"
)

// characterWorldTime is the in-fiction time an exchange is stamped with.
//
// It is read from the story's own scene state, so it advances only when a
// committed turn advances it: a player who reads slowly, waits for the model, or
// closes the program consumes no world time at all. When a story authors no
// clock, the committed turn ordinal stands in for one, which is monotonic for
// the same reason.
func characterWorldTime(storyContext interactive.StoryContext) string {
	if worldTime := interactive.StoryScene(storyContext.Snapshot).WorldTime; worldTime != "" {
		return worldTime
	}
	return fmt.Sprintf("Turn %d", storyContext.Snapshot.TurnCount)
}

// characterSceneFor projects the committed scene for a private recipient.
// Reachability is not physical presence. A remote recipient keeps the journal
// instant, but receives no local place, event label or witness list.
func characterSceneFor(storyContext interactive.StoryContext, characterID string) character.Scene {
	scene := interactive.StoryScene(storyContext.Snapshot)
	if scene.WorldTime == "" {
		scene.WorldTime = characterWorldTime(storyContext)
	}
	if scene.NodeID == "" {
		scene.NodeID = strings.TrimSpace(storyContext.Snapshot.BranchID)
	}
	if !character.ContainsID(scene.Present, characterID) {
		scene.Location = ""
		scene.Title = ""
		scene.Present = nil
	}
	return scene
}

// characterIdentityFor binds a prepared context to the pack, world, branch,
// profile revision and commit position it was built from. It is the cache
// identity, so a context built for one revision is a miss rather than a wrong
// answer after the character is edited or the branch moves.
func characterIdentityFor(storyContext interactive.StoryContext, branchID string, profile character.Profile, loreRevision string) character.IdentityRevision {
	revision := character.IdentityRevision{
		BranchID:        branchID,
		ProfileRevision: profile.Revision,
		LoreRevision:    loreRevision,
		FormalCommitID:  interactive.StoryScene(storyContext.Snapshot).NodeID,
	}
	if binding := storyContext.Meta.PackBinding; binding != nil {
		revision.PackID = binding.PackID
		// The binding's scope is the world resource identity: it is what every
		// lore read for this story is filtered by, so it is also what keeps a
		// same-named character in another world out of this context.
		revision.WorldID = binding.Scope
	}
	return revision
}

// characterLoreFor resolves the common knowledge this character could plausibly
// hold. It reads through the story's pack scope, so another world's resident
// lore cannot appear, and it keeps only entries the character's own profile or
// the current scene actually touches.
//
// The point is grounding, not completeness: handing a character the whole world
// book would let them answer from knowledge they never had a way to acquire.
func characterLoreFor(workspace, scope string, profile character.Profile, scene character.Scene, limit int, contexts ...interactive.StoryContext) []string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil
	}
	visible := lore.IsPublic
	if len(contexts) > 0 {
		storyVisible := interactive.StoryLoreVisibility(contexts[0].Meta, contexts[0].Snapshot.State)
		visible = func(item lore.Item) bool { return lore.IsPublic(item) && storyVisible(item) }
	}
	reader := lore.NewScopedStore(workspace, scope).WithVisibility(visible)
	items, err := reader.List()
	if err != nil {
		// A lore read failure narrows what the character may claim rather than
		// inventing it: an unread world book is not evidence that nothing exists.
		return nil
	}
	grounding := strings.ToLower(strings.Join([]string{
		profile.Name, profile.Description, profile.Role, profile.SpeechStyle,
		strings.Join(profile.Personality, " "), strings.Join(profile.Motives, " "),
		scene.Location, scene.Title,
	}, " "))
	if limit <= 0 {
		limit = character.DefaultMaxLoreEntries
	}
	out := make([]string, 0, limit)
	for _, item := range items {
		if len(out) >= limit {
			break
		}
		// Resident lore is already delivered as the story's stable context;
		// repeating it here would spend the character's budget twice.
		if item.LoadMode == lore.LoadModeResident {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" || !strings.Contains(grounding, strings.ToLower(name)) {
			continue
		}
		body := strings.TrimSpace(item.BriefDescription)
		if content := strings.TrimSpace(item.Content); content != "" {
			if body != "" {
				body += " "
			}
			body += content
		}
		out = append(out, fmt.Sprintf("%s (%s): %s", name, strings.TrimSpace(item.Type), oneLineText(body)))
	}
	return out
}

// characterStoryContext contains the recipient's projected scene and stance.
// Raw narration may contain private or unwitnessed events even when the person
// is present now. Their knowledge and exchanges are prepared separately.
func characterStoryContext(storyContext interactive.StoryContext, scene character.Scene, profile character.Profile) string {
	var sb strings.Builder
	if scene.Location != "" {
		fmt.Fprintf(&sb, "- Current location: %s\n", scene.Location)
	}
	if scene.Title != "" {
		fmt.Fprintf(&sb, "- Current scene: %s\n", scene.Title)
	}
	if scene.WorldTime != "" {
		fmt.Fprintf(&sb, "- Current time: %s\n", scene.WorldTime)
	}
	if len(scene.Present) > 0 {
		fmt.Fprintf(&sb, "- Present: %s\n", strings.Join(scene.Present, ", "))
	}
	if profile.Attitude != "" {
		fmt.Fprintf(&sb, "- Your current stance toward the player: %s\n", profile.Attitude)
	}
	return strings.TrimSpace(sb.String())
}

// latestTurnNarrative is a bounded tail of the current turn's prose. The window
// shows the character the situation the player is actually looking at, so an
// answer given in the window belongs to the scene on screen.
func latestTurnNarrative(snapshot interactive.Snapshot) string {
	if snapshot.CurrentTurn == nil {
		return ""
	}
	narrative := strings.Join(strings.Fields(strings.TrimSpace(snapshot.CurrentTurn.Narrative)), " ")
	if narrative == "" {
		return ""
	}
	const tailRunes = 1200
	runes := []rune(narrative)
	if len(runes) > tailRunes {
		narrative = string(runes[len(runes)-tailRunes:])
	}
	return narrative
}

func oneLineText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
