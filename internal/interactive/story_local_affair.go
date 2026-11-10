package interactive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"ta/internal/character"
)

const TurnWorldActionLocalAffair = "local_affair"
const localAffairRepeatWindow = 12

// Local affairs are small, causal situations encountered at existing places.
// They grant no equipment, money, movement, faction power or off-screen action.
// Their immutable definition and subsequent evidence live in the Story journal.
type LocalAffairStructure struct {
	Goal        string `json:"goal" jsonschema:"enum=understand,enum=recover,enum=protect,enum=reconcile,enum=gain_access,enum=choose_commitment" jsonschema_description:"具体目标的结构类别，不用人名地名替代。"`
	Obstacle    string `json:"obstacle" jsonschema:"enum=missing_evidence,enum=conflicting_accounts,enum=scarcity,enum=broken_process,enum=competing_duties,enum=access_boundary" jsonschema_description:"真正阻碍；同名换地不算新结构。"`
	Decision    string `json:"decision" jsonschema:"enum=verify_or_trust,enum=preserve_or_use,enum=share_or_withhold,enum=help_or_leave,enum=negotiate_or_refuse,enum=repair_or_reroute" jsonschema_description:"玩家的核心取舍。"`
	Consequence string `json:"consequence" jsonschema:"enum=corrected_belief,enum=preserved_evidence,enum=bounded_obligation,enum=changed_access,enum=relationship_boundary,enum=resource_tradeoff" jsonschema_description:"预期后果类型；不能凭此兑现钱物或权限，正式改变仍走已有规则。"`
}

type LocalAffairMethod struct {
	ID                 string `json:"id"`
	Label              string `json:"label" jsonschema_description:"玩家可理解的方法，不提前说答案。"`
	LocationID         string `json:"location_id" jsonschema_description:"该方法实际发生的已登记地点，必须本题材可达；不创造地图。"`
	Kind               string `json:"kind" jsonschema:"enum=observation,enum=witness" jsonschema_description:"observation仅正常感官可见痕迹或现场已公开记录；不是鉴定高阶药性、破禁制、解密终端或修理。需要技能/工具/权限的事仍走对应正式规则；witness要求本人实际说出证据。"`
	CharacterSourceKey string `json:"character_source_key,omitempty" jsonschema_description:"witness只填本局冻结目录的本人来源；observation留空。必须真实相遇和回应后才可据其所说核实。"`
	Evidence           string `json:"evidence" jsonschema_description:"未公开的具体证据命题。observation由本地实际调查核验（固定十分钟），witness须由character_facts先记本人的真实回应、下一轮引用exchange_id；不把猜测、传闻或远处作者答案当证据。"`
	Finding            string `json:"finding" jsonschema_description:"这一条证据怎样支持固定原因；核实前私有。每条独立方法都足以查明局部原因，不强制集齐两条。"`
}

type LocalAffairDefinition struct {
	ID         string               `json:"id"`
	Title      string               `json:"title"`
	LocationID string               `json:"location_id"`
	Hook       string               `json:"hook" jsonschema_description:"此刻实际见到的具体异常或机会，不替玩家接任务。"`
	Cause      string               `json:"cause" jsonschema_description:"首次线索出现前确定的有限原因。落盘后不可改谜底、追加幕后总反派或固定终局。"`
	Structure  LocalAffairStructure `json:"structure"`
	Methods    []LocalAffairMethod  `json:"methods" jsonschema_description:"2至3条独立来源路径；至少在地点或实际来源上不同，任一真实核实足以兑现原因。"`
}

