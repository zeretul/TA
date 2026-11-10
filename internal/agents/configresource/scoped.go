package configresource

import (
	"context"
	"fmt"
	"strings"
	"ta/internal/cultivation"
)

// The two resources a purpose-scoped assistant may write. They are not extra
// storage: each is a view onto a record that already exists — one story's own
// configuration, and one world's saved new-game defaults — so a change made
// here is the same change the Game settings page and the world page make.
const (
	StorySettingsResource = "story_settings"
	WorldDefaultsResource = "world_defaults"
	// WorldBookResource is the world's own authored book. It is a third view
	// onto an existing record rather than a third store: the entries a new game
	// of this world starts with are the same entries the world page edits, and
	// filing a story's fact into the book is a write to that one file.
	WorldBookResource = "world_book"
)

// Every field below is a pointer because the update is a partial update: an
// omitted field means "leave this value as it is". Without that distinction a
// request that only changes the reply length would clear the three styles, and
// a request that only changes the combat style would clear the other two. A
// read fills every pointer, so echoing a read value back is still a full write.

// StylePurposesValue selects a style for each generation purpose. An explicitly
// empty Dialogue or Combat makes that purpose follow the main narrative again.
type StylePurposesValue struct {
	Main     *string `json:"main,omitempty"`
	Dialogue *string `json:"dialogue,omitempty"`
	Combat   *string `json:"combat,omitempty"`
}

// CheckSettingsValue tunes checks and rule-state handling.
type CheckSettingsValue struct {
	DifficultyShift          *int    `json:"difficulty_shift,omitempty"`
	RollModifier             *int    `json:"roll_modifier,omitempty"`
	RuleStateConsumptionMode *string `json:"rule_state_consumption_mode,omitempty"`
	RuleVisibilityMode       *string `json:"rule_visibility_mode,omitempty"`
}

// StorySettingsValue is the part of one story's own configuration a
// purpose-scoped assistant may read and change. It mirrors the fields the Game
// settings page edits, so the assistant and the page update the same story
// record through the same store method instead of a second copy of the values.
//
// Narrative requirements are deliberately absent. The per-story creative
// direction layer was cancelled, so this surface must not reintroduce it as a
// third, invisible override.
type StorySettingsValue struct {
	// Revision is the story's current configuration revision. An update must
	// send back the revision it read, so an edit made in the page between the
	// read and the write is reported instead of overwritten.
	Revision           string              `json:"revision,omitempty"`
	PlanningMode       *string             `json:"planning_mode,omitempty"`
	PlanningTemplateID *string             `json:"planning_template_id,omitempty"`
	StylePurposes      *StylePurposesValue `json:"style_purposes,omitempty"`
	ReplyTargetChars   *int                `json:"reply_target_chars,omitempty"`
	ChoiceCount        *int                `json:"choice_count,omitempty"`
	CheckSettings      *CheckSettingsValue `json:"check_settings,omitempty"`
}

// WorldDefaultsValue is one world's saved new-game defaults. It is a default,
// not a lock: saving it changes the next game started from that world and never
// a story already under way.
//
// Narrative requirements are deliberately absent here too. The field is retired
// from every surface a player or an assistant can reach; a save keeps whatever
// text an older workspace already stored because the block is written on top of
// the current record, not because this surface carries it.
type WorldDefaultsValue struct {
	Revision           string              `json:"revision,omitempty"`
	PlanningMode       *string             `json:"planning_mode,omitempty"`
	PlanningTemplateID *string             `json:"planning_template_id,omitempty"`
	StylePurposes      *StylePurposesValue `json:"style_purposes,omitempty"`
	ReplyTargetChars   *int                `json:"reply_target_chars,omitempty"`
	CheckSettings      *CheckSettingsValue `json:"check_settings,omitempty"`
}

