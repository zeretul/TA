package exploration

import "fmt"

const MaxExploreApproaches = 64

func validateExploreRules(def DungeonDefinition) []Diagnostic {
	r := def.ExploreRules
	if r == nil {
		return nil
	}
	var ds []Diagnostic
	add := func(path, message string) { ds = append(ds, Diagnostic{"invalid_explore_rules", path, message}) }
	if !validID(r.NameKey) || !validID(r.DescriptionKey) || !validID(r.PressureLabelKey) || !validText(r.PressureLabel, MaxDescriptionBytes) {
		add("explore_rules", "name, description and pressure localization keys and pressure label are required")
	}
	if r.PressureLimit < 1 || r.PressureLimit > 100 {
		add("explore_rules.pressure_limit", "must be between one and one hundred")
	}
	if r.ReturnAfterSeconds < 0 {
		add("explore_rules.return_after_seconds", "cannot be negative")
	}
	if len(r.Approaches) == 0 || len(r.Approaches) > MaxExploreApproaches {
		add("explore_rules.approaches", "must have a bounded non-empty action set")
	}
	for _, entry := range def.EntryAreaIDs {
		if !containsString(def.ExitAreaIDs, entry) {
			add("exit_area_ids", "every exploration entry must also permit immediate retreat")
		}
	}
	for _, area := range def.Areas {
		if !validID(area.NameKey) || !validID(area.DescriptionKey) {
			add("areas", "exploration areas require stable localization keys")
		}
	}
	for _, objective := range def.Objectives {
		if !validID(objective.DescriptionKey) {
			add("objectives", "exploration objectives require a stable localization key")
		}
	}
	for _, ref := range def.ContentRefs {
		if !validID(ref.ReferenceID) {
			add("content_refs", "exploration content must reference a stable catalog identifier")
		}
	}
	incoming := map[string]int{}
	for _, c := range def.Connections {
		incoming[c.ToAreaID]++
		if !c.Bidirectional {
			add("connections", "exploration connections must preserve a bidirectional retreat")
		}
	}
	// The small authored map is an outward tree. Reversing a traversed edge is
	// therefore always a retreat, including after the forward route closes.
	forward := map[string]bool{}
	queue := append([]string{}, def.EntryAreaIDs...)
	for _, id := range queue {
		forward[id] = true
	}
	for len(queue) > 0 {
		here := queue[0]
		queue = queue[1:]
		for _, c := range def.Connections {
			if c.FromAreaID == here && !forward[c.ToAreaID] {
				forward[c.ToAreaID] = true
				queue = append(queue, c.ToAreaID)
			}
		}
	}
	for _, area := range def.Areas {
		expected := 1
		if containsString(def.EntryAreaIDs, area.ID) {
			expected = 0
		}
		if incoming[area.ID] != expected || !forward[area.ID] {
			add("connections", "exploration areas must form an outward tree with a known reverse retreat")
		}
	}
	ids := map[string]bool{}
	facts := map[string]InstanceDiscovery{}
	prepares := 0
	returnStarters := 0
	hasReturn := false
	rewardUsers := map[string]bool{}
	for i, a := range r.Approaches {
		path := fmt.Sprintf("explore_rules.approaches[%d]", i)
		if !validID(a.ID) || ids[a.ID] {
			add(path+".id", "must be a unique stable identifier")
		}
		ids[a.ID] = true
		if !hasArea(def, a.AreaID) {
			add(path+".area_id", "must reference a declared area")
		}
		if !validText(a.Name, MaxDescriptionBytes) || !validText(a.Description, MaxDescriptionBytes) || !validID(a.NameKey) || !validID(a.DescriptionKey) {
			add(path, "name, description and localization keys are required")
		}
		switch a.Kind {
		case ExploreSurvey, ExploreApproachRoute, ExploreResolve:
		case ExplorePrepare:
			prepares++
		default:
			add(path+".kind", "unknown exploration action kind")
		}
		if a.DurationSeconds <= 0 {
			add(path+".duration_seconds", "must be positive")
		}
		if a.PressureDelta < -r.PressureLimit || a.PressureDelta > r.PressureLimit {
			add(path+".pressure_delta", "must be within the pressure limit")
		}
		if a.PressureDelta < 0 && (a.Kind != ExplorePrepare || len(a.Costs) == 0) {
			add(path+".pressure_delta", "pressure reduction requires a one-use preparation with a real cost")
		}
		if a.CooperationProjectID != "" && !validID(a.CooperationProjectID) {
			add(path+".cooperation_project_id", "must be a stable project identifier verified by Story")
		}
		if a.ExclusiveGroup != "" && !validID(a.ExclusiveGroup) {
			add(path+".exclusive_group", "must be a stable identifier")
		}
		if a.AllowedAfterSetback && (a.PressureDelta != 0 || !a.RequiresSetback || a.RequiresReturn || a.StartsReturn) {
			add(path, "post-setback actions must require setback, add no pressure and not open a return")
		}
		if a.RequiresSetback && !a.AllowedAfterSetback {
			add(path, "setback-only actions must explicitly permit setback")
		}
		if a.StartsReturn {
			returnStarters++
			if a.Kind != ExploreResolve || r.ReturnAfterSeconds <= 0 || a.ExclusiveGroup == "" || len(a.RewardRefIDs) > 0 {
				add(path, "preservation requires a resolve choice, a positive delay and no simultaneous reward")
			}
		}
		if a.RequiresReturn {
			hasReturn = true
			if a.StartsReturn || a.AllowedAfterSetback {
				add(path, "return action cannot initiate itself or survive setback")
			}
		}
		if len(a.RequiredFactIDs)+len(a.ExcludedFactIDs)+len(a.Discoveries)+len(a.RewardRefIDs)+len(a.ResolveChallengeIDs)+len(a.Costs) > MaxKnowledgeItems {
			add(path, "action references exceed the supported bound")
		}
		costIDs := map[string]bool{}
		for _, cost := range a.Costs {
			if cost.Name != "" || cost.NameKey != "" || cost.EnergyCost != 0 {
				add(path+".costs", "authored costs cannot override projected actor display or energy")
			}
			key := string(cost.Kind) + ":" + cost.RefID
			if !validID(cost.RefID) || cost.Quantity <= 0 || cost.Quantity > MaxQuantity || costIDs[key] {
				add(path+".costs", "cost must have a unique registered identity and bounded positive quantity")
			}
			costIDs[key] = true
			switch cost.Kind {
			case ExploreConsumeItem, ExploreRequireItem:
			case ExploreUseSkill:
				if cost.Quantity != 1 {
					add(path+".costs", "a skill invocation quantity must be one")
				}
			default:
				add(path+".costs", "unknown cost kind")
			}
		}
		discovered := map[string]bool{}
		for _, d := range a.Discoveries {
			key := string(d.Kind) + ":" + d.ID
			if discovered[key] {
				add(path+".discoveries", "duplicate discovery")
			}
			discovered[key] = true
			if d.SourceRef != "" {
				add(path+".discoveries", "authored discoveries cannot supply an execution source")
			}
			if err := validateDiscoveryReference(def, d); err != nil {
				add(path+".discoveries", "discovery must reference valid content")
			}
			if d.Kind == DiscoveryFact {
				if !validID(d.TextKey) {
					add(path+".discoveries", "exploration facts require a stable localization key")
				}
				if !d.PubliclyDisplayable {
					add(path+".discoveries", "exploration facts must be publicly displayable")
				}
				if prior, ok := facts[d.ID]; ok && !sameDiscoveryContent(prior, d) {
					add(path+".discoveries", "fact identity has conflicting content")
				}
				facts[d.ID] = d
			}
		}
		seenChallenges := map[string]bool{}
		for _, id := range a.ResolveChallengeIDs {
			if !hasChallenge(def, id) || findChallenge(def, id).AreaID != a.AreaID || seenChallenges[id] {
				add(path+".resolve_challenge_ids", "challenge must be declared and unique")
			}
			seenChallenges[id] = true
		}
		seenRewards := map[string]bool{}
		for _, id := range a.RewardRefIDs {
			ref, ok := findContentRef(def, id)
			if !ok || ref.Kind != "reward" || ref.AreaID != a.AreaID || seenRewards[id] {
				add(path+".reward_ref_ids", "reward must be unique, declared and located at the action")
			}
			seenRewards[id] = true
			rewardUsers[id] = true
		}
		if len(a.RewardRefIDs) > 0 && a.Kind != ExploreResolve {
			add(path+".reward_ref_ids", "only resolve actions may automatically grant rewards")
		}
	}
	if prepares > 1 {
		add("explore_rules.approaches", "only one preparation is allowed per place")
	}
	if returnStarters > 1 || hasReturn && returnStarters != 1 || returnStarters > 0 && !hasReturn {
		add("explore_rules.approaches", "a return requires exactly one preservation and a return consumer")
	}
	for i, a := range r.Approaches {
		p := fmt.Sprintf("explore_rules.approaches[%d]", i)
		seen := map[string]bool{}
		for _, id := range a.RequiredFactIDs {
			if _, ok := facts[id]; !ok || seen[id] {
				add(p+".required_fact_ids", "required facts must be declared and unique")
			}
			seen[id] = true
		}
		for _, id := range a.ExcludedFactIDs {
			if _, ok := facts[id]; !ok || seen[id] {
				add(p+".excluded_fact_ids", "excluded facts must be declared, unique and not required")
			}
			seen[id] = true
		}
	}
	for _, ref := range def.ContentRefs {
		if ref.Kind == "reward" && !rewardUsers[ref.ID] {
			add("content_refs", "every exploration reward needs a resolving action")
		}
	}
	return ds
}

