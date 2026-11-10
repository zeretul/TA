package storypack

import (
	"fmt"
	"sort"
	"strings"

	"ta/internal/interactive"
)

const (
	LifeformHuman       = interactive.PackCatalogLifeformHuman
	LifeformAnimal      = interactive.PackCatalogLifeformAnimal
	LifeformSpiritBeast = interactive.PackCatalogLifeformSpiritBeast
)

// CatalogCharacter is one discoverable preset character a pack ships. It is the
// locating record, not the character: it carries what is needed to find the
// right person for the current scene — where they are, what they know about,
// what to call them — and points at the world-book entry that holds the actual
// profile body.
//
// The split matters. Sending every profile body every turn is what the design
// rules out; sending only a name is what makes the model invent a stranger who
// shares that name. A small card per candidate, and the full body read only for
// the one who actually appears, is the shape both constraints allow.
//
// Nothing here is player or NPC knowledge. The catalog is an author index: that
// a person exists in a place says nothing about whether the protagonist has met
// them, and the discovery block is written so the narrator cannot read it that
// way.
type CatalogCharacter struct {
	// SourceKey is the author's stable identity for this person, and the only
	// field that is an identity. Display names are not: two packs may ship a
	// 陈巧倩 and they are different people.
	SourceKey string `json:"source_key"`
	Name      string `json:"name"`
	// Aliases are names the author has verified as referring to this person.
	// An unverified nickname belongs in neither the catalog nor the story.
	Aliases     []string `json:"aliases,omitempty"`
	Country     string   `json:"country,omitempty"`
	FactionID   string   `json:"faction_id,omitempty"`
	FactionName string   `json:"faction_name,omitempty"`
	// MembershipAdmission is explicit author authority to admit a member to
	// this person's faction. Role text never grants this authority.
	MembershipAdmission []string `json:"membership_admission,omitempty"`
	Role                string   `json:"role,omitempty"`
	// Topics are the fields this person can actually answer for, in the pack's
	// own vocabulary. They locate a person for a request; they never assert
	// that a mechanic behind the topic is implemented.
	Topics []string `json:"topics,omitempty"`
	// PlaceCandidate is the author's initial placement, not the live location.
	// Where the person actually is comes from the story journal.
	PlaceCandidate string `json:"initial_place_candidate,omitempty"`
	PlaceLabel     string `json:"initial_place_label,omitempty"`
	// RegionTokens are the words that place this person in a scene: their
	// faction's name, their area's name, and the place label's distinctive
	// parts. They are matched against the scene's location text to decide
	// whether someone may appear here or may only be referred to.
	//
	// A token is evidence only when it names one place. The catalog's place
	// classes — 坊市, 山门, 居处, 静修处, 外居 — are used by people in several
	// countries, because every country has a market and a gate. Those are
	// resolved by newRegionIndex rather than trusted here, so that a person in
	// another country is not placed in this scene by a word that only says what
	// kind of place they are at.
	RegionTokens []string `json:"region_tokens,omitempty"`
	// Level is the authoring tier (背景/普通/重要/核心). It is not player-facing
	// and does not scale with anything.
	Level string `json:"level,omitempty"`
	// Brief is the public one-liner. It is what a stranger could reasonably be
	// introduced by, and it deliberately carries no secret.
	Brief string `json:"brief,omitempty"`
	// LoreID names the world-book entry holding this person's full profile body.
	// The catalog never duplicates that body.
	LoreID string `json:"lore_id,omitempty"`
	// InitialStage is the author's proposed starting cultivation. It is an
	// initial value, not a live one, and it is not a claim that the mechanics
	// behind it are implemented.
	InitialStage string `json:"initial_stage,omitempty"`
	// ContactGate says what it takes to reach this person at all. It keeps a
	// sect head from being summoned by a stranger asking directions.
	ContactGate string `json:"contact_gate,omitempty"`
	// RecordOnly retains a historical identity without declaring a living
	// participant in this pack's opening. It is frozen with the Story.
	RecordOnly bool `json:"record_only,omitempty"`
	// Age is the age the person's own authored body states, read once when the
	// catalog is built. Zero means the body states none. It is a real field
	// rather than a lookup because the intimacy gate must not re-read a body
	// that may have been edited since the story started: a story is held to the
	// age its pack stated when the story began.
	Age int `json:"age,omitempty"`
	// Gender is what the author actually decided about this person, either
	// stated on the row or written into their body. Empty is the honest value
	// for an author who never said, and it is not the same as a person whose
	// gender the story has simply not revealed yet: the first is a fact about
	// the author, the second about the player.
	//
	// It is never inferred from a name, a picture or a role. Those are not
	// statements, and a wrong value here would be pinned into every later turn.
	Gender string `json:"gender,omitempty"`
	// SpeechStyle is the author's stated voice, copied into the first story
	// profile when the character appears. Empty means the author left it open.
	SpeechStyle string `json:"speech_style,omitempty"`
	// Lifeform is an explicit author classification. It is never inferred from
	// names, descriptions or appearance; ordinary animals remain ineligible for
	// cultivation unless the frozen catalog also names a registered method.
	Lifeform            string `json:"lifeform,omitempty"`
	CultivationMethodID string `json:"cultivation_method_id,omitempty"`
}

