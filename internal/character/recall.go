package character

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Recall is the read-back path for material that did not fit in the standing
// memory block.
//
// The standing memory block is bounded: it carries the most recent knowledge
// records and nothing else. A character asked about something from long ago
// therefore has no route to it at all — the record is on the journal, but the
// prepared context never saw it, and a window reply is a single tool-less
// generation that cannot go looking. The result is a character who answers
// "I don't remember" about something they plainly lived through.
//
// This pass closes that gap the narrow way: the player's current question is
// used as a retrieval condition over the character's own committed records, and
// the matching ones are read back into the context. It reads the same
// projections preparation already reads — knowledge, exchanges, matters — so
// there is no second store, no index to rebuild, and no per-character process
// holding memory on the side.
type Recall struct {
	Items []RecallItem `json:"items,omitempty"`

	// Considered is how many candidate records were scored. Selected is how many
	// survived. They are recorded so a thin recall is visible as a thin recall
	// rather than read as the character having no past.
	Considered int `json:"considered"`
	Selected   int `json:"selected"`
	// Truncated says the pass itself was cut to its limit, so relevant material
	// exists that this context still does not carry.
	Truncated bool `json:"truncated,omitempty"`
	// Dropped says the budget removed every recalled item. It is distinct from an
	// empty pass: there was material, and the context could not hold it. The
	// rendered block then says so, because a silent gap reads to a model as a
	// record that never existed, and the character asserts they never knew it.
	Dropped bool `json:"dropped,omitempty"`
	// SourceFailed says the underlying read could not be completed. A failed read
	// narrows what the character may claim; it is not evidence that they never
	// knew it, and the rendered block says so.
	SourceFailed bool `json:"source_failed,omitempty"`
}

// RecallItem is one earlier record read back because it bears on the current
// question. It keeps its source and its nature so the material can be
// re-checked, retracted or discounted rather than taken on trust: a recalled
// rumour must still be spoken as a rumour.
type RecallItem struct {
	// Kind is which projection the record came from: knowledge, exchange or
	// matter.
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Text string `json:"text"`
	// Nature and Certainty carry the knowledge record's own provenance. They are
	// empty for exchanges and matters, which are not beliefs.
	Nature    KnowledgeNature `json:"nature,omitempty"`
	Certainty string          `json:"certainty,omitempty"`
	// Source names where the record came from, so a wrong recall is traceable to
	// a record rather than to the character's imagination.
	Source    string `json:"source,omitempty"`
	WorldTime string `json:"world_time,omitempty"`
	// Score is the relevance the question earned this item. It is kept for
	// diagnostics rather than shown to the model.
	Score int `json:"score"`
}

// RecallRequest is everything one recall pass needs. It is data rather than a
// live store handle, so the pass stays pure and cannot reach around the
// caller's scoping.
type RecallRequest struct {
	CharacterID string
	// Question is the player's current line. It is the retrieval condition: with
	// no question there is nothing to recall against, and the pass returns
	// nothing rather than guessing.
	Question string
	Scene    Scene
	// Knowledge, Exchanges and Matters are the character's projections, already
	// scoped to this story and branch by the caller. Knowledge may include
	// records written for other characters; the holder gate is applied here.
	Knowledge []KnowledgeRecord
	Exchanges []Exchange
	Matters   []Matter
	// Limit bounds the recalled items. Zero falls back to the default.
	Limit int
}

// DefaultMaxRecalledItems bounds one recall pass. It is deliberately small: the
// recalled material is there to answer the question being asked, not to
// reconstruct the whole past.
const DefaultMaxRecalledItems = 6

// recallMinScore is the relevance a candidate must reach to be recalled. A
// single matched character does not qualify, which is what keeps an incidental
// shared word from dragging an unrelated record into the context.
const recallMinScore = 2

