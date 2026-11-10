package interactive

import "ta/internal/character"

// Facts produced by the discarded main turn belong to that version, not to an
// independent window interaction after it. Keep the latter suffix intact.
func independentVersionCharacterSuffix(suffix []StoryEventRecord, discardedTurnID string) []StoryEventRecord {
	out := make([]StoryEventRecord, 0, len(suffix))
	for _, record := range suffix {
		if record.Envelope.Type == StoryEventTypeCharacterRecord {
			if turnID, _ := record.Raw["turn_id"].(string); turnID == discardedTurnID {
				continue
			}
		}
		out = append(out, record)
	}
	return out
}

func (s *Store) selectedVersionCharactersLocked(storyID, branchID string, ancestors map[string]bool, projected []StoryEventRecord) (*character.State, error) {
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return nil, err
	}
	state, err := s.charactersForStoryAncestorsLocked(handle, ancestors, branchID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &character.State{Version: character.StateVersion}
	}
	branch := &storyBranchProjection{Characters: state}
	for _, record := range projected {
		if record.Envelope.Type != StoryEventTypeCharacterRecord {
			continue
		}
		var event CharacterRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return nil, err
		}
		if err := applyCharacterRecord(branch, event); err != nil {
			return nil, err
		}
	}
	return branch.Characters, nil
}

// A projected independent record retains its logical fact IDs, but its strict
// record-local sources must refer to the new envelope. The immutable original
// remains identified by version_projection.source_event_id.
func rebindProjectedCharacterRecord(raw map[string]any, projectedID string) error {
	var event CharacterRecordEvent
	if err := mapToStruct(raw, &event); err != nil {
		return err
	}
	raw["cursor"] = projectedID
	if len(event.FactionMemberships) > 0 {
		for i := range event.FactionMemberships {
			event.FactionMemberships[i].SourceEventID = projectedID
		}
		raw["faction_memberships"] = event.FactionMemberships
	}
	if event.FactionMatter != nil {
		event.FactionMatter.SourceEventID = projectedID
		raw["faction_matter"] = event.FactionMatter
	}
	return nil
}