type LocalAffairActionInput struct {
	Conclude   bool                   `json:"conclude,omitempty" jsonschema_description:"仅verify：玩家此次要查明原因并收束时设true，任一来源核实后同回合兑现原因，无需额外resolve回合或跑第二条路。单纯采证可留false。"`
	Action     string                 `json:"action" jsonschema:"enum=encounter,enum=verify,enum=resolve,enum=defer,enum=resume" jsonschema_description:"encounter仅记录自发遇见，不接任务；verify核实已提交证据；resolve明确原因并记玩家自己的后续选择；defer暂放，不罚款不催办；resume自愿续查。"`
	ID         string                 `json:"id"`
	Definition *LocalAffairDefinition `json:"definition,omitempty" jsonschema_description:"仅encounter完整填写；后续不可改写。"`
	MethodID   string                 `json:"method_id,omitempty"`
	ExchangeID string                 `json:"exchange_id,omitempty" jsonschema_description:"仅verify+witness：复制遇见之后本人已提交、主角实际听到的回应ID；observation留空，规则结算在场调查十分钟。"`
}

type LocalAffairEvidence struct {
	MethodID      string `json:"method_id"`
	ExchangeID    string `json:"exchange_id"`
	SourceEventID string `json:"source_event_id"`
}

type LocalAffairState struct {
	Definition           LocalAffairDefinition `json:"definition"`
	ActivitySequence     int                   `json:"activity_sequence"`
	Status               string                `json:"status"`
	Evidence             []LocalAffairEvidence `json:"evidence,omitempty"`
	EarlierExchangeCount int                   `json:"earlier_exchange_count"`
	PlayerDecision       string                `json:"player_decision,omitempty"`
	OriginEventID        string                `json:"origin_event_id"`
	LastEventID          string                `json:"last_event_id"`
}

type LocalAffairRecordEvent struct {
	V           int              `json:"v"`
	Type        string           `json:"type"`
	ID          string           `json:"id"`
	ParentID    string           `json:"parent_id"`
	BranchID    string           `json:"branch_id"`
	Ts          string           `json:"ts"`
	TurnID      string           `json:"turn_id"`
	Action      string           `json:"action"`
	State       LocalAffairState `json:"state"`
	ClockBefore *StoryClock      `json:"clock_before,omitempty"`
	ClockAfter  *StoryClock      `json:"clock_after,omitempty"`
}

