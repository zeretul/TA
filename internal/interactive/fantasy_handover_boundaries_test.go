package interactive

import (
	"encoding/json"
	"reflect"
	"testing"

	"ta/internal/character"
	"ta/internal/quest"
)

// These bounded rule fixtures do not claim earned ownership, travel or live AI
// behavior. They isolate the new optional handover from unrelated stage gates.
// Requires handovers.patch and handovers_failure_preserve.patch.
// Not compiled or executed by the independent review.
func fantasyHandoverBoundaryFixture(t *testing.T) (AdventureRules, *storyBranchProjection, quest.Definition, quest.Instance) {
	t.Helper()
	r, a := fantasyOrigin(t, "sg-start-smith")
	a.Inventory["sg-oath-key"] = 1
	a.Acquired = map[string]bool{"sg-oath-key": true}
	p := questEffortProjectionIter4(a)
	setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storySceneLocationField), "倒旗墓道")
	setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{"keeper"})
	p.Characters = &character.State{Version: character.StateVersion, Profiles: []character.Profile{{ID: "keeper", StoryID: "fixture", Name: "乔文", SourceKey: "sg-p12", Revision: 1}}}
	for _, npc := range r.NPCs {
		if npc.CharacterSourceKey == "sg-p12" {
			owner := npc.Actor
			owner.SourceKey = npc.CharacterSourceKey
			p = intervalProjectionAfterOps(p, actorOpsForAdventureOwner(owner, "keeper", "fixture"))
		}
	}
	d, ok := quest.DefinitionByID("sg-crypt-inquiry")
	if !ok {
		t.Fatal("missing authored investigation")
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	c, ok := d.Choice("sg-crypt-inquiry.stage3.c1")
	if !ok {
		t.Fatal("missing authored handover")
	}
	c.AfterObjectives = nil
	c.RequiredCharacterSourceKeys = nil // The new receiving-owner gate must stand alone.
	c.ItemHandovers = []quest.ItemHandover{{CharacterSourceKey: "sg-p12", Items: map[string]int{"sg-oath-key": 1}}}
	d.Choices = []quest.Choice{c}
	d.Objectives = []quest.Objective{{ID: c.ObjectiveID, Evidence: []quest.Evidence{{Kind: quest.EvidenceChoice, RefID: c.ObjectiveID}}}}
	i := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
	return r, p, d, i
}

func fantasyHandoverBoundaryActor(t *testing.T, p *storyBranchProjection, id string, edit func(*AdventureActor)) {
	t.Helper()
	a, err := AdventureActorFromState(p.State, id)
	if err != nil {
		t.Fatal(err)
	}
	edit(&a)
	*p = *intervalProjectionAfterOps(p, actorOpsForAdventureOwner(a, id, "fixture-edit"))
}

func TestFantasyItemHandoverInvalidRecipientOrObjectIsAtomic(t *testing.T) {
	for _, bad := range []string{"absent", "duplicate", "self", "dead", "battle", "occupied", "malformed", "wrong-source", "charges", "cooldown", "already-holds-unique"} {
		t.Run(bad, func(t *testing.T) {
			r, p, d, i := fantasyHandoverBoundaryFixture(t)
			switch bad {
			case "absent":
				setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{})
			case "duplicate":
				p.Characters.Profiles = append(p.Characters.Profiles, character.Profile{ID: "second-keeper", StoryID: "fixture", SourceKey: "sg-p12"})
			case "self":
				p.Characters.Profiles[0].ID = DefaultActorID
				setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{DefaultActorID})
			case "dead":
				fantasyHandoverBoundaryActor(t, p, "keeper", func(a *AdventureActor) { a.Health = 0 })
			case "battle":
				fantasyHandoverBoundaryActor(t, p, "keeper", func(a *AdventureActor) {
					a.Battle = &AdventureBattle{ID: "ongoing", Status: "active", Scale: "person", PlayerScale: "person", OpponentID: "sg-practice", Health: 55, MaxHealth: 55}
				})
			case "occupied":
				setPath(p.State, actorStateFieldPath("keeper", xiuxianFieldArrangement), xiuxianArrangementOccupied)
			case "malformed":
				setPath(p.State, actorStateFieldPath("keeper", AdventureStateField), "not-an-actor")
			case "wrong-source":
				fantasyHandoverBoundaryActor(t, p, "keeper", func(a *AdventureActor) { a.SourceKey = "sg-p31" })
			case "charges", "cooldown":
				for n := range r.Items {
					if r.Items[n].ID == "sg-oath-key" {
						if bad == "charges" {
							r.Items[n].MaxCharges = 1
						} else {
							r.Items[n].CooldownSeconds = 60
						}
					}
				}
			case "already-holds-unique":
				fantasyHandoverBoundaryActor(t, p, "keeper", func(a *AdventureActor) { a.Inventory["sg-oath-key"] = 1 })
			}
			before, _ := json.Marshal(p)
			rulesBefore, _ := json.Marshal(r)
			_, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: d.Choices[0].ID})
			after, _ := json.Marshal(p)
			rulesAfter, _ := json.Marshal(r)
			if err == nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(rulesBefore, rulesAfter) {
				t.Fatalf("invalid %s handover was not atomic: %v", bad, err)
			}
		})
	}
}

