package character

import (
	"fmt"
	"strings"
)

// Affinity is one character's feeling toward the protagonist, as a direction.
// It is deliberately one integer and not a set of meters: the design asks for a
// single reading a player can understand, and for the actual relationship to be
// carried by facts rather than by a second score.
//
// The zero value is the honest starting point. A stranger has not decided
// anything about the protagonist yet, and 0 is what "not yet decided" looks
// like on the same scale that later carries real feeling.
const (
	AffinityMin = -100
	AffinityMax = 100
)

// AffinityBand is the behaviour reference a value falls into. It is not a hard
// lock: a band says what this person is generally willing to consider, while the
// specific request is still judged by their principles, the risk and their own
// situation. The bands exist so the model can be told what the number means
// instead of inventing a meaning for it each turn.
type AffinityBand string

const (
	AffinityHostile  AffinityBand = "hostile"  // -100..-60
	AffinityWary     AffinityBand = "wary"     // -59..-20
	AffinityNeutral  AffinityBand = "neutral"  // -19..19
	AffinityFamiliar AffinityBand = "familiar" // 20..39
	AffinityFriendly AffinityBand = "friendly" // 40..59
	AffinityClose    AffinityBand = "close"    // 60..79
	AffinityDeep     AffinityBand = "deep"     // 80..100
)

func (b AffinityBand) Valid() bool {
	switch b {
	case AffinityHostile, AffinityWary, AffinityNeutral, AffinityFamiliar, AffinityFriendly, AffinityClose, AffinityDeep:
		return true
	default:
		return false
	}
}

// BandFor maps a value onto its band. The thresholds are inclusive at both ends
// and cover every legal value exactly once, so a value never falls between two
// bands.
func BandFor(value int) AffinityBand {
	switch {
	case value <= -60:
		return AffinityHostile
	case value <= -20:
		return AffinityWary
	case value <= 19:
		return AffinityNeutral
	case value <= 39:
		return AffinityFamiliar
	case value <= 59:
		return AffinityFriendly
	case value <= 79:
		return AffinityClose
	default:
		return AffinityDeep
	}
}

// ClampAffinity keeps a value inside the legal range. The range is a limit on
// what can be earned, not a target: equipment, cultivation and paid resources
// never move it, and no single event can push past it.
func ClampAffinity(value int) int {
	if value < AffinityMin {
		return AffinityMin
	}
	if value > AffinityMax {
		return AffinityMax
	}
	return value
}

// AffinityKind is the scale of one complete interaction. It exists because the
// design fixes how far a single event may move the number, and a model left to
// choose freely would settle a rescue, a gift and a greeting at the same size.
//
// A kind is a magnitude, not a topic: the same act can be meaningful to one
// person and merely pleasant to another, and that is exactly what the profile's
// preferences decide.
type AffinityKind string

const (
	// AffinityNone is a real interaction that changed nothing: ordinary courtesy,
	// or evidence too thin to judge. It is legal and common; not every exchange
	// moves a relationship.
	AffinityNone AffinityKind = "none"
	// AffinityPleasant is an ordinary good conversation or a fitting small help.
	AffinityPleasant AffinityKind = "pleasant"
	// AffinityMeaningful is real help, a kept promise, or an act that suits what
	// this person actually values.
	AffinityMeaningful AffinityKind = "meaningful"
	// AffinityMajor is a hardship faced together or an important promise honoured.
	AffinityMajor AffinityKind = "major"
	// AffinityOffense is a slight, a broken word, or a clear touch on what this
	// person dislikes.
	AffinityOffense AffinityKind = "offense"
	// AffinityBetrayal is deception, serious bad faith, or damage to something
	// this person holds important.
	AffinityBetrayal AffinityKind = "betrayal"
)

func (k AffinityKind) Valid() bool {
	_, _, ok := k.DeltaRange()
	return ok
}

