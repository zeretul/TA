package storypack

import (
	"encoding/json"
	"strings"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"testing"
	"unicode"
)

func TestFantasyPackValidatesMaterializesAndFreezes(t *testing.T) {
	root := t.TempDir()
	library := NewLibrary(root)
	p, err := library.Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Validate(); err != nil {
		t.Fatal(err)
	}
	if err = p.ValidateModules(root); err != nil {
		t.Fatal(err)
	}
	mod, err := interactive.NewActorStateLibrary(root).Get(p.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	if mod.ActorState.AdventureRules == nil || mod.ActorState.AdventureRules.Family != "fantasy" {
		t.Fatal("wrong frozen family")
	}
	if _, ok := findWorldStateField(mod.ActorState, "精力"); !ok {
		t.Fatal("effort field missing")
	}
	for _, bad := range []string{"灵力", "内力", "气力", "境界"} {
		if _, ok := findWorldStateField(mod.ActorState, bad); ok {
			t.Fatalf("foreign field: %s", bad)
		}
	}
	before, _ := json.Marshal(p)
	english := p.Localize("en-US")
	for _, e := range english.World {
		if e.Content == "" || strings.IndexFunc(e.Content, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
			t.Fatalf("untranslated lore %s", e.ID)
		}
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("localization mutated canonical content")
	}
	frozen, err := p.FrozenGameplay()
	if err != nil {
		t.Fatal(err)
	}
	if frozen.AdventureRules.Skills[0].Localizations == nil || frozen.AdventureRules.Recipes[0].Localizations == nil {
		t.Fatal("definition translations were not frozen")
	}
	p.Story.Gameplay.Combat = "wuxia"
	if p.Validate() == nil {
		t.Fatal("cross-family combat accepted")
	}
}

func TestFantasyEveryOriginEnglishPreview(t *testing.T) {
	p, err := NewLibrary(t.TempDir()).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range p.Creation.Origins {
		t.Run(o.ID, func(t *testing.T) {
			identity, e := p.ResolveIdentity(IdentityChoice{Name: "Rowan", Age: 24, OriginID: o.ID})
			if e != nil {
				t.Fatal(e)
			}
			before, _ := json.Marshal(identity)
			shown := p.LocalizeIdentity(identity, "en-US")
			if shown.InitialState == nil || shown.InitialState.Family != "fantasy" || shown.InitialState.Currency != identity.InitialState.Currency {
				t.Fatal("preview changed authored start")
			}
			texts := append([]string{shown.OriginName, shown.OriginText, shown.InitialState.CurrencyLabel}, shown.InitialState.Items...)
			texts = append(texts, shown.InitialState.Techniques...)
			texts = append(texts, shown.InitialState.Equipped...)
			for _, text := range texts {
				if strings.IndexFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
					t.Fatalf("English preview leaked Chinese: %s", text)
				}
			}
			after, _ := json.Marshal(identity)
			if string(before) != string(after) {
				t.Fatal("translation changed canonical identity")
			}
		})
	}
}

func TestFantasyAllDefinitionsFreezeCompleteIndependentProse(t *testing.T) {
	p, err := NewLibrary(t.TempDir()).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := p.FrozenGameplay()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	check := func(id string, details cultivation.DefinitionDetails) {
		t.Helper()
		count++
		if err := details.ValidateComplete(); err != nil {
			t.Fatal(id, err)
		}
		if details.Localizations == nil || details.Localizations.English.Name == "" || details.Localizations.English.Description == "" || details.Localizations.English.Background == "" {
			t.Fatal("incomplete frozen independent English", id)
		}
	}
	for _, d := range frozen.AdventureRules.Skills {
		check(d.ID, d.DefinitionDetails)
	}
	for _, d := range frozen.AdventureRules.Items {
		check(d.ID, d.DefinitionDetails)
	}
	for _, d := range frozen.AdventureRules.Recipes {
		check(d.ID, d.DefinitionDetails)
	}
	if count != 214 {
		t.Fatal("unexpected catalog size", count)
	}
}
