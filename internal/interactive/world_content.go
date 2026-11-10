package interactive

import (
	"encoding/json"
	"fmt"

	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

// WorldContent is authored content frozen in the story's actor-state module.
// Readers and committers use this same snapshot; an empty catalog grants nothing.
// It contains definitions, not mutable inventory, quest progress or a second journal.
type WorldContent struct {
	QuestEnglish        map[string]quest.Text           `json:"quest_english"`
	SupplyRules         map[string]WorldSupplyRule      `json:"supply_rules"`
	QuestReviewItems    map[string]string               `json:"quest_review_items"`
	LoreGates           map[string]WorldLoreGate        `json:"lore_gates"`
	RoutePolicies       map[string]WorldRoutePolicy     `json:"route_policies"`
	RealmMinimumStages  map[string]string               `json:"realm_minimum_stages"`
	CooperationProjects []CooperationProjectDefinition  `json:"cooperation_projects"`
	PackID              string                          `json:"pack_id"`
	Labels              map[string]string               `json:"labels"`
	Aliases             map[string][]string             `json:"aliases"`
	Routes              []exploration.RouteDefinition   `json:"routes"`
	Quests              []quest.Definition              `json:"quests"`
	Realms              []exploration.DungeonDefinition `json:"realms"`
	Events              []publiquevent.Definition       `json:"events"`
}

func (content *WorldContent) Validate() error {
	if content == nil {
		return nil
	}
	if _, err := json.Marshal(content); err != nil {
		return fmt.Errorf("invalid world content: %w", err)
	}
	if content.PackID == "" {
		return fmt.Errorf("world content pack identity is empty")
	}
	seen := map[string]bool{}
	for _, route := range content.Routes {
		if seen[route.ID] {
			return fmt.Errorf("duplicate world route %q", route.ID)
		}
		seen[route.ID] = true
		if content.Labels[route.OriginLocationID] == "" || content.Labels[route.DestinationLocationID] == "" {
			return fmt.Errorf("world route %q references an unknown place", route.ID)
		}
		if _, _, diagnostics := exploration.ValidateRoute(route, journeySupportedMethods()); len(diagnostics) != 0 {
			return fmt.Errorf("invalid world route %q: %v", route.ID, diagnostics)
		}
	}
	for id := range content.Aliases {
		if content.Labels[id] == "" {
			return fmt.Errorf("world aliases reference unknown place %q", id)
		}
	}
	questIDs := map[string]bool{}
	for _, q := range content.Quests {
		if q.ID == "" || questIDs[q.ID] {
			return fmt.Errorf("duplicate or empty world quest %q", q.ID)
		}
		questIDs[q.ID] = true
		if d := quest.ValidateDefinition(q); len(d) > 0 {
			return fmt.Errorf("invalid world quest %q: %v", q.ID, d)
		}
		if q.PackageID != content.PackID {
			return fmt.Errorf("quest %q belongs to another world", q.ID)
		}
	}
	realmIDs := map[string]bool{}
	for _, r := range content.Realms {
		if r.ID == "" || realmIDs[r.ID] {
			return fmt.Errorf("duplicate or empty world realm %q", r.ID)
		}
		realmIDs[r.ID] = true
		if d := exploration.ValidateDungeonDefinition(r); len(d) > 0 {
			return fmt.Errorf("invalid world realm %q: %v", r.ID, d)
		}
		if r.PackageID != content.PackID {
			return fmt.Errorf("realm %q belongs to another world", r.ID)
		}
	}
	eventIDs := map[string]bool{}
	for _, e := range content.Events {
		if e.ID == "" || eventIDs[e.ID] {
			return fmt.Errorf("duplicate or empty world event %q", e.ID)
		}
		eventIDs[e.ID] = true
		if d := publiquevent.ValidateDefinition(e); len(d) > 0 {
			return fmt.Errorf("invalid world event %q: %v", e.ID, d)
		}
		if e.PackageID != content.PackID {
			return fmt.Errorf("event %q belongs to another world", e.ID)
		}
	}
	for id, p := range content.RoutePolicies {
		if !seen[id] {
			return fmt.Errorf("policy references unknown route %q", id)
		}
		if p.Event != nil && !eventIDs[p.Event.EventID] {
			return fmt.Errorf("route %q references unknown event", id)
		}
		for _, g := range p.AnyEcho {
			if !questIDs[g.TaskID] {
				return fmt.Errorf("route %q references unknown quest", id)
			}
		}
		if p.Discovery != nil && !realmIDs[p.Discovery.RealmID] {
			return fmt.Errorf("route %q references unknown realm", id)
		}
		if c := p.Crossing; c != nil {
			if c.MinimumStage == "" || c.ItemID == "" || c.Quantity <= 0 || c.HealthFraction < 0 || c.HealthFraction >= 1 || c.ManaFraction < 0 || c.ManaFraction > 1 {
				return fmt.Errorf("invalid crossing cost %q", id)
			}
		}
	}
	for id := range content.RealmMinimumStages {
		if !realmIDs[id] {
			return fmt.Errorf("minimum stage references unknown realm %q", id)
		}
	}
	projects := map[string]bool{}
	for _, p := range content.CooperationProjects {
		if p.ID == "" || projects[p.ID] || !realmIDs[p.RealmID] || content.Labels[p.LocationID] == "" || p.DurationSeconds <= 0 || p.PlayerItemQuantity < 0 {
			return fmt.Errorf("invalid cooperation project %q", p.ID)
		}
		projects[p.ID] = true
	}
	for id, g := range content.SupplyRules {
		if id == "" || content.Labels[g.LocationID] == "" || len(g.Merchants) == 0 {
			return fmt.Errorf("invalid supply source %q", id)
		}
	}
	for id := range content.QuestReviewItems {
		if !questIDs[id] {
			return fmt.Errorf("review source references unknown quest %q", id)
		}
	}
	for id := range content.QuestEnglish {
		if !questIDs[id] {
			return fmt.Errorf("translation references unknown quest %q", id)
		}
	}
	for id, g := range content.LoreGates {
		if id == "" {
			return fmt.Errorf("empty lore gate identity")
		}
		for _, place := range g.Locations {
			if content.Labels[place] == "" {
				return fmt.Errorf("lore gate %q references unknown place", id)
			}
		}
	}

	return nil
}

func CloneWorldContent(content *WorldContent) *WorldContent {
	if content == nil {
		return nil
	}
	data, err := json.Marshal(content)
	if err != nil {
		panic(fmt.Sprintf("encode validated world content: %v", err))
	}
	var cloned WorldContent
	if err := json.Unmarshal(data, &cloned); err != nil {
		panic(fmt.Sprintf("decode validated world content: %v", err))
	}
	return &cloned
}

func worldContentForMeta(meta StoryMeta) *WorldContent {
	if meta.ActorStateSchema != nil && meta.ActorStateSchema.System.WorldContent != nil {
		return meta.ActorStateSchema.System.WorldContent
	}
	if meta.PackBinding != nil {
		return &WorldContent{PackID: meta.PackBinding.PackID}
	}
	return nil
}

func (p *storyBranchProjection) bindWorld(meta StoryMeta) {
	p.WorldContent = worldContentForMeta(meta)
	p.CultivationRules = cultivationRulesForMeta(meta)
	p.WorldPackID = ""
	if meta.PackBinding != nil {
		p.WorldPackID = meta.PackBinding.PackID
	}
}

func worldJourneyRoutes(p *storyBranchProjection) []exploration.RouteDefinition {
	if p != nil && p.WorldContent != nil {
		return p.WorldContent.Routes
	}
	if p != nil && p.WorldPackID != "" {
		return nil
	}
	return journeyRouteDefinitions
}

func worldJourneyRoute(p *storyBranchProjection, id string) (exploration.RouteDefinition, bool) {
	for _, route := range worldJourneyRoutes(p) {
		if route.ID == id {
			return route, true
		}
	}
	return exploration.RouteDefinition{}, false
}

func (p *storyBranchProjection) worldContent() *WorldContent {
	if p == nil {
		return nil
	}
	if p.WorldContent != nil {
		return p.WorldContent
	}
	if p.WorldPackID != "" {
		return &WorldContent{PackID: p.WorldPackID}
	}
	return nil
}
func cloneAuthoredDefinition[T any](value T) T {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode authored definition: %v", err))
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		panic(fmt.Sprintf("decode authored definition: %v", err))
	}
	return out
}
func worldQuestDefinitions(c *WorldContent) []quest.Definition {
	if c == nil {
		return quest.Definitions()
	}
	return cloneAuthoredDefinition(c.Quests)
}
func worldQuestDefinition(c *WorldContent, id string) (quest.Definition, bool) {
	if c == nil {
		return quest.DefinitionByID(id)
	}
	for _, d := range c.Quests {
		if d.ID == id {
			return cloneAuthoredDefinition(d), true
		}
	}
	return quest.Definition{}, false
}