func (c CatalogCharacter) Normalize() CatalogCharacter {
	c.SourceKey = strings.TrimSpace(c.SourceKey)
	c.Name = strings.TrimSpace(c.Name)
	c.Country = strings.TrimSpace(c.Country)
	c.FactionID = strings.TrimSpace(c.FactionID)
	c.FactionName = strings.TrimSpace(c.FactionName)
	c.MembershipAdmission = normalizeTokenList(c.MembershipAdmission)
	c.Role = strings.TrimSpace(c.Role)
	c.PlaceCandidate = strings.TrimSpace(c.PlaceCandidate)
	c.PlaceLabel = strings.TrimSpace(c.PlaceLabel)
	c.Level = strings.TrimSpace(c.Level)
	c.Brief = strings.TrimSpace(c.Brief)
	c.LoreID = strings.TrimSpace(c.LoreID)
	c.InitialStage = strings.TrimSpace(c.InitialStage)
	c.ContactGate = strings.TrimSpace(c.ContactGate)
	c.Gender = normalizeGender(c.Gender)
	c.SpeechStyle = strings.TrimSpace(c.SpeechStyle)
	c.Lifeform = strings.ToLower(strings.TrimSpace(c.Lifeform))
	c.CultivationMethodID = strings.TrimSpace(c.CultivationMethodID)
	c.Aliases = normalizeTokenList(c.Aliases)
	c.Topics = normalizeTokenList(c.Topics)
	c.RegionTokens = normalizeTokenList(c.RegionTokens)
	return c
}

// Validate rejects a catalog row that cannot be located or identified. A row
// without a source key cannot be bound to an actor and would let the same
// person be created twice under two names; a row without a name cannot be
// offered to the narrator at all.
func (c CatalogCharacter) Validate() error {
	c = c.Normalize()
	if c.SourceKey == "" {
		return fmt.Errorf("人物目录条目缺少 source_key")
	}
	if c.Name == "" {
		return fmt.Errorf("人物目录条目缺少 name: %s", c.SourceKey)
	}
	if c.RecordOnly && (c.PlaceCandidate != "" || len(c.MembershipAdmission) > 0) {
		return fmt.Errorf("record-only character cannot have a current placement or admission authority: %s", c.SourceKey)
	}
	if len(c.MembershipAdmission) > 0 && (c.FactionID == "" || c.FactionName == "") {
		return fmt.Errorf("catalog membership_admission requires faction_id and faction_name: %s", c.SourceKey)
	}
	for _, admission := range c.MembershipAdmission {
		switch admission {
		case "formal", "temporary":
		default:
			return fmt.Errorf("invalid catalog membership_admission %q: %s", admission, c.SourceKey)
		}
	}
	switch c.Lifeform {
	case "", LifeformHuman, LifeformAnimal, LifeformSpiritBeast:
	default:
		return fmt.Errorf("人物目录条目生命类别无效: %s", c.SourceKey)
	}
	if c.Lifeform == LifeformSpiritBeast && c.CultivationMethodID == "" {
		return fmt.Errorf("可修炼灵兽目录条目缺少已注册方法: %s", c.SourceKey)
	}
	if c.Lifeform != LifeformSpiritBeast && c.CultivationMethodID != "" {
		return fmt.Errorf("只有可修炼灵兽目录条目可以绑定物种方法: %s", c.SourceKey)
	}
	return nil
}