// WorldEntryValue is one entry filed into a world's book.
//
// It is the world book's own shape, not a lore item's, because the book is
// where a collected entry has to end up: a second store would be a second copy
// of the same fact. EntryID and SourceRef are both optional; the collector
// derives an id from the name and treats a matching SourceRef as the same entry,
// so collecting one entity twice updates it instead of appending a duplicate.
type WorldEntryValue struct {
	Audience         string                      `json:"audience,omitempty"`
	CraftMethod      *cultivation.CraftMethodDef `json:"craft_method,omitempty"`
	EntryID          string                      `json:"entry_id,omitempty"`
	Name             string                      `json:"name"`
	Type             string                      `json:"type,omitempty"`
	Importance       string                      `json:"importance,omitempty"`
	Tags             []string                    `json:"tags,omitempty"`
	Keywords         []string                    `json:"keywords,omitempty"`
	LoadMode         string                      `json:"load_mode,omitempty"`
	BriefDescription string                      `json:"brief_description,omitempty"`
	Content          string                      `json:"content"`
	SourceRef        string                      `json:"source_ref,omitempty"`
}

// WorldEntryCollected is what a collection wrote, including the entry as it now
// stands so the assistant can read back what it filed without a second call.
type WorldEntryCollected struct {
	PackID         string          `json:"pack_id"`
	EntryID        string          `json:"entry_id"`
	Created        bool            `json:"created"`
	ContentVersion int             `json:"content_version"`
	Entry          WorldEntryValue `json:"entry"`
}

// WorldBookValue is one world's authored book as a scoped assistant reads it.
//
// Writable is reported rather than left to a failed write: a world that ships
// with the build has no file under the data root, so a collection into it would
// shadow a delivered asset for every story started from it. The assistant tells
// the player to save their own copy first instead of trying.
type WorldBookValue struct {
	CraftMethods []cultivation.CraftMethodDef `json:"craft_methods,omitempty"`
	WorldID      string                       `json:"world_id"`
	Writable     bool                         `json:"writable"`
	Entries      []WorldEntryValue            `json:"entries"`
}

// ScopedSettingsWriter is the application's persistence for the objects a
// purpose-scoped assistant may change. It is supplied by the host that owns the
// story and world records, so this package never re-implements their storage,
// validation or mutation leases.
//
// CollectWorldEntry is the world book's half: a story's own entity becomes part
// of the world it was played in, written through the same pack file the world
// page edits. It is deliberately part of this interface rather than a fourth
// store — the book a new game reads and the book this writes are one record.
type ScopedSettingsWriter interface {
	ReadStorySettings(ctx context.Context, storyID string) (StorySettingsValue, error)
	UpdateStorySettings(ctx context.Context, storyID string, value StorySettingsValue) (StorySettingsValue, error)
	ReadWorldDefaults(ctx context.Context, worldID string) (WorldDefaultsValue, error)
	UpdateWorldDefaults(ctx context.Context, worldID string, value WorldDefaultsValue, expectedRevision string) (WorldDefaultsValue, error)
	ReadWorldBook(ctx context.Context, worldID string) (WorldBookValue, error)
	CollectWorldEntry(ctx context.Context, worldID string, value WorldEntryValue) (WorldEntryCollected, error)
}

// ScopedTarget binds a purpose-scoped assistant to the objects it belongs to.
//
// The identifiers are the server's own resolution of the conversation's
// surface, never a value read out of message text: an assistant opened for one
// story writes that story and the world it is bound to, and nothing else. An
// empty identifier has no writable resource at all — that is the refusal to
// fall back to the shared libraries, not a wildcard.
type ScopedTarget struct {
	StoryID string
	WorldID string
	Writer  ScopedSettingsWriter
}

func (target *ScopedTarget) storyID() string {
	if target == nil || target.Writer == nil {
		return ""
	}
	return strings.TrimSpace(target.StoryID)
}

func (target *ScopedTarget) worldID() string {
	if target == nil || target.Writer == nil {
		return ""
	}
	return strings.TrimSpace(target.WorldID)
}

// readOnlyTemplate keeps a shared preset library as a read source. The game
// assistant reads installed styles, rules and image presets to choose from
// them; overwriting one in place would change every world at once, so the
// mutation operations are absent from the contract rather than merely refused.
func readOnlyTemplate(adapter Adapter) Adapter {
	descriptor := adapter.Descriptor()
	descriptor.Operations = []string{ReadList, ReadGet}
	return configResourceAdapter{descriptor: descriptor, list: adapter.List, get: adapter.Get}
}

