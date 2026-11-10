package interactive

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"ta/internal/character"
	"ta/internal/cultivation"
)

// TurnFactionMembershipInput proposes only the protagonist's known standing.
// The narrator cannot appoint an admissions officer or define a new permission.
type TurnFactionMembershipInput struct {
	FactionID            string `json:"faction_id" jsonschema_description:"本局已冻结的势力精确ID，不按同名标题合并。"`
	Status               string `json:"status" jsonschema:"enum=pending,enum=formal,enum=temporary,enum=left" jsonschema_description:"pending是已经当面提出、尚未获准的申请；formal正式成员；temporary临时身份记录（尚无期限结算，不自动授予通行或正式任务资格）；left已退出或申请撤回/被拒，不再享有资格。合理请求不保证接纳。"`
	Role                 string `json:"role,omitempty" jsonschema_description:"此次实际说明的身份称呼，仅展示，不授予领导权、公库或控制他人的权限。"`
	Statement            string `json:"statement" jsonschema_description:"主角实际知道的结果及依据，不超过240字。"`
	SourceText           string `json:"source_text,omitempty" jsonschema_description:"本人申请、接受临时身份或主动退出时，逐字引用玩家本轮原话；模型不得自己补授权。组织明确拒绝尚待审的申请可无此字段，但须本人真实通知；已有成员退出须玩家本人表达。"`
	RequestExchangeIndex int    `json:"request_exchange_index,omitempty" jsonschema_description:"本轮exchanges里主角向联系人实际申请的句子位置，从1起。pending/formal/temporary必填；不能替主角补写申请。"`
	SourceExchangeIndex  int    `json:"source_exchange_index,omitempty" jsonschema_description:"formal/temporary引用本轮有权接纳者在场向protagonist的明确回应，须在请求之后。pending引用主角实际申请；拒绝待定申请引用有权本人通知。主动退出可写0并提供真实source_text。"`
}

type factionMembershipContext struct {
	State       character.State
	StateData   map[string]any
	Combats     map[string]cultivation.CombatSession
	Catalog     *PackCatalogSnapshot
	Scene       character.Scene
	PlayerInput string
}

func validateTurnFactionMemberships(ctx factionMembershipContext, input *TurnCharacterFactsInput) error {
	if input == nil || len(input.Memberships) == 0 {
		return nil
	}
	if len(input.Memberships) > 8 {
		return fmt.Errorf("one turn supports at most eight membership changes")
	}
	input = canonicalMembershipInput(ctx, input)
	current := map[string]character.FactionMembership{}
	for _, m := range character.CurrentFactionMemberships(ctx.State.FactionMemberships) {
		current[m.FactionID] = m
	}
	seen := map[string]bool{}
	for i, draft := range input.Memberships {
		factionID := strings.TrimSpace(draft.FactionID)
		if factionID == "" || len(factionID) > 128 || seen[factionID] {
			return fmt.Errorf("membership %d needs a distinct stable faction id", i)
		}
		seen[factionID] = true
		if strings.TrimSpace(draft.Statement) == "" || utf8.RuneCountInString(draft.Statement) > 240 || utf8.RuneCountInString(draft.Role) > 80 {
			return fmt.Errorf("membership %d requires a bounded known result", i)
		}
		prior, exists := current[factionID]
		if draft.Status == "left" && draft.SourceExchangeIndex == 0 {
			if !exists || prior.Status == "left" {
				return fmt.Errorf("membership %d cannot leave an affiliation that does not stand", i)
			}
			if !membershipPlayerSource(ctx.PlayerInput, draft.SourceText) {
				return fmt.Errorf("membership %d needs the player's actual withdrawal", i)
			}
			continue
		}
		source, err := membershipExchange(input, draft.SourceExchangeIndex)
		if err != nil {
			return fmt.Errorf("membership %d: %w", i, err)
		}
		person, err := membershipContact(ctx, source.CharacterID, factionID)
		if err != nil {
			return fmt.Errorf("membership %d: %w", i, err)
		}
		switch draft.Status {
		case "pending":
			if exists && (prior.Status == "formal" || prior.Status == "temporary") {
				return fmt.Errorf("membership %d is already an active member", i)
			}
			if draft.RequestExchangeIndex != draft.SourceExchangeIndex || source.Speaker != "player" || source.AddressedTo != source.CharacterID || !membershipPlayerSource(ctx.PlayerInput, draft.SourceText) || !strings.Contains(source.Text, strings.TrimSpace(draft.SourceText)) {
				return fmt.Errorf("membership %d requires the protagonist's actual directed request", i)
			}
		case "formal", "temporary":
			if !character.ContainsID(person.MembershipAdmission, draft.Status) {
				return fmt.Errorf("membership %d: this person has no frozen authority to admit %s members", i, draft.Status)
			}
			request, err := membershipExchange(input, draft.RequestExchangeIndex)
			if err != nil {
				return fmt.Errorf("membership %d: %w", i, err)
			}
			if source.Speaker != "character" || source.AddressedTo != DefaultActorID || request.Speaker != "player" || request.CharacterID != source.CharacterID || request.AddressedTo != source.CharacterID || draft.RequestExchangeIndex >= draft.SourceExchangeIndex || !membershipPlayerSource(ctx.PlayerInput, draft.SourceText) || !strings.Contains(request.Text, strings.TrimSpace(draft.SourceText)) {
				return fmt.Errorf("membership %d requires the player's request followed by the authorized person's direct answer", i)
			}
		case "left":
			if !exists || prior.Status != "pending" || source.Speaker != "character" || source.AddressedTo != DefaultActorID || len(person.MembershipAdmission) == 0 {
				return fmt.Errorf("membership %d requires a pending application and an authorized person's actual refusal", i)
			}
		default:
			return fmt.Errorf("membership %d has invalid status %q", i, draft.Status)
		}
	}
	return nil
}

