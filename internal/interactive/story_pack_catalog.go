// This file is the stored form of a story's frozen catalog.
//
// The type lives here, in the package that owns the story format, and holds no
// knowledge of how a catalog is queried. That split is forced by the dependency
// direction — storypack already builds story requests and therefore imports this
// package — and it is the right split anyway: this is a stored format, and the
// query logic belongs with the catalog it queries.
package interactive

const (
	PackCatalogLifeformHuman       = "human"
	PackCatalogLifeformAnimal      = "animal"
	PackCatalogLifeformSpiritBeast = "spirit_beast"
)

// PackCatalogSnapshot is a story's frozen copy of the catalog it started on. It
// is written once, at creation, and read back on every later turn.
//
// The world book is already scoped to the pack revision that wrote it, so
// editing a pack never changes an in-progress story's lore. The catalog had no
// such copy: it was re-read from the pack file every turn and, once the file's
// fingerprint moved on, withheld entirely — so an old story lost every preset
// person the moment its pack was edited.
//
// It is deliberately not a second world state. It holds locating records and
// resolved placement facts, never profile bodies. Bodies stay in the
// revision-scoped world book, which is still the one readable source of what a
// person is, and nothing here is written after the story starts.
//
// The person rows duplicate the catalog's field set rather than sharing its
// type. This is a stored format: it must keep meaning what it meant when the
// story was written, even if the loader grows, renames or drops a field.
type PackCatalogSnapshot struct {
	// ContentVersion is the author's revision number as of the snapshot.
	ContentVersion int `json:"content_version,omitempty"`
	// Characters are the locating records, in the pack's own order.
	Characters []PackCatalogPerson `json:"characters,omitempty"`
	// TopicTerms is the world's own vocabulary for matching a request to a
	// person's topics. Without it "残阵" stops finding the people tagged 阵法.
	TopicTerms map[string][]string `json:"topic_terms,omitempty"`
	// Places is the resolved placement index. It is stored rather than derived
	// because deriving it reads the world book, and the world book belongs to a
	// separate revision history. Freezing the resolved answer keeps an old story
	// placing people exactly as it did, without shipping a second copy of the
	// world.
	Places []PackCatalogPlace `json:"places,omitempty"`
}

// PackCatalogPerson is one catalog row as the story stored it.
type PackCatalogPerson struct {
	SourceKey           string   `json:"source_key"`
	Name                string   `json:"name"`
	Aliases             []string `json:"aliases,omitempty"`
	Country             string   `json:"country,omitempty"`
	FactionID           string   `json:"faction_id,omitempty"`
	FactionName         string   `json:"faction_name,omitempty"`
	MembershipAdmission []string `json:"membership_admission,omitempty"`
	Role                string   `json:"role,omitempty"`
	Topics              []string `json:"topics,omitempty"`
	PlaceCandidate      string   `json:"initial_place_candidate,omitempty"`
	PlaceLabel          string   `json:"initial_place_label,omitempty"`
	RegionTokens        []string `json:"region_tokens,omitempty"`
	Level               string   `json:"level,omitempty"`
	Brief               string   `json:"brief,omitempty"`
	LoreID              string   `json:"lore_id,omitempty"`
	InitialStage        string   `json:"initial_stage,omitempty"`
	ContactGate         string   `json:"contact_gate,omitempty"`
	RecordOnly          bool     `json:"record_only,omitempty"`
	// Age is the age the pack's authored body stated when the story froze this
	// catalog. Zero means the body stated none.
	Age int `json:"age,omitempty"`
	// Gender is what the author decided for this person when the story froze
	// this catalog. Empty means the author never said, which is a state the
	// story keeps rather than fills in later.
	Gender              string `json:"gender,omitempty"`
	Lifeform            string `json:"lifeform,omitempty"`
	CultivationMethodID string `json:"cultivation_method_id,omitempty"`
}

// PackCatalogPlace is one resolved placement fact: which country a region token
// names, and which authored place it names there.
type PackCatalogPlace struct {
	Token   string `json:"token"`
	Country string `json:"country"`
	Place   string `json:"place"`
}

// ClonePackCatalogSnapshot deep-copies a snapshot so a caller cannot mutate the
// copy held in a story meta.
func ClonePackCatalogSnapshot(snapshot *PackCatalogSnapshot) *PackCatalogSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := &PackCatalogSnapshot{
		ContentVersion: snapshot.ContentVersion,
		Characters:     make([]PackCatalogPerson, 0, len(snapshot.Characters)),
		TopicTerms:     make(map[string][]string, len(snapshot.TopicTerms)),
		Places:         append([]PackCatalogPlace(nil), snapshot.Places...),
	}
	for _, person := range snapshot.Characters {
		person.MembershipAdmission = append([]string(nil), person.MembershipAdmission...)
		person.Aliases = append([]string(nil), person.Aliases...)
		person.Topics = append([]string(nil), person.Topics...)
		person.RegionTokens = append([]string(nil), person.RegionTokens...)
		cloned.Characters = append(cloned.Characters, person)
	}
	for topic, values := range snapshot.TopicTerms {
		cloned.TopicTerms[topic] = append([]string(nil), values...)
	}
	return cloned
}
