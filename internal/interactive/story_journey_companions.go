package interactive

import (
	"fmt"
	"slices"
	"strings"

	"ta/internal/character"
	"ta/internal/exploration"
)

// JourneyCompanionInput records a decision to take one route, not membership in
// a permanent party. The cited response may be a creature's observed gesture.
// The main-turn journal retains both the declaration and its actual exchange.
type JourneyCompanionInput struct {
	CharacterID         string `json:"character_id" jsonschema_description:"出发前已真实同场、未归档的角色或生灵ID。"`
	SourceExchangeIndex int    `json:"source_exchange_index" jsonschema_description:"本回合character_facts.exchanges中本人明确决定同行的现场回应，1起始；不接受主角替它发令、旧承诺或远程消息。"`
}

func validateJourneyCompanions(before, proposed map[string]any, routeID string, companions []JourneyCompanionInput, facts *TurnCharacterFactsInput, known []string) ([]JourneyCompanionInput, error) {
	if len(companions) == 0 {
		return nil, nil
	}
	if facts == nil || len(companions) > len(facts.Exchanges) {
		return nil, fmt.Errorf("journey companions need their own scene responses")
	}
	seen := map[string]bool{}
	out := make([]JourneyCompanionInput, 0, len(companions))
	for _, source := range companions {
		id := normalizeStatePanelActorID(source.CharacterID)
		if id == "" || id == DefaultActorID || id == DefaultStoryContextActorID || seen[id] || !journeyCompanionPresent(proposed, id) {
			return nil, fmt.Errorf("journey companion must already be an active participant in the departure scene")
		}
		if !journeyCompanionPresent(before, id) {
			// A real first encounter may answer and set out in this turn. An
			// already known remote or archived actor may not be moved here by
			// rewriting the proposed presence list.
			introduced := false
			if !actorRecordExists(before, id) && !actorIsArchived(before, id) && !slices.Contains(known, id) {
				for _, profile := range facts.Profiles {
					if normalizeStatePanelActorID(profile.CharacterID) == id {
						introduced = true
					}
				}
			}
			if !introduced {
				return nil, fmt.Errorf("journey companion was not at the departure scene")
			}
		}
		if source.SourceExchangeIndex < 1 || source.SourceExchangeIndex > len(facts.Exchanges) {
			return nil, fmt.Errorf("journey companion response index is invalid")
		}
		x := facts.Exchanges[source.SourceExchangeIndex-1]
		if normalizeStatePanelActorID(x.CharacterID) != id || x.Speaker != string(character.SpeakerCharacter) || x.Channel != string(character.ChannelScene) || normalizeStatePanelActorID(x.AddressedTo) != DefaultActorID || strings.TrimSpace(x.Text) == "" {
			return nil, fmt.Errorf("journey companion requires its own public response to the protagonist")
		}
		if !journeyParticipationAccepted(x.ParticipationDecision, routeID) {
			return nil, fmt.Errorf("journey companion did not accept this route")
		}
		for _, later := range facts.Exchanges[source.SourceExchangeIndex:] {
			if normalizeStatePanelActorID(later.CharacterID) == id && later.Speaker == string(character.SpeakerCharacter) && later.Channel == string(character.ChannelScene) && normalizeStatePanelActorID(later.AddressedTo) == DefaultActorID && later.ParticipationDecision != nil && later.ParticipationDecision.Kind == character.ParticipationJourney && later.ParticipationDecision.TargetID == routeID {
				return nil, fmt.Errorf("journey companion declaration must use its latest decision")
			}
		}
		seen[id] = true
		source.CharacterID = id
		out = append(out, source)
	}
	return out, nil
}

func journeyCompanionPresent(state map[string]any, id string) bool {
	if !actorRecordExists(state, id) || actorIsArchived(state, id) || !npcGrowthSharesScene(state, id) {
		return false
	}
	// An explicit defeated body cannot travel merely because its name remains
	// in the scene list. No missing health field is interpreted as zero.
	if value := actorStateFieldValue(state, id, xiuxianFieldHealth); value != nil {
		if n, ok := actorStateNumber(value); ok && n <= 0 {
			return false
		}
	}
	if getPath(state, actorStateFieldPath(id, AdventureStateField)) != nil {
		a, err := AdventureActorFromState(state, id)
		if err != nil || a.Health <= 0 {
			return false
		}
	}
	return true
}

func continuingJourneyTravelers(state map[string]any, prior []string) []string {
	out := []string{DefaultActorID}
	for _, id := range prior {
		if id != DefaultActorID && journeyCompanionPresent(state, id) {
			out = append(out, id)
		}
	}
	return out
}