func membershipPlayerSource(user, quote string) bool {
	quote = strings.TrimSpace(quote)
	return quote != "" && strings.Contains(user, quote)
}

func membershipExchange(input *TurnCharacterFactsInput, index int) (TurnCharacterExchangeInput, error) {
	if index < 1 || index > len(input.Exchanges) {
		return TurnCharacterExchangeInput{}, fmt.Errorf("membership source exchange does not exist")
	}
	e := input.Exchanges[index-1]
	if e.Channel != "scene" || strings.TrimSpace(e.Text) == "" {
		return e, fmt.Errorf("membership requires an actual scene exchange")
	}
	return e, nil
}

func membershipContact(ctx factionMembershipContext, id, factionID string) (PackCatalogPerson, error) {
	if actorStateSystemActor(strings.TrimSpace(id)) {
		return PackCatalogPerson{}, fmt.Errorf("membership admission must come from another actual person")
	}
	profile, ok := ctx.State.Profile(strings.TrimSpace(id))
	if !ok || profile.SourceKey == "" || !character.ContainsID(ctx.Scene.Present, profile.ID) || !membershipActorAvailable(ctx.StateData, profile.ID) {
		return PackCatalogPerson{}, fmt.Errorf("membership contact must be a known person actually present")
	}
	for _, combat := range ctx.Combats {
		if combat.Ended {
			continue
		}
		for _, combatant := range combat.Combatants {
			if normalizeStatePanelActorID(combatant.ID) == profile.ID {
				return PackCatalogPerson{}, fmt.Errorf("membership contact is in an active combat")
			}
		}
	}
	if ctx.Catalog != nil {
		for _, person := range ctx.Catalog.Characters {
			if person.SourceKey != profile.SourceKey || person.FactionID != factionID || strings.TrimSpace(person.FactionName) == "" {
				continue
			}
			if !catalogPersonHasName(person, profile.Name) {
				return PackCatalogPerson{}, fmt.Errorf("membership contact does not match the frozen identity")
			}
			for _, status := range person.MembershipAdmission {
				if status != "formal" && status != "temporary" {
					return PackCatalogPerson{}, fmt.Errorf("invalid frozen admission authority")
				}
			}
			return person, nil
		}
	}
	return PackCatalogPerson{}, fmt.Errorf("membership contact has no matching frozen faction identity")
}