func cloneLocalAffair(v LocalAffairState) LocalAffairState {
	v.Definition.Methods = append([]LocalAffairMethod(nil), v.Definition.Methods...)
	v.Evidence = append([]LocalAffairEvidence(nil), v.Evidence...)
	return v
}
func cloneLocalAffairs(v []LocalAffairState) []LocalAffairState {
	out := make([]LocalAffairState, len(v))
	for i := range v {
		out[i] = cloneLocalAffair(v[i])
	}
	return out
}
func cloneLocalAffairInput(v LocalAffairActionInput) LocalAffairActionInput {
	if v.Definition != nil {
		d := *v.Definition
		d.Methods = append([]LocalAffairMethod(nil), d.Methods...)
		v.Definition = &d
	}
	return v
}
func localAffairRequiresSource(w *TurnWorldActionInput) bool {
	return w != nil && w.Action == TurnWorldActionLocalAffair && w.LocalAffair != nil && w.LocalAffair.Action != "encounter"
}
func localAffairText(s string, limit int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= limit && !strings.ContainsRune(s, 0)
}
func localAffairID(s string) bool {
	return localAffairText(s, 100) && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "/\\\n\r\t ")
}
func localAffairEnum(s string, values string) bool {
	for _, value := range strings.Split(values, "|") {
		if s == value {
			return true
		}
	}
	return false
}
func validateLocalAffairDefinition(d LocalAffairDefinition) error {
	if !localAffairID(d.ID) || !localAffairID(d.LocationID) || !localAffairText(d.Title, 100) || !localAffairText(d.Hook, 600) || !localAffairText(d.Cause, 800) {
		return fmt.Errorf("local affair requires bounded identity, place, opening and fixed cause")
	}
	s := d.Structure
	if !localAffairEnum(s.Goal, "understand|recover|protect|reconcile|gain_access|choose_commitment") || !localAffairEnum(s.Obstacle, "missing_evidence|conflicting_accounts|scarcity|broken_process|competing_duties|access_boundary") || !localAffairEnum(s.Decision, "verify_or_trust|preserve_or_use|share_or_withhold|help_or_leave|negotiate_or_refuse|repair_or_reroute") || !localAffairEnum(s.Consequence, "corrected_belief|preserved_evidence|bounded_obligation|changed_access|relationship_boundary|resource_tradeoff") {
		return fmt.Errorf("local affair requires recognized structural categories")
	}
	if len(d.Methods) < 2 || len(d.Methods) > 3 {
		return fmt.Errorf("local affair requires two or three independent paths")
	}
	ids, sources := map[string]bool{}, map[string]bool{}
	for _, m := range d.Methods {
		if !localAffairID(m.ID) || ids[m.ID] || !localAffairID(m.LocationID) || !localAffairText(m.Label, 160) || !localAffairText(m.Evidence, 600) || !localAffairText(m.Finding, 600) {
			return fmt.Errorf("local affair method is incomplete or duplicated")
		}
		ids[m.ID] = true
		if m.Kind != "observation" && m.Kind != "witness" || m.Kind == "witness" && !localAffairID(m.CharacterSourceKey) || m.Kind == "observation" && m.CharacterSourceKey != "" {
			return fmt.Errorf("local affair method has no valid source")
		}
		source := m.Kind + ":" + m.LocationID + ":" + m.CharacterSourceKey
		// Two renamed inspections of the same place, or the same witness, do not
		// provide independent routes to an answer.
		if m.Kind == "witness" {
			source = m.Kind + ":" + m.CharacterSourceKey
		}
		if sources[source] {
			return fmt.Errorf("local affair paths repeat the same source")
		}
		sources[source] = true
	}
	return nil
}
func validateLocalAffairInput(w *TurnWorldActionInput) string {
	if w.LocalAffair == nil {
		return "地方事务缺少行动"
	}
	in := w.LocalAffair
	clean := *w
	clean.LocalAffair = nil
	clean.Action = ""
	clean.SourceText = ""
	if !clean.Empty() {
		return "地方事务不能混入另一项正式行动"
	}
	if !localAffairID(in.ID) {
		return "地方事务需要稳定标识"
	}
	if in.Conclude && in.Action != "verify" {
		return "只有实际核实可同时收束"
	}
	if in.Action == "encounter" {
		if in.Definition == nil || in.ID != in.Definition.ID || in.MethodID != "" || in.ExchangeID != "" {
			return "首次遇见需要一份完整且同标识的定义"
		}
		if err := validateLocalAffairDefinition(*in.Definition); err != nil {
			return err.Error()
		}
		return ""
	}
	if in.Definition != nil || !localAffairText(w.SourceText, 800) {
		return "后续不能改谜底，且需要玩家本轮行动原话"
	}
	switch in.Action {
	case "verify":
		if in.MethodID == "" {
			return "核实需要已登记方法；本人证言还需实际回应来源"
		}
	case "resolve", "defer", "resume":
		if in.MethodID != "" || in.ExchangeID != "" {
			return "该行动不接受额外证据字段"
		}
	default:
		return "未知地方事务行动"
	}
	return ""
}