func newStorySettingsResource(target *ScopedTarget) Adapter {
	return configResourceAdapter{
		descriptor: Descriptor{
			Name: StorySettingsResource,
			Description: "This story's own gameplay settings: planning mode and template, the style used for the main narrative, " +
				"dialogue and combat, reply length, choice count, and check settings. List or get it to read the current values " +
				"and the revision, then update only the fields the player asked to change and send that revision back. It changes " +
				"this story only; another story and the world's new-game defaults are separate records.",
			Operations: []string{ReadList, ReadGet, ApplyUpdate}, RevisionField: "revision",
			Reference: "references/story-settings.md",
		},
		list: func(ctx context.Context, _ ReadRequest) (any, error) {
			bound := target.storyID()
			if bound == "" {
				return nil, Missing(fmt.Errorf("this conversation is not bound to a story"))
			}
			value, err := target.Writer.ReadStorySettings(ctx, bound)
			if err != nil {
				return nil, err
			}
			return Catalog{Items: []any{value}}, nil
		},
		get: func(ctx context.Context, request ReadRequest) (any, error) {
			bound := target.storyID()
			ids := normalizeConfigIDs(request.IDs)
			if len(ids) != 1 {
				return nil, fmt.Errorf("story_settings get requires one id")
			}
			if bound == "" {
				return nil, Missing(fmt.Errorf("this conversation is not bound to a story"))
			}
			if ids[0] != bound {
				return nil, Missing(fmt.Errorf("story %q is not the story this conversation was opened for", ids[0]))
			}
			return target.Writer.ReadStorySettings(ctx, bound)
		},
		apply: func(ctx context.Context, mutation Mutation) (any, error) {
			bound := target.storyID()
			if bound == "" {
				return nil, fmt.Errorf("this conversation is not bound to a story, so story settings cannot be written")
			}
			if id := strings.TrimSpace(mutation.ID); id != bound {
				return nil, fmt.Errorf("story_settings is bound to story %q; %q is outside this conversation", bound, id)
			}
			current, err := target.Writer.ReadStorySettings(ctx, bound)
			if err != nil {
				return nil, err
			}
			if err := requireConfigRevision(StorySettingsResource, bound, mutation.Revision, current.Revision); err != nil {
				return nil, err
			}
			var value StorySettingsValue
			if err := decodeConfigValue(mutation.Value, &value); err != nil {
				return nil, err
			}
			updated, err := target.Writer.UpdateStorySettings(ctx, bound, value)
			if err != nil {
				return nil, err
			}
			return configMutationReceipt{
				Resource: StorySettingsResource, Operation: mutation.Operation,
				ID: bound, Revision: strings.TrimSpace(updated.Revision), Value: updated,
			}, nil
		},
	}
}

func newWorldBookResource(target *ScopedTarget) Adapter {
	return configResourceAdapter{
		descriptor: Descriptor{
			Name:        WorldBookResource,
			Description: "读取或收录当前世界的原典条目。create 可按 entry_id/source_ref 幂等更新。内置世界须先另存为用户世界。修仙世界收录制作方法时可附 craft_method：id、name_key、version、art、source、minimum_stage、minutes_per_batch、inputs[{def_id,quantity}]、outputs[{def_id,quantity,chance}]、loss_chance、requires_learning:true、failure_cost:consume_inputs、required_tools（非消耗的既有物品ID）、required_location（正式场景地点，空为不限）。产物只能引用已有目录效果，不接受自造效果。先读当前方法与版本，展示配方和代价供用户确认后保存；同ID修改必须提高version。保存仅影响以后新开局，旧故事保留冻结方法。",
			Operations:  []string{ReadList, ReadGet, ApplyCreate},
			Reference:   "references/world-book.md",
		},
		list: func(ctx context.Context, _ ReadRequest) (any, error) {
			bound := target.worldID()
			if bound == "" {
				return nil, Missing(fmt.Errorf("this conversation is not bound to a world"))
			}
			value, err := target.Writer.ReadWorldBook(ctx, bound)
			if err != nil {
				return nil, err
			}
			return Catalog{Items: []any{value}}, nil
		},
		get: func(ctx context.Context, request ReadRequest) (any, error) {
			bound := target.worldID()
			ids := normalizeConfigIDs(request.IDs)
			if len(ids) != 1 {
				return nil, fmt.Errorf("world_book get requires one id")
			}
			if bound == "" {
				return nil, Missing(fmt.Errorf("this conversation is not bound to a world"))
			}
			if ids[0] != bound {
				return nil, Missing(fmt.Errorf("world %q is not the world this conversation is bound to", ids[0]))
			}
			return target.Writer.ReadWorldBook(ctx, bound)
		},
		apply: func(ctx context.Context, mutation Mutation) (any, error) {
			bound := target.worldID()
			if bound == "" {
				return nil, fmt.Errorf("this conversation is not bound to a world, so its book cannot be written")
			}
			if mutation.Operation != ApplyCreate {
				// Collecting is the only world-book write a conversation has. A
				// delete or an in-place update of an authored entry belongs to the
				// world page, where the author sees the whole book.
				return nil, fmt.Errorf("world_book supports create only, got %q", mutation.Operation)
			}
			var value WorldEntryValue
			if err := decodeConfigValue(mutation.Value, &value); err != nil {
				return nil, err
			}
			if id := strings.TrimSpace(mutation.ID); id != "" && id != bound {
				return nil, fmt.Errorf("world_book is bound to world %q; %q is outside this conversation", bound, id)
			}
			collected, err := target.Writer.CollectWorldEntry(ctx, bound, value)
			if err != nil {
				return nil, err
			}
			return configMutationReceipt{
				Resource: WorldBookResource, Operation: mutation.Operation,
				ID: collected.EntryID, Value: collected,
			}, nil
		},
	}
}