// BuildRecall scores the character's own committed records against the current
// question and returns the ones that bear on it, strongest first.
//
// Only records the character is entitled to are considered, and only ones that
// are still live: a superseded belief must not be recalled as current, and a
// private conversation they were not part of is not theirs to remember.
func BuildRecall(request RecallRequest) Recall {
	characterID := strings.TrimSpace(request.CharacterID)
	terms := recallTerms(request.Question)
	if characterID == "" || len(terms) == 0 {
		return Recall{}
	}
	limit := request.Limit
	if limit <= 0 {
		limit = DefaultMaxRecalledItems
	}

	scene := request.Scene.Normalize()
	// The scene's own words are part of the retrieval condition: a character
	// asked about the room they are standing in should recall what they know
	// about it, which is grounding rather than a guess.
	terms = append(terms, recallTerms(strings.Join([]string{scene.Location, scene.Title}, " "))...)
	terms = dedupeTerms(terms)

	var candidates []RecallItem
	seen := map[string]bool{}

	for _, record := range request.Knowledge {
		record = record.Normalize()
		if !record.Live() || !HolderCanKnow(record, characterID) {
			continue
		}
		if seen[record.ID] {
			continue
		}
		score := recallScore(terms, record.Statement)
		if score < recallMinScore {
			continue
		}
		seen[record.ID] = true
		source := record.LearnedFrom
		if source == "" {
			source = record.SourceEventID
		}
		if source == "" {
			source = record.SourceExchangeID
		}
		if source == "" {
			source = record.SourceTurnID
		}
		candidates = append(candidates, RecallItem{
			Kind: "knowledge", ID: record.ID, Text: record.Statement,
			Nature: record.Nature, Certainty: record.Nature.Certainty(),
			Source: source, WorldTime: record.WorldTime, Score: score,
		})
	}

	for _, exchange := range request.Exchanges {
		exchange = exchange.Normalize()
		if !exchangeReaches(exchange, characterID) {
			continue
		}
		if seen[exchange.ID] {
			continue
		}
		score := recallScore(terms, exchange.Text)
		if score < recallMinScore {
			continue
		}
		seen[exchange.ID] = true
		speaker := "the player"
		if exchange.Speaker == SpeakerCharacter {
			speaker = "you"
		}
		candidates = append(candidates, RecallItem{
			Kind: "exchange", ID: exchange.ID,
			Text: fmt.Sprintf("%s said: %s", speaker, exchange.Text),
			// An exchange is something the character lived through on this
			// channel, which is the same footing as experienced knowledge.
			Nature: NatureExperienced, Certainty: NatureExperienced.Certainty(),
			Source: string(exchange.Channel), WorldTime: exchange.WorldTime, Score: score,
		})
	}

	for _, matter := range request.Matters {
		matter = matter.Normalize()
		if matter.CharacterID != characterID || seen[matter.ID] {
			continue
		}
		score := recallScore(terms, matter.Statement)
		if score < recallMinScore {
			continue
		}
		seen[matter.ID] = true
		candidates = append(candidates, RecallItem{
			Kind: "matter", ID: matter.ID,
			Text: fmt.Sprintf("[%s/%s] %s", matter.Kind, matter.Status, matter.Statement),
			Source: "matter", WorldTime: matter.WorldTime, Score: score,
		})
	}

	result := Recall{Considered: len(candidates)}
	// Strongest first, then most recent, then by ID so a rebuild is identical.
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.WorldTime != b.WorldTime {
			return a.WorldTime > b.WorldTime
		}
		return a.ID < b.ID
	})
	if len(candidates) > limit {
		result.Truncated = true
		candidates = candidates[:limit]
	}
	result.Items = candidates
	result.Selected = len(candidates)
	return result
}

// recallTerms splits text into retrieval terms. Chinese has no spaces, so a run
// of Han characters is cut into overlapping bigrams; a two-character run is one
// bigram and a longer run also contributes the whole phrase, so an exact phrase
// scores above two incidental neighbours. Latin runs are lowercased whole
// words.
func recallTerms(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	var terms []string
	seen := map[string]bool{}
	add := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	var han []rune
	flushHan := func() {
		if len(han) == 0 {
			return
		}
		if len(han) == 1 {
			add(string(han))
		} else {
			for i := 0; i+1 < len(han); i++ {
				add(string(han[i : i+2]))
			}
			if len(han) <= 8 {
				add(string(han))
			}
		}
		han = han[:0]
	}
	var latin strings.Builder
	flushLatin := func() {
		if latin.Len() > 0 {
			add(latin.String())
			latin.Reset()
		}
	}
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			latin.WriteRune(r)
		default:
			flushHan()
			flushLatin()
		}
	}
	flushHan()
	flushLatin()
	return terms
}

func dedupeTerms(terms []string) []string {
	seen := make(map[string]bool, len(terms))
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		out = append(out, term)
	}
	return out
}

// recallScore sums the rune length of each distinct query term the text
// contains, so a longer shared phrase outweighs a short one and a single
// character cannot carry a match on its own.
func recallScore(terms []string, text string) int {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return 0
	}
	score := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			score += utf8.RuneCountInString(term)
		}
	}
	return score
}

// RenderRecall writes the recalled material for the model. It states plainly
// that these are earlier records surfaced by the current question, so the model
// treats them as memory it holds rather than as new information, and it repeats
// each record's nature so a recalled rumour is still spoken as a rumour.
//
// A pass that was cut, or whose material the budget then dropped, renders a note
// saying so even with nothing to show. That note is the difference between a
// character who hedges and one who flatly denies something they lived through:
// without it an empty block is indistinguishable from an empty past.
func RenderRecall(recall Recall) string {
	if len(recall.Items) == 0 {
		if !recall.Truncated && !recall.Dropped {
			return ""
		}
		return "You remember that there was something relevant here, but the detail is not in front of you right now. Say you are not certain, or that you would need to think about it — do not say it never happened."
	}
	var sb strings.Builder
	sb.WriteString("Earlier things you remember, recalled because they bear on what is being asked now:\n")
	for _, item := range recall.Items {
		switch {
		case item.Nature != "":
			fmt.Fprintf(&sb, "- (%s, %s) %s\n", item.Nature, item.Certainty, oneLine(item.Text))
			if item.Source != "" {
				fmt.Fprintf(&sb, "  - Source: %s, %s.\n", item.Source, item.WorldTime)
			}
		default:
			fmt.Fprintf(&sb, "- (%s) %s\n", item.WorldTime, oneLine(item.Text))
		}
	}
	sb.WriteString("These are your own memories of what happened. Use them naturally; do not recite them as a list.\n")
	if recall.Truncated || recall.Dropped {
		sb.WriteString("More relevant memories exist than are shown here. If asked about something not listed, you may say you are not certain rather than that it never happened.\n")
	}
	return strings.TrimSpace(sb.String())
}