func worldRealmDefinitions(c *WorldContent) []exploration.DungeonDefinition {
	if c == nil {
		return exploration.RealmDefinitions()
	}
	return cloneAuthoredDefinition(c.Realms)
}
func worldRealmDefinition(c *WorldContent, id string) (exploration.DungeonDefinition, bool) {
	if c == nil {
		return exploration.RealmDefinition(id)
	}
	for _, d := range c.Realms {
		if d.ID == id {
			return cloneAuthoredDefinition(d), true
		}
	}
	return exploration.DungeonDefinition{}, false
}

func worldEventDefinitions(c *WorldContent) []publiquevent.Definition {
	if c == nil {
		return publiquevent.Definitions()
	}
	return cloneAuthoredDefinition(c.Events)
}
func worldEventDefinition(c *WorldContent, id string) (publiquevent.Definition, bool) {
	if c == nil {
		return publiquevent.DefinitionByID(id)
	}
	for _, d := range c.Events {
		if d.ID == id {
			return cloneAuthoredDefinition(d), true
		}
	}
	return publiquevent.Definition{}, false
}

func worldPlaceLabel(c *WorldContent, id string) string {
	if c == nil {
		return journeyPlaceLabel(id)
	}
	if name := c.Labels[id]; name != "" {
		return name
	}
	return id
}