// MatchNames are the strings that resolve to this person by name: their own
// name plus every verified alias.
func (c CatalogCharacter) MatchNames() []string {
	c = c.Normalize()
	out := make([]string, 0, 1+len(c.Aliases))
	if c.Name != "" {
		out = append(out, c.Name)
	}
	out = append(out, c.Aliases...)
	return out
}

// Reach is how close a candidate is to the current scene. It is the field that
// keeps a person in another country from walking into this one: only ReachLocal
// may be staged in person.
type Reach string

const (
	// ReachLocal means the person belongs to the place the scene is in. They may
	// appear, be met, or be introduced.
	ReachLocal Reach = "local"
	// ReachRegion means the person is in the same country but not at this place.
	// They are a lead: reachable by travel, by letter, or by someone who knows
	// them, never by simply being here.
	ReachRegion Reach = "region"
	// ReachRemote means the person is in another country. They may be referred
	// to and searched for; they may not appear.
	ReachRemote Reach = "remote"
)

// Candidate is one person the program prepared for this turn, with why.
type Candidate struct {
	Character CatalogCharacter
	Reach     Reach
	// Reasons are short human-readable justifications. They exist so the block
	// can tell the narrator why this person is being offered, which is what
	// stops the list reading as an arbitrary roster.
	Reasons []string
	// Named is true when the player asked for this person by name. A named
	// candidate is never dropped for capacity while an unnamed one remains.
	Named bool
	// Matched is true when this person's authored topics answer what the player
	// asked about. It is tracked apart from Reasons because a reason records
	// what the person is doing in this scene while a match records whether they
	// are any use for this request, and the page ranks on the second.
	Matched bool
}

// CandidateRequest is the scene-side input to candidate preparation. Every
// field is what the turn already knows before the model is asked anything.
type CandidateRequest struct {
	// Location is the current scene's location text, as the story recorded it.
	// It is prose, which is why region matching is by token containment.
	Location string
	// Action is what the player asked for this turn.
	Action string
	// Named are the names the player used explicitly.
	Named []string
	// Established are source keys this branch has already registered as actors.
	// They are left out: they are already in the cast block with their ids, and
	// offering them again is how a story re-introduces someone it has met.
	Established map[string]bool
	// Limit caps the candidate list. Zero means DefaultCandidateLimit.
	Limit int
	// LeadLimit caps how much of that page people who are not at this place may
	// take. Zero means DefaultCandidateLeadLimit.
	LeadLimit int
}

const (
	// DefaultCandidateLimit is the page size for candidate cards. The catalog
	// holds every verified person; a turn is offered a page of them.
	DefaultCandidateLimit = 12
	// DefaultCandidateLeadLimit is how much of that page a person who is not at
	// this place may take.
	//
	// Being elsewhere is not the same as being irrelevant: 查阵法 is answered by
	// the local array teacher and, when there is none, by the two people in the
	// same country who know arrays. Ranking purely by reach put those two behind
	// thirty-nine people who happened to be nearby and had nothing to say about
	// arrays, so the page offered one relevant person and eleven strangers. A
	// quarter of the page is enough to surface the leads a request really has,
	// and small enough that the people who can actually be met keep the rest.
	DefaultCandidateLeadLimit = 3
	// DefaultCandidateMaxBytes bounds the rendered block.
	DefaultCandidateMaxBytes = 12 * 1024
)

