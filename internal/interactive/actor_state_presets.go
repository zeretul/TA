package interactive

import "ta/internal/cultivation"

const (
	ActorStateDetectiveID      = "detective-investigation-v1"
	ActorStateXiuxianID        = "xiuxian-state"
	ActorStateWesternFantasyID = "western-fantasy-state"
	ActorStateApocalypseID     = "apocalypse-state"
	ActorStateInfiniteFlowID   = "infinite-flow-state"

	ActorStateImportantCharacterTemplateID = "important_character"
	ActorStateOpponentTemplateID           = "opponent"
	ActorStateStoryContextTemplateID       = "story_context"
	ActorStateWorldEntitiesTemplateID      = "world_entities"
	DefaultStoryContextActorID             = "story"
	DefaultWorldEntitiesActorID            = "world"
)

// actorStatePresetSpec contains only the deltas that genuinely vary by genre.
// All presets keep the same five-template boundary so genre additions do not
// reintroduce fragmented global ledgers.
type actorStatePresetSpec struct {
	ID          string
	Name        string
	Description string
	PanelFields []ActorStateField
	StateFields []ActorStateField

	ProtagonistFields        []ActorStateField
	ImportantCharacterFields []ActorStateField
	OpponentFields           []ActorStateField
	StoryFields              []ActorStateField

	AbilityGuidance      string
	ItemGuidance         string
	RelationshipGuidance string
	QuestGuidance        string
	LocationGuidance     string
	FactionGuidance      string
}

func builtinActorStateModules() []ActorStateModule {
	return []ActorStateModule{
		DefaultActorStateModule(),
		detectiveActorStateModule(),
		xiuxianActorStateModule(),
		westernFantasyActorStateModule(),
		apocalypseActorStateModule(),
		infiniteFlowActorStateModule(),
	}
}

func detectiveActorStateModule() ActorStateModule {
	module := DefaultActorStateModule()
	module.ID = ActorStateDetectiveID
	module.Name = "侦探调查状态"
	module.Description = "无等级成长的调查状态；四项能力按1至10记录，参与已注册的调查检定。"
	for index := range module.ActorState.Templates {
		template := &module.ActorState.Templates[index]
		switch template.ID {
		case DefaultActorID:
			template.Name = "侦探状态表"
			template.Description = "调查能力与通用人物状态；能力值通过悬疑调查的检定绑定真实生效。"
			fields := template.Fields[:0]
			for _, field := range template.Fields {
				if field.Name != "等级" {
					fields = append(fields, field)
				}
			}
			template.Fields = fields
			template.Fields = append(template.Fields,
				detectiveAbilityField("观察", "发现现场细节与线索的能力。"),
				detectiveAbilityField("逻辑", "比较证据、检验证词矛盾的能力。"),
				detectiveAbilityField("交涉", "争取证人合作的能力。"),
				detectiveAbilityField("体能", "追逐、脱困等行动的能力。"),
			)
		case ActorStateStoryContextTemplateID:
			fields := template.Fields[:0]
			for _, field := range template.Fields {
				if field.Name != "本次交战" {
					fields = append(fields, field)
				}
			}
			template.Fields = fields
		}
	}
	return normalizeActorStateModule(module)
}

func detectiveAbilityField(name, description string) ActorStateField {
	field := scaledNumberStateField("", name, description, "调查能力", 5, 1, 10, "1–10；5为普通水平。", "只在对应调查行动的受支持检定中读取，不因叙事成功自动成长。")
	return field
}

func builtinActorStateModuleByID(id string) (ActorStateModule, bool) {
	id = normalizeDirectorModuleID(id)
	for _, item := range builtinActorStateModules() {
		if item.ID == id {
			return item, true
		}
	}
	return ActorStateModule{}, false
}

// BuiltinActorStatePrivateFieldNames returns the fields each template of a
// built-in preset marks private, by template ID.
//
// A story freezes its schema at creation, which is what keeps a rules edit from
// rewriting a save. The other side of that guarantee is that a boundary added
// later never reaches a story already running: the frozen schema is the only
// thing the projection reads, and it has no private mark on a field that was
// public when the story started. The player then sees an opponent's purse in a
// save that began last week and not in one that begins tomorrow, which is not a
// boundary at all.
//
// So the projection asks this for the current declaration and treats it as a
// floor. Nothing in the save is rewritten — the journal keeps every value, and
// the field keeps whatever visibility its own schema says — which is why this
// can be additive without breaking a story that named its own custom preset:
// an unknown ID returns nothing and the frozen schema stands alone.
func BuiltinActorStatePrivateFieldNames(id string) map[string][]string {
	module, ok := builtinActorStateModuleByID(id)
	if !ok {
		return nil
	}
	names := make(map[string][]string, len(module.ActorState.Templates))
	for _, template := range module.ActorState.Templates {
		var private []string
		for _, field := range template.Fields {
			if field.Visibility != ActorStateVisibilityPrivate {
				continue
			}
			private = append(private, actorStateFieldID(field))
		}
		if len(private) > 0 {
			names[template.ID] = private
		}
	}
	return names
}