// DeltaRange is the legal size of one settlement of this kind. The gaps between
// the ranges are deliberate: a change of 16, or of -1, or of -9 is not one of
// the scales the design allows, so a model that invents one is corrected rather
// than believed.
func (k AffinityKind) DeltaRange() (min, max int, ok bool) {
	switch k {
	case AffinityNone:
		return 0, 0, true
	case AffinityPleasant:
		return 1, 3, true
	case AffinityMeaningful:
		return 4, 8, true
	case AffinityMajor:
		return 9, 15, true
	case AffinityOffense:
		return -8, -2, true
	case AffinityBetrayal:
		return -30, -10, true
	default:
		return 0, 0, false
	}
}

// AffinityChange is one settled change to one character's feeling toward the
// protagonist. It records the fact it settled, because a fact already counted
// must not be counted again when the player thanks someone a second time, in a
// different window, or on the next day.
//
// Before and After are both stored. The pair is what makes the record
// auditable: a replay recomputes the same After from the same Before, and a
// reader can see the value the change started from without replaying the whole
// journal.
type AffinityChange struct {
	ID          string       `json:"id"`
	StoryID     string       `json:"story_id"`
	BranchID    string       `json:"branch_id"`
	CharacterID string       `json:"character_id"`
	Kind        AffinityKind `json:"kind"`
	Before      int          `json:"before"`
	After       int          `json:"after"`
	Delta       int          `json:"delta"`
	Band        AffinityBand `json:"band"`
	// Basis is the one fact this settlement is for, in one sentence. It is kept
	// so a later turn can be shown what has already been counted, and so the
	// player can see why a number moved.
	Basis string `json:"basis"`
	// BasisRef names a committed record — an exchange or a matter — that the
	// fact already exists as. When it is set it is the settlement key, which
	// makes a repeat exact rather than a matter of wording.
	BasisRef      string `json:"basis_ref,omitempty"`
	WorldTime     string `json:"world_time"`
	TurnID        string `json:"turn_id,omitempty"`
	SourceEventID string `json:"source_event_id,omitempty"`
}

func (c AffinityChange) Normalize() AffinityChange {
	c.ID = strings.TrimSpace(c.ID)
	c.StoryID = strings.TrimSpace(c.StoryID)
	c.BranchID = strings.TrimSpace(c.BranchID)
	c.CharacterID = strings.TrimSpace(c.CharacterID)
	c.Basis = strings.Join(strings.Fields(strings.TrimSpace(c.Basis)), " ")
	c.BasisRef = strings.TrimSpace(c.BasisRef)
	c.WorldTime = strings.TrimSpace(c.WorldTime)
	c.TurnID = strings.TrimSpace(c.TurnID)
	c.SourceEventID = strings.TrimSpace(c.SourceEventID)
	c.Before = ClampAffinity(c.Before)
	c.After = ClampAffinity(c.After)
	if c.Band == "" {
		c.Band = BandFor(c.After)
	}
	return c
}

func (c AffinityChange) Validate() error {
	c = c.Normalize()
	if c.ID == "" || c.CharacterID == "" {
		return fmt.Errorf("affinity change requires an id and a character")
	}
	if c.StoryID == "" || c.BranchID == "" {
		return fmt.Errorf("affinity change %s requires a story and branch", c.ID)
	}
	if !c.Kind.Valid() {
		return fmt.Errorf("affinity change %s has an invalid kind: %q", c.ID, c.Kind)
	}
	if c.Basis == "" {
		return fmt.Errorf("affinity change %s has no basis", c.ID)
	}
	if c.After-c.Before != c.Delta {
		return fmt.Errorf("affinity change %s records delta %d but moved %d to %d", c.ID, c.Delta, c.Before, c.After)
	}
	if c.After < AffinityMin || c.After > AffinityMax {
		return fmt.Errorf("affinity change %s leaves the legal range: %d", c.ID, c.After)
	}
	if c.Band != BandFor(c.After) {
		return fmt.Errorf("affinity change %s is labelled %q but %d is %q", c.ID, c.Band, c.After, BandFor(c.After))
	}
	if c.WorldTime == "" {
		return fmt.Errorf("affinity change %s has no world time", c.ID)
	}
	return nil
}