func validateExploreState(def DungeonDefinition, s DungeonInstanceSnapshot) []Diagnostic {
	if def.ExploreRules == nil {
		if s.ExploreState != nil {
			return []Diagnostic{{"unexpected_explore_state", "explore_state", "legacy place cannot contain exploration state"}}
		}
		return nil
	}
	var ds []Diagnostic
	add := func(message string) { ds = append(ds, Diagnostic{"invalid_explore_state", "explore_state", message}) }
	state := s.ExploreState
	if state == nil {
		add("authored exploration state is missing")
		return ds
	}
	if len(state.UsedApproachIDs) > MaxExploreApproaches || len(state.Decisions) > MaxExploreApproaches {
		add("exploration history exceeds supported capacity")
		return ds
	}
	used := map[string]bool{}
	groups := map[string]string{}
	expectedRewards := map[string]bool{}
	expectedDiscoveries := map[string]InstanceDiscovery{}
	expectedChallenges := map[string]bool{}
	expectedObjectives := map[string]bool{}
	knownFacts := map[string]bool{}
	knownAreas := map[string]bool{}
	possibleMovedAreas := map[string]bool{}
	for _, id := range def.EntryAreaIDs {
		knownAreas[id] = true
		expectedDiscoveries[string(DiscoveryArea)+":"+id] = InstanceDiscovery{Kind: DiscoveryArea, ID: id}
	}
	pressure := 0
	setback := false
	preserved := false
	for _, id := range state.UsedApproachIDs {
		a, ok := FindExploreApproach(def, id)
		if !ok || used[id] {
			add("used approaches must exist and be unique")
			continue
		}
		used[id] = true
		expectedDiscoveries[string(DiscoveryArea)+":"+a.AreaID] = InstanceDiscovery{Kind: DiscoveryArea, ID: a.AreaID}
		if !knownAreas[a.AreaID] && !possibleMovedAreas[a.AreaID] {
			add("action area was not discovered before use")
		}
		for _, id := range a.RequiredFactIDs {
			if !knownFacts[id] {
				add("action required a fact not yet discovered")
			}
		}
		for _, id := range a.ExcludedFactIDs {
			if knownFacts[id] {
				add("action was excluded by an earlier fact")
			}
		}
		if setback && !a.AllowedAfterSetback {
			add("normal action cannot follow a setback")
		}
		if a.RequiresSetback && !setback {
			add("residue cannot precede a setback")
		}
		if a.RequiresReturn && !preserved {
			add("return cannot precede preservation")
		}
		if a.ExclusiveGroup != "" {
			if groups[a.ExclusiveGroup] != "" {
				add("exclusive group was used twice")
			}
			groups[a.ExclusiveGroup] = a.ID
		}
		pressure += a.PressureDelta
		if pressure < 0 {
			pressure = 0
		}
		crosses := !setback && pressure >= def.ExploreRules.PressureLimit
		if crosses {
			setback = true
			continue
		}
		for _, d := range a.Discoveries {
			expectedDiscoveries[string(d.Kind)+":"+d.ID] = d
			if d.Kind == DiscoveryFact {
				knownFacts[d.ID] = true
			}
			if d.Kind == DiscoveryArea {
				knownAreas[d.ID] = true
			}
			if d.Kind == DiscoveryConnection {
				if c, ok := findConnection(def, d.ID); ok {
					possibleMovedAreas[c.FromAreaID] = true
					possibleMovedAreas[c.ToAreaID] = true
				}
			}
		}
		for _, id := range a.ResolveChallengeIDs {
			expectedChallenges[id] = true
		}
		if a.StartsReturn {
			preserved = true
		}
		for _, id := range a.RewardRefIDs {
			if expectedRewards[id] {
				add("reward was resolved twice")
			}
			expectedRewards[id] = true
			ref, ok := findContentRef(def, id)
			if ok && ref.ObjectiveID != "" {
				expectedObjectives[ref.ObjectiveID] = true
				expectedDiscoveries[string(DiscoveryObjective)+":"+ref.ObjectiveID] = InstanceDiscovery{Kind: DiscoveryObjective, ID: ref.ObjectiveID}
			}
		}
	}
	actualDiscoveries := map[string]bool{}
	for _, d := range s.Discoveries {
		key := string(d.Kind) + ":" + d.ID
		expected, ok := expectedDiscoveries[key]
		if !ok && d.Kind == DiscoveryArea && possibleMovedAreas[d.ID] {
			expected = InstanceDiscovery{Kind: DiscoveryArea, ID: d.ID}
			ok = true
		}
		if !ok || !sameDiscoveryContent(expected, d) {
			add("discovery has no matching successful approach")
		}
		actualDiscoveries[key] = true
	}
	for key := range expectedDiscoveries {
		if !actualDiscoveries[key] {
			add("successful approach is missing a discovery")
		}
	}
	actualChallenges := map[string]bool{}
	for _, c := range s.ResolvedChallenges {
		actualChallenges[c.ID] = true
		if !expectedChallenges[c.ID] {
			add("challenge has no successful approach")
		}
	}
	if len(actualChallenges) != len(expectedChallenges) {
		add("successful approach is missing challenge progress")
	}
	actualObjectives := map[string]bool{}
	for _, o := range s.CompletedObjectives {
		actualObjectives[o.ID] = true
		if !expectedObjectives[o.ID] {
			add("objective has no granted reward")
		}
	}
	if len(actualObjectives) != len(expectedObjectives) {
		add("granted reward is missing objective progress")
	}
	if pressure != state.Pressure || setback != state.Setback {
		add("pressure and setback must match action history")
	}
	if state.ReturnDueSeconds < 0 || preserved != (state.ReturnDueSeconds > 0) {
		add("return moment must correspond to successful preservation")
	}
	seenGroups := map[string]bool{}
	for _, d := range state.Decisions {
		if groups[d.GroupID] != d.ApproachID || seenGroups[d.GroupID] {
			add("decision does not match its used exclusive approach")
		}
		seenGroups[d.GroupID] = true
	}
	if len(seenGroups) != len(groups) {
		add("exclusive approach is missing its decision")
	}
	actualRewards := map[string]bool{}
	for _, r := range s.GrantedRewards {
		actualRewards[r.ID] = true
		if !expectedRewards[r.ID] {
			add("reward has no successful resolving approach")
		}
	}
	if len(actualRewards) != len(expectedRewards) {
		add("successful resolving approach is missing its reward")
	}
	return ds
}
