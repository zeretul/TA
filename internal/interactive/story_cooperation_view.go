package interactive

import (
	"sort"
	"strings"

	"ta/internal/character"
	"ta/internal/exploration"
)

// cooperationProjectViews only reads committed clues and encountered profiles.
// Eligibility plans are discarded; opening this view never performs joint work.
func cooperationProjectViews(p *storyBranchProjection, meta StoryMeta) []CooperationProjectView {
	views := []CooperationProjectView{}
	if p == nil || p.Characters == nil {
		return views
	}
	ids := make([]string, 0, len(p.Realms))
	for id := range p.Realms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	for _, id := range ids {
		instance := p.Realms[id]
		definition, ok := cooperationViewDefinition(instance, p.worldContent())
		if !ok || id != instance.Snapshot.Binding.InstanceID || instance.Snapshot.Binding.WorldID != realmWorldID(meta, instance.Snapshot.Binding.BranchID) || !storyPackAllows(meta, definition.PackageID, definition.PackageVersion) || !cooperationViewHasClue(instance, definition) {
			continue
		}
		for _, project := range worldCooperationProjects(p.worldContent()) {
			if project.RealmID != definition.ID {
				continue
			}
			partner, known := cooperationPartner(p, project)
			if !known {
				continue
			}
			_, energy, reason := cooperationWorkEligibility(p, p.State, meta, project, instance)
			view := CooperationProjectView{
				Name: project.Name, Description: project.Description, PlayerRole: project.PlayerRole, PartnerRole: project.PartnerRole,
				ClueSourceRef: "clue:" + instance.ExploreSources["clue"],
				ProjectID:     project.ID, ProjectVersion: project.Version, RealmID: project.RealmID, InstanceID: id,
				NameKey: project.NameKey, DescriptionKey: project.DescriptionKey,
				PlayerRoleKey: project.PlayerRoleKey, PartnerRoleKey: project.PartnerRoleKey,
				PartnerID: partner.ID, PartnerName: partner.DisplayName(), PartnerSourceKey: partner.SourceKey, LocationID: project.LocationID,
				LocationName: worldPlaceLabel(p.worldContent(), project.LocationID), Phase: "none", DurationSeconds: project.DurationSeconds,
				PartnerEnergyCost: energy, PartnerEnergyLabelKey: project.PartnerEnergyLabelKey,
				PlayerCosts: []exploration.ExploreResourceCost{}, ReportFacts: []CooperationReportFactView{},
			}
			for _, discovery := range instance.Snapshot.Discoveries {
				if discovery.Kind == exploration.DiscoveryFact && discovery.ID == definition.ID+".clue" && discovery.PubliclyDisplayable {
					view.ClueText = discovery.Text
					view.ClueTextKey = discovery.TextKey
					break
				}
			}
			if project.PlayerItemID != "" {
				cost := exploration.ExploreResourceCost{Kind: exploration.ExploreConsumeItem, RefID: project.PlayerItemID, Quantity: int64(project.PlayerItemQuantity)}
				realmExploreCostLabel(&cost, system, p.State)
				view.PlayerCosts = append(view.PlayerCosts, cost)
			}
			matter, exists := cooperationMatter(p, project.ID, id)
			if exists && matter.Cooperation != nil && matter.CharacterID == partner.ID && matter.Cooperation.ProjectVersion == project.Version {
				view.Phase = string(matter.Cooperation.Phase)
			}
			finished := view.Phase == string(character.CooperationWorked) || view.Phase == string(character.CooperationUsed) || view.Phase == string(character.CooperationReported)
			_, meetingReason := cooperationParticipants(p, p.State, meta, project)
			if finished {
				reported := map[string]bool{}
				for _, ref := range matter.Cooperation.ReportSourceRefs {
					reported[ref] = true
				}
				for _, fact := range cooperationReportFacts(p, id) {
					if !reported[fact.SourceRef] {
						view.ReportFacts = append(view.ReportFacts, fact)
					}
				}
				view.CanReport = meetingReason == "" && len(view.ReportFacts) > 0
				view.BlockedReason = meetingReason
				if view.BlockedReason == "" {
					view.BlockedReason = "already_worked"
				}
			} else {
				if meetingReason != "" {
					reason = meetingReason
				}
				view.CanAsk = reason == "" || reason == "missing_material" || reason == "partner_energy"
				view.CanWorkIfAgreed = reason == ""
				view.CanWork = view.CanWorkIfAgreed && view.Phase == string(character.CooperationAgreed) && cooperationLatestConsent(p, project, id, partner.ID) != nil
				view.BlockedReason = reason
				if view.BlockedReason == "" && !view.CanWork {
					view.BlockedReason = "not_agreed"
				}
			}
			views = append(views, view)
		}
	}
	return views
}

