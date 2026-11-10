package character

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

// The draft types below are what a model turn produces: facts about what
// characters said and learned, with no IDs, no audience and no timestamps. The
// backend assigns identity, resolves who could perceive it, and stamps the
// world time and commit position. A model that supplied those itself would be
// able to invent a witness list.

type ExchangeDraft struct {
	CharacterID string  `json:"character_id"`
	Speaker     Speaker `json:"speaker"`
	Channel     Channel `json:"channel"`
	Text        string  `json:"text"`
	// SceneNode optionally overrides the turn's scene for this line.
	SceneNode string `json:"scene_node,omitempty"`
	// GroupID files this line in a group. It is required when Channel is
	// ChannelGroup and refused otherwise, so a line cannot be half in a group.
	GroupID string `json:"group_id,omitempty"`
	// AuthorID names who spoke a group line. For a character line it is the
	// speaker's own id; the projection fills it in rather than trusting a draft
	// to say it, because a model that chose the author could attribute its words
	// to anyone.
	AuthorID string `json:"author_id,omitempty"`
	// AddressedTo is the one person the line was actually aimed at. It is the
	// only way a committed exchange records a direction, since CharacterID
	// names the speaker for a character's own line.
	AddressedTo string `json:"addressed_to,omitempty"`
	// CooperationDecision is only a response candidate. Projection preserves
	// its real speaker and source; the story decides whether it is usable consent.
	CooperationDecision   *CooperationDecision   `json:"cooperation_decision,omitempty"`
	ParticipationDecision *ParticipationDecision `json:"participation_decision,omitempty"`
}

type KnowledgeDraft struct {
	Visual      *VisualObservationDraft `json:"visual,omitempty"`
	CharacterID string                  `json:"character_id"`
	Statement   string                  `json:"statement"`
	Nature      KnowledgeNature         `json:"nature"`
	Audience    KnowledgeAudience       `json:"audience"`
	// Channel is where the character came to know it. It resolves the audience
	// when the draft does not state one, so a fact learned in a private
	// conversation is not filed as public merely because the audience was
	// omitted.
	Channel Channel `json:"channel,omitempty"`
	// LearnedFrom names the character who told them, for told records.
	LearnedFrom string `json:"learned_from,omitempty"`
	// Holders names the participants for participants-only records. The subject
	// is always included.
	Holders []string `json:"holders,omitempty"`
	// Supersedes and Retracts point at earlier records by ID. They are how a
	// belief evolves instead of accumulating contradictions.
	Supersedes string `json:"supersedes,omitempty"`
	Retracts   string `json:"retracts,omitempty"`
}

type MatterDraft struct {
	AfterSeconds        int64        `json:"after_seconds,omitempty" jsonschema_description:"仅本人本轮明确约定的最早复查/回应间隔（游戏秒），用于新建普通开放promise/action；需source_exchange_index。不是截止时限或自动履约。"`
	SourceExchangeIndex int          `json:"source_exchange_index,omitempty" jsonschema_description:"本轮exchanges中本人实际作出延迟约定、或到时确认完成的1起算序号。不能引用主角的期待；放弃可不填。"`
	CharacterID         string       `json:"character_id"`
	Kind                MatterKind   `json:"kind"`
	Status              MatterStatus `json:"status"`
	Statement           string       `json:"statement"`
	Counterparty        string       `json:"counterparty,omitempty"`
	Deadline            string       `json:"deadline,omitempty"`
	// Resolves names an existing matter this draft settles. Settling an already
	// settled matter is a no-op rather than a second settlement.
	Resolves string `json:"resolves,omitempty"`
	// Letter is set only by the app for an explicit player action. It is kept
	// out of the model-facing facts schema; projectMatter stamps the formal
	// branch clock and derives the earliest delivery instant.
	Letter *LetterDeliveryDraft `json:"-"`
	// Cooperation is set only by the dedicated application path after checking
	// real consent and shared discoveries. Model-supplied promises cannot set it.
	Cooperation *CooperationTerms `json:"-"`
	// LetterReply records the words of a written reply, never an oral answer.
	LetterReply *LetterReply `json:"letter_reply,omitempty"`
}

type LetterDeliveryDraft struct {
	Body                 string
	DeliveryAfterSeconds int64
}

// MessageDraft is an incoming message a character sent on their own initiative.
//
// It is a draft because the backend owns what a model must not choose: the
// message's identity, its original in-world send time, and the commit position
// it is caused by. A model that supplied those itself could backdate an
// invitation or invent a reason for contacting the player.
type MessageDraft struct {
	CharacterID string `json:"character_id"`
	Text        string `json:"text"`
	// Basis is why this message exists at all: the committed event, appointment
	// or circumstance that caused it. A message without one would be a character
	// contacting the player for no reason, which is the failure mode of
	// messaging on a fixed schedule.
	Basis string `json:"basis"`
	// Deadline is the in-fiction time the message's matter lapses, when the
	// message named one. It is never rewritten once committed, so an old
	// invitation still reports its original deadline.
	Deadline string `json:"deadline,omitempty"`
}