func newWorldDefaultsResource(target *ScopedTarget) Adapter {
	return configResourceAdapter{
		descriptor: Descriptor{
			Name: WorldDefaultsResource,
			Description: "The saved new-game defaults of this story's world: planning mode and template, the style used for the main " +
				"narrative, dialogue and combat, reply length, creative direction, and check settings. List or get it to read the " +
				"current values and the revision, then update only the fields the player asked to change and send that revision " +
				"back. These are the values the next game started from this world begins with; they do not change a story already " +
				"under way.",
			Operations: []string{ReadList, ReadGet, ApplyUpdate}, RevisionField: "revision",
			Reference: "references/world-defaults.md",
		},
		list: func(ctx context.Context, _ ReadRequest) (any, error) {
			bound := target.worldID()
			if bound == "" {
				return nil, Missing(fmt.Errorf("this conversation is not bound to a world"))
			}
			value, err := target.Writer.ReadWorldDefaults(ctx, bound)
			if err != nil {
				return nil, err
			}
			return Catalog{Items: []any{value}}, nil
		},
		get: func(ctx context.Context, request ReadRequest) (any, error) {
			bound := target.worldID()
			ids := normalizeConfigIDs(request.IDs)
			if len(ids) != 1 {
				return nil, fmt.Errorf("world_defaults get requires one id")
			}
			if bound == "" {
				return nil, Missing(fmt.Errorf("this conversation is not bound to a world"))
			}
			if ids[0] != bound {
				return nil, Missing(fmt.Errorf("world %q is not the world this conversation is bound to", ids[0]))
			}
			return target.Writer.ReadWorldDefaults(ctx, bound)
		},
		apply: func(ctx context.Context, mutation Mutation) (any, error) {
			bound := target.worldID()
			if bound == "" {
				return nil, fmt.Errorf("this conversation is not bound to a world, so world defaults cannot be written")
			}
			if id := strings.TrimSpace(mutation.ID); id != bound {
				return nil, fmt.Errorf("world_defaults is bound to world %q; %q is outside this conversation", bound, id)
			}
			current, err := target.Writer.ReadWorldDefaults(ctx, bound)
			if err != nil {
				return nil, err
			}
			if err := requireConfigRevision(WorldDefaultsResource, bound, mutation.Revision, current.Revision); err != nil {
				return nil, err
			}
			var value WorldDefaultsValue
			if err := decodeConfigValue(mutation.Value, &value); err != nil {
				return nil, err
			}
			updated, err := target.Writer.UpdateWorldDefaults(ctx, bound, value, current.Revision)
			if err != nil {
				return nil, err
			}
			return configMutationReceipt{
				Resource: WorldDefaultsResource, Operation: mutation.Operation,
				ID: bound, Revision: strings.TrimSpace(updated.Revision), Value: updated,
			}, nil
		},
	}
}
