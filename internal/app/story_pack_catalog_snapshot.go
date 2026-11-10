package app

import (
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// catalogSnapshot converts a pack's frozen catalog into the stored form a story
// keeps.
//
// It lives in this package rather than in the story package because of the
// dependency direction: the catalog package already builds story requests and so
// imports the story package, which means the story package cannot import it back.
// This is the one layer that may see both.
//
// Every field is named rather than copied reflectively. A stored format and a
// query type are different things: the query type may gain a field the format
// does not have yet, and a mechanical copy would then silently write or read a
// field the format never promised.
func catalogSnapshot(frozen *storypack.Frozen) *interactive.PackCatalogSnapshot {
	if frozen == nil {
		return nil
	}
	snapshot := &interactive.PackCatalogSnapshot{
		ContentVersion: frozen.ContentVersion,
		Characters:     make([]interactive.PackCatalogPerson, 0, len(frozen.People)),
		TopicTerms:     make(map[string][]string, len(frozen.TopicTerms)),
		Places:         make([]interactive.PackCatalogPlace, 0, len(frozen.Places)),
	}
	for _, person := range frozen.People {
		snapshot.Characters = append(snapshot.Characters, interactive.PackCatalogPerson{
			SourceKey:           person.SourceKey,
			Name:                person.Name,
			Aliases:             append([]string(nil), person.Aliases...),
			Country:             person.Country,
			FactionID:           person.FactionID,
			FactionName:         person.FactionName,
			MembershipAdmission: append([]string(nil), person.MembershipAdmission...),
			Role:                person.Role,
			Topics:              append([]string(nil), person.Topics...),
			PlaceCandidate:      person.PlaceCandidate,
			PlaceLabel:          person.PlaceLabel,
			RegionTokens:        append([]string(nil), person.RegionTokens...),
			Level:               person.Level,
			Brief:               person.Brief,
			LoreID:              person.LoreID,
			InitialStage:        person.InitialStage,
			ContactGate:         person.ContactGate,
			RecordOnly:          person.RecordOnly,
			Age:                 person.Age,
			Gender:              person.Gender,
			// The species classification and its registered method travel with
			// the snapshot, because a story's growth rules are decided from the
			// frozen catalog. Dropping them here left every spirit beast in a
			// running story unqualified: the catalog still said 灵兽 in the world
			// book, but the stored row no longer carried the one fact that makes
			// it eligible to cultivate.
			Lifeform:            person.Lifeform,
			CultivationMethodID: person.CultivationMethodID,
		})
	}
	for topic, values := range frozen.TopicTerms {
		snapshot.TopicTerms[topic] = append([]string(nil), values...)
	}
	for _, place := range frozen.Places {
		snapshot.Places = append(snapshot.Places, interactive.PackCatalogPlace{
			Token:   place.Token,
			Country: place.Country,
			Place:   place.Place,
		})
	}
	return snapshot
}