// AffinityDraft is a proposed change of feeling toward the protagonist.
//
// The model proposes the scale and the size; the backend decides whether the
// proposal is legal, what the value actually becomes, and whether the fact was
// already counted. A draft that names a scale but an out-of-range size is
// rejected rather than clamped, because a model that asked for +40 for a
// greeting has misunderstood what happened, and a silent clamp would record a
// settlement nobody intended.
type AffinityDraft struct {
	CharacterID string       `json:"character_id"`
	Kind        AffinityKind `json:"kind"`
	Delta       int          `json:"delta"`
	// Basis is the one fact this settlement is for. It is the settlement key
	// when no committed record is named, so restating it adds nothing.
	Basis string `json:"basis"`
	// BasisRef optionally names the committed record the fact already exists
	// as. When set it is the exact settlement key.
	BasisRef string `json:"basis_ref,omitempty"`
}

// RelationshipDraft is a proposed formal change of relationship. It cites the
// lines that expressed and answered it by their position in this turn's
// exchanges, because a relationship is established by what was said and the
// backend mints the exchange ids itself.
type RelationshipDraft struct {
	CharacterID string           `json:"character_id"`
	Kind        RelationshipKind `json:"kind"`
	Statement   string           `json:"statement"`
	// ExpressedByIndex and AnsweredByIndex are 1-based positions in this turn's
	// Exchanges. Both must exist and belong to this character.
	ExpressedByIndex int `json:"expressed_by_index"`
	AnsweredByIndex  int `json:"answered_by_index"`
}

// TurnCharacterFacts is one committed turn's character-side outcome.
type TurnCharacterFacts struct {
	Exchanges     []ExchangeDraft     `json:"exchanges,omitempty"`
	Knowledge     []KnowledgeDraft    `json:"knowledge,omitempty"`
	Matters       []MatterDraft       `json:"matters,omitempty"`
	Messages      []MessageDraft      `json:"messages,omitempty"`
	Affinities    []AffinityDraft     `json:"affinities,omitempty"`
	Relationships []RelationshipDraft `json:"relationships,omitempty"`
}

func (f TurnCharacterFacts) Empty() bool {
	return len(f.Exchanges) == 0 && len(f.Knowledge) == 0 && len(f.Matters) == 0 && len(f.Messages) == 0 &&
		len(f.Affinities) == 0 && len(f.Relationships) == 0
}

// ProjectionContext is the backend-owned identity a projection stamps onto
// every record. None of it comes from the model.
type ProjectionContext struct {
	// VisualContext is supplied only by the formal main-turn commit path.
	VisualContext *VisualObservationContext
	StoryID       string
	BranchID      string
	TurnID        string
	WorldTime     string
	// CommitID is the canonical commit position. Records are keyed by it, so a
	// replayed or retried turn cannot write the same fact twice.
	CommitID string
	// ClockSeconds is the branch's formal instant at this commit, the same
	// number the quest and cultivation rules stamp from. A deadline that states
	// an interval is bound to this reading, because the display text beside it
	// is a description and only the clock is arithmetic. Zero means the caller
	// has no clock, and then no deadline is bound: an unbound record keeps its
	// text and stays uncertain rather than being guessed into expiring.
	ClockSeconds int64
	// Calendar is the year length the story froze, which is what lets a deadline
	// that states an absolute position — 第3日 酉时 — be placed on the same axis
	// instead of being ordered as text. A zero value means the story froze none,
	// and then absolute labels stay unbound exactly as before.
	Calendar CalendarUnits
	// Scene is where the turn happened. It resolves audience for every
	// exchange, which is why the model cannot choose its own witnesses.
	Scene Scene
	// KnownCharacters is the set of characters that exist in this story. A
	// draft naming anyone else is rejected rather than silently creating a
	// character that no one has ever met.
	KnownCharacters []string
	// Groups is the membership this projection resolves group audiences against,
	// keyed by group id. A group line whose group is absent is refused: its
	// audience could not be resolved honestly.
	Groups []Group
	// ExistingAffinities is what this branch has already settled. A draft whose
	// fact is already on this list is dropped rather than counted twice, and the
	// value it produces is computed from here, not from what the model believes.
	ExistingAffinities []AffinityChange
	// ExistingRelationships is the branch's formal relationship history. A
	// relationship draft is checked against the current one.
	ExistingRelationships []Relationship
	// ExistingMatters supplies the original agreement for maturity checks.
	ExistingMatters []Matter
	// NextID mints record IDs. Injected so a caller can make projection
	// deterministic in tests.
	NextID func(prefix string) string
}

// Projected is the validated, identity-stamped result of one turn.
type Projected struct {
	Exchanges     []Exchange        `json:"exchanges,omitempty"`
	Knowledge     []KnowledgeRecord `json:"knowledge,omitempty"`
	Matters       []Matter          `json:"matters,omitempty"`
	Messages      []Message         `json:"messages,omitempty"`
	Groups        []Group           `json:"groups,omitempty"`
	Affinities    []AffinityChange  `json:"affinities,omitempty"`
	Relationships []Relationship    `json:"relationships,omitempty"`
	// IntimacyActivities carries the activity records. They are not projected
	// from drafts: an activity's answers and roster are decided by the player's
	// own actions, so the caller states them and this only carries them through.
	IntimacyActivities []IntimacyActivity `json:"intimacy_activities,omitempty"`
}

func (p Projected) Empty() bool {
	return len(p.Exchanges) == 0 && len(p.Knowledge) == 0 && len(p.Matters) == 0 && len(p.Messages) == 0 &&
		len(p.Groups) == 0 && len(p.Affinities) == 0 && len(p.Relationships) == 0 && len(p.IntimacyActivities) == 0
}

