package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This file makes one real realm reachable during play.
//
// §11 of 自制-探索与行路 asks for one complete flow: the player enters an authored
// realm, explores it, meets its challenge or takes what it holds, leaves, and
// comes back to find the place as they left it. Three properties are deliberate.
//
//   - The rules run before the prose. What rooms exist, which connection is
//     legal, whether a reward is really there and whether it was already taken
//     are all answered by the instance snapshot, not by a sentence. A narrative
//     afterwards describes what was committed.
//
//   - The instance is not regenerated. §11.4 keeps a re-entered realm's terrain,
//     resources and resolved conditions. The same instance id is re-entered, so a
//     second visit is a return rather than a fresh copy.
//
//   - The reward is a real item. §11.3 lets the player leave with what they found,
//     which means the grant has to be written into the character's own pack in
//     the same transaction as the realm record. A treasure the panel shows and
//     the character does not hold is exactly the mismatch this prevents.

// RealmAppRequest asks for one action inside an authored realm.
type RealmAppRequest struct {
	StoryID  string
	BranchID string
	// Locale affects presentation only, never command identity or settlement.
	Locale string
	// WorldTime is the story's display text for this moment. Empty leaves the
	// scene's own wording alone.
	WorldTime string
	// RealmID names the authored realm. Required to enter.
	RealmID string
	// InstanceID names the instance a later action acts on. Empty uses the
	// branch's single instance.
	InstanceID string
	// Action is enter, move, reward, exit, reenter, survey, prepare, approach or resolve.
	Action string
	// ApproachID names the authored exploration choice, never client-supplied costs.
	ApproachID string
	// ConnectionID and DestinationAreaID name one declared directed connection.
	ConnectionID      string
	DestinationAreaID string
	// ContentRefID names the declared reward to take.
	ContentRefID string
	// CommandID identifies one attempt, so a retry adds no second reward.
	CommandID string
}

// RealmAppResult is one committed realm action as the UI reports it.
type RealmAppResult struct {
	StoryID    string `json:"story_id"`
	BranchID   string `json:"branch_id"`
	EventID    string `json:"event_id"`
	InstanceID string `json:"instance_id"`
	RealmID    string `json:"realm_id"`
	Action     string `json:"action"`
	// Presence is "inside" or "outside" after this action.
	Presence string `json:"presence"`
	// Instance is the whole realm after this action.
	Instance exploration.DungeonInstanceSnapshot `json:"instance"`
	// View is the player-known projection: only what has really been discovered.
	View exploration.DungeonInstanceView `json:"view"`
	// Reward is the reward this action granted, if any.
	Reward *RealmRewardView `json:"reward,omitempty"`
	// Rewards retains all grants from one authored approach; no grant is an empty array.
	Rewards         []RealmRewardView `json:"rewards"`
	DurationSeconds int64             `json:"duration_seconds"`
	// Replayed reports that this answer came from an action already committed
	// under the same command, so nothing was applied twice.
	Replayed bool `json:"replayed"`
}

// RealmRewardView is one granted reward in the story's own wording.
type RealmRewardView struct {
	ID        string `json:"id,omitempty"`
	RefID     string `json:"ref_id"`
	NameKey   string `json:"name_key"`
	Name      string `json:"name,omitempty"`
	Quantity  int64  `json:"quantity"`
	SourceRef string `json:"source_ref,omitempty"`
}

// RealmAppState is the realm entry's read: the realms this branch has entered,
// and the authored realms it could still enter.
type RealmAppState struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	// ClockSeconds lets the panel compare an authored return time with committed time.
	ClockSeconds int64 `json:"clock_seconds"`
	// Location is the scene's committed location text, which is what the entry
	// shows as where the character stands.
	Location string `json:"location,omitempty"`
	// Instances are the realms this branch has entered, in stable id order.
	Instances []interactive.RealmStateView `json:"instances"`
	// Available are the authored realms that can still be entered.
	Available    []interactive.RealmOption            `json:"available"`
	Cooperations []interactive.CooperationProjectView `json:"cooperations"`
}