// CharacterCandidates selects the people worth offering this turn, ordered by
// the design's relevance ladder: explicitly named, then matched to this place
// and this request, then reachable in the same country, then everyone else as a
// reference.
//
// The ladder is the whole point of the function. Truncating a catalog by ID
// makes the same handful of people the only ones a story can ever find, and
// makes the rest unreachable no matter what the player asks for. Selection here
// is by fit, and capacity is applied after ranking, so a low-ranked person is
// dropped for being irrelevant rather than for being late in the alphabet.
//
// Reach is a constraint, not the ranking. Someone who can answer the question is
// worth a card even when they are a country away, because a lead is how a
// request in a small place is answered at all; someone who is merely nearby and
// says nothing about it is a roster entry. So the page is filled in two passes:
// the people at this place take the seats first, and the leads that matched take
// a bounded share of what is left.
func (p Pack) CharacterCandidates(req CandidateRequest) []Candidate {
	catalog := p.normalizedCatalog()
	if len(catalog) == 0 {
		return nil
	}
	location := strings.ToLower(strings.TrimSpace(req.Location))
	action := strings.ToLower(strings.TrimSpace(req.Action))
	named := make(map[string]bool, len(req.Named))
	for _, name := range req.Named {
		if key := normalizeNameKey(name); key != "" {
			named[key] = true
		}
	}

	// Where the scene is, and which of a person's region tokens are evidence of
	// being there, both come from the pack's region index. The scene's country is
	// inferred from the places its text names rather than from whoever happens to
	// match, so one person's place-class token cannot decide it for everyone.
	index := p.newRegionIndex()
	sceneCountries := index.countriesAt(location)

	terms := p.normalizedTopicTerms()
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
		locationMatch := !character.RecordOnly && index.placesScene(character, location, sceneCountries)
		if locationMatch {
			candidate.Reasons = append(candidate.Reasons, "位于当前所在区域")
		}
		switch {
		case character.RecordOnly:
			candidate.Reach = ReachRemote
			candidate.Reasons = append(candidate.Reasons, "仅供史料引用，不是本开局可到场人物")
		case locationMatch:
			candidate.Reach = ReachLocal
		case character.Country != "" && sceneCountries[strings.ToLower(character.Country)]:
			candidate.Reach = ReachRegion
			candidate.Reasons = append(candidate.Reasons, "同国异地，需行路或引荐")
		default:
			candidate.Reach = ReachRemote
			candidate.Reasons = append(candidate.Reasons, "异地，只能作为线索提及")
		}
		// A person nobody asked for, who is neither here nor related to the
		// request, is not a candidate: it is a roster entry.
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

// selectCandidatePage takes the ranked list and fills one page from it.
//
// Named people are taken first and are never held to a share: an explicit name
// is an instruction, not a preference. The rest of the page is then divided by
// reserving seats for leads before the people at this place take any.
//
// Reserving rather than appending is what makes the share real. A large town can
// easily hold forty people, and appending leads after them put the two people
// who could answer a question about arrays at positions forty and forty-one —
// a page of twelve that named one relevant person and eleven who had nothing to
// say about it. The seats are reserved only when there are leads that matched,
// so a scene with nobody relevant elsewhere still fills its page locally.
func selectCandidatePage(ranked []Candidate, limit, leadLimit int) []Candidate {
	if len(ranked) <= limit {
		return ranked
	}
	taken := make([]bool, len(ranked))
	page := make([]Candidate, 0, limit)
	take := func(at int) bool {
		if len(page) >= limit {
			return false
		}
		taken[at] = true
		page = append(page, ranked[at])
		return true
	}
	for at, candidate := range ranked {
		if candidate.Named {
			take(at)
		}
	}
	// How many seats the leads may actually use: the quota, capped by how many
	// leads there are to fill it. A quota reserved against an empty pool would
	// hold seats no one can take.
	available := 0
	for at, candidate := range ranked {
		if !taken[at] && candidate.Reach != ReachLocal && candidate.Matched {
			available++
		}
	}
	reserved := leadLimit
	if available < reserved {
		reserved = available
	}
	localCap := limit - reserved
	for at, candidate := range ranked {
		if len(page) >= localCap {
			break
		}
		if taken[at] || candidate.Reach != ReachLocal {
			continue
		}
		take(at)
	}
	leads := 0
	for at, candidate := range ranked {
		if len(page) >= limit || leads >= leadLimit {
			break
		}
		if taken[at] || candidate.Reach == ReachLocal || !candidate.Matched {
			continue
		}
		if take(at) {
			leads++
		}
	}
	for at, candidate := range ranked {
		if len(page) >= limit {
			break
		}
		if taken[at] || candidate.Reach != ReachLocal {
			continue
		}
		take(at)
	}
	for at, candidate := range ranked {
		if len(page) >= limit || leads >= leadLimit {
			break
		}
		if taken[at] || candidate.Reach == ReachLocal {
			continue
		}
		if take(at) {
			leads++
		}
	}
	return page
}

// candidateLess orders by the relevance ladder, and only then by identity so the
// result is deterministic. Identity is a tiebreak, never the selector.
func candidateLess(a, b Candidate) bool {
	if a.Named != b.Named {
		return a.Named
	}
	if a.Reach != b.Reach {
		return reachRank(a.Reach) < reachRank(b.Reach)
	}
	if a.Matched != b.Matched {
		return a.Matched
	}
	if len(a.Reasons) != len(b.Reasons) {
		return len(a.Reasons) > len(b.Reasons)
	}
	return a.Character.SourceKey < b.Character.SourceKey
}

func reachRank(reach Reach) int {
	switch reach {
	case ReachLocal:
		return 0
	case ReachRegion:
		return 1
	default:
		return 2
	}
}

// regionIndex answers the placement questions the catalog asks, from the pack's
// own authored tokens. It exists because a token's meaning depends on the whole
// catalog, not on one row: 黄枫谷 names one place, while 坊市 names a kind of
// place that people in three countries are at.
type regionIndex struct {
	// places maps a region token to the distinct places it names, each place
	// being the country it belongs to plus the label it came from. A token
	// naming places in more than one country does not place anyone in a scene.
	places map[string]map[string]bool
	// byCountry maps a lowercased country to its own place tokens, which is how
	// the scene's country is worked out from the place it names.
	byCountry map[string][]string
}

// newRegionIndex builds the index from the catalog's rows and their world-book
// entries. Both halves of a row's placement are read, so a token that a label
// contributes in one country and a name contributes in another is counted once
// per place rather than trusted as one.
func (p Pack) newRegionIndex() regionIndex {
	index := regionIndex{
		places:    map[string]map[string]bool{},
		byCountry: map[string][]string{},
	}
	worldByID := make(map[string]string, len(p.World))
	for _, entry := range p.World {
		worldByID[strings.TrimSpace(entry.ID)] = strings.Join(
			[]string{entry.Name, entry.Content}, "\n")
	}
	add := func(country, token, place string) {
		if token == "" || country == "" {
			return
		}
		if index.places[token] == nil {
			index.places[token] = map[string]bool{}
		}
		index.places[token][place] = true
		country = strings.ToLower(country)
		for _, existing := range index.byCountry[country] {
			if existing == token {
				return
			}
		}
		index.byCountry[country] = append(index.byCountry[country], token)
	}
	for _, character := range p.normalizedCatalog() {
		country := strings.ToLower(character.Country)
		place := country + "\x00" + character.PlaceCandidate + "\x00" + character.PlaceLabel
		tokens := append([]string{}, character.RegionTokens...)
		// The place label is authored to name the person's place, so its
		// distinctive parts are placement evidence even though only the label
		// carries them.
		if body := worldByID[character.LoreID]; body != "" {
			if label := strings.TrimSpace(character.PlaceLabel); label != "" && strings.Contains(body, label) {
				tokens = append(tokens, label)
			}
		}
		for _, token := range tokens {
			add(country, strings.ToLower(token), place)
		}
	}
	// The creation page's own place list is the author's registry of places a
	// story can stand in, each with the country it is in. A person's rows name
	// the halls and stalls inside one of those places, so without this the scene
	// text a journey commits — the place's own name, 公共坊市 — names nothing the
	// index can read, and every person in the world reads as being abroad.
	if p.Creation != nil {
		for _, option := range p.Creation.Locations {
			name := strings.ToLower(strings.TrimSpace(option.Name))
			if name == "" || strings.TrimSpace(option.Country) == "" {
				continue
			}
			country := strings.ToLower(strings.TrimSpace(option.Country))
			add(country, name, country+"\x00"+strings.TrimSpace(option.ID)+"\x00"+option.Name)
		}
	}
	for country := range index.byCountry {
		sort.Strings(index.byCountry[country])
	}
	return index
}

// countriesAt returns the countries the scene's location text could be in. The
// scene's country is read from the places the text names — a country named
// outright, or a token that names exactly one place — and never from whoever
// happened to match, which is how one person's place-class token would decide the
// scene's country for the whole catalog.
func (index regionIndex) countriesAt(location string) map[string]bool {
	countries := map[string]bool{}
	for country, tokens := range index.byCountry {
		// A country's own name in the text places the scene there.
		if country != "" && strings.Contains(location, country) {
			countries[country] = true
			continue
		}
		for _, token := range tokens {
			if index.tokenPlacesScene(token) && strings.Contains(location, token) {
				countries[country] = true
				break
			}
		}
	}
	return countries
}

// tokenPlacesScene reports whether a region token names places in exactly one
// country. A word several countries use says only what kind of place this is, so
// on its own it never places anyone here.
//
// A token one country uses at several places still places the scene in that
// country: 黄枫谷 is twenty-four halls in 越国 and nowhere else, and a scene that
// says 黄枫谷 is in 越国 no matter which hall. Which person is actually standing
// there is a different question, and placesScene answers it against each person's
// own tokens rather than against the scene's.
func (index regionIndex) tokenPlacesScene(token string) bool {
	country := ""
	for key := range index.places[token] {
		name, _, ok := strings.Cut(key, "\x00")
		if !ok {
			continue
		}
		if country == "" {
			country = name
			continue
		}
		if name != country {
			return false
		}
	}
	return country != ""
}

// placesScene reports whether this person belongs to the place the scene is at.
// Three things must hold: the scene's text names one of the person's own tokens,
// the person's country is one the scene is in, and the token is theirs rather than
// another country's. Together they keep a person in 元武国 out of a 越国 market
// even though both countries' people carry the token 坊市.
func (index regionIndex) placesScene(character CatalogCharacter, location string, sceneCountries map[string]bool) bool {
	if location == "" {
		return false
	}
	country := strings.ToLower(character.Country)
	if country == "" || !sceneCountries[country] {
		return false
	}
	for _, token := range character.RegionTokens {
		token = strings.ToLower(token)
		if token == "" || !index.ownsToken(country, token) {
			continue
		}
		if strings.Contains(location, token) {
			return true
		}
	}
	return false
}

// ownsToken reports whether a country is one of the countries that uses a token.
// It is what stops one country's token from placing another country's people:
// 越国 and 元武国 both use 山门, but 元武国's 山门 token must not place anyone at a
// 越国 山门.
func (index regionIndex) ownsToken(country, token string) bool {
	for key := range index.places[token] {
		if strings.HasPrefix(key, country+"\x00") {
			return true
		}
	}
	return false
}

// LocationNamesAPlace reports whether a scene's location text names somewhere this
// catalog can place people. It exists so a caller can tell "nobody here fits" from
// "this scene has no location", which are different answers and must not be
// reported the same way: an unresolved location makes every person read as being in
// another country, which is a claim about the world the program cannot support.
func (p Pack) LocationNamesAPlace(location string) bool {
	return len(p.newRegionIndex().countriesAt(
		strings.ToLower(strings.TrimSpace(location)))) > 0
}

// matchedTopics returns the topics this person can answer for that the current
// request actually touches. The pack's topic terms let "残阵" and "布阵" both
// resolve to 阵法 without the code carrying a Chinese synonym list of its own.
func (c CatalogCharacter) matchedTopics(action string, terms map[string][]string) []string {
	if action == "" || len(c.Topics) == 0 {
		return nil
	}
	matched := make([]string, 0, len(c.Topics))
	for _, topic := range c.Topics {
		if topic == "" {
			continue
		}
		if strings.Contains(action, strings.ToLower(topic)) {
			matched = append(matched, topic)
			continue
		}
		for _, term := range terms[topic] {
			if term != "" && strings.Contains(action, strings.ToLower(term)) {
				matched = append(matched, topic)
				break
			}
		}
	}
	return matched
}

// normalizedCatalog returns the pack's catalog with each row normalized, in a
// stable order.
//
// The age is filled here rather than by whichever caller needs it. The live
// catalog and the frozen snapshot must answer the same question the same way —
// a story's adult gate reads the age off the row — so both have to get it from
// the pack's own world book and nowhere else.
func (p Pack) normalizedCatalog() []CatalogCharacter {
	if len(p.Characters) == 0 {
		return nil
	}
	ages := p.authoredAges()
	genders := p.authoredGenders()
	out := make([]CatalogCharacter, 0, len(p.Characters))
	for _, character := range p.Characters {
		if character = character.Normalize(); character.SourceKey != "" && character.Name != "" {
			if character.Age <= 0 {
				character.Age = ages[character.LoreID]
			}
			// A row's own statement wins: the author wrote it about this person
			// specifically, while the body's sentence is read out of prose. Only
			// when the row is silent does the body answer.
			if character.Gender == "" {
				character.Gender = genders[character.LoreID]
			}
			out = append(out, character)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SourceKey < out[j].SourceKey })
	return out
}

// authoredGenders indexes every world entry's stated gender by entry id. One
// pass over the world book serves the whole catalog, the same way ages do.
func (p Pack) authoredGenders() map[string]string {
	if len(p.World) == 0 {
		return nil
	}
	genders := make(map[string]string, len(p.World))
	for _, entry := range p.World {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		if gender := ParseGender(entry.Content); gender != "" {
			genders[id] = gender
		}
	}
	return genders
}

// authoredAges indexes every world entry's stated age by entry id. One pass
// over the world book serves the whole catalog.
func (p Pack) authoredAges() map[string]int {
	if len(p.World) == 0 {
		return nil
	}
	ages := make(map[string]int, len(p.World))
	for _, entry := range p.World {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		if age := parseAuthoredAge(entry.Content); age > 0 {
			ages[id] = age
		}
	}
	return ages
}

func (p Pack) normalizedTopicTerms() map[string][]string {
	if len(p.TopicTerms) == 0 {
		return nil
	}
	out := make(map[string][]string, len(p.TopicTerms))
	for topic, values := range p.TopicTerms {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		out[topic] = normalizeTokenList(values)
	}
	return out
}

// CatalogBySourceKey indexes the catalog by the author's stable key.
func (p Pack) CatalogBySourceKey() map[string]CatalogCharacter {
	catalog := p.normalizedCatalog()
	out := make(map[string]CatalogCharacter, len(catalog))
	for _, character := range catalog {
		out[character.SourceKey] = character
	}
	return out
}

// CatalogByName indexes the catalog by each name the author verified for a
// person: their own name and their aliases, normalized the way the discovery page
// prints them.
//
// A name is only an index when it identifies exactly one person. Two people who
// share a name make it ambiguous, and an ambiguous name is dropped rather than
// resolved by a first match: the design forbids merging people for sharing a
// name, and picking one of two is that same error with one fewer witness. The
// same rule applies across a name and another person's alias, so a nickname never
// silently steals a person from the name it was meant to stand in for.
func (p Pack) CatalogByName() map[string]string {
	catalog := p.normalizedCatalog()
	byName := make(map[string]string, len(catalog)*2)
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
	for _, character := range catalog {
		claim(character.Name, character.SourceKey)
		for _, alias := range character.Aliases {
			claim(alias, character.SourceKey)
		}
	}
	return byName
}

// CatalogByLoreID indexes the catalog by the world-book entry that holds each
// person's profile body. It is how a loaded entry is traced back to the person
// it is about, which is what lets a first appearance bind the right actor.
func (p Pack) CatalogByLoreID() map[string]CatalogCharacter {
	catalog := p.normalizedCatalog()
	out := make(map[string]CatalogCharacter, len(catalog))
	for _, character := range catalog {
		if character.LoreID != "" {
			out[character.LoreID] = character
		}
	}
	return out
}

// MentionedNames returns the catalog names and aliases that appear verbatim in
// a piece of player text. It is how an explicit request to find someone by name
// is recognized: the player wrote the name, so the person is named regardless of
// where they are or what the turn is about.
//
// Matching is a verbatim substring test on the normalized text. It deliberately
// does not fuzzy-match, because a near-miss would turn "找一位懂阵法的同道"
// into a claim that the player asked for 辛如音 by name.
func (p Pack) MentionedNames(text string) []string {
	text = normalizeNameKey(text)
	if text == "" {
		return nil
	}
	var out []string
	for _, character := range p.normalizedCatalog() {
		for _, name := range character.MatchNames() {
			if key := normalizeNameKey(name); key != "" && strings.Contains(text, key) {
				out = append(out, character.Name)
				break
			}
		}
	}
	return out
}

// FindCatalogByName resolves a spoken name to the people who answer to it. It
// returns every match rather than the first, because two people may legitimately
// share a name and silently picking one is how a story merges them.
func (p Pack) FindCatalogByName(name string) []CatalogCharacter {
	key := normalizeNameKey(name)
	if key == "" {
		return nil
	}
	var out []CatalogCharacter
	for _, character := range p.normalizedCatalog() {
		for _, candidate := range character.MatchNames() {
			if normalizeNameKey(candidate) == key {
				out = append(out, character)
				break
			}
		}
	}
	return out
}

// CandidateCardsMarkdown renders the prepared candidates for the model. It is a
// locating index, not a character sheet: it says who exists, where they are
// relative to this scene, and what to read before using them.
//
// The block states the reach of each person in words the model has to act on,
// because the failure this guards against is a person in another country being
// written into the scene. Capacity is checked before each card is written, so a
// card is never half-emitted.
//
// A page that fits no card renders nothing at all. A header over an empty list
// would tell the model that this world has a verified roster it is not being
// shown, which is the state §8 rules out: the honest answer is that no page was
// prepared, leaving the model to work from what it already has.
func CandidateCardsMarkdown(candidates []Candidate, maxBytes int) string {
	if len(candidates) == 0 {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = DefaultCandidateMaxBytes
	}
	var sb strings.Builder
	sb.WriteString(discoverablePeopleHeader)
	for _, candidate := range candidates {
		card := candidateCard(candidate)
		// Measure before writing: a block that overflows its budget must lose a
		// whole card, not half of one.
		if sb.Len()+len([]byte(card)) > maxBytes {
			break
		}
		sb.WriteString(card)
	}
	if sb.Len() == len(discoverablePeopleHeader) {
		return ""
	}
	return strings.TrimSpace(sb.String())
}

// discoverablePeopleHeader is the fixed preamble CandidateCardsMarkdown writes
// before any card. It is a constant so the renderer can tell a page that holds
// no cards from one that holds some, and withhold the empty page.
const discoverablePeopleHeader = "## Discoverable People (source: this world's verified author catalog, page of candidates for this turn)\n\n" +
	"These people exist in this world. This is an index, not knowledge: the protagonist has not met them, and being listed here does not mean they are present, reachable, or willing. Read a person's full entry from the world book before they appear in prose.\n\n"

func candidateCard(candidate Candidate) string {
	character := candidate.Character
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s (source_key: %s)\n", character.Name, character.SourceKey)
	if character.RecordOnly {
		sb.WriteString("- 接触边界：仅可引用史料；不能安排本人到场、通信、授业或参战，也不能把后世经历当作已经发生。\n")
	} else {
		fmt.Fprintf(&sb, "- Availability: %s\n", reachInstruction(candidate.Reach))
	}
	if len(candidate.Reasons) > 0 {
		fmt.Fprintf(&sb, "- Why offered: %s\n", strings.Join(candidate.Reasons, "；"))
	}
	if character.Role != "" {
		fmt.Fprintf(&sb, "- Role: %s\n", character.Role)
	}
	if character.Gender != "" {
		// The author's own statement, printed so the profile this turn submits
		// carries the same answer instead of the model deciding again. It is not
		// a hint about personality: the card already carries role and topics,
		// and this line says only who the person is.
		fmt.Fprintf(&sb, "- Gender: %s (stated by the author; copy it into the profile you submit)\n", character.Gender)
	}
	if character.Age > 0 {
		fmt.Fprintf(&sb, "- Age: %d (stated by the author; copy it into the profile you submit)\n", character.Age)
	}
	if character.SpeechStyle != "" {
		fmt.Fprintf(&sb, "- Voice: %s (stated by the author; copy it into the profile you submit)\n", character.SpeechStyle)
	}
	if character.Brief != "" {
		fmt.Fprintf(&sb, "- Publicly visible as: %s\n", character.Brief)
	}
	if len(character.Topics) > 0 {
		fmt.Fprintf(&sb, "- Can speak to: %s\n", strings.Join(character.Topics, "、"))
	}
	if character.PlaceLabel != "" {
		fmt.Fprintf(&sb, "- Author's initial placement: %s\n", character.PlaceLabel)
	}
	if character.ContactGate != "" {
		fmt.Fprintf(&sb, "- Contact gate: %s\n", character.ContactGate)
	}
	if character.LoreID != "" {
		fmt.Fprintf(&sb, "- Full entry: read_lore_items id=%s before this person appears, speaks, or is described.\n", character.LoreID)
	}
	sb.WriteString("\n")
	return sb.String()
}

func reachInstruction(reach Reach) string {
	switch reach {
	case ReachLocal:
		return "at this place — may be met, introduced, or appear in this scene if it fits"
	case ReachRegion:
		return "in the same country but not here — may be named as a lead, reached by travel, letter, or someone who knows them; must not appear in this scene"
	default:
		return "in another country — may only be referred to as a distant lead; must not appear, arrive, or be contacted without a real route and permission"
	}
}

func normalizeTokenList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeNameKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), ""))
}
