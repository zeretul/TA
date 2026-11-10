// This file makes a stored catalog snapshot answer the same questions a live
// pack does.
//
// The reason it exists: the world book a story reads is already scoped to the
// pack revision that wrote it, so editing a pack never changes an in-progress
// story's lore. The catalog had no such copy. It was re-read from the pack file
// every turn and, when the file's fingerprint had moved on, withheld entirely —
// so an old story silently lost every preset person the moment its pack was
// edited. A snapshot written at story start and read back through this type is
// what keeps that from happening.
//
// What the snapshot must not become: a second world state. It stores locating
// records and resolved placement facts only. Profile bodies stay in the
// revision-scoped world book, which is still the one readable source of what a
// person is, and nothing here is writable after the story starts.
package storypack

import (
	"sort"
	"strings"
)

// FrozenPerson is one catalog row as a story stored it. It mirrors
// CatalogCharacter field for field so the two can be converted without loss;
// the stored form is a separate type in package interactive because that is the
// format, and this one is the query form.
type FrozenPerson struct {
	SourceKey           string
	Name                string
	Aliases             []string
	Country             string
	FactionName         string   `json:"faction_name,omitempty"`
	MembershipAdmission []string `json:"membership_admission,omitempty"`
	FactionID           string
	Role                string
	Topics              []string
	PlaceCandidate      string
	PlaceLabel          string
	RegionTokens        []string
	Level               string
	Brief               string
	LoreID              string
	InitialStage        string
	ContactGate         string
	RecordOnly          bool
	// Age is the age the authored body stated, copied so a story keeps the age
	// it started on even if the pack is edited afterwards.
	Age int
	// Gender is what the author decided, copied for the same reason: a story
	// keeps the person it started with.
	Gender              string
	SpeechStyle         string
	Lifeform            string
	CultivationMethodID string
}

// FrozenPlace is one resolved placement fact: a region token, the country it
// belongs to, and the authored place it names there.
//
// The placement index is stored rather than rebuilt because rebuilding it reads
// the world book, and the world book belongs to a different revision history.
// Freezing the resolved answer keeps an old story placing people exactly where
// it did, without shipping a copy of the world.
type FrozenPlace struct {
	Token   string `json:"token"`
	Country string `json:"country"`
	Place   string `json:"place"`
}

// Frozen is a story's catalog as of the revision it started on. It answers the
// questions the discovery path asks of a pack, and nothing else: it is not a
// Pack, cannot be written to, and has no world book of its own.
type Frozen struct {
	// ContentVersion is the author's revision number as of the snapshot. It is
	// kept for display and for diagnosing a report; it is never used to decide
	// what the story may read.
	ContentVersion int
	// ContentHash is the fingerprint of the pack revision the snapshot was taken
	// from, so a report can say which revision an old story is actually on.
	ContentHash string
	People      []FrozenPerson
	TopicTerms  map[string][]string
	Places      []FrozenPlace
}

// NewFrozen builds the query form from a story's stored snapshot parts. The
// caller owns the stored format; this package owns what a catalog means, and
// this is the seam between them.
//
// Rows are normalized the same way the live loader normalizes them, so a name
// that resolved before the pack was edited still resolves now.
func NewFrozen(contentVersion int, people []FrozenPerson, topicTerms map[string][]string, places []FrozenPlace) *Frozen {
	return newFrozenFromParts(contentVersion, "", people, topicTerms, places)
}

// newFrozenFromParts builds the query form from already-normalized parts.
func newFrozenFromParts(contentVersion int, contentHash string, people []FrozenPerson, topicTerms map[string][]string, places []FrozenPlace) *Frozen {
	frozen := &Frozen{
		ContentVersion: contentVersion,
		ContentHash:    strings.TrimSpace(contentHash),
		People:         make([]FrozenPerson, 0, len(people)),
		TopicTerms:     make(map[string][]string, len(topicTerms)),
		Places:         make([]FrozenPlace, 0, len(places)),
	}
	for _, person := range people {
		character := person.character().Normalize()
		if character.SourceKey == "" || character.Name == "" {
			continue
		}
		frozen.People = append(frozen.People, personFromCatalog(character))
	}
	sort.SliceStable(frozen.People, func(i, j int) bool {
		return frozen.People[i].SourceKey < frozen.People[j].SourceKey
	})
	for topic, values := range topicTerms {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		frozen.TopicTerms[topic] = normalizeTokenList(values)
	}
	for _, place := range places {
		token := strings.ToLower(strings.TrimSpace(place.Token))
		country := strings.ToLower(strings.TrimSpace(place.Country))
		if token == "" || country == "" {
			continue
		}
		frozen.Places = append(frozen.Places, FrozenPlace{
			Token:   token,
			Country: country,
			Place:   place.Place,
		})
	}
	sort.SliceStable(frozen.Places, func(i, j int) bool {
		if frozen.Places[i].Token != frozen.Places[j].Token {
			return frozen.Places[i].Token < frozen.Places[j].Token
		}
		if frozen.Places[i].Country != frozen.Places[j].Country {
			return frozen.Places[i].Country < frozen.Places[j].Country
		}
		return frozen.Places[i].Place < frozen.Places[j].Place
	})
	return frozen
}