// SettlementKey is the identity of the fact this change settled. Two changes
// with the same key for the same character are the same settlement, however
// many windows or turns they were proposed in.
//
// A named record is exact. A basis with no record falls back to its own
// wording, normalized, which catches a restatement but not a paraphrase; that
// limit is real and is why the model is shown what has already been counted.
func (c AffinityChange) SettlementKey() string {
	c = c.Normalize()
	if c.BasisRef != "" {
		return "ref:" + c.BasisRef
	}
	return "text:" + strings.ToLower(c.Basis)
}

// restatementThreshold is how much of one basis's own wording has to reappear
// in another before the two are treated as the same fact said twice.
//
// Exact wording is not enough on its own: a model asked to describe one act in
// a new turn rephrases it, and the design is explicit that a fact already
// counted does not count again "however it is worded later". Two separate acts
// that a person would count separately almost never share this much phrasing,
// because the wording that makes them separate is exactly what differs.
const restatementThreshold = 0.6

// Restates reports whether this change is another way of stating the fact that
// other already settled. It is deliberately conservative: a short basis has to
// be nearly identical, and a long one has to keep most of its substance.
func (c AffinityChange) Restates(other AffinityChange) bool {
	c, other = c.Normalize(), other.Normalize()
	if c.BasisRef != "" || other.BasisRef != "" {
		return c.SettlementKey() == other.SettlementKey()
	}
	if c.CharacterID != other.CharacterID {
		return false
	}
	if c.SettlementKey() == other.SettlementKey() {
		return true
	}
	left, right := basisBigrams(c.Basis), basisBigrams(other.Basis)
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	shared := 0
	for gram := range left {
		if right[gram] {
			shared++
		}
	}
	smaller := len(left)
	if len(right) < smaller {
		smaller = len(right)
	}
	return float64(shared)/float64(smaller) >= restatementThreshold
}

// basisBigrams cuts a basis into adjacent-character pairs. Chinese has no word
// boundaries, so bigrams are the cheapest stable unit: they keep a phrase
// together without needing a dictionary, and they are insensitive to the
// filler that changes between two tellings of one act.
func basisBigrams(text string) map[string]bool {
	runes := []rune(strings.ToLower(strings.Join(strings.Fields(text), "")))
	out := make(map[string]bool, len(runes))
	for i := 0; i+1 < len(runes); i++ {
		out[string(runes[i:i+2])] = true
	}
	return out
}

// RelationshipKind is a formal relationship, as opposed to a feeling. The two
// are separate on purpose: a high affinity can make someone a close friend or a
// sworn companion, and it can make romance possible, but it never makes romance
// true on its own.
type RelationshipKind string

const (
	RelationshipFriend      RelationshipKind = "friend"
	RelationshipCloseFriend RelationshipKind = "close_friend"
	RelationshipSworn       RelationshipKind = "sworn"
	RelationshipRomantic    RelationshipKind = "romantic"
	RelationshipPartner     RelationshipKind = "partner"
	RelationshipEstranged   RelationshipKind = "estranged"
	RelationshipHostile     RelationshipKind = "hostile"
)

func (k RelationshipKind) Valid() bool {
	switch k {
	case RelationshipFriend, RelationshipCloseFriend, RelationshipSworn, RelationshipRomantic, RelationshipPartner, RelationshipEstranged, RelationshipHostile:
		return true
	default:
		return false
	}
}

// Romantic reports whether this kind is a romantic bond rather than a bond of
// friendship or enmity. It decides whether the record needs a mutual exchange.
func (k RelationshipKind) Romantic() bool {
	return k == RelationshipRomantic || k == RelationshipPartner
}

// Negative reports whether this kind is a break rather than a bond.
func (k RelationshipKind) Negative() bool {
	return k == RelationshipEstranged || k == RelationshipHostile
}

