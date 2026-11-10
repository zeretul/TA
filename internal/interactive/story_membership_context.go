package interactive

import "ta/internal/character"

// MembershipAdmissionContact is narrator-only rule input. Being authorized to
// admit somebody neither implies willingness nor gives the protagonist knowledge
// of an unspoken role. It is never serialized into the player's world view.
type MembershipAdmissionContact struct {
	CharacterID string
	FactionID   string
	FactionName string
	Statuses    []string
}

func membershipContextFrom(p *storyBranchProjection, catalog *PackCatalogSnapshot) ([]character.FactionMembership, []MembershipAdmissionContact) {
	if p == nil || p.Characters == nil {
		return nil, nil
	}
	memberships := character.CurrentFactionMemberships(p.Characters.FactionMemberships)
	ctx := factionMembershipContext{State: *p.Characters, StateData: p.State, Combats: p.Combats, Scene: StorySceneFromState(p.State, p.LatestTurnID), Catalog: catalog}
	var contacts []MembershipAdmissionContact
	if catalog == nil {
		return memberships, nil
	}
	for _, id := range ctx.Scene.Present {
		profile, exists := p.Characters.Profile(id)
		if !exists {
			continue
		}
		for _, person := range catalog.Characters {
			if person.SourceKey != profile.SourceKey || len(person.MembershipAdmission) == 0 {
				continue
			}
			if _, err := membershipContact(ctx, profile.ID, person.FactionID); err != nil {
				continue
			}
			contacts = append(contacts, MembershipAdmissionContact{CharacterID: profile.ID, FactionID: person.FactionID, FactionName: person.FactionName, Statuses: append([]string(nil), person.MembershipAdmission...)})
			break
		}
	}
	return memberships, contacts
}

// MembershipContextFromView uses the same committed parent view as submission.
// The caller provides the matching snapshot state, including during regeneration.
func MembershipContextFromView(state map[string]any, view TurnWorldView, catalog *PackCatalogSnapshot) ([]character.FactionMembership, []MembershipAdmissionContact) {
	return membershipContextFrom(&storyBranchProjection{State: state, Characters: view.Characters, Combats: view.Combats, LatestTurnID: view.LatestTurnID}, catalog)
}