func TestFantasyItemHandoverDoesNotMintAnOriginalFromGeneratedSubstitute(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(map[bool]string{false: "only-substitute", true: "split-receivers-share-one-original"}[split], func(t *testing.T) {
			r, p, d, i := fantasyHandoverBoundaryFixture(t)
			a, err := AdventureActorFromState(p.State, DefaultActorID)
			if err != nil {
				t.Fatal(err)
			}
			count := 1
			if split {
				count = 2
			}
			a.Inventory["sg-journal"] = count
			adapted := AdventureGeneratedDefinition{Category: "equipment", TemplateID: "sg-journal", Name: "Rebound field journal", Source: "A held fixture journal rebound without changing its facts"}
			a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &adapted}, "handover-substitute")
			if err != nil {
				t.Fatal(err)
			}
			generatedID, _ := adapted.id()
			costs, err := adventureItemCosts(a, map[string]int{"sg-journal": count})
			if err != nil || costs[generatedID] != -1 {
				t.Fatal("fixture did not establish an actual generated substitute", costs, err)
			}
			*p = *intervalProjectionAfterOps(p, adventureActorOps(a, "fixture-adapted"))
			d.Choices[0].ItemCosts = map[string]int{"sg-journal": count}
			d.Choices[0].ItemHandovers[0].Items = map[string]int{"sg-journal": 1}
			if split {
				p.Characters.Profiles = append(p.Characters.Profiles, character.Profile{ID: "fio", StoryID: "fixture", Name: "菲奥", SourceKey: "sg-p31", Revision: 1})
				setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{"keeper", "fio"})
				d.Choices[0].ItemHandovers = append(d.Choices[0].ItemHandovers, quest.ItemHandover{CharacterSourceKey: "sg-p31", Items: map[string]int{"sg-journal": 1}})
			}
			before, _ := json.Marshal(p)
			_, err = planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: d.Choices[0].ID})
			after, _ := json.Marshal(p)
			if err == nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("generated substitution created an original or spent state: %v", err)
			}
		})
	}
}