// RelationshipMinimum is the affinity a positive relationship needs before it
// can be formalized. It is a floor, never a trigger: reaching it does not create
// anything, and a record still needs a real expression and a real answer.
func RelationshipMinimum(kind RelationshipKind) (int, bool) {
	switch kind {
	case RelationshipFriend:
		return 20, true
	case RelationshipCloseFriend:
		return 40, true
	case RelationshipSworn:
		return 60, true
	case RelationshipRomantic:
		return 60, true
	case RelationshipPartner:
		return 80, true
	default:
		return 0, false
	}
}

// Relationship is one formal change of relationship, with the exchange that
// expressed it and the exchange that answered it. It is a record of something
// that was actually said between two people, not a consequence of a number.
type Relationship struct {
	ID          string           `json:"id"`
	StoryID     string           `json:"story_id"`
	BranchID    string           `json:"branch_id"`
	CharacterID string           `json:"character_id"`
	Kind        RelationshipKind `json:"kind"`
	Statement   string           `json:"statement"`
	// ExpressedBy and AnsweredBy are committed exchange ids on this branch. A
	// relationship is established by what was said, so it cites what was said.
	ExpressedBy string `json:"expressed_by,omitempty"`
	AnsweredBy  string `json:"answered_by,omitempty"`
	WorldTime   string `json:"world_time"`
	TurnID      string `json:"turn_id,omitempty"`
	// SourceEventID is the character record that committed this change.
	SourceEventID string `json:"source_event_id,omitempty"`
}

func (r Relationship) Normalize() Relationship {
	r.ID = strings.TrimSpace(r.ID)
	r.StoryID = strings.TrimSpace(r.StoryID)
	r.BranchID = strings.TrimSpace(r.BranchID)
	r.CharacterID = strings.TrimSpace(r.CharacterID)
	r.Statement = strings.Join(strings.Fields(strings.TrimSpace(r.Statement)), " ")
	r.ExpressedBy = strings.TrimSpace(r.ExpressedBy)
	r.AnsweredBy = strings.TrimSpace(r.AnsweredBy)
	r.WorldTime = strings.TrimSpace(r.WorldTime)
	r.TurnID = strings.TrimSpace(r.TurnID)
	r.SourceEventID = strings.TrimSpace(r.SourceEventID)
	return r
}

func (r Relationship) Validate() error {
	r = r.Normalize()
	if r.ID == "" || r.CharacterID == "" {
		return fmt.Errorf("relationship requires an id and a character")
	}
	if r.StoryID == "" || r.BranchID == "" {
		return fmt.Errorf("relationship %s requires a story and branch", r.ID)
	}
	if !r.Kind.Valid() {
		return fmt.Errorf("relationship %s has an invalid kind: %q", r.ID, r.Kind)
	}
	if r.Statement == "" {
		return fmt.Errorf("relationship %s has no statement", r.ID)
	}
	if r.WorldTime == "" {
		return fmt.Errorf("relationship %s has no world time", r.ID)
	}
	// A bond, and the ending of one, is something that was said and answered.
	// A negative turn needs the one exchange that broke it; a positive or
	// romantic one needs both halves.
	if r.ExpressedBy == "" {
		return fmt.Errorf("relationship %s cites no expression; a relationship is established by what was said", r.ID)
	}
	if !r.Kind.Negative() && r.AnsweredBy == "" {
		return fmt.Errorf("relationship %s cites no answer; %q needs both an expression and a response", r.ID, r.Kind)
	}
	return nil
}