// Project turns one turn's drafts into durable records. It rejects anything it
// cannot honestly attribute: an unknown character, a private line whose
// audience cannot be resolved, a record with no source. Rejection is an error
// rather than a silent drop because a dropped fact is a character who forgets.
func Project(ctx ProjectionContext, facts TurnCharacterFacts) (Projected, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	ctx.BranchID = strings.TrimSpace(ctx.BranchID)
	ctx.WorldTime = strings.TrimSpace(ctx.WorldTime)
	ctx.CommitID = strings.TrimSpace(ctx.CommitID)
	if ctx.StoryID == "" || ctx.BranchID == "" {
		return Projected{}, fmt.Errorf("character projection requires a story and branch")
	}
	if ctx.WorldTime == "" {
		return Projected{}, fmt.Errorf("character projection requires a world time")
	}
	if ctx.NextID == nil {
		return Projected{}, fmt.Errorf("character projection requires an id generator")
	}
	known := make(map[string]bool, len(ctx.KnownCharacters))
	for _, id := range ctx.KnownCharacters {
		known[strings.TrimSpace(id)] = true
	}
	if ctx.VisualContext != nil && ctx.VisualContext.ObserverID != "" {
		known[ctx.VisualContext.ObserverID] = true
	}
	groups := make(map[string]Group, len(ctx.Groups))
	for _, group := range ctx.Groups {
		group = group.Normalize()
		if group.ID != "" {
			groups[group.ID] = group
		}
	}
	scene := ctx.Scene.Normalize()

	var out Projected
	for index, draft := range facts.Exchanges {
		exchange, err := projectExchange(ctx, scene, known, groups, draft)
		if err != nil {
			return Projected{}, fmt.Errorf("exchange %d: %w", index, err)
		}
		out.Exchanges = append(out.Exchanges, exchange)
	}
	for index, draft := range facts.Knowledge {
		record, err := projectKnowledge(ctx, scene, known, draft)
		if err != nil {
			return Projected{}, fmt.Errorf("knowledge %d: %w", index, err)
		}
		if draft.Visual != nil {
			visual, err := projectVisualObservation(ctx, scene, draft, out.Exchanges)
			if err != nil {
				return Projected{}, fmt.Errorf("knowledge %d visual observation: %w", index, err)
			}
			record.Visual = visual
			record.SourceExchangeID = visual.ContactExchangeID
		}
		out.Knowledge = append(out.Knowledge, record)
	}
	for index, draft := range facts.Matters {
		matter, err := projectMatter(ctx, known, out.Exchanges, draft)
		if err != nil {
			return Projected{}, fmt.Errorf("matter %d: %w", index, err)
		}
		out.Matters = append(out.Matters, matter)
	}
	for index, draft := range facts.Messages {
		message, err := projectMessage(ctx, known, draft)
		if err != nil {
			return Projected{}, fmt.Errorf("message %d: %w", index, err)
		}
		out.Messages = append(out.Messages, message)
	}
	// Affinity is projected after exchanges because a relationship cites the
	// lines that expressed and answered it, and those ids are minted above.
	if len(facts.Affinities) > 0 {
		affinities, err := projectAffinities(ctx, known, facts.Affinities, out.Exchanges)
		if err != nil {
			return Projected{}, err
		}
		out.Affinities = affinities
	}
	if len(facts.Relationships) > 0 {
		relationships, err := projectRelationships(ctx, known, facts.Relationships, out.Exchanges, out.Affinities, facts.Affinities)
		if err != nil {
			return Projected{}, err
		}
		out.Relationships = relationships
	}
	return out, nil
}

// projectAffinities turns proposed feelings into settled changes.
//
// The rule the design fixes is that one real fact is settled once. That is why
// a draft carries a basis at all: the basis is the settlement key, and a fact
// already on this branch is dropped rather than counted again however many
// windows, days or turns it was proposed in. A 0 draft is legal and settles
// nothing, which is the honest answer for ordinary courtesy.
//
// The value is computed here from the branch's own history. The model supplies
// a scale and a size within that scale; it never supplies the resulting number.
func projectAffinities(ctx ProjectionContext, known map[string]bool, drafts []AffinityDraft, exchanges []Exchange) ([]AffinityChange, error) {
	out := make([]AffinityChange, 0, len(drafts))
	for index, draft := range drafts {
		characterID := strings.TrimSpace(draft.CharacterID)
		if characterID == "" {
			return nil, fmt.Errorf("affinity %d: a change must name the character it belongs to", index)
		}
		if !known[characterID] {
			return nil, fmt.Errorf("affinity %d: unknown character %q", index, characterID)
		}
		basis := strings.Join(strings.Fields(strings.TrimSpace(draft.Basis)), " ")
		if basis == "" {
			return nil, fmt.Errorf("affinity %d: a change must state the fact it settles", index)
		}
		min, max, ok := draft.Kind.DeltaRange()
		if !ok {
			return nil, fmt.Errorf("affinity %d: unknown scale %q", index, draft.Kind)
		}
		if draft.Delta < min || draft.Delta > max {
			return nil, fmt.Errorf("affinity %d: %q allows %d to %d, not %d", index, draft.Kind, min, max, draft.Delta)
		}
		change := AffinityChange{
			ID:            ctx.NextID("aff"),
			StoryID:       ctx.StoryID,
			BranchID:      ctx.BranchID,
			CharacterID:   characterID,
			Kind:          draft.Kind,
			Delta:         draft.Delta,
			Basis:         basis,
			BasisRef:      strings.TrimSpace(draft.BasisRef),
			WorldTime:     ctx.WorldTime,
			TurnID:        ctx.TurnID,
			SourceEventID: ctx.CommitID,
		}
		if change.Delta == 0 {
			// Nothing happened. It is not recorded, so the same fact can still be
			// judged later when there is something to judge.
			continue
		}
		// A named record must be one of this turn's own committed exchanges, or an
		// earlier record already on the branch. The second case is checked by the
		// caller's existing set; here a reference that names nothing this turn is
		// refused rather than stored as a key that can never match again.
		if change.BasisRef != "" && !affinityRefExists(change.BasisRef, exchanges) {
			return nil, fmt.Errorf("affinity %d: basis_ref %q is not a committed exchange in this turn", index, change.BasisRef)
		}
		key := change.SettlementKey()
		if AffinitySettled(characterID, key, ctx.ExistingAffinities) || AffinityRestated(change, ctx.ExistingAffinities) {
			// Already counted. The same fact proposed again is a no-op, which is
			// what stops a repeated gift or a second thank-you from scoring.
			continue
		}
		// A repeat within this same turn is also dropped: a model that submits the
		// same basis twice is describing one fact, not two.
		if AffinitySettled(characterID, key, out) || AffinityRestated(change, out) {
			continue
		}
		before := CurrentAffinity(characterID, append(append([]AffinityChange(nil), ctx.ExistingAffinities...), out...))
		after := ClampAffinity(before + change.Delta)
		change.Before = before
		change.After = after
		// The recorded delta is what actually happened, which can be smaller than
		// proposed at the cap. Storing the proposed size would make the record
		// disagree with its own before and after.
		change.Delta = after - before
		change.Band = BandFor(after)
		if err := change.Validate(); err != nil {
			return nil, err
		}
		out = append(out, change.Normalize())
	}
	return out, nil
}

