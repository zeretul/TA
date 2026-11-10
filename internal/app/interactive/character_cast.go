package interactiveapp

import (
	"fmt"
	"sort"
	"strings"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// storyCharacterState reads this branch's character projection once for a whole
// context assembly.
//
// The cast block, the current-scene block and the lore working set all need the
// same projection: who exists, who is present, and which author identity a
// preset person carries. Reading it three times would let the three blocks
// disagree if a write landed between two reads, which is exactly the drift the
// cast block exists to prevent.
func (c *Conversation) storyCharacterState(storyCtx interactive.StoryContext) (character.State, bool) {
	if c == nil || c.store == nil {
		return character.State{}, false
	}
	state, err := c.store.CharacterState(c.storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		// A character read failure narrows what the turn can say about people
		// rather than inventing them: an unreadable registry is not evidence
		// that nobody exists.
		return character.State{}, false
	}
	return state.Normalize(), true
}

// characterCastContext is the narrator's view of the story's established
// characters: who exists, with which stable ID, how each one speaks, what is
// still unfinished with the protagonist, and what has already been said on
// every channel.
//
// It is deliberately not a character.Prepared context. The narrator is not one
// of the characters and is not limited to one person's knowledge; what it needs
// is the roster of real identities so a line in the prose is recorded against a
// character who exists rather than against a name it just invented.
//
// This is the read half of same-scene continuity. The character window writes
// exchanges, matters and knowledge into the branch's character projection; this
// block is what makes the next story turn see them, so an answer the player gave
// a guard in the window is already on the record when the story resumes.
func (c *Conversation) characterCastContext(storyCtx interactive.StoryContext, state character.State, ok bool, ages map[string]character.AgeFact) string {
	if !ok {
		return ""
	}
	scene := interactive.StoryScene(storyCtx.Snapshot)
	members := character.StoryCastAt(state, scene, scene.WorldTime, interactive.SnapshotClockSeconds(storyCtx.Snapshot))
	attention := interactive.PlayerAttentionFromState(storyCtx.Snapshot.State)
	preferred := make(map[string]bool, len(attention.Characters))
	for _, id := range attention.Characters {
		preferred[id] = true
	}
	// Sort only the existing eligible cast. Attention never adds a secret or
	// moves an absent person into the scene, and present speakers stay first.
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].Present != members[j].Present {
			return members[i].Present
		}
		return preferred[members[i].Profile.ID] && !preferred[members[j].Profile.ID]
	})
	for i := range members {
		if age, found := ages[members[i].Profile.ID]; found {
			members[i].Age = &age
		}
	}
	cast := character.CastMarkdown(members, state.Exchanges, character.DefaultMaxCastBytes) + interactive.PlayerAttentionContext(storyCtx.Snapshot.State)
	methods := eligibleSpiritBeastMethods(storyCtx.Meta.PackCatalog, interactive.StoryCultivationRules(storyCtx.Meta))
	if len(methods) == 0 {
		return cast
	}
	var eligibility strings.Builder
	eligibility.WriteString("\n## Eligible methods for a newly registered spirit_beast (source: this story's frozen world catalog)\n")
	eligibility.WriteString("Use one of these exact ids only when the first-appearance scene and state support it. Do not assign a method to an ordinary animal, and never borrow a catalog source_key for an invented creature.\n")
	eligibility.WriteString("First registration must carry both the character profile and its dedicated Actor. If the frozen catalog or the confirmed first-appearance scene establishes its cultivation stage, initialize 当前境界 with that exact registered stage key in the Actor's create state (for example qi_1 for refining layer one). Writing the stage only inside 基本身份 or prose leaves its numeric stage unknown and cannot qualify it for growth. This is initial registration, not an upgrade: never copy the protagonist's stage, invent initial progress or rewards, or rewrite an established actor's cultivation. If no stage is established, keep it unknown; do not grant growth.\n")
	for _, method := range methods {
		fmt.Fprintf(&eligibility, "- %s (%s; growth coefficient %.2f)\n", method.ID, method.Name, method.GrowthCoefficient)
	}
	return cast + eligibility.String()
}

func eligibleSpiritBeastMethods(catalog *interactive.PackCatalogSnapshot, profiles ...*cultivation.Rules) []cultivation.TechniqueDef {
	if catalog == nil {
		return nil
	}
	ids := make(map[string]struct{})
	for _, person := range catalog.Characters {
		if person.Lifeform == interactive.PackCatalogLifeformSpiritBeast && person.CultivationMethodID != "" {
			ids[person.CultivationMethodID] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	methods := make([]cultivation.TechniqueDef, 0, len(ordered))
	for _, id := range ordered {
		if method, ok := cultivation.TechniqueByID(id, profiles...); ok {
			methods = append(methods, method)
		}
	}
	return methods
}