func buildTurnFactionMemberships(ctx factionMembershipContext, input *TurnCharacterFactsInput, projected []character.Exchange, turn TurnEvent, recordID, worldTime string, clockSeconds int64) ([]character.FactionMembership, error) {
	if err := validateTurnFactionMemberships(ctx, input); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, nil
	}
	input = canonicalMembershipInput(ctx, input)
	current := map[string]character.FactionMembership{}
	for _, m := range character.CurrentFactionMemberships(ctx.State.FactionMemberships) {
		current[m.FactionID] = m
	}
	var out []character.FactionMembership
	for _, draft := range input.Memberships {
		m := character.FactionMembership{ID: newID("mem"), FactionID: strings.TrimSpace(draft.FactionID), Status: draft.Status, Role: draft.Role, Statement: draft.Statement,
			SourceKind: "scene", WorldTime: worldTime, ClockSeconds: clockSeconds, TurnID: turn.ID, SourceEventID: recordID}
		if draft.Status == "left" && draft.SourceExchangeIndex == 0 {
			m.FactionName = current[m.FactionID].FactionName
			m.SourceKind = "player"
		} else {
			e := input.Exchanges[draft.SourceExchangeIndex-1]
			person, _ := membershipContact(ctx, e.CharacterID, m.FactionID)
			m.FactionName = person.FactionName
			m.SourceExchangeID = projected[draft.SourceExchangeIndex-1].ID
			if e.Speaker == "character" {
				m.AuthorityID = e.CharacterID
			}
		}
		if err := m.Validate(); err != nil {
			return nil, err
		}
		out = append(out, m.Normalize())
	}
	return out, nil
}

// Presence alone cannot make an archived, unconscious or occupied actor speak.
func membershipActorAvailable(state map[string]any, actorID string) bool {
	if state == nil {
		return true
	}
	if actorIsArchived(state, actorID) {
		return false
	}
	if actorStateFieldValue(state, actorID, xiuxianFieldHealth) != nil && xiuxianStoredInt(state, actorID, xiuxianFieldHealth) <= 0 {
		return false
	}
	if getPath(state, actorStateFieldPath(actorID, AdventureStateField)) != nil {
		actor, err := AdventureActorFromState(state, actorID)
		if err != nil || actor.Health <= 0 || actor.Battle != nil && actor.Battle.Status == "active" {
			return false
		}
	}
	arrangement, _ := actorStateFieldValue(state, actorID, xiuxianFieldArrangement).(string)
	return arrangement != xiuxianArrangementOccupied && arrangement != xiuxianArrangementTravel && arrangement != xiuxianArrangementRecovering
}

// The same frozen source key keeps its established actor ID in both submission
// and commit. A display alias cannot create a second admissions officer.
func canonicalMembershipInput(ctx factionMembershipContext, input *TurnCharacterFactsInput) *TurnCharacterFactsInput {
	if input == nil {
		return nil
	}
	aliases := TurnCharacterAliases(input, ctx.State)
	if len(aliases) == 0 {
		return input
	}
	copy := *input
	copy.Exchanges = append([]TurnCharacterExchangeInput(nil), input.Exchanges...)
	for i := range copy.Exchanges {
		if id := aliases[copy.Exchanges[i].CharacterID]; id != "" {
			copy.Exchanges[i].CharacterID = id
		}
		if id := aliases[copy.Exchanges[i].AddressedTo]; id != "" {
			copy.Exchanges[i].AddressedTo = id
		}
	}
	return &copy
}

func membershipContextForSubmission(validation TurnSubmissionContext) factionMembershipContext {
	ctx := factionMembershipContext{State: character.State{Profiles: validation.ExistingProfiles, FactionMemberships: validation.ExistingFactionMemberships}, StateData: validation.CurrentState, Catalog: validation.Meta.PackCatalog, Scene: validation.Scene, PlayerInput: validation.CurrentPlayerInput}
	if validation.WorldView != nil {
		ctx.Combats = validation.WorldView.Combats
	}
	return ctx
}