// LocalAffairsSupported uses the settlement's real location capability. The
// narrator must not maintain a separate world-ID whitelist or invent places.
func LocalAffairsSupported(meta StoryMeta) bool {
	c := worldContentForMeta(meta)
	return c != nil && len(c.Labels) > 0
}
func localAffairPlaceInPack(meta StoryMeta, location string) bool {
	c := worldContentForMeta(meta)
	return c != nil && c.Labels[location] != ""
}
func localAffairRepeated(prior []LocalAffairState, d LocalAffairDefinition) bool {
	start := max(0, len(prior)-localAffairRepeatWindow)
	for _, p := range prior[start:] {
		if strings.Join(strings.Fields(p.Definition.Cause), "") == strings.Join(strings.Fields(d.Cause), "") {
			return true
		}
		a, b := p.Definition.Structure, d.Structure
		if a.Goal == b.Goal && a.Obstacle == b.Obstacle && (a.Decision == b.Decision || a.Consequence == b.Consequence) {
			return true
		}
	}
	return false
}
func localAffairNextSequence(affairs []LocalAffairState) int {
	n := 0
	for _, a := range affairs {
		n = max(n, a.ActivitySequence)
	}
	return n + 1
}
func localAffairsByActivity(affairs []LocalAffairState) []LocalAffairState {
	out := cloneLocalAffairs(affairs)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ActivitySequence < out[j].ActivitySequence })
	return out
}

// Existing route gates remain authoritative. A place label by itself is not a
// promise that either method can be reached; no generated route is registered.
func localAffairReachable(p *storyBranchProjection, meta StoryMeta, origin, target string) bool {
	seen := map[string]bool{origin: true}
	queue := []string{origin}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		if at == target {
			return true
		}
		for _, r := range journeyRouteDefinitions {
			if r.OriginLocationID == at && !seen[r.DestinationLocationID] && localAffairPlaceInPack(meta, r.DestinationLocationID) && adventureRouteAvailable(p, r.ID) {
				seen[r.DestinationLocationID] = true
				queue = append(queue, r.DestinationLocationID)
			}
		}
	}
	return false
}
func localAffairMethod(d LocalAffairDefinition, id string) (LocalAffairMethod, bool) {
	for _, m := range d.Methods {
		if m.ID == id {
			return m, true
		}
	}
	return LocalAffairMethod{}, false
}

