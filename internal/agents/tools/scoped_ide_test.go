package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	agent "ta/agent"

	"ta/config"
	"ta/internal/agents/configresource"
	"ta/internal/book/lore"
)

// gameAssistantScope mirrors the Game Settings assistant: one world's book and
// only the content resources that world and its story own.
func gameAssistantScope(scope string) IDEContext {
	return IDEContext{
		LoreScope: &scope,
		ConfigResources: []string{
			"style_reference", "narrative_style", "game_planning", "event_package",
			"rule_system", "state_system", "image_preset",
		},
	}
}

func scopedIDESettings() config.ResolvedAgentToolSettings {
	return config.ResolvedAgentToolSettings{
		config.AgentToolFilesystemRead:  true,
		config.AgentToolWorkspaceWrite:  true,
		config.AgentToolShell:           true,
		config.AgentToolWebSearch:       true,
		config.AgentToolWebFetch:        true,
		config.AgentToolBrowser:         true,
		config.AgentToolAsk:             true,
		config.AgentToolTodo:            true,
		config.AgentToolSkills:          true,
		config.AgentToolDelegation:      true,
		config.AgentToolScript:          true,
		config.AgentToolLoreRead:        true,
		config.AgentToolLoreWrite:       true,
		config.AgentToolConfigRead:      true,
		config.AgentToolConfigApply:     true,
		config.AgentToolImageGeneration: true,
	}
}

func toolNamesByDefinition(t *testing.T, definitions []agent.ToolDefinition) map[string]agent.ToolDefinition {
	t.Helper()
	names := make(map[string]agent.ToolDefinition, len(definitions))
	for _, definition := range definitions {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = definition
	}
	return names
}

func runToolDefinition(t *testing.T, definition agent.ToolDefinition, input string) (string, error) {
	t.Helper()
	result, err := definition.Tool.Run(context.Background(), input)
	return result.ModelContent, err
}

func TestScopedIDERegistersOnlyTheScopedProductTools(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	definitions, err := NewCatalog(cfg, nil, RuntimeExecutables{}).ScopedIDE(gameAssistantScope("world-a"))(scopedIDESettings())
	if err != nil {
		t.Fatal(err)
	}
	names := toolNamesByDefinition(t, definitions)
	for _, required := range []string{"read_lore_items", "list_lore_items", "write_lore_items", "config_read", "config_apply", "generate_image"} {
		if _, ok := names[required]; !ok {
			t.Fatalf("scoped IDE is missing %q; has %v", required, mapKeys(names))
		}
	}
	for _, forbidden := range []string{"read", "glob", "grep", "write", "edit", "bash", "pwsh", "web_search", "web_fetch", "skill"} {
		if _, ok := names[forbidden]; ok {
			t.Fatalf("scoped IDE registered %q, which is outside the purpose", forbidden)
		}
	}
}

