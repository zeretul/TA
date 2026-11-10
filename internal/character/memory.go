package character

import (
	"fmt"
	"strings"
)

// KnowledgeNature records how a character came to know something. It is not
// decoration: a character may state an experienced fact as fact, must hedge
// hearsay, and may not treat their own conjecture as established. Collapsing
// these into "known" is what lets a rumour harden into truth after two turns.
type KnowledgeNature string

const (
	// NatureExperienced is something the character personally perceived.
	NatureExperienced KnowledgeNature = "experienced"
	// NatureTold is something a specific person told them, first hand.
	NatureTold KnowledgeNature = "told"
	// NatureHearsay is second-hand or unattributed talk.
	NatureHearsay KnowledgeNature = "hearsay"
	// NatureConjecture is the character's own inference. It may be wrong, and
	// must never be presented as established fact.
	NatureConjecture KnowledgeNature = "conjecture"
	// NaturePromise is an undertaking the character made or received. It is
	// binding on the character's behaviour without being a fact about the world.
	NaturePromise KnowledgeNature = "promise"
	// NatureSecret is the character's own private knowledge. They may hold it,
	// and may lie about it, but it does not become public by being recorded.
	NatureSecret KnowledgeNature = "secret"
)

func (n KnowledgeNature) Valid() bool {
	switch n {
	case NatureExperienced, NatureTold, NatureHearsay, NatureConjecture, NaturePromise, NatureSecret:
		return true
	default:
		return false
	}
}

// Certainty is how firmly the character may assert a record. It exists so the
// prompt can say "you believe" rather than "you know" without the model having
// to re-derive that from the nature each time.
func (n KnowledgeNature) Certainty() string {
	switch n {
	case NatureExperienced, NatureSecret:
		return "certain"
	case NatureTold, NaturePromise:
		return "relayed"
	default:
		return "uncertain"
	}
}

// KnowledgeAudience is who else could learn this record from the same event.
// It is separate from the nature: a secret witnessed by two people is still
// shared knowledge between those two.
type KnowledgeAudience string

const (
	// AudienceWorld means the event was public within its scene and any
	// character present may hold it.
	AudienceWorld KnowledgeAudience = "world"
	// AudienceParticipants means only the listed participants hold it. A private
	// window conversation is participants-only because of where it happened.
	AudienceParticipants KnowledgeAudience = "participants"
	// AudienceSingle means only the subject holds it.
	AudienceSingle KnowledgeAudience = "single"
)

func (a KnowledgeAudience) Valid() bool {
	switch a {
	case AudienceWorld, AudienceParticipants, AudienceSingle:
		return true
	default:
		return false
	}
}