// Player-facing dialogue often lists only NPCs in its audience. An explicit
// directed reply to the protagonist is therefore also evidence of receipt;
// a private exchange addressed to somebody else is never such evidence.
func localAffairExchangeHeard(e character.Exchange) bool {
	return (e.Channel == character.ChannelScene || e.Channel == character.ChannelPrivate) && (e.AddressedTo == DefaultActorID || character.ContainsID(e.Audience, DefaultActorID))
}
func localAffairWitness(p *storyBranchProjection, a LocalAffairState, m LocalAffairMethod, id string) (character.Exchange, bool) {
	if p == nil || p.Characters == nil || id == "" {
		return character.Exchange{}, false
	}
	partner, ok := questEchoPartner(p, m.CharacterSourceKey)
	if !ok {
		return character.Exchange{}, false
	}
	for i, e := range p.Characters.Exchanges {
		if i >= a.EarlierExchangeCount && e.ID == id && e.SourceEventID != "" && e.CharacterID == partner.ID && e.Speaker == character.SpeakerCharacter && strings.TrimSpace(e.Text) == strings.TrimSpace(m.Evidence) && localAffairExchangeHeard(e) {
			return e, true
		}
	}
	return character.Exchange{}, false
}
func planLocalAffairAction(p *storyBranchProjection, scene map[string]any, meta StoryMeta, w TurnWorldActionInput) (LocalAffairState, error) {
	fail := func(s string) (LocalAffairState, error) {
		return LocalAffairState{}, fmt.Errorf("local affair refused: %s", s)
	}
	if reason := validateLocalAffairInput(&w); reason != "" {
		return fail(reason)
	}
	if p == nil {
		return fail("committed branch missing")
	}
	in := w.LocalAffair
	// A partial journey retains the last stop in scene state. That stale label
	// must not permit a new encounter or inspection back at the origin.
	if in.Action == "encounter" || in.Action == "verify" {
		for _, journey := range p.Journeys {
			if !journeyFinished(journey) {
				return fail("arrive before investigating a registered place")
			}
		}
	}
	if err := realmActionBlocked(p, scene, meta); err != nil {
		return fail(err.Error())
	}
	location, here := storyLocationID(scene, worldContentForMeta(meta))
	if !here {
		return fail("actual registered location is unknown")
	}
	var prior *LocalAffairState
	for _, a := range p.LocalAffairs {
		if a.Definition.ID == in.ID {
			v := cloneLocalAffair(a)
			prior = &v
			break
		}
	}
	if in.Action == "encounter" {
		if prior != nil {
			return fail("an encountered identity cannot be reused or rewritten")
		}
		d := *in.Definition
		if d.LocationID != location || !localAffairPlaceInPack(meta, location) {
			return fail("encounter must be at the actual place in this world")
		}
		if localAffairRepeated(p.LocalAffairs, d) {
			return fail("recent affair repeats goal and obstacle without changing both decision and consequence")
		}
		for _, m := range d.Methods {
			if !localAffairPlaceInPack(meta, m.LocationID) || !localAffairReachable(p, meta, location, m.LocationID) {
				return fail("a method names an unavailable world location")
			}
			if m.Kind == "witness" {
				found := false
				if meta.PackCatalog != nil {
					for _, c := range meta.PackCatalog.Characters {
						if c.SourceKey == m.CharacterSourceKey {
							found = true
							break
						}
					}
				}
				if !found {
					return fail("witness is not in this story's frozen catalog")
				}
			}
		}
		next := LocalAffairState{Definition: d, Status: "noticed", ActivitySequence: localAffairNextSequence(p.LocalAffairs)}
		if p.Characters != nil {
			next.EarlierExchangeCount = len(p.Characters.Exchanges)
		}
		return next, nil
	}
	if prior == nil {
		return fail("no encountered affair exists on this branch")
	}
	next := *prior
	next.ActivitySequence = localAffairNextSequence(p.LocalAffairs)
	if next.Status == "resolved" {
		return fail("resolved affairs cannot restart or pay twice")
	}
	switch in.Action {
	case "defer":
		if next.Status == "deferred" {
			return fail("affair is already deferred")
		}
		next.Status = "deferred"
	case "resume":
		if next.Status != "deferred" {
			return fail("affair is not deferred")
		}
		next.Status = "noticed"
	case "verify":
		if next.Status == "deferred" {
			return fail("player must choose to resume before investigating")
		}
		m, ok := localAffairMethod(next.Definition, in.MethodID)
		if !ok || m.LocationID != location {
			return fail("method is not at the actual location")
		}
		for _, e := range next.Evidence {
			if e.MethodID == in.MethodID || in.ExchangeID != "" && e.ExchangeID == in.ExchangeID {
				return fail("evidence has already been recorded")
			}
		}
		evidence := LocalAffairEvidence{MethodID: m.ID}
		if m.Kind == "observation" {
			if in.ExchangeID != "" {
				return fail("an observation cannot borrow testimony")
			}
			evidence.SourceEventID = "pending-inspection"
		} else {
			e, ok := localAffairWitness(p, next, m, in.ExchangeID)
			if !ok {
				return fail("testimony must be fresh, committed, personally heard and from the present fixed witness")
			}
			evidence.ExchangeID = e.ID
			evidence.SourceEventID = e.SourceEventID
		}
		next.Evidence = append(next.Evidence, evidence)
		next.Status = "investigated"
		if in.Conclude {
			next.Status = "resolved"
			next.PlayerDecision = w.SourceText
		}
	case "resolve":
		if next.Status == "deferred" || len(next.Evidence) == 0 {
			return fail("one real independent source is sufficient, but an unverified guess is not proof")
		}
		// The branch already recorded the actual investigation or personally
		// heard exchange. Resolving elsewhere must not summon the witness again.
		next.Status = "resolved"
		next.PlayerDecision = w.SourceText
	}
	return next, nil
}