// affinityRefExists reports whether a reference names an exchange in this turn.
// An earlier committed exchange is not in that list; such a reference is left
// for the caller's existing set to recognise.
func affinityRefExists(ref string, exchanges []Exchange) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	for _, exchange := range exchanges {
		if exchange.ID == ref {
			return true
		}
	}
	return false
}

// projectRelationships turns a proposed relationship change into a record.
//
// A relationship is not a consequence of a number. It needs a real expression
// and a real answer, both cited from this turn's own exchanges, and a positive
// relationship additionally needs an affinity floor. The floor is a requirement
// and never a trigger: crossing it creates nothing.
func projectRelationships(ctx ProjectionContext, known map[string]bool, drafts []RelationshipDraft, exchanges []Exchange, affinities []AffinityChange, affinityDrafts []AffinityDraft) ([]Relationship, error) {
	out := make([]Relationship, 0, len(drafts))
	settled := append([]Relationship(nil), ctx.ExistingRelationships...)
	for index, draft := range drafts {
		characterID := strings.TrimSpace(draft.CharacterID)
		if characterID == "" {
			return nil, fmt.Errorf("relationship %d: a change must name the character it belongs to", index)
		}
		if !known[characterID] {
			return nil, fmt.Errorf("relationship %d: unknown character %q", index, characterID)
		}
		if !draft.Kind.Valid() {
			return nil, fmt.Errorf("relationship %d: unknown kind %q", index, draft.Kind)
		}
		statement := strings.Join(strings.Fields(strings.TrimSpace(draft.Statement)), " ")
		if statement == "" {
			return nil, fmt.Errorf("relationship %d: a change must state what changed", index)
		}
		expressed, err := exchangeAt(exchanges, draft.ExpressedByIndex, characterID)
		if err != nil {
			return nil, fmt.Errorf("relationship %d: %w", index, err)
		}
		if err := validateRelationshipExchange(expressed, characterID); err != nil {
			return nil, fmt.Errorf("relationship %d: %w", index, err)
		}
		answered := ""
		if draft.AnsweredByIndex > 0 {
			answer, err := exchangeAt(exchanges, draft.AnsweredByIndex, characterID)
			if err != nil {
				return nil, fmt.Errorf("relationship %d: %w", index, err)
			}
			// Compare positions, not generated IDs: submission-time projection
			// intentionally uses the same sentinel identity for each exchange.
			if draft.AnsweredByIndex <= draft.ExpressedByIndex {
				return nil, fmt.Errorf("relationship %d: an answer must follow a different offered declaration", index)
			}
			if expressed.Speaker == answer.Speaker {
				return nil, fmt.Errorf("relationship %d: the protagonist and character must each speak for themselves", index)
			}
			if err := validateRelationshipExchange(answer, characterID); err != nil {
				return nil, fmt.Errorf("relationship %d: %w", index, err)
			}
			answered = answer.ID
		}
		if !draft.Kind.Negative() && answered == "" {
			return nil, fmt.Errorf("relationship %d: %q needs both the words that offered it and the answer that accepted it", index, draft.Kind)
		}
		if minimum, ok := RelationshipMinimum(draft.Kind); ok {
			value := CurrentAffinity(characterID, append(append([]AffinityChange(nil), ctx.ExistingAffinities...), affinities...))
			// A relationship proposed in the same turn as the change that earns it
			// is judged against what that change makes the value become, so the
			// order of the two arrays does not decide whether the turn succeeds.
			if value < minimum {
				return nil, fmt.Errorf("relationship %d: %q needs affinity %d, and this character stands at %d", index, draft.Kind, minimum, value)
			}
		}
		// Earlier changes in this same turn are already part of the standing.
		// Comparing only with the pre-turn state would drop a later parting
		// after reconciliation (or the reconciliation after a parting).
		current, exists := CurrentRelationship(characterID, settled)
		if exists && current.Kind == draft.Kind {
			// The relationship already stands. Repeating it is not a second
			// establishment, and recording it again would make the window's title
			// flicker without anything having changed.
			continue
		}
		relationship := Relationship{
			ID:            ctx.NextID("rel"),
			StoryID:       ctx.StoryID,
			BranchID:      ctx.BranchID,
			CharacterID:   characterID,
			Kind:          draft.Kind,
			Statement:     statement,
			ExpressedBy:   expressed.ID,
			AnsweredBy:    answered,
			WorldTime:     ctx.WorldTime,
			TurnID:        ctx.TurnID,
			SourceEventID: ctx.CommitID,
		}
		if err := relationship.Validate(); err != nil {
			return nil, err
		}
		out = append(out, relationship.Normalize())
		settled = append(settled, relationship.Normalize())
	}
	return out, nil
}