// RealmStateFor reads the branch's realms.
//
// It is a plain read with no model call, so opening the entry costs no world time
// and no generation. Everything it reports is already committed.
func (a *App) RealmStateFor(storyID, branchID string) (RealmAppState, error) {
	return a.RealmStateForLocale(storyID, branchID, "")
}

// RealmStateForLocale localizes a read-only copy using the bound pack revision.
func (a *App) RealmStateForLocale(storyID, branchID, locale string) (RealmAppState, error) {
	if a == nil {
		return RealmAppState{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return RealmAppState{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(storyID), strings.TrimSpace(branchID))
	if err != nil {
		return RealmAppState{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.RealmStateFor(strings.TrimSpace(storyID), resolvedBranch)
	if err != nil {
		return RealmAppState{}, err
	}
	state.Instances = append([]interactive.RealmStateView{}, state.Instances...)
	if state.Available == nil {
		state.Available = []interactive.RealmOption{}
	}
	names := a.realmAdventureNames(storyContext.Meta.PackBinding, locale)
	state.Cooperations = a.realmCooperationViews(state.Cooperations, storyContext.Meta.PackBinding, locale, names)
	for index := range state.Instances {
		state.Instances[index].Granted = append([]interactive.RealmGrantedReward{}, state.Instances[index].Granted...)
		for grantIndex := range state.Instances[index].Granted {
			grant := &state.Instances[index].Granted[grantIndex]
			if name := names[grant.RefID]; name != "" {
				grant.Name = name
			}
		}
		if state.Instances[index].Explore == nil {
			continue
		}
		// Keep the API's collections stable without altering a cached Story view.
		explore := *state.Instances[index].Explore
		explore.Approaches = append([]exploration.ExploreApproachView{}, explore.Approaches...)
		for approachIndex := range explore.Approaches {
			approach := &explore.Approaches[approachIndex]
			approach.Costs = append([]exploration.ExploreResourceCost{}, approach.Costs...)
			for costIndex := range approach.Costs {
				cost := &approach.Costs[costIndex]
				if name := names[cost.RefID]; name != "" {
					cost.Name = name
				}
			}
		}
		state.Instances[index].Explore = &explore
	}
	return RealmAppState{
		StoryID: strings.TrimSpace(storyID), BranchID: resolvedBranch,
		Location: a.realmLocationName(storyContext.Meta.PackBinding, locale, state.CurrentLocation), ClockSeconds: state.ClockSeconds,
		Instances: state.Instances, Available: state.Available, Cooperations: state.Cooperations,
	}, nil
}

// realmLocationName translates only an exact registered location ID. Authored
// prose and story-specific aliases remain untouched, as does the stored state.
func (a *App) realmLocationName(binding *interactive.StoryPackBinding, locale, location string) string {
	if binding == nil || location == "" {
		return location
	}
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	if cfg == nil {
		return location
	}
	pack, err := storypack.NewLibrary(cfg.DataDir()).Get(binding.PackID)
	if err != nil || pack.ContentHash() != binding.ContentHash {
		return location
	}
	for _, place := range pack.World {
		if place.ID != location {
			continue
		}
		if strings.HasPrefix(locale, "en") {
			if name := pack.Localizations["en-US"].World[place.ID].Name; name != "" {
				return name
			}
		}
		return firstNonEmpty(place.Name, location)
	}
	// Some cultivation route endpoints are registered as authored placement
	// labels rather than World entries. Use only their place label, never the
	// character's identity or the placement as evidence of present company.
	for _, person := range pack.Characters {
		if person.PlaceCandidate != location || person.PlaceLabel == "" {
			continue
		}
		if strings.HasPrefix(locale, "en") {
			if name := pack.Localizations["en-US"].Characters[person.SourceKey].PlaceName; name != "" {
				return name
			}
		}
		return person.PlaceLabel
	}
	return location
}

// realmCooperationViews localizes detached known views only. Canonical author
// names may be translated; a story-specific alias is never replaced with a
// previously unknown full name from the author's catalog.
func (a *App) realmCooperationViews(source []interactive.CooperationProjectView, binding *interactive.StoryPackBinding, locale string, itemNames map[string]string) []interactive.CooperationProjectView {
	views := append([]interactive.CooperationProjectView{}, source...)
	var names map[string][2]string
	var places map[string]string
	if len(views) > 0 && binding != nil && strings.HasPrefix(locale, "en") {
		a.mu.RLock()
		cfg := a.cfg
		a.mu.RUnlock()
		if cfg != nil {
			if pack, err := storypack.NewLibrary(cfg.DataDir()).Get(binding.PackID); err == nil && pack.ContentHash() == binding.ContentHash {
				names, places = map[string][2]string{}, map[string]string{}
				english := pack.Localizations["en-US"]
				for _, person := range pack.Characters {
					names[person.SourceKey] = [2]string{person.Name, english.Characters[person.SourceKey].Name}
				}
				for id, place := range english.World {
					places[id] = place.Name
				}
				// Author placement labels translate a registered meeting-place name
				// only; eligibility still comes exclusively from committed presence.
				for _, person := range pack.Characters {
					if person.PlaceCandidate != "" && places[person.PlaceCandidate] == "" {
						places[person.PlaceCandidate] = english.Characters[person.SourceKey].PlaceName
					}
				}
				for _, place := range pack.World {
					if name := english.World[place.ID].Name; name != "" {
						places[place.Name] = name
					}
				}
			}
		}
	}
	for i := range views {
		view := &views[i]
		view.PlayerCosts = append([]exploration.ExploreResourceCost{}, view.PlayerCosts...)
		view.ReportFacts = append([]interactive.CooperationReportFactView{}, view.ReportFacts...)
		for j := range view.PlayerCosts {
			if name := itemNames[view.PlayerCosts[j].RefID]; name != "" {
				view.PlayerCosts[j].Name = name
			}
		}
		if name := names[view.PartnerSourceKey]; name[0] == view.PartnerName && name[1] != "" {
			view.PartnerName = name[1]
		}
		if name := firstNonEmpty(places[view.LocationID], places[view.LocationName]); name != "" {
			view.LocationName = name
		}
	}
	return views
}

// ApplyRealm settles one realm action and commits it.
//
// The rules run first and the commit is one transaction, so the realm record, the
// item write and the place write either all exist on the journal or none of them
// do. A retry under the same command ID returns the first attempt's answer with
// no second reward.
func (a *App) ApplyRealm(request RealmAppRequest) (RealmAppResult, error) {
	if a == nil {
		return RealmAppResult{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return RealmAppResult{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(request.StoryID), strings.TrimSpace(request.BranchID))
	if err != nil {
		return RealmAppResult{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	commandID := strings.TrimSpace(request.CommandID)
	result, err := store.ApplyRealm(strings.TrimSpace(request.StoryID), interactive.RealmRequest{
		BranchID:          resolvedBranch,
		WorldTime:         strings.TrimSpace(request.WorldTime),
		RealmID:           strings.TrimSpace(request.RealmID),
		InstanceID:        strings.TrimSpace(request.InstanceID),
		Action:            strings.TrimSpace(request.Action),
		ApproachID:        strings.TrimSpace(request.ApproachID),
		ConnectionID:      strings.TrimSpace(request.ConnectionID),
		DestinationAreaID: strings.TrimSpace(request.DestinationAreaID),
		ContentRefID:      strings.TrimSpace(request.ContentRefID),
		CommandID:         commandID,
		// The fingerprint covers what actually decides the action — the branch,
		// the realm, the target — and is resolved against the branch the store
		// will really write to, so an empty branch and an explicit current branch
		// are the same command rather than a conflict.
		CommandHash: realmCommandHash(request, resolvedBranch, commandID),
	})
	if err != nil {
		return RealmAppResult{}, err
	}
	view := RealmAppResult{
		StoryID: strings.TrimSpace(request.StoryID), BranchID: resolvedBranch,
		EventID: result.EventID, InstanceID: result.InstanceID,
		RealmID: result.RealmID, Action: result.Action,
		Presence: string(result.Instance.Presence),
		Instance: result.Instance, View: result.View,
		Replayed:        result.Replayed,
		DurationSeconds: result.DurationSeconds,
		Rewards:         make([]RealmRewardView, 0, len(result.Rewards)),
	}
	var rules *interactive.AdventureRules
	if storyContext.Snapshot.ActorStateSchema != nil {
		rules = storyContext.Snapshot.ActorStateSchema.System.AdventureRules
	}
	if result.Reward != nil {
		reward := realmRewardView(*result.Reward, rules, interactive.StoryCultivationRules(storyContext.Meta))
		view.Reward = &reward
	}
	for _, reward := range result.Rewards {
		view.Rewards = append(view.Rewards, realmRewardView(reward, rules, interactive.StoryCultivationRules(storyContext.Meta)))
	}
	names := a.realmAdventureNames(storyContext.Meta.PackBinding, request.Locale)
	if view.Reward != nil {
		if name := names[view.Reward.RefID]; name != "" {
			view.Reward.Name = name
		}
	}
	for index := range view.Rewards {
		if name := names[view.Rewards[index].RefID]; name != "" {
			view.Rewards[index].Name = name
		}
	}
	return view, nil
}

// A newer author's sidecar must not rename an older saved catalogue. The
// canonical state is never translated; callers apply these names to view copies.
func (a *App) realmAdventureNames(binding *interactive.StoryPackBinding, locale string) map[string]string {
	if binding == nil || !strings.HasPrefix(locale, "en") {
		return nil
	}
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	if cfg == nil {
		return nil
	}
	pack, err := storypack.NewLibrary(cfg.DataDir()).Get(binding.PackID)
	if err != nil || pack.ContentHash() != binding.ContentHash {
		return nil
	}
	return pack.Localizations["en-US"].Adventure
}

// The frozen catalog owns native item names. Cultivation uses its existing
// localisation keys; an unknown reference never becomes a fabricated label.
func realmRewardView(reward exploration.InstanceReward, rules *interactive.AdventureRules, profiles ...*cultivation.Rules) RealmRewardView {
	view := RealmRewardView{ID: reward.ID, RefID: reward.RefID, NameKey: realmRewardNameKey(reward.RefID, profiles...), Quantity: reward.Quantity, SourceRef: reward.SourceRef}
	if rules != nil {
		for _, item := range rules.Items {
			if item.ID == reward.RefID {
				view.Name = item.Name
				break
			}
		}
	}
	return view
}

// realmRewardNameKey is the catalogue's localisation key for a granted reward.
//
// The rules layer names the definition; the wording is the interface's, so the
// server reports the key rather than inventing a name.
func realmRewardNameKey(refID string, profiles ...*cultivation.Rules) string {
	var rules *cultivation.Rules
	if len(profiles) > 0 {
		rules = profiles[0]
	}
	return interactive.StoryItemNameKey(rules, refID)
}

// realmCommandHash fingerprints the content behind a realm command ID.
//
// It covers what actually decides the action, so reusing one command ID for a
// genuinely different action is refused rather than answered with the wrong
// result. The branch is the resolved one, so an empty branch and an explicit
// current branch fingerprint the same and a retry is not mistaken for a conflict.
func realmCommandHash(request RealmAppRequest, branchID, commandID string) string {
	if commandID == "" {
		// No identity means no idempotency to protect. The store refuses a command
		// id without a hash, and this keeps the two consistent.
		return ""
	}
	parts := []string{
		"denova.realm",
		strings.TrimSpace(request.StoryID),
		strings.TrimSpace(branchID),
		"realm\x00" + strings.TrimSpace(request.RealmID),
		"instance\x00" + strings.TrimSpace(request.InstanceID),
		"action\x00" + strings.TrimSpace(request.Action),
		"connection\x00" + strings.TrimSpace(request.ConnectionID),
		"destination\x00" + strings.TrimSpace(request.DestinationAreaID),
		"reward\x00" + strings.TrimSpace(request.ContentRefID),
	}
	// Keep old commands byte-identical: adding an empty field would invalidate
	// persisted receipts made before authored exploration choices existed.
	if approachID := strings.TrimSpace(request.ApproachID); approachID != "" {
		parts = append(parts, "approach\x00"+approachID)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x01")))
	return "sha256:" + hex.EncodeToString(sum[:])
}