func journeyPresenceOps(state map[string]any, system StoryDirectorActorStateSystem, trip exploration.JourneySnapshot, position exploration.JourneyPosition) []ActorStateOp {
	present := make([]any, 0, len(trip.TravelerIDs))
	for _, id := range trip.TravelerIDs {
		present = append(present, id)
	}
	ops := []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: present, SourceKind: StateOpSourceJourney, SourceID: trip.JourneyID, Reason: "行路只保留本次实际同行者，不携带原地旁观者。"}}
	// This same presence list drives chat, encounters and NPC environments.
	// Update an existing human-readable actor location too, without inventing a
	// second formal position field for templates which do not have one.
	place := worldPlaceLabel(system.WorldContent, position.LocationID)
	if position.LocationID == "" {
		place = "途中：" + worldPlaceLabel(system.WorldContent, trip.OriginLocationID) + " → " + worldPlaceLabel(system.WorldContent, trip.DestinationLocationID)
	}
	for _, id := range trip.TravelerIDs {
		if id == DefaultActorID {
			continue
		}
		templateID, ok := actorTemplateIDFromStateOrSystem(state, system, id)
		if !ok {
			continue
		}
		field, ok := actorStateFieldByID(actorStateTemplateByID(system, templateID), "出场与位置")
		if !ok {
			continue
		}
		ops = append(ops, ActorStateOp{Op: "set", ActorID: id, FieldID: actorStateFieldID(field), Value: place, SourceKind: StateOpSourceJourney, SourceID: trip.JourneyID, Reason: "本人实际同行的已结算位置。"})
	}
	return ops
}

func validateJourneyTravelersRecord(branch *storyBranchProjection, event JourneyRecordEvent, prior exploration.JourneySnapshot, continuing bool) error {
	ids := event.Journey.TravelerIDs
	// Older journals did not record a roster. Their existing state operations
	// remain authoritative; this does not retrofit companions into old trips.
	if len(ids) == 0 && len(event.CompanionSources) == 0 && len(prior.TravelerIDs) == 0 {
		return nil
	}
	if len(ids) == 0 || ids[0] != DefaultActorID {
		return fmt.Errorf("journey traveler roster is missing the protagonist")
	}
	seen := map[string]bool{DefaultActorID: true}
	for _, id := range ids[1:] {
		if id == "" || normalizeStatePanelActorID(id) != id || seen[id] || branch.State != nil && !journeyCompanionPresent(branch.State, id) {
			return fmt.Errorf("journey traveler is not a unique active participant")
		}
		seen[id] = true
		if continuing && !slices.Contains(prior.TravelerIDs, id) {
			return fmt.Errorf("journey continuation added a companion")
		}
	}
	if continuing {
		if len(event.CompanionSources) != 0 {
			return fmt.Errorf("journey continuation replaced its participation source")
		}
		return nil
	}
	if len(event.CompanionSources) != len(ids)-1 {
		return fmt.Errorf("journey companion sources do not match its roster")
	}
	if len(event.CompanionSources) == 0 {
		return nil
	}
	if event.TurnID == "" {
		return fmt.Errorf("journey companion source turn is missing")
	}
	// Clock-only checkpoint rebuilding has no actor or dialogue projection. The
	// canonical replay validates these sources; the clock path still checks the
	// frozen roster and refuses adding members on a continuation.
	if branch.State == nil {
		return nil
	}
	if branch.Characters == nil {
		return fmt.Errorf("journey companion source turn is missing")
	}
	var exchanges []character.Exchange
	for _, x := range branch.Characters.Exchanges {
		if x.TurnID == event.TurnID {
			exchanges = append(exchanges, x)
		}
	}
	for i, source := range event.CompanionSources {
		if source.CharacterID != ids[i+1] || source.SourceExchangeIndex < 1 || source.SourceExchangeIndex > len(exchanges) {
			return fmt.Errorf("journey companion source is invalid")
		}
		x := exchanges[source.SourceExchangeIndex-1]
		if !journeyParticipationAccepted(x.ParticipationDecision, event.RouteID) {
			return fmt.Errorf("journey companion source did not accept this route")
		}
		if x.CharacterID != source.CharacterID || x.Speaker != character.SpeakerCharacter || x.Channel != character.ChannelScene || x.AddressedTo != DefaultActorID || strings.TrimSpace(x.Text) == "" {
			return fmt.Errorf("journey companion source is not its own public response")
		}
		for _, later := range exchanges[source.SourceExchangeIndex:] {
			if later.CharacterID == source.CharacterID && later.Speaker == character.SpeakerCharacter && later.Channel == character.ChannelScene && later.AddressedTo == DefaultActorID && later.ParticipationDecision != nil && later.ParticipationDecision.Kind == character.ParticipationJourney && later.ParticipationDecision.TargetID == event.RouteID {
				return fmt.Errorf("journey companion source is not its latest decision")
			}
		}
	}
	return nil
}

func journeyParticipationAccepted(d *character.ParticipationDecision, routeID string) bool {
	return d != nil && d.Kind == character.ParticipationJourney && d.TargetID == routeID && d.Decision == character.ParticipationAccept
}
