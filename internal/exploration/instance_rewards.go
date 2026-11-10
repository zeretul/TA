package exploration

import (
	"fmt"
	"sort"
)

// This file makes a realm's reward a real, once-only fact.
//
// §11.3 of 自制-探索与行路 says a player may leave with part of what they found and
// that an ended realm does not grow another door to keep them; §11.4 says a
// re-entered realm keeps its history rather than resetting its drops. Both come
// down to one requirement: taking a reward has to be a fact on the instance, not
// a sentence in the prose and not a table the caller reads again later.
//
// The instance already carries Discoveries and completed objectives. A granted
// reward is deliberately its own list rather than a discovery, because a
// discovery says what the character knows and a grant says what the character
// has; conflating them would let a second inspection look like a second copy.

// MaxInstanceRewards bounds the granted rewards one instance may carry. It is
// the same order as the definition's own content-reference bound, because a
// realm cannot grant more distinct rewards than it declares.
const MaxInstanceRewards = MaxContentRefs

// InstanceReward is one reward the instance has really handed over.
//
// ID is the definition's content reference, so the same declared reward cannot
// be granted twice under two spellings, and RefID is the registered catalogue
// entry it produced. Both are kept because a reader needs to know which declared
// reward was taken as well as what it became.
type InstanceReward struct {
	ID        string `json:"id"`
	RefID     string `json:"ref_id"`
	Quantity  int64  `json:"quantity"`
	SourceRef string `json:"source_ref"`
}

// RealmRewardRequest asks to take one declared reward from the area the
// character is really standing in.
type RealmRewardRequest struct {
	// ContentRefID names the definition's content_refs entry. The caller never
	// supplies the catalogue reference or the quantity: both are the authored
	// definition's, so a request cannot turn one declared pill into ten.
	ContentRefID string
	// SourceRef is the trusted source this grant is attributed to, which is the
	// same discipline every other instance action uses.
	SourceRef string
}

// GrantInstanceReward proposes taking one declared reward.
//
// The rules that matter are all refusals, because each one is a way a reward
// could be conjured rather than found:
//
//   - the content reference must be declared by the definition;
//   - it must be a reward, not a resource or an enemy placeholder;
//   - its area must be the area the character is actually in;
//   - the same reference must not already be granted.
//
// A refusal changes nothing, so a retry after a lost response finds the same
// instance and either returns the earlier grant's result through the caller's
// own idempotency or refuses identically. What it can never do is hand over a
// second copy.
func GrantInstanceReward(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot, ctx InstanceActionContext, request RealmRewardRequest) (InstanceTransition, InstanceReward, error) {
	if def.ExploreRules != nil {
		return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: authored exploration rewards require their resolving approach", ErrActionNotAllowed)
	}
	if snapshot.Presence != InstanceInside {
		return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: a reward is taken from inside the realm", ErrInvalidInstanceAction)
	}
	referenceID := request.ContentRefID
	if err := validateActionContext(def, scope, snapshot, ctx, ActionDiscover, referenceID, "", "", false); err != nil {
		return InstanceTransition{}, InstanceReward{}, err
	}
	if request.SourceRef == "" || request.SourceRef != ctx.Permit.SourceRef {
		return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: reward source must match the trusted action permit", ErrInvalidInstanceAction)
	}
	reference, ok := findContentRef(def, referenceID)
	if !ok {
		return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: content reference %s is not declared", ErrInvalidInstanceAction, referenceID)
	}
	if reference.Kind != "reward" {
		return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: %s is a %s, not a reward", ErrInvalidInstanceAction, referenceID, reference.Kind)
	}
	if reference.AreaID != snapshot.CurrentAreaID {
		return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: %s is in %s, the character is in %s",
			ErrInvalidInstanceAction, referenceID, reference.AreaID, snapshot.CurrentAreaID)
	}
	for _, granted := range snapshot.GrantedRewards {
		if granted.ID == referenceID {
			return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: %s was already taken", ErrInstanceConflict, referenceID)
		}
	}
	if len(snapshot.GrantedRewards) >= MaxInstanceRewards {
		return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: reward capacity %d reached", ErrInstanceCapacity, MaxInstanceRewards)
	}

	reward := InstanceReward{ID: referenceID, RefID: reference.ReferenceID, Quantity: reference.Quantity, SourceRef: request.SourceRef}
	after := cloneInstanceSnapshot(snapshot)
	after.GrantedRewards = append(after.GrantedRewards, reward)
	sort.Slice(after.GrantedRewards, func(i, j int) bool { return after.GrantedRewards[i].ID < after.GrantedRewards[j].ID })

	// The area itself becomes a known place, which is honest: the character had
	// to be standing in it to take the reward, so a later reader must not be
	// shown a grant from an area the projection hides.
	if err := applyDiscovery(&after, def, InstanceDiscovery{Kind: DiscoveryArea, ID: reference.AreaID, SourceRef: request.SourceRef}); err != nil {
		return InstanceTransition{}, InstanceReward{}, err
	}
	// Taking the thing the character came for is what completes the objective the
	// definition ties it to. The completion is recorded here rather than inferred
	// by a reader, so a tick on the panel is always a fact on the instance.
	if objectiveID := reference.ObjectiveID; objectiveID != "" {
		if !hasObjective(def, objectiveID) {
			return InstanceTransition{}, InstanceReward{}, fmt.Errorf("%w: reward %s completes an objective that does not exist", ErrInvalidInstanceAction, referenceID)
		}
		if err := applyDiscovery(&after, def, InstanceDiscovery{Kind: DiscoveryObjective, ID: objectiveID, SourceRef: request.SourceRef}); err != nil {
			return InstanceTransition{}, InstanceReward{}, err
		}
		completed, err := appendProgress(after.CompletedObjectives, objectiveID, request.SourceRef)
		if err != nil {
			return InstanceTransition{}, InstanceReward{}, err
		}
		after.CompletedObjectives = completed
	}
	transition, err := finishInstanceTransition(snapshot, after, request.SourceRef)
	if err != nil {
		return InstanceTransition{}, InstanceReward{}, err
	}
	return transition, reward, nil
}

// findContentRef looks a declared content reference up by its own id.
func findContentRef(def DungeonDefinition, id string) (ContentRef, bool) {
	for _, reference := range def.ContentRefs {
		if reference.ID == id {
			return reference, true
		}
	}
	return ContentRef{}, false
}
