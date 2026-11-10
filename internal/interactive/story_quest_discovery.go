package interactive

import (
	"strings"

	"ta/internal/quest"
)

// questPubliclyKnown is an authored public source, not an inference from a
// definition existing. Before the opening, even the starting location has not
// been experienced; reading a menu cannot establish that encounter.
func questPubliclyKnown(projection *storyBranchProjection, definition quest.Definition) bool {
	return projection != nil && questMembershipGateOpen(projection, definition) && questEchoGateOpen(projection, definition.RequiresEcho) && projection.LatestTurnID != "" &&
		len(definition.PublicLocationIDs) > 0 && storyLocationIsAny(projection.State, definition.PublicLocationIDs, projection.worldContent())
}

// questGiverPresent makes a personal card available to the narrator as a
// possible conversation, not to the player as already-known business. The
// offer record is the committed disclosure; no second knowledge store is used.
func questGiverPresent(projection *storyBranchProjection, definition quest.Definition) bool {
	if projection == nil || projection.Characters == nil || definition.Giver == "" || !questMembershipGateOpen(projection, definition) {
		return false
	}
	present := StorySceneFromState(projection.State, projection.LatestTurnID).Present
	for _, profile := range projection.Characters.Profiles {
		if profile.SourceKey != definition.Giver {
			continue
		}
		for _, id := range present {
			if strings.TrimSpace(id) == profile.ID {
				return true
			}
		}
	}
	return false
}

// questActionDiscovered is shared by the direct commit and the main turn.
// Recorded offers remain known after leaving their giver. A private candidate
// may first be offered in that person's scene, but cannot be accepted merely
// by supplying its ID before an actual offer has reached the player.
func questActionDiscovered(projection *storyBranchProjection, definition quest.Definition, action string) bool {
	// Leaving revokes new member-only commissions, not payment already earned.
	if (action == QuestActionOffer || action == QuestActionAccept) && !questMembershipGateOpen(projection, definition) {
		return false
	}
	gate := definition.RequiresEcho
	if projection != nil {
		if _, exists := projection.Quests[definition.ID]; exists {
			// The committed disclosure remains known after a content update adds
			// acknowledgement. Retain other authored invalidation conditions.
			if gate == nil {
				return true
			}
			knownGate := *gate
			knownGate.RequiresAcknowledged = false
			return questEchoGateOpen(projection, &knownGate)
		}
	}
	if !questEchoGateOpen(projection, gate) {
		return false
	}
	if questPubliclyKnown(projection, definition) {
		return true
	}
	return action == QuestActionOffer && questGiverPresent(projection, definition)
}

func questOption(definition quest.Definition) QuestOption {
	return QuestOption{Choices: quest.VisibleChoices(definition, nil), TaskID: definition.ID, TaskVersion: definition.Version,
		Kind: string(definition.Kind), Title: definition.Title, Brief: definition.Brief,
		Giver: definition.Giver, DeadlineSeconds: definition.DeadlineSeconds}
}