func TestScopedIDEDescribesOnlyContentResources(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	definitions, err := NewCatalog(cfg, nil, RuntimeExecutables{}).ScopedIDE(gameAssistantScope("world-a"))(scopedIDESettings())
	if err != nil {
		t.Fatal(err)
	}
	read, ok := toolNamesByDefinition(t, definitions)["config_read"]
	if !ok {
		t.Fatal("scoped IDE has no config_read")
	}
	output, err := runToolDefinition(t, read, `{"operation":"describe"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range gameAssistantScope("world-a").ConfigResources {
		if !strings.Contains(output, `"name":"`+name+`"`) {
			t.Fatalf("describe is missing content resource %q:\n%s", name, output)
		}
	}
	for _, name := range []string{"skill", "agent_profile"} {
		if strings.Contains(output, `"name":"`+name+`"`) {
			t.Fatalf("describe exposed process-wide resource %q:\n%s", name, output)
		}
	}
}

func TestScopedIDERefusesToApplyAProcessWideResource(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	definitions, err := NewCatalog(cfg, nil, RuntimeExecutables{}).ScopedIDE(gameAssistantScope("world-a"))(scopedIDESettings())
	if err != nil {
		t.Fatal(err)
	}
	apply, ok := toolNamesByDefinition(t, definitions)["config_apply"]
	if !ok {
		t.Fatal("scoped IDE has no config_apply")
	}
	for _, resource := range []string{"skill", "agent_profile"} {
		_, err := runToolDefinition(t, apply, `{"operation":"create","resource":"`+resource+`","value":{"name":"x"}}`)
		if err == nil {
			t.Fatalf("config_apply accepted the process-wide resource %q", resource)
		}
		if !strings.Contains(err.Error(), "unknown config resource") {
			t.Fatalf("%s refusal = %v, want an unknown-resource error", resource, err)
		}
	}
}

func TestScopedIDEWritesLoreOnlyIntoItsOwnWorld(t *testing.T) {
	workspace := t.TempDir()
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: workspace}

	// A sibling world already owns an entry, and the user owns a shared one.
	otherStore := lore.NewScopedStore(workspace, "world-b")
	if _, err := otherStore.Create(lore.ItemInput{ID: "sibling", Type: "character", Name: "Sibling"}); err != nil {
		t.Fatal(err)
	}
	sharedStore := lore.NewStore(workspace)
	if _, err := sharedStore.Create(lore.ItemInput{ID: "user-note", Type: "other", Name: "User note"}); err != nil {
		t.Fatal(err)
	}

	definitions, err := NewCatalog(cfg, nil, RuntimeExecutables{}).ScopedIDE(gameAssistantScope("world-a"))(scopedIDESettings())
	if err != nil {
		t.Fatal(err)
	}
	names := toolNamesByDefinition(t, definitions)
	write, ok := names["write_lore_items"]
	if !ok {
		t.Fatal("scoped IDE has no write_lore_items")
	}
	read, ok := names["read_lore_items"]
	if !ok {
		t.Fatal("scoped IDE has no read_lore_items")
	}

	// A legitimate write succeeds and is stamped with the assistant's world.
	if _, err := runToolDefinition(t, write, `{"items":[{"id":"hero","type":"character","name":"Hero","content":"A hero of world A."}]}`); err != nil {
		t.Fatalf("legitimate write failed: %v", err)
	}
	created, err := lore.NewStore(workspace).Get("hero")
	if err != nil {
		t.Fatal(err)
	}
	if created.Scope != "world-a" {
		t.Fatalf("created scope = %q, want world-a", created.Scope)
	}

	// Re-writing the same ID is an update, not a duplicate create.
	if _, err := runToolDefinition(t, write, `{"items":[{"id":"hero","content":"A hero of world A, revised."}]}`); err != nil {
		t.Fatalf("same-ID update failed: %v", err)
	}
	all, err := lore.NewStore(workspace).ListAll()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range all {
		if item.ID == "hero" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("same-ID write produced %d entries, want 1", count)
	}

	// Another world's entry is invisible and unwritable.
	if _, err := runToolDefinition(t, read, `{"ids":["sibling"]}`); err != nil {
		t.Fatalf("reading another world must report a miss, not fail: %v", err)
	}
	output, err := runToolDefinition(t, read, `{"ids":["sibling"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "Sibling") {
		t.Fatalf("scoped read reached another world's entry:\n%s", output)
	}
	if _, err := runToolDefinition(t, write, `{"items":[{"id":"sibling","content":"overwritten"}]}`); err == nil {
		t.Fatal("scoped write reached another world's entry")
	}
	sibling, err := lore.NewStore(workspace).Get("sibling")
	if err != nil {
		t.Fatal(err)
	}
	if sibling.Content == "overwritten" {
		t.Fatal("another world's entry was rewritten")
	}

	// The user's shared entry stays readable but is not this world's to edit.
	if _, err := runToolDefinition(t, write, `{"items":[{"id":"user-note","content":"rewritten by a world"}]}`); err == nil {
		t.Fatal("scoped write reached a shared user entry")
	}
	note, err := lore.NewStore(workspace).Get("user-note")
	if err != nil {
		t.Fatal(err)
	}
	if note.Content == "rewritten by a world" {
		t.Fatal("the shared user entry was rewritten")
	}
}

func TestUnscopedIDEKeepsTheWholeEditorSurface(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	definitions, err := NewCatalog(cfg, nil, RuntimeExecutables{}).IDE()(scopedIDESettings())
	if err != nil {
		t.Fatal(err)
	}
	names := toolNamesByDefinition(t, definitions)
	read, ok := names["config_read"]
	if !ok {
		t.Fatal("unscoped IDE has no config_read")
	}
	output, err := runToolDefinition(t, read, `{"operation":"describe"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skill", "agent_profile"} {
		if !strings.Contains(output, `"name":"`+name+`"`) {
			t.Fatalf("unscoped IDE lost process-wide resource %q:\n%s", name, output)
		}
	}
}

func mapKeys(values map[string]agent.ToolDefinition) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

// settingsFixture stands in for the application's story and world records. The
// tool layer only ever sees a ScopedSettingsWriter, so this is the whole of the
// storage a scoped conversation can reach: two stories and two worlds, with
// the partial-update and revision semantics the real records have.
type settingsFixture struct {
	mu      sync.Mutex
	stories map[string]configresource.StorySettingsValue
	worlds  map[string]configresource.WorldDefaultsValue
	// writes records "<resource>:<id>" for every accepted update, which is how a
	// test proves a refusal happened before storage rather than after it.
	writes []string
}

func newSettingsFixture() *settingsFixture {
	return &settingsFixture{
		stories: map[string]configresource.StorySettingsValue{
			"story-a": {Revision: "story-a-0", ReplyTargetChars: intPointer(900), ChoiceCount: intPointer(3)},
			"story-b": {Revision: "story-b-0", ReplyTargetChars: intPointer(900), ChoiceCount: intPointer(3)},
		},
		worlds: map[string]configresource.WorldDefaultsValue{
			"world-a": {Revision: "world-a-0", ReplyTargetChars: intPointer(1200)},
			"world-b": {Revision: "world-b-0", ReplyTargetChars: intPointer(1200)},
		},
	}
}

func (fixture *settingsFixture) ReadStorySettings(_ context.Context, storyID string) (configresource.StorySettingsValue, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	value, ok := fixture.stories[storyID]
	if !ok {
		return configresource.StorySettingsValue{}, fmt.Errorf("story %q does not exist", storyID)
	}
	return value, nil
}

func (fixture *settingsFixture) UpdateStorySettings(_ context.Context, storyID string, value configresource.StorySettingsValue) (configresource.StorySettingsValue, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	current, ok := fixture.stories[storyID]
	if !ok {
		return configresource.StorySettingsValue{}, fmt.Errorf("story %q does not exist", storyID)
	}
	if value.Revision != "" && value.Revision != current.Revision {
		return configresource.StorySettingsValue{}, fmt.Errorf("story %q revision conflict", storyID)
	}
	next := current
	if value.PlanningMode != nil {
		next.PlanningMode = value.PlanningMode
	}
	if value.PlanningTemplateID != nil {
		next.PlanningTemplateID = value.PlanningTemplateID
	}
	if value.StylePurposes != nil {
		if next.StylePurposes == nil {
			next.StylePurposes = &configresource.StylePurposesValue{}
		}
		if value.StylePurposes.Main != nil {
			next.StylePurposes.Main = value.StylePurposes.Main
		}
		if value.StylePurposes.Dialogue != nil {
			next.StylePurposes.Dialogue = value.StylePurposes.Dialogue
		}
		if value.StylePurposes.Combat != nil {
			next.StylePurposes.Combat = value.StylePurposes.Combat
		}
	}
	if value.ReplyTargetChars != nil {
		next.ReplyTargetChars = value.ReplyTargetChars
	}
	if value.ChoiceCount != nil {
		next.ChoiceCount = value.ChoiceCount
	}
	if value.CheckSettings != nil {
		if next.CheckSettings == nil {
			next.CheckSettings = &configresource.CheckSettingsValue{}
		}
		if value.CheckSettings.DifficultyShift != nil {
			next.CheckSettings.DifficultyShift = value.CheckSettings.DifficultyShift
		}
		if value.CheckSettings.RollModifier != nil {
			next.CheckSettings.RollModifier = value.CheckSettings.RollModifier
		}
		if value.CheckSettings.RuleStateConsumptionMode != nil {
			next.CheckSettings.RuleStateConsumptionMode = value.CheckSettings.RuleStateConsumptionMode
		}
		if value.CheckSettings.RuleVisibilityMode != nil {
			next.CheckSettings.RuleVisibilityMode = value.CheckSettings.RuleVisibilityMode
		}
	}
	next.Revision = nextRevision(current.Revision)
	fixture.stories[storyID] = next
	fixture.writes = append(fixture.writes, configresource.StorySettingsResource+":"+storyID)
	return next, nil
}

func (fixture *settingsFixture) ReadWorldDefaults(_ context.Context, worldID string) (configresource.WorldDefaultsValue, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	value, ok := fixture.worlds[worldID]
	if !ok {
		return configresource.WorldDefaultsValue{}, fmt.Errorf("world %q does not exist", worldID)
	}
	return value, nil
}

func (fixture *settingsFixture) UpdateWorldDefaults(_ context.Context, worldID string, value configresource.WorldDefaultsValue, expectedRevision string) (configresource.WorldDefaultsValue, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	current, ok := fixture.worlds[worldID]
	if !ok {
		return configresource.WorldDefaultsValue{}, fmt.Errorf("world %q does not exist", worldID)
	}
	if expectedRevision != "" && expectedRevision != current.Revision {
		return configresource.WorldDefaultsValue{}, fmt.Errorf("world %q revision conflict", worldID)
	}
	next := current
	if value.PlanningMode != nil {
		next.PlanningMode = value.PlanningMode
	}
	if value.PlanningTemplateID != nil {
		next.PlanningTemplateID = value.PlanningTemplateID
	}
	if value.StylePurposes != nil {
		if next.StylePurposes == nil {
			next.StylePurposes = &configresource.StylePurposesValue{}
		}
		if value.StylePurposes.Main != nil {
			next.StylePurposes.Main = value.StylePurposes.Main
		}
		if value.StylePurposes.Dialogue != nil {
			next.StylePurposes.Dialogue = value.StylePurposes.Dialogue
		}
		if value.StylePurposes.Combat != nil {
			next.StylePurposes.Combat = value.StylePurposes.Combat
		}
	}
	if value.ReplyTargetChars != nil {
		next.ReplyTargetChars = value.ReplyTargetChars
	}
	if value.CheckSettings != nil {
		if next.CheckSettings == nil {
			next.CheckSettings = &configresource.CheckSettingsValue{}
		}
		if value.CheckSettings.DifficultyShift != nil {
			next.CheckSettings.DifficultyShift = value.CheckSettings.DifficultyShift
		}
		if value.CheckSettings.RollModifier != nil {
			next.CheckSettings.RollModifier = value.CheckSettings.RollModifier
		}
		if value.CheckSettings.RuleStateConsumptionMode != nil {
			next.CheckSettings.RuleStateConsumptionMode = value.CheckSettings.RuleStateConsumptionMode
		}
		if value.CheckSettings.RuleVisibilityMode != nil {
			next.CheckSettings.RuleVisibilityMode = value.CheckSettings.RuleVisibilityMode
		}
	}
	next.Revision = nextRevision(current.Revision)
	fixture.worlds[worldID] = next
	fixture.writes = append(fixture.writes, configresource.WorldDefaultsResource+":"+worldID)
	return next, nil
}

func (fixture *settingsFixture) ReadWorldBook(_ context.Context, worldID string) (configresource.WorldBookValue, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if _, ok := fixture.worlds[worldID]; !ok {
		return configresource.WorldBookValue{}, fmt.Errorf("world %q does not exist", worldID)
	}
	return configresource.WorldBookValue{WorldID: worldID, Writable: true}, nil
}

func (fixture *settingsFixture) CollectWorldEntry(_ context.Context, worldID string, value configresource.WorldEntryValue) (configresource.WorldEntryCollected, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if _, ok := fixture.worlds[worldID]; !ok {
		return configresource.WorldEntryCollected{}, fmt.Errorf("world %q does not exist", worldID)
	}
	if strings.TrimSpace(value.Name) == "" {
		return configresource.WorldEntryCollected{}, fmt.Errorf("world entry needs a name")
	}
	fixture.writes = append(fixture.writes, "world_entry:"+worldID)
	entry := value
	if entry.EntryID == "" {
		entry.EntryID = "entry-" + value.Name
	}
	return configresource.WorldEntryCollected{
		PackID: worldID, EntryID: entry.EntryID, Created: true, ContentVersion: 2, Entry: entry,
	}, nil
}

func (fixture *settingsFixture) writeLog() []string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return append([]string(nil), fixture.writes...)
}

func nextRevision(current string) string {
	const separator = "-"
	index := strings.LastIndex(current, separator)
	if index < 0 {
		return current + separator + "1"
	}
	var counter int
	if _, err := fmt.Sscanf(current[index+1:], "%d", &counter); err != nil {
		return current + separator + "1"
	}
	return fmt.Sprintf("%s%s%d", current[:index], separator, counter+1)
}

func intPointer(value int) *int { return &value }

func stringPointer(value string) *string { return &value }

// boundScope is the Game settings assistant's scope for one story and one
// world, with the shared preset libraries demoted to read sources.
func boundScope(loreScope string, target *configresource.ScopedTarget) IDEContext {
	scope := gameAssistantScope(loreScope)
	scope.SettingsTarget = target
	return scope
}

func settingsTarget(storyID, worldID string, writer configresource.ScopedSettingsWriter) *configresource.ScopedTarget {
	return &configresource.ScopedTarget{StoryID: storyID, WorldID: worldID, Writer: writer}
}

func scopedToolsFor(t *testing.T, scope IDEContext) map[string]agent.ToolDefinition {
	t.Helper()
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	definitions, err := NewCatalog(cfg, nil, RuntimeExecutables{}).ScopedIDE(scope)(scopedIDESettings())
	if err != nil {
		t.Fatal(err)
	}
	return toolNamesByDefinition(t, definitions)
}

// readRevision reads one bound settings record through the tool surface and
// returns the revision the model would send back, which is what a real update
// carries.
func readRevision(t *testing.T, tools map[string]agent.ToolDefinition, resource, id string) string {
	t.Helper()
	read, ok := tools["config_read"]
	if !ok {
		t.Fatal("scoped IDE has no config_read")
	}
	output, err := runToolDefinition(t, read, fmt.Sprintf(`{"operation":"get","resource":%q,"ids":[%q]}`, resource, id))
	if err != nil {
		t.Fatalf("read %s %q: %v", resource, id, err)
	}
	var response struct {
		Items []struct {
			Revision string `json:"revision"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode %s read: %v\n%s", resource, err, output)
	}
	if len(response.Items) != 1 {
		t.Fatalf("%s read returned %d items:\n%s", resource, len(response.Items), output)
	}
	return response.Items[0].Revision
}

func applyUpdate(t *testing.T, tools map[string]agent.ToolDefinition, resource, id, revision, value string) (string, error) {
	t.Helper()
	apply, ok := tools["config_apply"]
	if !ok {
		t.Fatal("scoped IDE has no config_apply")
	}
	input := fmt.Sprintf(`{"operation":"update","resource":%q,"id":%q,"revision":%q,"value":%s}`, resource, id, revision, value)
	return runToolDefinition(t, apply, input)
}

// TestScopedSettingsWriteTheBoundStoryAndWorld is the acceptance case the
// review asked for: a conversation bound to one story and one world reads a
// value, changes it, and reads the change back.
func TestScopedSettingsWriteTheBoundStoryAndWorld(t *testing.T) {
	fixture := newSettingsFixture()
	tools := scopedToolsFor(t, boundScope("world-a", settingsTarget("story-a", "world-a", fixture)))

	revision := readRevision(t, tools, configresource.StorySettingsResource, "story-a")
	receipt, err := applyUpdate(t, tools, configresource.StorySettingsResource, "story-a", revision, `{"reply_target_chars":640}`)
	if err != nil {
		t.Fatalf("bound story update failed: %v", err)
	}
	if !strings.Contains(receipt, `"id":"story-a"`) {
		t.Fatalf("receipt does not name the bound story:\n%s", receipt)
	}

	read, _ := tools["config_read"]
	output, err := runToolDefinition(t, read, `{"operation":"get","resource":"story_settings","ids":["story-a"]}`)
	if err != nil {
		t.Fatalf("reading the change back failed: %v", err)
	}
	if !strings.Contains(output, `"reply_target_chars":640`) {
		t.Fatalf("the change did not survive:\n%s", output)
	}
	// A partial update leaves every unnamed field alone.
	if !strings.Contains(output, `"choice_count":3`) {
		t.Fatalf("a reply-length edit cleared another field:\n%s", output)
	}

	worldRevision := readRevision(t, tools, configresource.WorldDefaultsResource, "world-a")
	if _, err := applyUpdate(t, tools, configresource.WorldDefaultsResource, "world-a", worldRevision, `{"reply_target_chars":800}`); err != nil {
		t.Fatalf("bound world update failed: %v", err)
	}
	output, err = runToolDefinition(t, read, `{"operation":"get","resource":"world_defaults","ids":["world-a"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"reply_target_chars":800`) {
		t.Fatalf("the world change did not survive:\n%s", output)
	}
	// The retired creative-direction layer is gone from this surface entirely, so
	// neither reading nor writing it is possible for a scoped assistant.
	if strings.Contains(output, "narrative_requirements") {
		t.Fatalf("the retired direction layer reappeared on the world surface:\n%s", output)
	}
}

// TestScopedSettingsRefuseAnotherConversationsRecords is the cross-world
// regression: a conversation bound to world B must not write the story or the
// world another conversation was opened for, even when it knows the exact id.
func TestScopedSettingsRefuseAnotherConversationsRecords(t *testing.T) {
	fixture := newSettingsFixture()
	tools := scopedToolsFor(t, boundScope("world-b", settingsTarget("story-b", "world-b", fixture)))

	for _, testCase := range []struct {
		resource string
		id       string
		value    string
	}{
		{configresource.StorySettingsResource, "story-a", `{"reply_target_chars":1}`},
		{configresource.WorldDefaultsResource, "world-a", `{"reply_target_chars":1}`},
	} {
		_, err := applyUpdate(t, tools, testCase.resource, testCase.id, "whatever-revision", testCase.value)
		if err == nil {
			t.Fatalf("%s accepted a write to %q, which belongs to another conversation", testCase.resource, testCase.id)
		}
		if !strings.Contains(err.Error(), "outside this conversation") {
			t.Fatalf("%s refusal = %v, want an out-of-scope refusal", testCase.resource, err)
		}
	}
	if writes := fixture.writeLog(); len(writes) != 0 {
		t.Fatalf("a refused write still reached storage: %v", writes)
	}
	// Reading another conversation's record is a miss, not a reach.
	read := tools["config_read"]
	output, err := runToolDefinition(t, read, `{"operation":"get","resource":"story_settings","ids":["story-a"]}`)
	if err == nil && strings.Contains(output, `"reply_target_chars":900`) {
		t.Fatalf("a scoped read reached another conversation's story:\n%s", output)
	}
}

// TestScopedSettingsDoNotLeakIntoASibling keeps the two records apart: editing
// this story changes neither the other story nor the world's new-game defaults.
func TestScopedSettingsDoNotLeakIntoASibling(t *testing.T) {
	fixture := newSettingsFixture()
	tools := scopedToolsFor(t, boundScope("world-a", settingsTarget("story-a", "world-a", fixture)))

	revision := readRevision(t, tools, configresource.StorySettingsResource, "story-a")
	if _, err := applyUpdate(t, tools, configresource.StorySettingsResource, "story-a", revision, `{"reply_target_chars":300}`); err != nil {
		t.Fatal(err)
	}

	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if got := *fixture.stories["story-a"].ReplyTargetChars; got != 300 {
		t.Fatalf("bound story reply length = %d, want 300", got)
	}
	if got := *fixture.stories["story-b"].ReplyTargetChars; got != 900 {
		t.Fatalf("the sibling story changed to %d", got)
	}
	if got := *fixture.worlds["world-a"].ReplyTargetChars; got != 1200 {
		t.Fatalf("the world's new-game defaults changed to %d", got)
	}
	if got := *fixture.worlds["world-b"].ReplyTargetChars; got != 1200 {
		t.Fatalf("another world's defaults changed to %d", got)
	}
}

// TestScopedSettingsWithoutABoundTargetWriteNothing is the refusal to fall back
// to the shared write scope: a conversation with no story or world still has a
// target, and that target has nothing it may name.
func TestScopedSettingsWithoutABoundTargetWriteNothing(t *testing.T) {
	fixture := newSettingsFixture()
	tools := scopedToolsFor(t, boundScope("world-a", settingsTarget("", "", fixture)))

	for _, resource := range []string{configresource.StorySettingsResource, configresource.WorldDefaultsResource} {
		_, err := applyUpdate(t, tools, resource, "story-a", "any-revision", `{"reply_target_chars":1}`)
		if err == nil {
			t.Fatalf("%s wrote with no bound target", resource)
		}
		if !strings.Contains(err.Error(), "not bound") {
			t.Fatalf("%s refusal = %v, want an unbound refusal", resource, err)
		}
		read := tools["config_read"]
		if _, err := runToolDefinition(t, read, fmt.Sprintf(`{"operation":"list","resource":%q}`, resource)); err == nil {
			t.Fatalf("%s listed values with no bound target", resource)
		}
	}
	if writes := fixture.writeLog(); len(writes) != 0 {
		t.Fatalf("an unbound conversation still wrote: %v", writes)
	}
}

// TestScopedSettingsRefuseAnOutdatedRevision keeps the compare-and-swap: the
// revision from a read is required, and a stale one is a conflict rather than
// an overwrite of an edit the page made in between.
func TestScopedSettingsRefuseAnOutdatedRevision(t *testing.T) {
	fixture := newSettingsFixture()
	tools := scopedToolsFor(t, boundScope("world-a", settingsTarget("story-a", "world-a", fixture)))

	stale := readRevision(t, tools, configresource.StorySettingsResource, "story-a")
	if _, err := applyUpdate(t, tools, configresource.StorySettingsResource, "story-a", stale, `{"reply_target_chars":400}`); err != nil {
		t.Fatal(err)
	}
	if _, err := applyUpdate(t, tools, configresource.StorySettingsResource, "story-a", stale, `{"reply_target_chars":500}`); err == nil {
		t.Fatal("an update with the pre-edit revision was accepted")
	} else if !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale revision refusal = %v", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if got := *fixture.stories["story-a"].ReplyTargetChars; got != 400 {
		t.Fatalf("story reply length = %d, want the first edit's 400", got)
	}
}

// TestScopedSharedLibrariesAreReadOnlyTemplates is the original defect: every
// shared preset library is a process-wide store, so a scoped conversation reads
// them as templates and cannot overwrite one in place to stand in for an edit
// to its own story or world.
func TestScopedSharedLibrariesAreReadOnlyTemplates(t *testing.T) {
	fixture := newSettingsFixture()
	tools := scopedToolsFor(t, boundScope("world-a", settingsTarget("story-a", "world-a", fixture)))
	apply := tools["config_apply"]

	for _, resource := range gameAssistantScope("world-a").ConfigResources {
		for _, mutation := range []string{
			fmt.Sprintf(`{"operation":"create","resource":%q,"value":{"id":"x","name":"x"}}`, resource),
			fmt.Sprintf(`{"operation":"update","resource":%q,"id":"x","revision":"r","value":{"id":"x","name":"x"}}`, resource),
			fmt.Sprintf(`{"operation":"delete","resource":%q,"id":"x","revision":"r"}`, resource),
		} {
			if _, err := runToolDefinition(t, apply, mutation); err == nil {
				t.Fatalf("%s accepted a mutation in a scoped conversation", resource)
			} else if !strings.Contains(err.Error(), "does not support operation") {
				t.Fatalf("%s refusal = %v, want an operation refusal", resource, err)
			}
		}
	}

	// They stay readable: choosing a style or an image preset is the reason the
	// assistant sees them at all.
	read := tools["config_read"]
	output, err := runToolDefinition(t, read, `{"operation":"describe"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range append(append([]string(nil), gameAssistantScope("world-a").ConfigResources...), configresource.StorySettingsResource, configresource.WorldDefaultsResource) {
		if !strings.Contains(output, `"name":"`+resource+`"`) {
			t.Fatalf("describe is missing %q:\n%s", resource, output)
		}
	}
}

// TestScopedSettingsSurviveARebuiltScope is the recovery case: the tool
// surface is rebuilt from the recorded purpose on every follow-up cycle, and
// the rebuilt surface must refuse exactly what the first one refused.
func TestScopedSettingsSurviveARebuiltScope(t *testing.T) {
	fixture := newSettingsFixture()
	first := scopedToolsFor(t, boundScope("world-a", settingsTarget("story-a", "world-a", fixture)))
	rebuilt := scopedToolsFor(t, boundScope("world-a", settingsTarget("story-a", "world-a", fixture)))

	revision := readRevision(t, first, configresource.StorySettingsResource, "story-a")
	if _, err := applyUpdate(t, rebuilt, configresource.StorySettingsResource, "story-a", revision, `{"reply_target_chars":700}`); err != nil {
		t.Fatalf("the rebuilt surface lost the binding: %v", err)
	}
	if _, err := applyUpdate(t, rebuilt, configresource.StorySettingsResource, "story-b", revision, `{"reply_target_chars":700}`); err == nil {
		t.Fatal("the rebuilt surface widened to another story")
	}
	apply := rebuilt["config_apply"]
	if _, err := runToolDefinition(t, apply, `{"operation":"update","resource":"image_preset","id":"x","revision":"r","value":{"id":"x"}}`); err == nil {
		t.Fatal("the rebuilt surface widened back to the shared libraries")
	}
}

// TestUnscopedPresetEditingIsUnchanged keeps the narrowing to the game
// assistant: the Writing Agent still edits presets in place, so the change adds
// no loss to the consumers that rely on it.
func TestUnscopedPresetEditingIsUnchanged(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	definitions, err := NewCatalog(cfg, nil, RuntimeExecutables{}).Configuration()(scopedIDESettings())
	if err != nil {
		t.Fatal(err)
	}
	apply, ok := toolNamesByDefinition(t, definitions)["config_apply"]
	if !ok {
		t.Fatal("the writing agent has no config_apply")
	}
	// A target-less registry keeps every mutation operation on every content
	// resource; the error here is about the missing resource, never the
	// operation, which proves the operation survived.
	_, err = runToolDefinition(t, apply, `{"operation":"update","resource":"image_preset","id":"x","revision":"r","value":{"id":"x"}}`)
	if err == nil {
		t.Fatal("updating a preset that does not exist must fail")
	}
	if strings.Contains(err.Error(), "does not support operation") {
		t.Fatalf("the writing agent lost preset editing: %v", err)
	}
}

// TestScopedSurfaceIsReadOnlyEvenWithoutATarget is the defence in depth: a
// scoped construction never regains the shared libraries' write operations, so
// a caller that forgets to supply a target cannot reopen the original defect.
func TestScopedSurfaceIsReadOnlyEvenWithoutATarget(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	for name, factory := range map[string]Factory{
		"scoped IDE":           NewCatalog(cfg, nil, RuntimeExecutables{}).ScopedIDE(gameAssistantScope("world-a")),
		"scoped configuration": NewCatalog(cfg, nil, RuntimeExecutables{}).ScopedConfiguration(gameAssistantScope("world-a")),
	} {
		definitions, err := factory(scopedIDESettings())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		apply, ok := toolNamesByDefinition(t, definitions)["config_apply"]
		if !ok {
			t.Fatalf("%s has no config_apply", name)
		}
		if _, err := runToolDefinition(t, apply, `{"operation":"update","resource":"image_preset","id":"x","revision":"r","value":{"id":"x"}}`); err == nil {
			t.Fatalf("%s allowed a shared preset mutation with no target", name)
		} else if !strings.Contains(err.Error(), "does not support operation") {
			t.Fatalf("%s refusal = %v, want an operation refusal", name, err)
		}
	}
}