// exchangeAt resolves a 1-based position in this turn's exchanges to one that
// belongs to the named character. Positions are used rather than ids because
// the model is not shown the ids the backend is about to mint, and a
// relationship must cite something that was actually said.
func exchangeAt(exchanges []Exchange, position int, characterID string) (Exchange, error) {
	if position <= 0 || position > len(exchanges) {
		return Exchange{}, fmt.Errorf("cited exchange %d does not exist; this turn has %d", position, len(exchanges))
	}
	exchange := exchanges[position-1]
	if exchange.CharacterID != characterID {
		return Exchange{}, fmt.Errorf("cited exchange %d belongs to %q, not %q", position, exchange.CharacterID, characterID)
	}
	return exchange, nil
}

func projectExchange(ctx ProjectionContext, scene Scene, known map[string]bool, groups map[string]Group, draft ExchangeDraft) (Exchange, error) {
	characterID := strings.TrimSpace(draft.CharacterID)
	groupID := strings.TrimSpace(draft.GroupID)
	if draft.Channel == ChannelGroup {
		if groupID == "" {
			return Exchange{}, fmt.Errorf("a group exchange must name its group")
		}
		group, ok := groups[groupID]
		if !ok {
			return Exchange{}, fmt.Errorf("unknown group %q; a group line cannot be resolved without its membership", groupID)
		}
		if characterID != "" && !ContainsID(group.ActiveMembers(), characterID) {
			return Exchange{}, fmt.Errorf("character %q is not a member of group %q", characterID, groupID)
		}
	} else if groupID != "" {
		return Exchange{}, fmt.Errorf("group %q is named on a %q exchange; only a group line belongs to a group", groupID, draft.Channel)
	}
	if characterID == "" && draft.Channel != ChannelGroup {
		return Exchange{}, fmt.Errorf("an exchange must name the character it belongs to")
	}
	if characterID != "" && !known[characterID] {
		return Exchange{}, fmt.Errorf("unknown character %q; register a profile before speaking as them", characterID)
	}
	// A group line's author is the person who spoke, and the backend owns it. A
	// draft cannot attribute a line to someone else.
	authorID := ""
	if draft.Channel == ChannelGroup {
		if draft.Speaker == SpeakerCharacter {
			if characterID == "" {
				return Exchange{}, fmt.Errorf("a group line spoken by a character must name them")
			}
			authorID = characterID
		} else if draft.AuthorID != "" {
			return Exchange{}, fmt.Errorf("the player is not a character and cannot be a group line's author")
		}
	}
	text := strings.TrimSpace(draft.Text)
	if text == "" {
		return Exchange{}, fmt.Errorf("an exchange must have text")
	}
	if utf8.RuneCountInString(text) > maxExchangeTextRunes {
		return Exchange{}, fmt.Errorf("an exchange exceeds %d characters", maxExchangeTextRunes)
	}
	sceneNode := strings.TrimSpace(draft.SceneNode)
	if sceneNode == "" {
		sceneNode = scene.NodeID
	}
	audience := scene.AudienceFor(draft.Channel, characterID)
	if draft.Channel == ChannelGroup {
		// A group line reaches the group as it stands now, captured here. It is
		// deliberately not the scene's present list: a remote group and a room full
		// of bystanders are different audiences.
		audience = groups[groupID].AudienceFor(authorID, scene.Present)
	}
	exchange := Exchange{
		ID:                    ctx.NextID("xch"),
		StoryID:               ctx.StoryID,
		BranchID:              ctx.BranchID,
		CharacterID:           characterID,
		Speaker:               draft.Speaker,
		Channel:               draft.Channel,
		GroupID:               groupID,
		AuthorID:              authorID,
		AddressedTo:           strings.TrimSpace(draft.AddressedTo),
		Text:                  text,
		WorldTime:             ctx.WorldTime,
		TurnID:                ctx.TurnID,
		SceneNode:             sceneNode,
		Audience:              audience,
		SourceEventID:         ctx.CommitID,
		CooperationDecision:   draft.CooperationDecision,
		ParticipationDecision: draft.ParticipationDecision,
	}
	if err := exchange.Validate(); err != nil {
		return Exchange{}, err
	}
	return exchange.Normalize(), nil
}