// FreezeCatalog takes the catalog snapshot a story keeps for its whole life.
//
// It is called once, when the story starts, from the pack revision that story
// binds to. Everything the discovery path needs to locate a person is resolved
// here — including the placement index, which is the only part that reads the
// world book — so the story never has to read a later revision to find its own
// people.
//
// Topic terms and placement are copied rather than referenced: the caller keeps
// the pack and may edit it, and a snapshot that changed under an old story would
// defeat the point of taking one.
func (p Pack) FreezeCatalog() *Frozen {
	people := make([]FrozenPerson, 0, len(p.Characters))
	for _, character := range p.normalizedCatalog() {
		// The catalog row already carries the age read from this revision's
		// world book, so an old story keeps the age its world stated rather
		// than one a later pack edit introduced.
		people = append(people, personFromCatalog(character))
	}
	terms := p.normalizedTopicTerms()
	index := p.newRegionIndex()
	places := make([]FrozenPlace, 0, len(index.places))
	for token, named := range index.places {
		for place := range named {
			country, _, ok := strings.Cut(place, "\x00")
			if !ok {
				continue
			}
			places = append(places, FrozenPlace{
				Token:   token,
				Country: country,
				Place:   place,
			})
		}
	}
	return newFrozenFromParts(p.ContentVersion, p.ContentHash(), people, terms, places)
}

// CatalogCharacters returns the frozen rows as catalog characters, in the
// stable order the live catalog uses.
func (f *Frozen) CatalogCharacters() []CatalogCharacter {
	if f == nil {
		return nil
	}
	out := make([]CatalogCharacter, 0, len(f.People))
	for _, person := range f.People {
		out = append(out, person.character())
	}
	return out
}

// CatalogBySourceKey indexes the frozen catalog by the author's stable key.
func (f *Frozen) CatalogBySourceKey() map[string]CatalogCharacter {
	if f == nil {
		return nil
	}
	out := make(map[string]CatalogCharacter, len(f.People))
	for _, person := range f.People {
		character := person.character()
		out[character.SourceKey] = character
	}
	return out
}

// CatalogByName indexes the frozen catalog by every name the author verified for
// a person, dropping a name that identifies more than one. The ambiguity rule is
// the live catalog's, and it matters here for the same reason: two people who
// share a name are different people, and picking one of them is how a story
// merges them.
func (f *Frozen) CatalogByName() map[string]string {
	if f == nil {
		return nil
	}
	byName := make(map[string]string, len(f.People)*2)
	ambiguous := make(map[string]bool, 0)
	claim := func(name, key string) {
		name = normalizeNameKey(name)
		if name == "" || key == "" || ambiguous[name] {
			return
		}
		if existing, ok := byName[name]; ok && existing != key {
			delete(byName, name)
			ambiguous[name] = true
			return
		}
		byName[name] = key
	}
	for _, person := range f.People {
		character := person.character()
		claim(character.Name, character.SourceKey)
		for _, alias := range character.Aliases {
			claim(alias, character.SourceKey)
		}
	}
	return byName
}

// MentionedNames returns the frozen names and aliases that appear verbatim in a
// piece of player text, which is how an explicit request by name is recognized.
func (f *Frozen) MentionedNames(text string) []string {
	if f == nil {
		return nil
	}
	text = normalizeNameKey(text)
	if text == "" {
		return nil
	}
	var out []string
	for _, person := range f.People {
		character := person.character()
		for _, name := range character.MatchNames() {
			if key := normalizeNameKey(name); key != "" && strings.Contains(text, key) {
				out = append(out, character.Name)
				break
			}
		}
	}
	return out
}

// LocationNamesAPlace reports whether a scene's location text names somewhere
// this frozen catalog can place people. It is what lets a caller tell "nobody
// here fits" from "this scene has no location", which are different answers and
// must not be reported the same way.
func (f *Frozen) LocationNamesAPlace(location string) bool {
	if f == nil {
		return false
	}
	return len(f.regionIndex().countriesAt(
		strings.ToLower(strings.TrimSpace(location)))) > 0
}

