package storypack

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"ta/internal/interactive"
)

func TestAdventureIdentityPreviewMatchesCanonicalStart(t *testing.T) {
	for _, pack := range adventureContentPacks(t) {
		for _, origin := range pack.Creation.Origins {
			t.Run(pack.ID+"/"+origin.ID, func(t *testing.T) {
				identity, err := pack.ResolveIdentity(IdentityChoice{Name: "云舟", Gender: "female", Age: 24, OriginID: origin.ID})
				if err != nil {
					t.Fatal(err)
				}
				template, _ := pack.ProtagonistTemplate(identity.TemplateID)
				raw, _ := json.Marshal(template.State[interactive.AdventureStateField])
				var actor interactive.AdventureActor
				if err := json.Unmarshal(raw, &actor); err != nil {
					t.Fatal(err)
				}
				// New creation resolves the template plus the authored default kit.
				if identity.AdventureState != nil {
					actor = *identity.AdventureState
				}
				initial := identity.InitialState
				if initial == nil || len(initial.Items) == 0 || len(initial.Techniques) == 0 {
					t.Fatalf("adventure preview lost starting possessions and learned skills: %+v", initial)
				}
				if identity.Name != "云舟" || identity.Gender != "female" || identity.OriginID != origin.ID || initial.Currency != actor.Currency || initial.Realm != "" || initial.RealmLabel != "" {
					t.Fatalf("preview changed the selected identity or its formal state: %+v", identity)
				}
				wantItems, wantSkills := []string{}, []string{}
				for _, item := range pack.Story.Gameplay.AdventureRules.Items {
					if actor.Inventory[item.ID] > 0 {
						wantItems = append(wantItems, item.Name)
					}
				}
				for _, skill := range pack.Story.Gameplay.AdventureRules.Skills {
					if actor.Skills[skill.ID] > 0 {
						wantSkills = append(wantSkills, skill.Name)
					}
				}
				if !reflect.DeepEqual(initial.Items, wantItems) || !reflect.DeepEqual(initial.Techniques, wantSkills) {
					t.Fatalf("preview is not the canonical owned/learned catalog: got %v / %v; want %v / %v", initial.Items, initial.Techniques, wantItems, wantSkills)
				}
				encoded, _ := json.Marshal(initial)
				var view map[string]any
				_ = json.Unmarshal(encoded, &view)
				if view["family"] != actor.Family || view["rank"] != float64(actor.Rank) || view["currency_label"] != pack.Story.Gameplay.AdventureRules.Currency {
					t.Fatalf("preview omitted topic-specific display metadata: %s", encoded)
				}
			})
		}
	}
}

func TestAdventureIdentityPreviewUsesCanonicalValuesNotDisplayProjections(t *testing.T) {
	pack := adventureContentPacks(t)[0]
	// This fixture models an older authored rank-three origin with no kit choices.
	creation := *pack.Creation
	creation.AdventureClasses, creation.AdventureWeapons, creation.AdventureMethods = nil, nil, nil
	pack.Creation = &creation
	origin := &pack.Creation.Origins[0]
	template := &pack.ProtagonistTemplates[0]
	if origin.TemplateID != template.ID {
		t.Fatal("fixture origin and template differ")
	}
	actor := template.State[interactive.AdventureStateField].(map[string]any)
	actor["currency"], actor["rank"] = 37, 3
	template.State["钱财"], template.State["历练等级"] = 900, 9
	template.State["当前境界"], template.State["灵石"] = "qi_9", 999
	origin.InitialState = &CreationInitialState{Realm: "qi_9", RealmLabel: "炼气9层", Currency: 999}
	before, _ := json.Marshal(template.State)
	identity, err := pack.ResolveIdentity(IdentityChoice{Name: "云舟", Age: 24, OriginID: origin.ID})
	if err != nil {
		t.Fatal(err)
	}
	initial := identity.InitialState
	if initial.Currency != 37 || initial.Rank != 3 || initial.Realm != "" || initial.RealmLabel != "" {
		t.Fatalf("preview trusted narrative projections instead of canonical state: %+v", initial)
	}
	after, _ := json.Marshal(template.State)
	if string(before) != string(after) {
		t.Fatal("preview rewrote the canonical starting state")
	}
}

func TestAdventureIdentityPreviewLocalizesOnlyDisplayCopy(t *testing.T) {
	for _, pack := range adventureContentPacks(t) {
		for _, origin := range pack.Creation.Origins {
			t.Run(pack.ID+"/"+origin.ID, func(t *testing.T) {
				identity, err := pack.ResolveIdentity(IdentityChoice{Name: "云舟", Age: 24, OriginID: origin.ID})
				if err != nil {
					t.Fatal(err)
				}
				before, _ := json.Marshal(identity)
				shown := pack.LocalizeIdentity(identity, "en-US")
				initial, canonical := shown.InitialState, identity.InitialState
				text := pack.Localizations["en-US"].InitialState
				if text.CurrencyLabel == "" || initial.CurrencyLabel != text.CurrencyLabel || initial.CurrencyLabel == canonical.CurrencyLabel {
					t.Fatalf("currency was not localized: %+v", initial)
				}
				if initial.Family != canonical.Family || initial.Rank != canonical.Rank || initial.Currency != canonical.Currency {
					t.Fatalf("localization changed mechanical values: %+v vs %+v", initial, canonical)
				}
				for i, name := range canonical.Items {
					if text.Items[name] == "" || initial.Items[i] != text.Items[name] {
						t.Fatalf("starting item not localized: %q -> %q", name, initial.Items[i])
					}
				}
				for i, name := range canonical.Techniques {
					if text.Techniques[name] == "" || initial.Techniques[i] != text.Techniques[name] {
						t.Fatalf("starting skill not localized: %q -> %q", name, initial.Techniques[i])
					}
				}
				after, _ := json.Marshal(identity)
				if string(before) != string(after) {
					t.Fatal("display localization rewrote the confirmed identity")
				}
			})
		}
	}
}

func TestAdventureIdentityPreviewRejectsMissingCanonicalState(t *testing.T) {
	pack := adventureContentPacks(t)[0]
	delete(pack.ProtagonistTemplates[0].State, interactive.AdventureStateField)
	_, err := pack.ResolveIdentity(IdentityChoice{Name: "云舟", Age: 24, OriginID: pack.Creation.Origins[0].ID})
	var refusal *IdentityError
	if !errors.As(err, &refusal) || refusal.Key != "api.storyPack.identity.originStateMissing" {
		t.Fatalf("missing canonical state became a zero-valued preview: %v", err)
	}
}