func validateLocalAffairRecord(e LocalAffairRecordEvent) error {
	if e.Type != StoryEventTypeLocalAffairRecord {
		return fmt.Errorf("invalid local affair event type")
	}
	if err := validateStoryEventEnvelope(StoryEventEnvelope{V: e.V, Type: e.Type, ID: e.ID, ParentID: e.ParentID, BranchID: e.BranchID, Ts: e.Ts}); err != nil {
		return err
	}
	if e.State.ActivitySequence < 1 || e.State.EarlierExchangeCount < 0 || e.TurnID == "" || e.State.OriginEventID == "" || e.State.LastEventID != e.ID {
		return fmt.Errorf("local affair requires its canonical turn and source")
	}
	if err := validateLocalAffairDefinition(e.State.Definition); err != nil {
		return err
	}
	if !localAffairEnum(e.State.Status, "noticed|investigated|deferred|resolved") {
		return fmt.Errorf("invalid local affair status")
	}
	if e.State.Status == "resolved" && (len(e.State.Evidence) == 0 || e.State.PlayerDecision == "") {
		return fmt.Errorf("resolved local affair requires evidence and player decision")
	}
	if !localAffairEnum(e.Action, "encounter|verify|resolve|defer|resume") {
		return fmt.Errorf("invalid local affair record action")
	}
	expected := map[string]string{"encounter": "noticed", "verify": "investigated", "resolve": "resolved", "defer": "deferred", "resume": "noticed"}[e.Action]
	if e.Action == "verify" && e.State.Status == "resolved" {
		expected = "resolved"
	}
	if e.State.Status != expected {
		return fmt.Errorf("local affair action and state disagree")
	}
	if e.State.Status != "resolved" && e.State.PlayerDecision != "" {
		return fmt.Errorf("unresolved affairs cannot claim a player decision")
	}
	if e.Action == "verify" {
		if len(e.State.Evidence) == 0 {
			return fmt.Errorf("verification requires actual evidence")
		}
		last := e.State.Evidence[len(e.State.Evidence)-1]
		m, _ := localAffairMethod(e.State.Definition, last.MethodID)
		if m.Kind == "observation" && (e.ClockBefore == nil || last.ExchangeID != "" || last.SourceEventID != e.ID) || m.Kind == "witness" && (last.ExchangeID == "" || e.ClockBefore != nil) {
			return fmt.Errorf("verification source and interval do not match its method")
		}
	}
	seen := map[string]bool{}
	for _, v := range e.State.Evidence {
		if _, ok := localAffairMethod(e.State.Definition, v.MethodID); !ok || v.SourceEventID == "" || seen[v.MethodID] {
			return fmt.Errorf("invalid local affair evidence")
		}
		seen[v.MethodID] = true
	}
	if (e.ClockBefore == nil) != (e.ClockAfter == nil) {
		return fmt.Errorf("local investigation requires a complete interval")
	}
	if e.ClockBefore != nil {
		if e.Action != "verify" || len(e.State.Evidence) == 0 {
			return fmt.Errorf("only actual investigation advances local affair time")
		}
		last := e.State.Evidence[len(e.State.Evidence)-1]
		m, _ := localAffairMethod(e.State.Definition, last.MethodID)
		after, err := AdvanceStoryClock(*e.ClockBefore, 600)
		if err != nil || after != *e.ClockAfter || m.Kind != "observation" || last.SourceEventID != e.ID {
			return fmt.Errorf("local investigation must consume its own ten-minute interval")
		}
	}

	return nil
}
func applyLocalAffairRecord(p *storyBranchProjection, e LocalAffairRecordEvent) error {
	if err := validateLocalAffairRecord(e); err != nil {
		return err
	}
	for i, a := range p.LocalAffairs {
		if a.Definition.ID != e.State.Definition.ID {
			continue
		}
		if reflect.DeepEqual(a, e.State) {
			return nil
		}
		if e.State.ActivitySequence != localAffairNextSequence(p.LocalAffairs) || a.Status == "resolved" || e.Action == "encounter" || !reflect.DeepEqual(a.Definition, e.State.Definition) || a.OriginEventID != e.State.OriginEventID || a.EarlierExchangeCount != e.State.EarlierExchangeCount {
			return fmt.Errorf("local affair replay cannot rewrite established cause or origin")
		}
		if len(e.State.Evidence) < len(a.Evidence) || len(a.Evidence) > 0 && !reflect.DeepEqual(a.Evidence, e.State.Evidence[:len(a.Evidence)]) {
			return fmt.Errorf("local affair replay cannot remove evidence")
		}
		extra := len(e.State.Evidence) - len(a.Evidence)
		if e.Action == "verify" && (a.Status == "deferred" || extra != 1) || e.Action != "verify" && extra != 0 || e.Action == "resume" && a.Status != "deferred" || e.Action == "defer" && a.Status == "deferred" || e.Action == "resolve" && a.Status == "deferred" {
			return fmt.Errorf("invalid local affair state transition")
		}
		if err := applyLocalAffairClock(p, e); err != nil {
			return err
		}
		p.LocalAffairs[i] = cloneLocalAffair(e.State)
		return nil
	}
	if e.State.ActivitySequence != localAffairNextSequence(p.LocalAffairs) || e.Action != "encounter" || e.State.OriginEventID != e.ID || e.State.Status != "noticed" || len(e.State.Evidence) != 0 || localAffairRepeated(p.LocalAffairs, e.State.Definition) {
		return fmt.Errorf("local affair replay requires a fresh, non-repeated encounter")
	}
	if err := applyLocalAffairClock(p, e); err != nil {
		return err
	}
	p.LocalAffairs = append(p.LocalAffairs, cloneLocalAffair(e.State))
	return nil
}