func cultivationRulesForMeta(meta StoryMeta) *cultivation.Rules {
	if meta.ActorStateSchema != nil {
		return meta.ActorStateSchema.System.CultivationRules
	}
	return nil
}

func (p *storyBranchProjection) cultivationRules() *cultivation.Rules {
	if p == nil {
		return nil
	}
	return p.CultivationRules
}
func cultivationRulesFromSystems(systems []StoryDirectorActorStateSystem) *cultivation.Rules {
	if len(systems) == 0 {
		return nil
	}
	return systems[0].CultivationRules
}

// StoryCultivationRules reads the immutable rules already frozen in a story context.
// Callers must not mutate the returned data or substitute another world's defaults.
func StoryCultivationRules(meta StoryMeta) *cultivation.Rules { return cultivationRulesForMeta(meta) }

// StoryQuestDefinition reads a copied definition from the story-owned catalog.
func StoryQuestDefinition(meta StoryMeta, id string) (quest.Definition, bool) {
	return worldQuestDefinition(worldContentForMeta(meta), id)
}

// StoryWorldContent is the immutable authored catalog for this story.
func StoryWorldContent(meta StoryMeta) *WorldContent { return worldContentForMeta(meta) }

func firstWorldContent(worlds []*WorldContent) *WorldContent {
	if len(worlds) > 0 {
		return worlds[0]
	}
	return nil
}

func LocalizedStoryQuest(meta StoryMeta, definition quest.Definition, locale string) quest.Definition {
	if c := worldContentForMeta(meta); c != nil {
		return quest.LocalizedDefinition(definition, locale, c.QuestEnglish)
	}
	return quest.LocalizedDefinition(definition, locale)
}