// cooperationReportFacts exposes four bounded outcome classes, each with its
// own real journal source. A shared event never implicitly shares another kind.
func cooperationReportFacts(p *storyBranchProjection, instanceID string) []CooperationReportFactView {
	facts := []CooperationReportFactView{}
	if p == nil {
		return facts
	}
	instance, exists := p.Realms[instanceID]
	definition, valid := cooperationViewDefinition(instance, p.worldContent())
	if !exists || !valid || instance.Snapshot.Binding.InstanceID != instanceID {
		return facts
	}
	snapshot := instance.Snapshot
	add := func(kind, valueKey string) {
		if source := strings.TrimSpace(instance.ExploreSources[kind]); source != "" {
			description := ""
			if p.worldContent() != nil && definition.ExploreRules != nil {
				for _, m := range definition.ExploreRules.Approaches {
					if m.NameKey == valueKey && valueKey != "" {
						description = "主角实际完成了“" + m.Name + "”。"
					}
				}
			}
			facts = append(facts, CooperationReportFactView{Description: description, Kind: kind, SourceRef: kind + ":" + source, DescriptionKey: "game.cooperation.reportFacts." + kind, ValueKey: valueKey})
		}
	}
	for _, decision := range snapshot.ExploreState.Decisions {
		method, ok := exploration.FindExploreApproach(definition, decision.ApproachID)
		if !ok {
			continue
		}
		switch decision.GroupID {
		case "ingress":
			for _, progress := range snapshot.ResolvedChallenges {
				if progress.ID == definition.ID+".obstacle" && progress.SourceRef != "" {
					add("ingress", method.NameKey)
					break
				}
			}
		case "disposition":
			if method.ID == definition.ID+".preserve" && cooperationViewFact(snapshot, definition.ID+".preserved") || method.ID == definition.ID+".strip" && cooperationViewFact(snapshot, definition.ID+".core_taken") {
				add("disposition", method.NameKey)
			}
		}
	}
	if snapshot.ExploreState.Setback {
		add("setback", "")
	}
	for _, discovery := range snapshot.Discoveries {
		if discovery.Kind == exploration.DiscoveryArea && discovery.ID == definition.ID+".side" && discovery.SourceRef != "" {
			add("side", "")
			break
		}
	}
	return facts
}

func cooperationViewDefinition(instance RealmInstanceState, content *WorldContent) (exploration.DungeonDefinition, bool) {
	snapshot := instance.Snapshot
	definition, known := worldRealmDefinition(content, snapshot.Binding.DefinitionID)
	if !known || definition.ExploreRules == nil || snapshot.ExploreState == nil {
		return exploration.DungeonDefinition{}, false
	}
	scope := exploration.DungeonScope{WorldID: snapshot.Binding.WorldID, BranchID: snapshot.Binding.BranchID}
	return definition, len(exploration.ValidateDungeonInstance(definition, scope, snapshot)) == 0
}

func cooperationViewHasClue(instance RealmInstanceState, definition exploration.DungeonDefinition) bool {
	if strings.TrimSpace(instance.ExploreSources["clue"]) == "" || !cooperationViewFact(instance.Snapshot, definition.ID+".clue") {
		return false
	}
	for _, id := range instance.Snapshot.ExploreState.UsedApproachIDs {
		if id == definition.ID+".survey" {
			return true
		}
	}
	return false
}

func cooperationViewFact(snapshot exploration.DungeonInstanceSnapshot, id string) bool {
	for _, discovery := range snapshot.Discoveries {
		if discovery.Kind == exploration.DiscoveryFact && discovery.ID == id && discovery.PubliclyDisplayable && strings.TrimSpace(discovery.SourceRef) != "" {
			return true
		}
	}
	return false
}