// KnowledgeRecord is one thing a character knows, together with where it came
// from. Every record carries its provenance because a record without a source
// cannot be re-checked, retracted, or distinguished from a fabrication.
type KnowledgeRecord struct {
	// Visual is a source-bound observation, never an image-display permission.
	Visual      *VisualObservation `json:"visual,omitempty"`
	ID          string             `json:"id"`
	CharacterID string             `json:"character_id"`
	Statement   string             `json:"statement"`
	Nature      KnowledgeNature    `json:"nature"`
	Audience    KnowledgeAudience  `json:"audience"`

	// Provenance. At least one of SourceEventID, SourceExchangeID and
	// SourceTurnID must be present: an unattributable record is not memory, it
	// is invention.
	SourceEventID    string `json:"source_event_id,omitempty"`
	SourceExchangeID string `json:"source_exchange_id,omitempty"`
	SourceTurnID     string `json:"source_turn_id,omitempty"`
	// LearnedFrom names the character who told them, for told/hearsay records.
	LearnedFrom string `json:"learned_from,omitempty"`

	// WorldTime is when the character learned it, not when the player read it.
	WorldTime string `json:"world_time"`
	// FormalCommitID is the canonical journal position that made this record
	// real. Records are only ever created at a formal commit, so a retried or
	// replayed turn cannot double-write one.
	FormalCommitID string `json:"formal_commit_id,omitempty"`

	// Holders lists the characters entitled to this record when the audience is
	// participants-only. The subject is always a holder.
	Holders []string `json:"holders,omitempty"`

	// Supersedes and Retracts point forward, at the earlier record this one
	// replaces or disproves. SupersededBy and RetractedBy point back, and are
	// written by ApplyKnowledge onto the earlier record. Both directions exist
	// so a character's belief evolves instead of silently accumulating
	// contradictions, and so the earlier record's own text stays immutable.
	Supersedes   string `json:"supersedes,omitempty"`
	Retracts     string `json:"retracts,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
	RetractedBy  string `json:"retracted_by,omitempty"`
}

func (r KnowledgeRecord) Normalize() KnowledgeRecord {
	if r.Visual != nil {
		v := *r.Visual
		r.Visual = &v
	}
	r.ID = strings.TrimSpace(r.ID)
	r.CharacterID = strings.TrimSpace(r.CharacterID)
	r.Statement = strings.TrimSpace(r.Statement)
	r.SourceEventID = strings.TrimSpace(r.SourceEventID)
	r.SourceExchangeID = strings.TrimSpace(r.SourceExchangeID)
	r.SourceTurnID = strings.TrimSpace(r.SourceTurnID)
	r.LearnedFrom = strings.TrimSpace(r.LearnedFrom)
	r.WorldTime = strings.TrimSpace(r.WorldTime)
	r.FormalCommitID = strings.TrimSpace(r.FormalCommitID)
	r.Supersedes = strings.TrimSpace(r.Supersedes)
	r.Retracts = strings.TrimSpace(r.Retracts)
	r.SupersededBy = strings.TrimSpace(r.SupersededBy)
	r.RetractedBy = strings.TrimSpace(r.RetractedBy)
	r.Holders = normalizeIDs(append(append([]string{}, r.Holders...), r.CharacterID))
	return r
}

func (r KnowledgeRecord) Validate() error {
	r = r.Normalize()
	if r.ID == "" || r.CharacterID == "" {
		return fmt.Errorf("knowledge record requires an id and a character id")
	}
	if r.Statement == "" {
		return fmt.Errorf("knowledge record %s has no statement", r.ID)
	}
	if !r.Nature.Valid() {
		return fmt.Errorf("knowledge record %s has an invalid nature: %q", r.ID, r.Nature)
	}
	if !r.Audience.Valid() {
		return fmt.Errorf("knowledge record %s has an invalid audience: %q", r.ID, r.Audience)
	}
	if r.SourceEventID == "" && r.SourceExchangeID == "" && r.SourceTurnID == "" {
		return fmt.Errorf("knowledge record %s has no source", r.ID)
	}
	if r.WorldTime == "" {
		return fmt.Errorf("knowledge record %s has no world time", r.ID)
	}
	if r.Nature == NatureTold && r.LearnedFrom == "" {
		return fmt.Errorf("a told record %s must name who told the character", r.ID)
	}
	return nil
}

// Live reports whether the record is still the character's current belief. A
// superseded or retracted record is retained for history but must not be
// prepared as present knowledge.
func (r KnowledgeRecord) Live() bool {
	return r.SupersededBy == "" && r.RetractedBy == ""
}

// HolderCanKnow reports whether one character is entitled to a record. This is
// the retrieval gate: it runs before any material is handed to the model, so a
// character cannot be asked about a conversation they were not part of and
// then plausibly answer.
func HolderCanKnow(record KnowledgeRecord, characterID string) bool {
	record = record.Normalize()
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return false
	}
	if record.CharacterID == characterID {
		return true
	}
	switch record.Audience {
	case AudienceParticipants:
		// A private conversation is participants-only because of where it
		// happened. An independent window does not make it confidential, and it
		// does not make it public either.
		return ContainsID(record.Holders, characterID)
	default:
		// A world or single record belongs to the character it was written for.
		// A character who also witnessed a public event holds their own record,
		// projected from the same commit, rather than reading someone else's.
		return false
	}
}

// KnowledgeView is what one character may currently be told. It is assembled
// per request and never cached across characters, because the whole point is
// that two characters standing in the same room get different material.
type KnowledgeView struct {
	CharacterID string            `json:"character_id"`
	Records     []KnowledgeRecord `json:"records"`
}

// BuildKnowledgeView filters a character's records down to live, permissible
// ones, most recent first, bounded to a limit. Ordering is by world time then
// record ID so a rebuild produces the same view.
func BuildKnowledgeView(characterID string, records []KnowledgeRecord, limit int) KnowledgeView {
	characterID = strings.TrimSpace(characterID)
	view := KnowledgeView{CharacterID: characterID}
	if characterID == "" {
		return view
	}
	for _, record := range records {
		record = record.Normalize()
		if !record.Live() || !HolderCanKnow(record, characterID) {
			continue
		}
		view.Records = append(view.Records, record)
	}
	sortKnowledge(characterID, view.Records)
	if limit > 0 && len(view.Records) > limit {
		view.Records = view.Records[:limit]
	}
	return view
}

func sortKnowledge(characterID string, records []KnowledgeRecord) {
	// Newest world time first; own records before others' at the same time;
	// then by ID for a stable rebuild.
	less := func(i, j int) bool {
		a, b := records[i], records[j]
		if a.WorldTime != b.WorldTime {
			return a.WorldTime > b.WorldTime
		}
		aOwn := a.CharacterID == characterID
		bOwn := b.CharacterID == characterID
		if aOwn != bOwn {
			return aOwn
		}
		return a.ID < b.ID
	}
	sortSlice(records, less)
}

// sortSlice is a tiny insertion sort. Knowledge views are bounded lists, and
// keeping the ordering inline avoids pulling a comparator closure into a
// package whose whole job is to be obvious.
func sortSlice(records []KnowledgeRecord, less func(i, j int) bool) {
	for i := 1; i < len(records); i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			records[j], records[j-1] = records[j-1], records[j]
		}
	}
}
