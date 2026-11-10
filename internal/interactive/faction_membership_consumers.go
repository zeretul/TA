package interactive

import (
	"ta/internal/character"
	"ta/internal/quest"
)

func questMembershipGateOpen(projection *storyBranchProjection, definition quest.Definition) bool {
	if definition.RequiresFactionMembership == "" {
		return true
	}
	if projection == nil || projection.Characters == nil {
		return false
	}
	for _, membership := range character.CurrentFactionMemberships(projection.Characters.FactionMemberships) {
		if membership.FactionID == definition.RequiresFactionMembership {
			return membership.Status == "formal"
		}
	}
	return false
}
