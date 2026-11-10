package interactiveapp

import (
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This file converts between a story's stored catalog snapshot and the package
// that knows how to query a catalog.
//
// The conversion lives here because it is the only place that may see both: the
// stored format belongs to the story package, the query logic belongs to the
// catalog package, and the catalog package already builds story requests — so it
// imports the story package and cannot be imported back by it.
//
// The conversion names every field rather than copying reflectively. A stored
// format and a query type are different things: the query type may gain a field
// the format does not have yet, and a mechanical copy would then silently write
// or read a field the format never promised.

// frozenCatalog converts a story's stored snapshot into the query form. It
// returns nil when the story has no snapshot, which the caller must read as
// "this story predates freezing", not as "this world has no people".
func frozenCatalog(snapshot *interactive.PackCatalogSnapshot) *storypack.Frozen {
	if snapshot == nil {
		return nil
	}
	people := make([]storypack.FrozenPerson, 0, len(snapshot.Characters))
	for _, person := range snapshot.Characters {
		people = append(people, storypack.FrozenPerson{
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
			Lifeform:            person.Lifeform,
			CultivationMethodID: person.CultivationMethodID,
		})
	}
	terms := make(map[string][]string, len(snapshot.TopicTerms))
	for topic, values := range snapshot.TopicTerms {
		terms[topic] = append([]string(nil), values...)
	}
	places := make([]storypack.FrozenPlace, 0, len(snapshot.Places))
	for _, place := range snapshot.Places {
		places = append(places, storypack.FrozenPlace{
			Token:   place.Token,
			Country: place.Country,
			Place:   place.Place,
		})
	}
	return storypack.NewFrozen(snapshot.ContentVersion, people, terms, places)
}

// catalogSnapshot converts a pack's frozen catalog into the stored form, which is
// what story creation writes.
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