// actorStatePrivateFloorForMeta resolves the preset a story actually runs on
// and asks for its private declaration.
//
// The frozen module graph is preferred over the editable selection because it
// is what the story started with; the selection is the fallback for a story old
// enough to have no frozen graph. An unknown or custom module answers nothing,
// and the frozen schema stands alone.
func actorStatePrivateFloorForMeta(meta StoryMeta) map[string][]string {
	refs := StoryDirectorModuleRefs{}
	if meta.FrozenModules != nil {
		refs = NormalizeStoryDirectorModuleRefs(meta.FrozenModules.ModuleRefs)
	}
	if refs.ActorStateID == "" && meta.ModuleRefs != nil {
		refs = NormalizeStoryDirectorModuleRefs(*meta.ModuleRefs)
	}
	if refs.ActorStateDisabled {
		return nil
	}
	return BuiltinActorStatePrivateFieldNames(refs.ActorStateID)
}

// actorStateSchemaWithPresetFloor is the read-side fix for a boundary that was
// declared after a story started.
//
// It returns the schema a reader should see: the frozen one, plus every private
// mark the story's preset declares today. Nothing is written — the schema file
// and the journal keep exactly what they had — so a save that began before a
// field became private starts withholding it on the next read, and the rules
// keep settling on the full record.
//
// The mark is additive. A field the frozen schema already marks private keeps
// its mark, and a field the preset no longer marks stays private, because the
// story was played under the stricter declaration and reopening it is not this
// function's business.
func actorStateSchemaWithPresetFloor(schema *ActorStateSchemaSnapshot, meta StoryMeta) *ActorStateSchemaSnapshot {
	if schema == nil {
		return nil
	}
	floor := actorStatePrivateFloorForMeta(meta)
	if len(floor) == 0 {
		return schema
	}
	var next *ActorStateSchemaSnapshot
	for templateIndex := range schema.System.Templates {
		template := &schema.System.Templates[templateIndex]
		names := floor[template.ID]
		if len(names) == 0 {
			continue
		}
		wanted := make(map[string]bool, len(names))
		for _, name := range names {
			wanted[name] = true
		}
		for fieldIndex := range template.Fields {
			field := &template.Fields[fieldIndex]
			if field.Visibility == ActorStateVisibilityPrivate || !wanted[actorStateFieldID(*field)] {
				continue
			}
			if next == nil {
				next = cloneActorStateSchemaForReader(schema)
			}
			next.System.Templates[templateIndex].Fields[fieldIndex].Visibility = ActorStateVisibilityPrivate
		}
	}
	if next == nil {
		return schema
	}
	return next
}

func cloneActorStateSchemaForReader(schema *ActorStateSchemaSnapshot) *ActorStateSchemaSnapshot {
	next := *schema
	next.System = schema.System
	next.System.Templates = make([]ActorStateTemplate, len(schema.System.Templates))
	for templateIndex, template := range schema.System.Templates {
		next.System.Templates[templateIndex] = template
		next.System.Templates[templateIndex].Fields = append([]ActorStateField(nil), template.Fields...)
	}
	return &next
}

func xiuxianActorStateModule() ActorStateModule {
	return actorStatePresetModule(xiuxianActorStatePresetSpec())
}

func westernFantasyActorStateModule() ActorStateModule {
	return actorStatePresetModule(westernFantasyActorStatePresetSpec())
}

func apocalypseActorStateModule() ActorStateModule {
	return actorStatePresetModule(apocalypseActorStatePresetSpec())
}

func infiniteFlowActorStateModule() ActorStateModule {
	return actorStatePresetModule(infiniteFlowActorStatePresetSpec())
}

func actorStatePresetModule(spec actorStatePresetSpec) ActorStateModule {
	return normalizeActorStateModule(ActorStateModule{
		Version:     storyDirectorModuleVersion,
		ID:          spec.ID,
		Name:        spec.Name,
		Description: spec.Description,
		ActorState: actorStateSystemForPreset(cultivation.ReferenceRules(),
			spec),
	})
}

func actorStateSystemForPreset(cultivationRules *cultivation.Rules, spec actorStatePresetSpec) StoryDirectorActorStateSystem {
	if spec.ID != ActorStateXiuxianID {
		cultivationRules = nil
	}
	return normalizeActorStateSystem(StoryDirectorActorStateSystem{
		CultivationRules: cultivationRules,
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(cultivationRules,
				spec),
			storyContextStateTemplate(spec),
			importantCharacterStateTemplate(cultivationRules,
				spec),
			opponentStateTemplate(cultivationRules,
				spec),
			worldEntitiesStateTemplate(spec),
		},
		InitialActors: defaultActorStateInitialActors(),
	})
}

func defaultActorStateInitialActors() []ActorStateInitialActor {
	return []ActorStateInitialActor{
		{
			ID:          DefaultActorID,
			Name:        "主角",
			TemplateID:  DefaultActorID,
			Role:        "protagonist",
			Description: "当前故事的可玩主角；身份、状态、技能、重要物品和关系集中维护在此 Actor。",
		},
		{
			ID:          DefaultStoryContextActorID,
			Name:        "故事状态",
			TemplateID:  ActorStateStoryContextTemplateID,
			Role:        "story_context",
			Description: "当前场景、世界局势与当前任务的统一状态对象。",
		},
		{
			ID:          DefaultWorldEntitiesActorID,
			Name:        "世界实体",
			TemplateID:  ActorStateWorldEntitiesTemplateID,
			Role:        "world",
			Description: "仍对当前故事有行动价值的重要地点与势力。",
		},
	}
}

func actorStatePresetTemplate(id, name, description string, fields []ActorStateField) ActorStateTemplate {
	return ActorStateTemplate{
		ID:          id,
		Name:        name,
		Description: description,
		Fields:      fields,
	}
}