func projectKnowledge(ctx ProjectionContext, scene Scene, known map[string]bool, draft KnowledgeDraft) (KnowledgeRecord, error) {
	characterID := strings.TrimSpace(draft.CharacterID)
	if characterID == "" {
		return KnowledgeRecord{}, fmt.Errorf("a knowledge record must name its holder")
	}
	if !known[characterID] {
		return KnowledgeRecord{}, fmt.Errorf("unknown character %q", characterID)
	}
	statement := strings.TrimSpace(draft.Statement)
	if statement == "" {
		return KnowledgeRecord{}, fmt.Errorf("a knowledge record must state something")
	}
	record := KnowledgeRecord{
		ID:             ctx.NextID("knw"),
		CharacterID:    characterID,
		Statement:      statement,
		Nature:         draft.Nature,
		Audience:       draft.Audience,
		SourceTurnID:   ctx.TurnID,
		SourceEventID:  ctx.CommitID,
		LearnedFrom:    strings.TrimSpace(draft.LearnedFrom),
		WorldTime:      ctx.WorldTime,
		FormalCommitID: ctx.CommitID,
		Supersedes:     strings.TrimSpace(draft.Supersedes),
		Retracts:       strings.TrimSpace(draft.Retracts),
	}
	if !record.Audience.Valid() {
		// The draft omitted an audience. It is resolved from the channel the
		// character learned it on, which the backend owns: a fact picked up in a
		// private conversation is participants-only, not public.
		record.Audience = audienceForKnowledgeChannel(scene, draft.Channel, characterID)
	}
	if draft.Nature == NatureSecret {
		// A secret is held by its subject and, when the draft names specific
		// confidants, by those alone. Audience is backend-owned, so declaring a
		// secret does not make it public: the model cannot widen an audience by
		// labelling something a secret, and the turn is not failed over it.
		record.Audience = AudienceParticipants
	}
	// Holders are resolved after the audience, because the audience is what the
	// drafted list is allowed to narrow.
	record.Holders = resolveKnowledgeHolders(scene, draft.Channel, characterID, record.Audience, draft.Nature, draft.Holders, known)
	if err := record.Validate(); err != nil {
		return KnowledgeRecord{}, err
	}
	record = record.Normalize()
	return record, nil
}

// resolveKnowledgeHolders decides who besides the subject may read a record.
//
// The draft is a claim about who was there, and the scene already answers that
// question, so a drafted list can only narrow it. §5.2 lists 本人未听见或未被告知
// 的他人私聊 as a source a character must not have, and being named in a draft is
// not a channel: a model that lists a confidant in another city would otherwise
// hand them a private moment they never saw. The subject is always kept, because
// 人物可以知道自己的秘密 and a record nobody can read is a fact the story forgot.
//
// A participants-only record whose drafted list names nobody reachable falls
// back to the channel's own audience rather than to the subject alone: a scene
// several people stood in is not a secret, and reading it as one would make the
// narrator describe witnesses who remember nothing.
func resolveKnowledgeHolders(scene Scene, channel Channel, characterID string, audience KnowledgeAudience, nature KnowledgeNature, drafted []string, known map[string]bool) []string {
	reachable := make(map[string]bool)
	for _, id := range scene.AudienceFor(channel, characterID) {
		if known[id] {
			reachable[id] = true
		}
	}
	reachable[characterID] = true
	holders := make([]string, 0, len(drafted)+1)
	for _, id := range drafted {
		id = strings.TrimSpace(id)
		if id == "" || !reachable[id] {
			continue
		}
		holders = append(holders, id)
	}
	holders = normalizeIDs(append(holders, characterID))
	if nature == NatureSecret {
		// A secret belongs to the person whose knowledge it is. Someone the scene
		// put in reach of it may hold it too, but the subject is the floor.
		return holders
	}
	if len(holders) == 1 && audience == AudienceParticipants {
		return normalizeIDs(scene.AudienceFor(channel, characterID))
	}
	return holders
}

// audienceForKnowledgeChannel resolves an omitted audience from where the
// character learned the fact. A scene event is public within its scene, so
// everyone present may hold it; anything said on a private channel or in an
// incoming message reached only the people in that conversation.
func audienceForKnowledgeChannel(scene Scene, channel Channel, characterID string) KnowledgeAudience {
	switch channel {
	case ChannelPrivate, ChannelMessage:
		return AudienceParticipants
	case ChannelScene:
		if len(scene.Normalize().Present) > 0 {
			return AudienceWorld
		}
		return AudienceSingle
	default:
		return AudienceSingle
	}
}

