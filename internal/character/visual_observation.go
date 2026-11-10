package character

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VisualObservationDraft is part of the ordinary formal knowledge submission.
// It cites witnessed utterances, not an image or an unlock command. Scene, identity,
// appearance and time are read from the same accepted turn by the backend.
type VisualObservationDraft struct {
	SubjectID                string `json:"subject_id" jsonschema_description:"本回合实际见到并已识别的人物Actor ID。不能凭GM名册得知身份。"`
	Form                     string `json:"form" jsonschema_description:"本回合亲眼所见的具体形态原文，必须也已作为此人物外貌与装扮状态的首个完整肯定短句保存（形态原文后为句终或明确标点）。不能靠子串包含、否定或不确定描述匹配。不是原始种族推断；伪装、变化或看不清时不得填普通形态。"`
	ContactExchangeIndex     int    `json:"contact_exchange_index" jsonschema_description:"本回合exchanges中此人物向主角当面说话的1起算序号。"`
	RecognitionExchangeIndex int    `json:"recognition_exchange_index" jsonschema_description:"本人向主角当面说话的1起算序号；同一句真实自我介绍同时证明接触与识别时，可与contact_exchange_index相同，不要求另造第二句话。现仅接收实际发生的、以我是或我叫紧接完整KnownAs开头的肯定姓名自述；否认、转述、猜测不建立身份。Profile.KnownAs必须本身已明确；Profile不是主角已知职务的证据，职务门槛另核对真实来源。不能从配图要求反写，不能为此编造话语。其他真实识别方式暂不填visual。"`
}

type VisualObservation struct {
	SubjectID string `json:"subject_id"`
	SourceKey string `json:"source_key"`
	KnownAs   string `json:"known_as"`
	// Role retains the legacy source-supported role snapshot. Name-only
	// observations instead bind ProfileRevision without copying the GM role.
	Role                  string `json:"role,omitempty"`
	ProfileRevision       int    `json:"profile_revision,omitempty"`
	Form                  string `json:"form"`
	Appearance            string `json:"appearance"`
	Lifeform              string `json:"lifeform"`
	Gender                string `json:"gender"`
	Age                   int    `json:"age,omitempty"`
	Year                  int    `json:"year"`
	Location              string `json:"location"`
	StoryID               string `json:"story_id"`
	BranchID              string `json:"branch_id"`
	ContactExchangeID     string `json:"contact_exchange_id"`
	RecognitionExchangeID string `json:"recognition_exchange_id"`
}

type VisualSubject struct {
	Profile    Profile
	Appearance string
}
type VisualObservationContext struct {
	ObserverID string
	Subjects   map[string]VisualSubject
}

// ExplicitVisualYear reads only an explicit leading civil year. Relative game
// clocks, chapter numbers, era names and source_episode are not calendars.
var explicitVisualYear = regexp.MustCompile(`^(?:公元)?([1-9][0-9]{0,3})(?:年(?:$|[0-9一二三四五六七八九十正冬腊]|\s)|-[0-9]{2}-[0-9]{2}(?:$|[ T]))`)