func applyLocalAffairClock(p *storyBranchProjection, e LocalAffairRecordEvent) error {
	if e.ClockBefore == nil {
		return nil
	}
	if p.Clock != nil && *p.Clock != *e.ClockBefore {
		return fmt.Errorf("local investigation starts at a stale clock")
	}
	p.Clock = cloneStoryClock(e.ClockAfter)
	return nil
}

// This public projection never includes unverified evidence or the fixed cause.
type LocalAffairView struct {
	ID             string                  `json:"id"`
	Title          string                  `json:"title"`
	LocationID     string                  `json:"location_id"`
	Hook           string                  `json:"hook"`
	Status         string                  `json:"status"`
	Methods        []LocalAffairMethodView `json:"methods"`
	Findings       []string                `json:"findings,omitempty"`
	Cause          string                  `json:"cause,omitempty"`
	PlayerDecision string                  `json:"player_decision,omitempty"`
	SourceEventID  string                  `json:"source_event_id"`
}
type LocalAffairMethodView struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	LocationID string `json:"location_id"`
}

func localAffairPublicViews(affairs []LocalAffairState) []LocalAffairView {
	out := make([]LocalAffairView, 0, len(affairs))
	for _, a := range localAffairsByActivity(affairs) {
		d := a.Definition
		v := LocalAffairView{ID: d.ID, Title: d.Title, LocationID: d.LocationID, Hook: d.Hook, Status: a.Status, SourceEventID: a.LastEventID}
		for _, m := range d.Methods {
			v.Methods = append(v.Methods, LocalAffairMethodView{ID: m.ID, Label: m.Label, LocationID: m.LocationID})
		}
		for _, e := range a.Evidence {
			m, _ := localAffairMethod(d, e.MethodID)
			v.Findings = append(v.Findings, m.Finding)
		}
		if a.Status == "resolved" {
			v.Cause = d.Cause
			v.PlayerDecision = a.PlayerDecision
		}
		out = append(out, v)
	}
	return out
}