// CharacterCandidates selects the people worth offering this turn from the
// frozen catalog, using the same relevance ladder as the live pack. Selection is
// by fit and capacity is applied after ranking, so a low-ranked person is
// dropped for being irrelevant rather than for being late in the alphabet.
func (f *Frozen) CharacterCandidates(req CandidateRequest) []Candidate {
	if f == nil || len(f.People) == 0 {
		return nil
	}
	catalog := f.CatalogCharacters()
	location := strings.ToLower(strings.TrimSpace(req.Location))
	action := strings.ToLower(strings.TrimSpace(req.Action))
	named := make(map[string]bool, len(req.Named))
	for _, name := range req.Named {
		if key := normalizeNameKey(name); key != "" {
			named[key] = true
		}
	}

	index := f.regionIndex()
	sceneCountries := index.countriesAt(location)
	terms := f.TopicTerms

	candidates := make([]Candidate, 0, len(catalog))
	for _, character := range catalog {
		if req.Established[character.SourceKey] {
			continue
		}
		candidate := Candidate{Character: character}
		for _, name := range character.MatchNames() {
			if named[normalizeNameKey(name)] {
				candidate.Named = true
				candidate.Reasons = append(candidate.Reasons, "点名寻找")
				break
			}
		}
		if matched := character.matchedTopics(action, terms); len(matched) > 0 {
			candidate.Matched = true
			candidate.Reasons = append(candidate.Reasons, "与本轮事项相关："+strings.Join(matched, "、"))
		}
		locationMatch := index.placesScene(character, location, sceneCountries)
		if locationMatch {
			candidate.Reasons = append(candidate.Reasons, "位于当前所在区域")
		}
		switch {
		case locationMatch:
			candidate.Reach = ReachLocal
		case character.Country != "" && sceneCountries[strings.ToLower(character.Country)]:
			candidate.Reach = ReachRegion
			candidate.Reasons = append(candidate.Reasons, "同国异地，需行路或引荐")
		default:
			candidate.Reach = ReachRemote
			candidate.Reasons = append(candidate.Reasons, "异地，只能作为线索提及")
		}
		if !candidate.Named && len(candidate.Reasons) == 1 && candidate.Reach != ReachLocal {
			continue
		}
		candidates = append(candidates, candidate)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidateLess(candidates[i], candidates[j])
	})
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultCandidateLimit
	}
	leadLimit := req.LeadLimit
	if leadLimit <= 0 {
		leadLimit = DefaultCandidateLeadLimit
	}
	return selectCandidatePage(candidates, limit, leadLimit)
}

// regionIndex rebuilds the placement index from stored facts. It is the same
// index the live catalog uses, fed from the snapshot instead of from a pack file
// and its world book, so a token that names two places still places nobody.
func (f *Frozen) regionIndex() regionIndex {
	index := regionIndex{
		places:    map[string]map[string]bool{},
		byCountry: map[string][]string{},
	}
	for _, place := range f.Places {
		if index.places[place.Token] == nil {
			index.places[place.Token] = map[string]bool{}
		}
		index.places[place.Token][place.Place] = true
		found := false
		for _, existing := range index.byCountry[place.Country] {
			if existing == place.Token {
				found = true
				break
			}
		}
		if !found {
			index.byCountry[place.Country] = append(index.byCountry[place.Country], place.Token)
		}
	}
	for country := range index.byCountry {
		sort.Strings(index.byCountry[country])
	}
	return index
}

func (p FrozenPerson) character() CatalogCharacter {
	return CatalogCharacter{
		SourceKey:           p.SourceKey,
		Name:                p.Name,
		Aliases:             append([]string(nil), p.Aliases...),
		Country:             p.Country,
		FactionID:           p.FactionID,
		FactionName:         p.FactionName,
		MembershipAdmission: append([]string(nil), p.MembershipAdmission...),
		Role:                p.Role,
		Topics:              append([]string(nil), p.Topics...),
		PlaceCandidate:      p.PlaceCandidate,
		PlaceLabel:          p.PlaceLabel,
		RegionTokens:        append([]string(nil), p.RegionTokens...),
		Level:               p.Level,
		Brief:               p.Brief,
		LoreID:              p.LoreID,
		InitialStage:        p.InitialStage,
		RecordOnly:          p.RecordOnly,
		ContactGate:         p.ContactGate,
		Age:                 p.Age,
		Gender:              p.Gender,
		SpeechStyle:         p.SpeechStyle,
		Lifeform:            p.Lifeform,
		CultivationMethodID: p.CultivationMethodID,
	}
}

func personFromCatalog(c CatalogCharacter) FrozenPerson {
	return FrozenPerson{
		SourceKey:           c.SourceKey,
		Name:                c.Name,
		Aliases:             append([]string(nil), c.Aliases...),
		Country:             c.Country,
		FactionID:           c.FactionID,
		FactionName:         c.FactionName,
		MembershipAdmission: append([]string(nil), c.MembershipAdmission...),
		Role:                c.Role,
		Topics:              append([]string(nil), c.Topics...),
		PlaceCandidate:      c.PlaceCandidate,
		PlaceLabel:          c.PlaceLabel,
		RegionTokens:        append([]string(nil), c.RegionTokens...),
		Level:               c.Level,
		Brief:               c.Brief,
		LoreID:              c.LoreID,
		InitialStage:        c.InitialStage,
		RecordOnly:          c.RecordOnly,
		ContactGate:         c.ContactGate,
		Age:                 c.Age,
		Gender:              c.Gender,
		SpeechStyle:         c.SpeechStyle,
		Lifeform:            c.Lifeform,
		CultivationMethodID: c.CultivationMethodID,
	}
}
