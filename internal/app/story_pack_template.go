package app

import (
	"fmt"
	"strings"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

// withPackTemplate resolves a story's ready-made protagonist template.
//
// The template is read from the pack file here, on the server, and never from
// the request. That is the point: a template's initial state is a rule outcome
// — the realm a character starts at and therefore every attribute the
// settlement derives from it — so a client may choose *which* template to play
// but cannot supply what it is worth.
//
// The template fills only what the request left empty. A player who edits the
// prefilled name or profile before starting keeps that edit; a request that
// names neither gets the template's own. The initial state has no such
// override: it is merged in unconditionally, and a key the bound actor-state
// module does not declare is a reported error rather than a field that quietly
// disappears at normalization.
func (s *InteractiveAppService) withPackTemplate(req interactive.CreateStoryRequest) (interactive.CreateStoryRequest, error) {
	if req.PackTemplate == nil {
		return req, nil
	}
	packID := strings.TrimSpace(req.PackTemplate.PackID)
	templateID := strings.TrimSpace(req.PackTemplate.TemplateID)
	if packID == "" || templateID == "" {
		return req, fmt.Errorf("快捷模板缺少剧本包或模板 ID")
	}
	// A templated start is still a start of that world. Without the binding the
	// story would read the shared book instead of the pack's, so a character
	// built for one world would begin in none of them.
	if req.PackBinding == nil || strings.TrimSpace(req.PackBinding.PackID) != packID {
		return req, fmt.Errorf("快捷模板必须绑定其所属剧本包: %s", packID)
	}
	cfg := s.cfg()
	if cfg == nil || strings.TrimSpace(cfg.DataDir()) == "" {
		return req, ErrNoWorkspace
	}
	pack, err := storypack.NewLibrary(cfg.DataDir()).Get(packID)
	if err != nil {
		return req, err
	}
	// The browser supplies the requested pack identity, but the capability
	// declaration is read from the saved pack on the server. A client must not
	// switch a no-combat world into a combat-enabled story by editing its request.
	boundAt := req.PackBinding.BoundAt
	req.PackBinding = pack.Binding()
	req.PackBinding.BoundAt = boundAt
	template, ok := pack.ProtagonistTemplate(templateID)
	if !ok {
		return req, fmt.Errorf("剧本包 %s 没有快捷模板 %s", packID, templateID)
	}
	if strings.TrimSpace(req.Origin) == "" {
		req.Origin = strings.TrimSpace(template.Origin)
	}
	if strings.TrimSpace(req.Protagonist.Name) == "" && strings.TrimSpace(req.Protagonist.Profile) == "" {
		req.Protagonist = interactive.StoryProtagonist{
			Mode:    interactive.StoryProtagonistModeCustom,
			Name:    strings.TrimSpace(template.CharacterName),
			Profile: strings.TrimSpace(template.Profile),
		}
	}
	// The template's authored first scene is not replayed into the request.
	// Writing it as a custom opening would present text the player never typed
	// as their own draft, which is the one thing the two-path start forbids; the
	// scene a templated character wakes in is the model's to write from the
	// template's origin and state.
	return seedPackTemplateState(req, template, req.IdentityStateOverlay)
}

// seedPackTemplateState writes a template's initial state into the protagonist
// Actor of the already-resolved actor-state system.
//
// It runs after the story-creation defaults because that is the first moment the
// bound module's templates are in hand, and before CreateStory because that is
// where BuildActorStateInitialChanges turns InitialActors into the story's first
// state delta.
//
// overlay is the confirmed character's prose fields. It is applied after the
// template so a created character is not the ready-made one, and before the name
// so the player's own name always wins. It may only touch keys the template
// declares, which is what keeps a creation draft out of 当前境界 and 灵石.
func seedPackTemplateState(req interactive.CreateStoryRequest, template storypack.ProtagonistTemplate, overlay map[string]any) (interactive.CreateStoryRequest, error) {
	if len(template.State) == 0 && len(overlay) == 0 {
		return req, nil
	}
	if req.ActorState == nil {
		return req, fmt.Errorf("快捷模板 %s 需要人物状态模块，但该剧本包没有绑定", template.ID)
	}
	index := -1
	for i, actor := range req.ActorState.InitialActors {
		if actor.ID == interactive.DefaultActorID || actor.TemplateID == interactive.DefaultActorID {
			index = i
			break
		}
	}
	if index < 0 {
		return req, fmt.Errorf("人物状态模块没有主角初始人物，快捷模板 %s 无法写入初态", template.ID)
	}
	actor := req.ActorState.InitialActors[index]
	merged := make(map[string]any, len(template.State)+len(overlay))
	for key, value := range template.State {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	if err := validateTemplateStateKeys(template, merged, *req.ActorState, actor.TemplateID); err != nil {
		return req, err
	}
	state := make(map[string]any, len(actor.State)+len(merged))
	for key, value := range actor.State {
		state[key] = value
	}
	for key, value := range merged {
		state[key] = value
	}
	// The protagonist template's 姓名 field is the state's own copy of the name,
	// and the name is the one identity a player is most likely to edit. Without
	// this the panel would show the template's character while the narrative
	// called the player something else.
	if name := strings.TrimSpace(req.Protagonist.Name); name != "" {
		state[protagonistNameField] = name
	}
	actor.State = state
	req.ActorState.InitialActors[index] = actor
	return req, nil
}

// protagonistNameField is the protagonist template's own name field, which is
// distinct from the Actor's display name.
const protagonistNameField = "姓名"

// validateTemplateStateKeys rejects a template field the bound module does not
// declare.
//
// Without this check the field would not fail: the initial-actor normalizer
// drops unknown keys silently, so a template naming 境界 instead of 当前境界
// would start the character at the module's default realm with nothing reported
// anywhere. Naming the field here keeps a pack's mistake a pack's mistake.
func validateTemplateStateKeys(template storypack.ProtagonistTemplate, state map[string]any, system interactive.StoryDirectorActorStateSystem, templateID string) error {
	var fields []interactive.ActorStateField
	for _, candidate := range system.Templates {
		if candidate.ID == templateID {
			fields = candidate.Fields
			break
		}
	}
	if len(fields) == 0 {
		return fmt.Errorf("人物状态模块缺少模板 %s，快捷模板 %s 无法写入初态", templateID, template.ID)
	}
	declared := make(map[string]bool, len(fields))
	for _, field := range fields {
		declared[strings.TrimSpace(field.Name)] = true
	}
	for key := range state {
		if !declared[strings.TrimSpace(key)] {
			return fmt.Errorf("快捷模板 %s 的状态字段不在模板 %s 中: %s", template.ID, templateID, key)
		}
	}
	if strings.TrimSpace(template.CharacterName) != "" && !declared[protagonistNameField] {
		return fmt.Errorf("人物状态模块模板 %s 缺少“%s”字段，快捷模板 %s 无法写入角色名", templateID, protagonistNameField, template.ID)
	}
	return nil
}