// ApplyAffinities merges settled changes into the record set.
//
// It is the single place the "settle a fact once" rule lives. A change whose
// settlement key is already present for the same character is dropped, so
// thanking someone again, in another window or on another day, adds nothing.
// A 0 change is never recorded: it says nothing happened, and keeping it would
// stop the same fact from ever being judged later.
func ApplyAffinities(existing []AffinityChange, projected []AffinityChange) []AffinityChange {
	out := make([]AffinityChange, 0, len(existing)+len(projected))
	seenID := make(map[string]bool, len(existing)+len(projected))
	seenKey := make(map[string]bool, len(existing)+len(projected))
	for _, change := range existing {
		change = change.Normalize()
		if change.ID == "" || seenID[change.ID] {
			continue
		}
		seenID[change.ID] = true
		seenKey[change.CharacterID+"\x00"+change.SettlementKey()] = true
		out = append(out, change)
	}
	for _, change := range projected {
		change = change.Normalize()
		if change.ID == "" || seenID[change.ID] || change.Delta == 0 {
			continue
		}
		key := change.CharacterID + "\x00" + change.SettlementKey()
		if seenKey[key] {
			continue
		}
		seenID[change.ID] = true
		seenKey[key] = true
		out = append(out, change)
	}
	return out
}

// ApplyRelationships merges formal relationship changes. A record already on
// the journal is not written twice, and two records of the same kind for the
// same person in one turn collapse to the first.
func ApplyRelationships(existing []Relationship, projected []Relationship) []Relationship {
	out := make([]Relationship, 0, len(existing)+len(projected))
	seen := make(map[string]bool, len(existing)+len(projected))
	for _, relationship := range existing {
		relationship = relationship.Normalize()
		if relationship.ID == "" || seen[relationship.ID] {
			continue
		}
		seen[relationship.ID] = true
		out = append(out, relationship)
	}
	for _, relationship := range projected {
		relationship = relationship.Normalize()
		if relationship.ID == "" || seen[relationship.ID] {
			continue
		}
		seen[relationship.ID] = true
		out = append(out, relationship)
	}
	return out
}

// CurrentAffinity reads the value one character currently holds. No changes
// means no judgement was ever formed, which is 0 and not a missing value.
func CurrentAffinity(characterID string, changes []AffinityChange) int {
	characterID = strings.TrimSpace(characterID)
	value := 0
	for _, change := range changes {
		change = change.Normalize()
		if change.CharacterID == characterID {
			value = change.After
		}
	}
	return value
}

// AffinitySettled reports whether a fact was already settled for this character.
// It is the read the projector takes before accepting a second settlement.
func AffinitySettled(characterID, key string, changes []AffinityChange) bool {
	characterID = strings.TrimSpace(characterID)
	for _, change := range changes {
		change = change.Normalize()
		if change.CharacterID == characterID && change.SettlementKey() == key {
			return true
		}
	}
	return false
}

// AffinityRestated reports whether this fact is already settled on the branch,
// either word for word or in different words. It is the check a projection uses
// before scoring anything; AffinitySettled stays as the exact-key form so a
// record written with a named basis_ref is still matched by its reference.
func AffinityRestated(change AffinityChange, existing []AffinityChange) bool {
	for _, prior := range existing {
		if change.Restates(prior) {
			return true
		}
	}
	return false
}

// RecentAffinityChanges returns one character's most recent settlements,
// oldest first, bounded to the last n. The character's own context and the
// window both read it: the context so the model does not count the same fact
// again, the window so the player can see why a number moved.
func RecentAffinityChanges(characterID string, changes []AffinityChange, limit int) []AffinityChange {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return nil
	}
	out := make([]AffinityChange, 0, len(changes))
	for _, change := range changes {
		change = change.Normalize()
		if change.CharacterID == characterID && change.Delta != 0 {
			out = append(out, change)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// CurrentRelationship returns the most recent formal relationship for one
// character. Later records replace earlier ones: a friendship that became a
// partnership is one current relationship with a history, not two live ones.
func CurrentRelationship(characterID string, relationships []Relationship) (Relationship, bool) {
	characterID = strings.TrimSpace(characterID)
	var current Relationship
	found := false
	for _, relationship := range relationships {
		relationship = relationship.Normalize()
		if relationship.CharacterID != characterID {
			continue
		}
		current = relationship
		found = true
	}
	return current, found
}