func projectMatter(ctx ProjectionContext, known map[string]bool, exchanges []Exchange, draft MatterDraft) (Matter, error) {
	characterID := strings.TrimSpace(draft.CharacterID)
	if characterID == "" {
		return Matter{}, fmt.Errorf("a matter must name the character it is with")
	}
	if !known[characterID] {
		return Matter{}, fmt.Errorf("unknown character %q", characterID)
	}
	matter := Matter{
		ID:            ctx.NextID("mat"),
		StoryID:       ctx.StoryID,
		BranchID:      ctx.BranchID,
		CharacterID:   characterID,
		Kind:          draft.Kind,
		Status:        draft.Status,
		Statement:     strings.TrimSpace(draft.Statement),
		LetterReply:   draft.LetterReply,
		Cooperation:   draft.Cooperation,
		Counterparty:  strings.TrimSpace(draft.Counterparty),
		Deadline:      strings.TrimSpace(draft.Deadline),
		WorldTime:     ctx.WorldTime,
		TurnID:        ctx.TurnID,
		SourceEventID: ctx.CommitID,
	}
	if err := bindMatterMaturity(ctx, exchanges, draft, &matter); err != nil {
		return Matter{}, err
	}
	if draft.Letter != nil {
		letter := *draft.Letter
		letter.Body = strings.TrimSpace(letter.Body)
		if draft.Kind != MatterAction || matter.Status != MatterOpen || letter.Body == "" || letter.DeliveryAfterSeconds < 24*60*60 || letter.DeliveryAfterSeconds > 30*24*60*60 {
			return Matter{}, fmt.Errorf("a sent letter needs an open action and a bounded body and delivery interval")
		}
		matter.Letter = &LetterDelivery{
			Body: letter.Body, SentAtSeconds: ctx.ClockSeconds,
			EarliestDeliverySeconds: ctx.ClockSeconds + letter.DeliveryAfterSeconds,
		}
	}
	if seconds, ok := BindDeadlineSecondsOnCalendar(matter.Deadline, ctx.ClockSeconds, ctx.Calendar); ok {
		matter.DeadlineSeconds = seconds
	}
	if matter.Status == "" {
		matter.Status = MatterOpen
	}
	// A draft may point at an earlier matter it settles. Resolving is applied by
	// ApplyMatters, which knows the existing records. A draft that resolves
	// something must itself be resolved: an open matter cannot discharge another.
	matter.SettledBy = strings.TrimSpace(draft.Resolves)
	if matter.SettledBy != "" {
		// The reference must at least look like a matter id. The model is shown
		// matters as "[kind] statement" and is never shown an id, so when it is
		// asked for one it tends to write the sentence it means instead. Rejecting
		// that here turns it into a submission diagnostic, which triggers a repair
		// pass; accepting it would persist prose as a link that ApplyMatters then
		// silently drops, leaving the thread open with no one told why.
		if !looksLikeMatterID(matter.SettledBy) {
			return Matter{}, fmt.Errorf("matter %s resolves %q, which is not a matter id", matter.ID, matter.SettledBy)
		}
		if matter.Status == MatterOpen {
			return Matter{}, fmt.Errorf("matter %s resolves %s but is itself unresolved", matter.ID, matter.SettledBy)
		}
	}
	if err := matter.Validate(); err != nil {
		return Matter{}, err
	}
	return matter.Normalize(), nil
}

// looksLikeMatterID reports whether a reference is shaped like an id rather than
// like a sentence about what happened. The check is deliberately loose — it only
// rejects text that could not be an id at all — because it exists to catch a
// model writing prose into a field that wants an identifier, not to constrain
// what an id may look like.
func looksLikeMatterID(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 64 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == ':':
		default:
			// Any other rune — a Han character, a space, punctuation — means this
			// is a sentence, not an identifier.
			return false
		}
	}
	return true
}

// projectMessage stamps an incoming message with the identity and time the
// model must not choose.
//
// The message's ID is derived from its cause and its content rather than minted
// at random. A message is "the same message" when the same character sent the
// same words for the same reason on the same branch, so a replayed or retried
// commit reproduces one record instead of delivering the invitation twice. The
// send time is the turn's own world time, frozen here: it is when the character
// wrote, not when the player next opened the program.
func projectMessage(ctx ProjectionContext, known map[string]bool, draft MessageDraft) (Message, error) {
	characterID := strings.TrimSpace(draft.CharacterID)
	if characterID == "" {
		return Message{}, fmt.Errorf("a message must name the character who sent it")
	}
	if !known[characterID] {
		return Message{}, fmt.Errorf("unknown character %q; register a profile before they can send a message", characterID)
	}
	text := strings.TrimSpace(draft.Text)
	if text == "" {
		return Message{}, fmt.Errorf("a message must have text")
	}
	if utf8.RuneCountInString(text) > maxExchangeTextRunes {
		return Message{}, fmt.Errorf("a message exceeds %d characters", maxExchangeTextRunes)
	}
	basis := strings.TrimSpace(draft.Basis)
	if basis == "" {
		return Message{}, fmt.Errorf("a message must state its basis; messages are caused, not scheduled")
	}
	message := Message{
		ID:           messageID(ctx, characterID, basis, text),
		StoryID:      ctx.StoryID,
		BranchID:     ctx.BranchID,
		CharacterID:  characterID,
		Text:         text,
		Basis:        basis,
		BasisEventID: ctx.CommitID,
		BasisTurnID:  ctx.TurnID,
		WorldTime:    ctx.WorldTime,
		Deadline:     strings.TrimSpace(draft.Deadline),
		Status:       MessageUnread,
	}
	if seconds, ok := BindDeadlineSecondsOnCalendar(message.Deadline, ctx.ClockSeconds, ctx.Calendar); ok {
		message.DeadlineSeconds = seconds
	}
	if err := message.Validate(); err != nil {
		return Message{}, err
	}
	return message.Normalize(), nil
}

// messageID is the stable identity of a caused message. It hashes what makes the
// message that message, so the same character saying the same thing for the same
// reason on the same branch is one record however many times the commit is
// replayed.
func messageID(ctx ProjectionContext, characterID, basis, text string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		ctx.StoryID, ctx.BranchID, characterID, basis, text,
	}, "\x00")))
	return "msg_" + hex.EncodeToString(sum[:16])
}