func ExplicitVisualYear(label string) int {
	m := explicitVisualYear.FindStringSubmatch(strings.TrimSpace(label))
	if len(m) != 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func projectVisualObservation(ctx ProjectionContext, scene Scene, draft KnowledgeDraft, exchanges []Exchange) (*VisualObservation, error) {
	v := draft.Visual
	vc := ctx.VisualContext
	if v == nil || vc == nil || vc.ObserverID == "" || ctx.TurnID == "" || ctx.CommitID == "" || draft.CharacterID != vc.ObserverID || draft.Nature != NatureExperienced || draft.Channel != ChannelScene {
		return nil, fmt.Errorf("visual knowledge requires the formal main-turn observer and experienced scene source")
	}
	subject, ok := vc.Subjects[strings.TrimSpace(v.SubjectID)]
	p := subject.Profile.Normalize()
	form := strings.TrimSpace(v.Form)
	if !ok || p.ID == vc.ObserverID || p.SourceKey == "" || p.KnownAs == "" || p.Role == "" || scene.Location == "" || !ContainsID(scene.Present, p.ID) || !ContainsID(scene.Present, vc.ObserverID) || form == "" || subject.Appearance == "" || !VisualAppearanceSupports(subject.Appearance, form) {
		return nil, fmt.Errorf("visual knowledge lacks observed appearance, known identity or co-presence")
	}
	year := ExplicitVisualYear(scene.WorldTime)
	// Observations preserve the actual story clock, including worlds without a
	// civil year. Only the reviewed asset rule may authorize yearless pictures.
	if strings.TrimSpace(scene.WorldTime) == "" || scene.WorldTime != ctx.WorldTime {
		return nil, fmt.Errorf("visual knowledge requires a consistent current scene time")
	}
	ci, ri := v.ContactExchangeIndex-1, v.RecognitionExchangeIndex-1
	if ci < 0 || ri < 0 || ci >= len(exchanges) || ri >= len(exchanges) {
		return nil, fmt.Errorf("visual knowledge requires valid contact and recognition utterance indices")
	}
	contact, recognition := exchanges[ci], exchanges[ri]
	for _, e := range []Exchange{contact, recognition} {
		if e.Channel != ChannelScene || e.StoryID != ctx.StoryID || e.BranchID != ctx.BranchID || e.SourceEventID != ctx.CommitID || e.TurnID != ctx.TurnID || e.SceneNode != scene.NodeID || e.WorldTime != scene.WorldTime || !ContainsID(e.Audience, vc.ObserverID) || !ContainsID(e.Audience, p.ID) {
			return nil, fmt.Errorf("visual source is outside the witnessed formal scene")
		}
	}
	if contact.Speaker != SpeakerCharacter || contact.CharacterID != p.ID || contact.AddressedTo != vc.ObserverID {
		return nil, fmt.Errorf("contact source must be the observed person's words to the observer")
	}
	// The recognition utterance must name the identity the player
	// actually uses. Canonical Profile.Name alone never proves recognition.
	if !VisualIdentitySupports(recognition, p, vc.ObserverID) {
		return nil, fmt.Errorf("recognition source does not identify the observed person")
	}
	role, profileRevision := p.Role, 0
	if !VisualRecognitionSupports(recognition, p, vc.ObserverID) {
		role, profileRevision = "", p.Revision
		if profileRevision < 1 {
			return nil, fmt.Errorf("name-only observation requires a committed profile revision")
		}
	}
	return &VisualObservation{SubjectID: p.ID, SourceKey: p.SourceKey, KnownAs: p.KnownAs, Role: role, ProfileRevision: profileRevision, Form: form, Appearance: subject.Appearance, Lifeform: p.Lifeform, Gender: p.Gender, Age: p.Age, Year: year, Location: scene.Location, StoryID: ctx.StoryID, BranchID: ctx.BranchID, ContactExchangeID: contact.ID, RecognitionExchangeID: recognition.ID}, nil
}

// VisualRoleSnapshotMatches accepts old role-bearing observations and new
// name-only snapshots without publishing an unlearned role in knowledge.
func VisualRoleSnapshotMatches(v VisualObservation, p Profile) bool {
	if p.Role == "" {
		return false
	}
	if v.ProfileRevision != 0 {
		return v.Role == "" && v.ProfileRevision > 0 && v.ProfileRevision == p.Revision
	}
	return v.Role == p.Role
}

// VisualRecognitionSupports retains the strict role-bearing source contract for
// occupational or other substantive identity gates. A profile role alone is not
// evidence that the observer learned it.
func VisualRecognitionSupports(e Exchange, p Profile, observer string) bool {
	if p.Role == "" || !VisualIdentitySupports(e, p, observer) {
		return false
	}
	text := strings.TrimSpace(e.Text)
	for _, namePrefix := range []string{"我是" + p.KnownAs, "我叫" + p.KnownAs} {
		if !strings.HasPrefix(text, namePrefix) {
			continue
		}
		rest := []rune(strings.TrimSpace(strings.TrimPrefix(text, namePrefix)))
		if len(rest) == 0 || !strings.ContainsRune("，,。.;；：:！!", rest[0]) {
			continue
		}
		clause := strings.TrimSpace(string(rest[1:]))
		// Exact affirmative clauses only: a title mentioned in a wish, another
		// person's role, or a qualifying suffix is not the speaker's current role.
		// The full authored role may itself contain punctuation. Compare it intact;
		// a reviewed short phrase never inherits the omitted author's annotations.
		for _, roleClause := range []string{p.Role, "我是" + p.Role} {
			for _, terminal := range []string{"", "。", ".", "！", "!"} {
				if clause == roleClause+terminal {
					return true
				}
			}
		}
	}
	return false
}

// VisualIdentitySupports verifies the narrow name-bearing source relation only.
// Neither the profile role snapshot nor a background classification is player
// knowledge. This is source consistency, not a natural-language truth detector.
func VisualIdentitySupports(e Exchange, p Profile, observer string) bool {
	text := strings.TrimSpace(e.Text)
	if p.KnownAs == "" || e.Speaker != SpeakerCharacter || e.CharacterID != p.ID || e.AddressedTo != observer {
		return false
	}
	if !visualAffirmativePrefix(text, "我是"+p.KnownAs) && !visualAffirmativePrefix(text, "我叫"+p.KnownAs) {
		return false
	}
	for _, marker := range []string{"不是", "并非", "不叫", "否认", "假冒", "冒充", "据说", "听说", "传闻", "谎称", "声称", "可能", "或许", "?", "？", "如果", "假如"} {
		if strings.Contains(text, marker) {
			return false
		}
	}
	return true
}

// A name or form must be a complete first clause, not the prefix of a different
// identity such as "X的助手" or a different form such as "人类女性雕像".
func visualAffirmativePrefix(text, prefix string) bool {
	if !strings.HasPrefix(text, prefix) {
		return false
	}
	suffix := strings.TrimSpace(strings.TrimPrefix(text, prefix))
	if suffix == "" {
		return true
	}
	for _, r := range suffix {
		return strings.ContainsRune("，,。.;；：:！!", r)
	}
	return false
}

// VisualAppearanceSupports accepts a narrow affirmative leading form clause
// in the already committed appearance field. It deliberately does not infer
// shape from prose. These explicit denial/uncertainty constructions are not
// observations; more complex semantics still require narrative validation.
func VisualAppearanceSupports(appearance, form string) bool {
	appearance = strings.TrimSpace(appearance)
	form = strings.TrimSpace(form)
	if form == "" || !visualAffirmativePrefix(appearance, form) {
		return false
	}
	for _, marker := range []string{"不是" + form, "并非" + form, "是否" + form, "不确定", "看不清", "无法确认", "不能确认"} {
		if strings.Contains(appearance, marker) {
			return false
		}
	}
	return true
}
