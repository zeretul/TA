package storypack

import (
	"encoding/json"
	"reflect"
	"ta/internal/interactive"
	"testing"
)

func adventureIdentityChoice(origin string) IdentityChoice {
	return IdentityChoice{Name: "行舟", Gender: "female", Age: 23, OriginID: origin, AffiliationID: "none", LocationID: "auto"}
}

func TestAdventureCreationChoicesGrantConcreteStarts(t *testing.T) {
	total := 0
	for _, p := range adventureContentPacks(t) {
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
		r := *p.Story.Gameplay.AdventureRules
		for _, origin := range p.Creation.Origins {
			templateID := origin.TemplateID
			if templateID == "" {
				templateID = origin.DefaultTemplateID
			}
			template, ok := p.ProtagonistTemplate(templateID)
			if !ok {
				t.Fatalf("missing base for %s", origin.ID)
			}
			raw, _ := json.Marshal(template.State[interactive.AdventureStateField])
			var base interactive.AdventureActor
			_ = json.Unmarshal(raw, &base)
			var choices []IdentityChoice
			if r.Family == "space" {
				for _, o := range p.Creation.AdventureClasses {
					c := adventureIdentityChoice(origin.ID)
					c.AdventureClassID = o.ID
					choices = append(choices, c)
				}
			} else {
				for _, w := range p.Creation.AdventureWeapons {
					for _, m := range p.Creation.AdventureMethods {
						c := adventureIdentityChoice(origin.ID)
						c.AdventureWeaponID = w.ID
						c.AdventureMethodID = m.ID
						choices = append(choices, c)
					}
				}
			}
			for _, choice := range choices {
				t.Run(origin.ID+choice.AdventureClassID+choice.AdventureWeaponID+choice.AdventureMethodID, func(t *testing.T) {
					identity, err := p.ResolveIdentity(choice)
					if err != nil {
						t.Fatal(err)
					}
					total++
					if identity.AdventureState == nil {
						t.Fatal("choice did not resolve to an actual actor")
					}
					a := *identity.AdventureState
					if a.Rank != base.Rank || a.Health != base.Health || a.MaxHealth != base.MaxHealth || a.Energy != base.Energy || a.MaxEnergy != base.MaxEnergy || a.Currency != base.Currency || !reflect.DeepEqual(a.OpeningHazards, base.OpeningHazards) {
						t.Fatal("choice rewrote origin vitality, money or actual opening danger")
					}
					for id, n := range base.Inventory {
						if a.Inventory[id] < n {
							t.Fatalf("origin possession removed: %s", id)
						}
					}
					for id, n := range base.Skills {
						if a.Skills[id] < n {
							t.Fatalf("origin experience removed: %s", id)
						}
					}
					ids := map[string]bool{identity.AdventureClassID: true, identity.AdventureWeaponID: true, identity.AdventureMethodID: true}
					budget := 0
					for _, kit := range r.StartingLoadouts {
						if !ids[kit.ID] {
							continue
						}
						for _, id := range kit.Skills {
							if a.Skills[id] < 1 {
								t.Fatal("chosen skill was only a label")
							}
						}
						for slot, id := range kit.Equipment {
							if a.Equipment[slot] != id || a.Inventory[id] < 1 {
								t.Fatal("chosen equipment not actually equipped and owned")
							}
						}
						for id, n := range kit.Items {
							for _, item := range r.Items {
								if item.ID == id {
									budget += item.Price * n
								}
							}
						}
					}
					want := 130
					if r.Family == "wuxia" {
						want = 160
					}
					if budget != want {
						t.Fatalf("unequal choice grant budget: %d want %d", budget, want)
					}
					copyTemplate := template
					copyTemplate.State = map[string]any{interactive.AdventureStateField: a}
					preview, err := p.identityInitialState(copyTemplate)
					if err != nil || !reflect.DeepEqual(preview, identity.InitialState) {
						t.Fatalf("preview diverges from committed canonical actor: %v", err)
					}
					if err := interactive.ValidateAdventureActor(r, a); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
	if total != 120 {
		t.Fatalf("combination coverage %d, want 120", total)
	}
}

func TestAdventureCreationChoicesRejectUnknownIDsAndPreserveLegacyOrigins(t *testing.T) {
	for _, p := range adventureContentPacks(t) {
		c := adventureIdentityChoice(p.Creation.Origins[0].ID)
		for _, change := range []func(*IdentityChoice){func(c *IdentityChoice) { c.AdventureClassID = "invented" }, func(c *IdentityChoice) { c.AdventureWeaponID = "invented" }, func(c *IdentityChoice) { c.AdventureMethodID = "invented" }} {
			bad := c
			change(&bad)
			if _, err := p.ResolveIdentity(bad); err == nil {
				t.Fatal("unknown nonempty choice silently became a default")
			}
		}
		got, err := p.ResolveIdentity(c)
		if err != nil || got.AdventureState == nil {
			t.Fatalf("new starts cannot use authored defaults: %v", err)
		}
		old := p
		creation := *p.Creation
		creation.AdventureClasses = nil
		creation.AdventureWeapons = nil
		creation.AdventureMethods = nil
		old.Creation = &creation
		legacy, err := old.ResolveIdentity(c)
		if err != nil || legacy.AdventureState != nil {
			t.Fatalf("old metadata gained a modern loadout: %v", err)
		}
		template, _ := old.ProtagonistTemplate(legacy.TemplateID)
		before, _ := old.identityInitialState(template)
		if !reflect.DeepEqual(before, legacy.InitialState) {
			t.Fatal("legacy initial inventory changed")
		}
		raw, _ := json.Marshal(c)
		var wire map[string]any
		_ = json.Unmarshal(raw, &wire)
		wire["adventure_state"] = map[string]any{"rank": 60, "currency": 999999}
		raw, _ = json.Marshal(wire)
		var tampered IdentityChoice
		_ = json.Unmarshal(raw, &tampered)
		safe, err := p.ResolveIdentity(tampered)
		if err != nil || safe.AdventureState.Rank != 1 || safe.AdventureState.Currency != got.AdventureState.Currency {
			t.Fatal("client-supplied numbers reached creation")
		}
	}
}

func TestAdventureCreationEnglishAndKitRulesFreezeSeparatelyFromLabels(t *testing.T) {
	for _, p := range adventureContentPacks(t) {
		localized := p.Localize("en-US")
		for key, options := range map[string][]CreationOption{"class": localized.Creation.AdventureClasses, "weapon": localized.Creation.AdventureWeapons, "method": localized.Creation.AdventureMethods} {
			for _, option := range options {
				if option.Name == "" || option.Description == "" {
					t.Errorf("%s untranslated %s", key, option.ID)
				}
			}
		}
		c := adventureIdentityChoice(p.Creation.Origins[0].ID)
		identity, err := p.ResolveIdentity(c)
		if err != nil {
			t.Fatal(err)
		}
		shown := p.LocalizeIdentity(identity, "en-US")
		if shown.AdventureTrainingNames() == identity.AdventureTrainingNames() {
			t.Fatal("creation names were not localized")
		}
		before := p.ContentHash()
		// The numeric kit belongs to gameplay, not the otherwise cosmetic creation labels.
		raw, _ := json.Marshal(p)
		var changed Pack
		_ = json.Unmarshal(raw, &changed)
		for id := range changed.Story.Gameplay.AdventureRules.StartingLoadouts[0].Items {
			changed.Story.Gameplay.AdventureRules.StartingLoadouts[0].Items[id]++
			break
		}
		after := changed.ContentHash()
		if before == after {
			t.Fatal("loadout mechanics omitted from content hash")
		}
		if _, err = interactive.ApplyAdventureStartingLoadouts(*p.Story.Gameplay.AdventureRules, *identity.AdventureState, "missing"); err == nil {
			t.Fatal("unknown grant succeeded")
		}
	}
}

func TestAdventureCreationCrossTrainingUsesPurchasedSourcesAndRealTime(t *testing.T) {
	for _, p := range adventureContentPacks(t) {
		c := adventureIdentityChoice(p.Creation.Origins[0].ID)
		if p.ID == "xinghai-689" {
			c.AdventureClassID = "xinghai-class-pugilist"
		} else {
			c.AdventureWeaponID = "lanchuan-weapon-blade"
			c.AdventureMethodID = "lanchuan-method-restorative-breath"
		}
		identity, err := p.ResolveIdentity(c)
		if err != nil {
			t.Fatal(err)
		}
		a := *identity.AdventureState
		r := *p.Story.Gameplay.AdventureRules
		target := "xinghai-starter-mental-pulse"
		if r.Family == "wuxia" {
			target = "lanchuan-starter-sword-form"
		}
		var skill interactive.AdventureSkill
		for _, s := range r.Skills {
			if s.ID == target {
				skill = s
			}
		}
		if _, _, err = interactive.ResolveAdventureAction(r, a, interactive.AdventureAction{Action: "learn", SkillID: target, Hours: skill.TrainingHours}, "no-source"); err == nil {
			t.Fatal("cross-training created a source from a class label")
		}
		market := r.Markets[0]
		for _, m := range r.Markets {
			if m.WorkService != nil && m.WorkService.RequiredSkill == "" {
				market = m
				break
			}
		}
		for a.Currency < 20 {
			a, _, err = interactive.ResolveAdventureAction(r, a, interactive.AdventureAction{Action: "work", MarketID: market.ID, Hours: 1}, "earn-book")
			if err != nil {
				t.Fatal(err)
			}
		}
		before := a.Currency
		a, out, err := interactive.ResolveAdventureAction(r, a, interactive.AdventureAction{Action: "buy", MarketID: market.ID, ItemID: skill.RequiredItem, Quantity: 1}, "buy-book")
		if err != nil || a.Currency != before-20 {
			t.Fatalf("source purchase failed: %+v %v", out, err)
		}
		a, out, err = interactive.ResolveAdventureAction(r, a, interactive.AdventureAction{Action: "learn", SkillID: target, Hours: skill.TrainingHours}, "cross-learn")
		if err != nil || a.Skills[target] != 1 || out.DurationSeconds != int64(skill.TrainingHours*3600) {
			t.Fatalf("initial choice became a permanent class lock: %+v %v", out, err)
		}
	}
}