// ApplyMessages merges newly projected messages into the existing set. A message
// whose ID is already present is dropped, which is the second half of the
// no-duplicate rule: identity makes the record stable, and this makes a replay
// of it a no-op.
func ApplyMessages(existing []Message, projected []Message) []Message {
	out := make([]Message, 0, len(existing)+len(projected))
	seen := make(map[string]bool, len(existing)+len(projected))
	for _, message := range existing {
		message = message.Normalize()
		if message.ID == "" || seen[message.ID] {
			continue
		}
		seen[message.ID] = true
		out = append(out, message)
	}
	for _, message := range projected {
		message = message.Normalize()
		if message.ID == "" || seen[message.ID] {
			continue
		}
		seen[message.ID] = true
		out = append(out, message)
	}
	return out
}

// ApplyMatters merges newly projected matters into the existing record set.
//
// It is the single place the "do not settle the same thing twice" rule lives.
// A draft that restates a thread already on record is dropped, so repeating a
// request the world has answered does not create a second obligation. A draft
// that resolves an earlier matter is itself recorded — a permission and the act
// it permits are separate facts — and settles its target in place.
//
// Resolution is applied in a second pass, after every record from this turn is
// present, so a draft may resolve a matter projected in the same turn and a
// replay settles nothing twice because the target is already resolved.
//
// It cannot fail. A link that names nothing on record is dropped rather than
// refused: the journal is replayed on every read, so an error here would turn
// one bad reference into a story that can never be opened again.
func ApplyMatters(existing []Matter, projected []Matter) []Matter {
	out := make([]Matter, 0, len(existing)+len(projected))
	index := make(map[string]int, len(existing))
	for _, matter := range existing {
		matter = matter.Normalize()
		if _, ok := index[matter.ID]; ok {
			continue
		}
		index[matter.ID] = len(out)
		out = append(out, matter)
	}
	for _, matter := range projected {
		matter = matter.Normalize()
		if _, ok := index[matter.ID]; ok {
			continue
		}
		if findDuplicateMatter(out, matter) >= 0 {
			// The same thread stated again. Keep the original, which carries the
			// earlier provenance.
			continue
		}
		index[matter.ID] = len(out)
		out = append(out, matter)
	}
	for _, matter := range projected {
		matter = matter.Normalize()
		resolveID := strings.TrimSpace(matter.SettledBy)
		if resolveID == "" {
			continue
		}
		// A restated thread was dropped above in favour of the original, which
		// carries the earlier provenance, so a draft naming the restatement still
		// settles the thread it restates.
		target, ok := index[resolveID]
		if !ok {
			// The target is not on record. An unhonoured link is dropped rather
			// than treated as fatal: it joins two matters, and dropping it leaves
			// both of them intact. Returning an error here would make one bad
			// reference on a persisted record fail every later reduction of the
			// journal — every branch, every window read — which is a worse outcome
			// than a thread that simply stays open.
			if self, ok := index[matter.ID]; ok {
				out[self].SettledBy = ""
			}
			continue
		}
		if out[target].Faction != nil || out[target].Status != MatterOpen {
			// Already resolved. A retry, a recovery or a repeated request must
			// not settle it a second time.
			continue
		}
		out[target].Status = matter.Status
		out[target].SettledBy = matter.ID
	}
	return out
}

func findDuplicateMatter(existing []Matter, candidate Matter) int {
	key := matterKey(candidate)
	for i, matter := range existing {
		if matterKey(matter) == key {
			return i
		}
	}
	return -1
}

func matterKey(matter Matter) string {
	if matter.Faction != nil {
		return "faction\x00" + matter.StoryID + "\x00" + matter.Faction.ProjectID
	}
	if matter.Cooperation != nil {
		// Dedicated cooperation is not a prose promise. Its scope has one
		// canonical record, even when later descriptions use different words.
		return strings.Join([]string{
			"cooperation", strings.TrimSpace(matter.StoryID), strings.TrimSpace(matter.BranchID),
			strings.TrimSpace(matter.CharacterID),
			strings.TrimSpace(matter.Cooperation.ProjectID),
			strings.TrimSpace(matter.Cooperation.RealmInstanceID),
		}, "\x00")
	}
	return strings.Join([]string{
		"ordinary",
		strings.TrimSpace(matter.CharacterID),
		string(matter.Kind),
		strings.Join(strings.Fields(strings.ToLower(matter.Statement)), " "),
	}, "\x00")
}

// ApplyKnowledge merges new records and wires supersede/retract links so a
// character's belief evolves rather than contradicting itself. It returns the
// merged set; the caller persists it verbatim.
//
// A newly projected record may carry Supersedes or Retracts naming an earlier
// record; the link is written onto that earlier record here, because the model
// cannot know the stored IDs of what it is replacing.
func ApplyKnowledge(existing []KnowledgeRecord, projected []KnowledgeRecord) []KnowledgeRecord {
	out := make([]KnowledgeRecord, 0, len(existing)+len(projected))
	index := make(map[string]int, len(existing))
	for _, record := range existing {
		record = record.Normalize()
		if _, ok := index[record.ID]; ok {
			continue
		}
		index[record.ID] = len(out)
		out = append(out, record)
	}
	for _, record := range projected {
		record = record.Normalize()
		if record.ID != "" {
			if _, ok := index[record.ID]; ok {
				// A replayed commit position must not duplicate a record.
				continue
			}
		}
		if supersedes := strings.TrimSpace(record.Supersedes); supersedes != "" {
			if target, ok := index[supersedes]; ok {
				out[target].SupersededBy = record.ID
			}
		}
		if retracts := strings.TrimSpace(record.Retracts); retracts != "" {
			if target, ok := index[retracts]; ok {
				out[target].RetractedBy = record.ID
			}
		}
		index[record.ID] = len(out)
		out = append(out, record)
	}
	return out
}