func TestFantasyFailedItemHandoverRetainsOriginalButPaysOtherCosts(t *testing.T) {
	r, p, d, i := fantasyHandoverBoundaryFixture(t)
	d.Choices[0].Difficulty = 10
	d.Choices[0].CurrencyCost = 2
	d.Choices[0].ItemCosts["sg-iron"] = 1
	fantasyHandoverBoundaryActor(t, p, DefaultActorID, func(a *AdventureActor) { a.Inventory["sg-iron"] = 1 })
	beforePlayer, err := AdventureActorFromState(p.State, DefaultActorID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: d.Choices[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	view := questProjectionAfter(p, plan)
	player, err := AdventureActorFromState(view.State, DefaultActorID)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := AdventureActorFromState(view.State, "keeper")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Instance.Choices[d.Choices[0].ObjectiveID].Outcome != "failure" || player.Inventory["sg-oath-key"] != 1 || owner.Inventory["sg-oath-key"] != 0 || player.Inventory["sg-iron"] != 0 || player.Currency != beforePlayer.Currency-2 || player.Energy != 97 || plan.ClockAfter.Seconds-plan.ClockBefore.Seconds != 3600 {
		t.Fatal("failure did not preserve the undelivered original while paying consumables, fees and effort")
	}
	for _, op := range plan.Ops {
		if op.ActorID == "keeper" {
			t.Fatal("failed attempt wrote a recipient inventory")
		}
	}
}

func TestFantasyItemHandoverMergesSameRecipientsGiftAndAllowsProvenanceReturn(t *testing.T) {
	for _, mode := range []string{"gift-and-receive", "provenance-return", "once-only"} {
		t.Run(mode, func(t *testing.T) {
			r, p, d, i := fantasyHandoverBoundaryFixture(t)
			if mode == "gift-and-receive" {
				for n := range r.Items {
					if r.Items[n].ID == "sg-old-seal" {
						r.Items[n].Unique = true
					}
				}
				for n := range r.NPCs {
					if r.NPCs[n].CharacterSourceKey == "sg-p12" {
						r.NPCs[n].Actor.Inventory["sg-old-seal"] = 1
					}
				}
				fantasyHandoverBoundaryActor(t, p, "keeper", func(a *AdventureActor) { a.Inventory["sg-old-seal"] = 1 })
				d.Giver = "sg-p12"
				d.Choices[0].Rewards = append(d.Choices[0].Rewards, quest.Reward{Kind: quest.RewardItem, RefID: "sg-old-seal", Quantity: 1})
			}
			if mode == "provenance-return" {
				for n := range r.NPCs {
					if r.NPCs[n].CharacterSourceKey == "sg-p12" {
						r.NPCs[n].Actor.Inventory["sg-oath-key"] = 1
					}
				}
				reserved, err := adventureUniqueItemOwner(r, p.State, "sg-oath-key")
				if err != nil || reserved != "sg-p12" {
					t.Fatal("fixture did not retain the initial provenance reservation", reserved, err)
				}
			}
			before, _ := json.Marshal(p)
			rulesBefore, _ := json.Marshal(r)
			plan, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: d.Choices[0].ID})
			if err != nil {
				t.Fatal(err)
			}
			view := questProjectionAfter(p, plan)
			player, err := AdventureActorFromState(view.State, DefaultActorID)
			if err != nil {
				t.Fatal(err)
			}
			owner, err := AdventureActorFromState(view.State, "keeper")
			if err != nil {
				t.Fatal(err)
			}
			if player.Inventory["sg-oath-key"] != 0 || owner.Inventory["sg-oath-key"] != 1 {
				t.Fatal("handover lost or duplicated the exact original")
			}
			if mode == "gift-and-receive" && (player.Inventory["sg-old-seal"] != 1 || owner.Inventory["sg-old-seal"] != 0) {
				t.Fatal("incoming handover restored the recipient's already gifted item")
			}
			if _, err := planQuestChoice(view, StoryDirectorActorStateSystem{AdventureRules: &r}, d, plan.Instance, true, QuestRequest{ChoiceID: d.Choices[0].ID}); err == nil {
				t.Fatal("new command repeated an already settled original")
			}
			after, _ := json.Marshal(p)
			rulesAfter, _ := json.Marshal(r)
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(rulesBefore, rulesAfter) {
				t.Fatal("planning mutated the input branch or frozen provenance")
			}
		})
	}
}

func TestFantasyFailedHandoverPreservesOnlyDeclaredSubsetAndLegacyCosts(t *testing.T) {
	for _, withHandover := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy-empty", true: "explicit-subset"}[withHandover], func(t *testing.T) {
			r, p, d, i := fantasyHandoverBoundaryFixture(t)
			d.Choices[0].Difficulty = 10
			d.Choices[0].ItemCosts = map[string]int{"sg-journal": 2}
			d.Choices[0].ItemHandovers = nil
			if withHandover {
				d.Choices[0].ItemHandovers = []quest.ItemHandover{{CharacterSourceKey: "sg-p12", Items: map[string]int{"sg-journal": 1}}}
			}
			fantasyHandoverBoundaryActor(t, p, DefaultActorID, func(a *AdventureActor) { a.Inventory["sg-journal"] = 2 })
			plan, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: d.Choices[0].ID})
			if err != nil {
				t.Fatal(err)
			}
			view := questProjectionAfter(p, plan)
			player, err := AdventureActorFromState(view.State, DefaultActorID)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if withHandover {
				want = 1
			}
			if plan.Instance.Choices[d.Choices[0].ObjectiveID].Outcome != "failure" || player.Inventory["sg-journal"] != want {
				t.Fatal("failed attempt preserved more than the declared handover subset or changed legacy costs")
			}
			for _, op := range plan.Ops {
				if op.ActorID == "keeper" {
					t.Fatal("failed subset handover wrote the recipient")
				}
			}
		})
	}
}