// LocalAffairsNarratorContext is private model material, not a user projection.
// Recent structural summaries and relevant frozen answers have independent
// budgets, so a long history does not require loading every secret every turn.
func (s *Store) LocalAffairsNarratorContext(storyID, branchID, replaceTurnID string, query ...string) (string, error) {
	view, err := s.TurnWorldView(storyID, branchID, replaceTurnID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("## 地方小事（私有规则上下文，不是玩家知识）\n可围绕当前已到达地点、具体生计和已有事实自然生成local_affair，首次可自定稳定小事ID，后续必须沿用；不要把所有生活变成任务，也不要为填满窗口堆新悬念。通常只推进玩家关心的一条、旁侧一两条，其余可自然生活或暂放。encounter只呈现一条公开线索，不替玩家接下；两条独立路径和原因首次固定。钱物、技能、地图、势力、NPC承诺仍须原有正式规则，绝不可借这里奖励或远程改变。玩家可defer，不催办、不惩罚、不设最终总终局。结构近窗12件，同目标+障碍须同时改变决定与后果；换名换地点不算新内容。没有真实变化就写平常生活。\nobservation方法必须在实际地点执行verify调查并消耗十分钟；witness方法先通过正常character_facts记录本人实际回应（addressed_to=protagonist，证据短句须完整独立，否认/转述不是确认），之后verify引用可见exchange_id。一句猜测或作者私密答案不能伪造证据。任意一条方法足以resolve；本次要查明即收束时verify.conclude=true在同回合兑现，不再强迫一次收尾点击。无须再跑第二条或强塞反转；玩家提前猜中也直接围绕已有证据验证，不加新谜底拖延。以下私有cause/evidence不能提前写进正文或UI。\n")
	start := max(0, len(view.LocalAffairs)-localAffairRepeatWindow)
	for _, a := range view.LocalAffairs[start:] {
		s, _ := json.Marshal(a.Definition.Structure)
		fmt.Fprintf(&b, "recent %s %s %s\n", a.Definition.ID, a.Status, s)
	}
	count := 0
	recent := localAffairsByActivity(view.LocalAffairs)
	requested := func(a LocalAffairState) bool {
		for _, text := range query {
			if strings.Contains(text, a.Definition.ID) || strings.Contains(text, a.Definition.Title) {
				return true
			}
		}
		return false
	}
	sort.SliceStable(recent, func(i, j int) bool { return !requested(recent[i]) && requested(recent[j]) })
	for i := len(recent) - 1; i >= 0 && count < 6; i-- {
		a := recent[i]
		if a.Status == "resolved" && !requested(a) {
			continue
		}
		data, _ := json.Marshal(a)
		if b.Len()+len(data)+len("frozen \n") > 24000 {
			break
		}
		fmt.Fprintf(&b, "frozen %s\n", data)
		count++
		if view.Characters != nil {
			for _, m := range a.Definition.Methods {
				if m.Kind != "witness" {
					continue
				}
				for i, e := range view.Characters.Exchanges {
					profile, known := view.Characters.Profile(e.CharacterID)
					if i >= a.EarlierExchangeCount && known && profile.SourceKey == m.CharacterSourceKey && strings.TrimSpace(e.Text) == strings.TrimSpace(m.Evidence) && e.Speaker == character.SpeakerCharacter && localAffairExchangeHeard(e) {
						entry := fmt.Sprintf("possible_witness method=%s exchange_id=%s (仍核验本人来源及在场)\n", m.ID, e.ID)
						if b.Len()+len(entry) > 24000 {
							break
						}
						b.WriteString(entry)
					}
				}
			}
		}

	}
	return b.String(), nil
}

// LocalAffairsPublicAt restores public knowledge at the same target parent as
// the model's regenerated turn; live resolved answers must not travel backward.
func (s *Store) LocalAffairsPublicAt(storyID, branchID, replaceTurnID string) ([]LocalAffairView, error) {
	view, err := s.TurnWorldView(storyID, branchID, replaceTurnID)
	if err != nil {
		return nil, err
	}
	return localAffairPublicViews(view.LocalAffairs), nil
}
