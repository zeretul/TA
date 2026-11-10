package storypack

import (
	"encoding/json"
	"reflect"
	"strings"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"testing"
)

func TestQualityDefinitionSidecarsStayOutsideCanonicalPack(t *testing.T) {
	for _, pack := range builtinPacks() {
		if pack.ID != "fanren-jie" && pack.ID != "lanchuan-qunxia" && pack.ID != "xinghai-689" {
			continue
		}
		before, _ := json.Marshal(pack)
		if strings.Contains(string(before), `"localizations"`) {
			t.Fatalf("inline translation in canonical %s", pack.ID)
		}
		frozen, err := pack.FrozenGameplay()
		if err != nil {
			t.Fatal(err)
		}
		if pack.ID == "fanren-jie" {
			for id, d := range frozen.DefinitionDetails {
				if d.Localizations == nil || d.Localizations.English != pack.Localizations["en-US"].Definitions[id] {
					t.Fatalf("sidecar not frozen %s", id)
				}
				if pack.Story.Gameplay.DefinitionDetails[id].Localizations != nil {
					t.Fatal("canonical method acquired inline English")
				}
			}
		} else {
			module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.DefaultActorStateModuleID)
			if err != nil {
				t.Fatal(err)
			}
			canonical := buildWorldActorStateModule(module, "test", pack.Story.Gameplay)
			for _, skill := range canonical.ActorState.AdventureRules.Skills {
				if skill.Localizations != nil {
					t.Fatal("reusable mechanics module acquired English")
				}
			}
			for i, skill := range frozen.AdventureRules.Skills {
				if skill.Localizations == nil || skill.Localizations.English != pack.Localizations["en-US"].Definitions[skill.ID] {
					t.Fatalf("skill sidecar not frozen %s", skill.ID)
				}
				frozen.AdventureRules.Skills[i].Localizations.English.Description = "mutated copy"
			}
		}
		after, _ := json.Marshal(pack)
		if string(before) != string(after) {
			t.Fatalf("freeze mutated canonical %s", pack.ID)
		}
	}
}

func TestQualityDefinitionSidecarEditAffectsOnlyNextFrozenCopy(t *testing.T) {
	pack := builtinPacks()[0]
	before, err := pack.FrozenGameplay()
	if err != nil {
		t.Fatal(err)
	}
	hash := pack.ContentHash()
	locale := pack.Localizations["en-US"]
	original := locale.Definitions["fire_bolt"]
	updated := original
	updated.Description = "Revised independent English prose"
	locale.Definitions["fire_bolt"] = updated
	locale.Definitions["not-a-definition"] = cultivation.DefinitionProse{Description: "private unknown metadata"}
	pack.Localizations["en-US"] = locale
	after, err := pack.FrozenGameplay()
	if err != nil {
		t.Fatal(err)
	}
	if pack.ContentHash() != hash {
		t.Fatal("sidecar edit changed canonical content fingerprint")
	}
	if before.DefinitionDetails["fire_bolt"].Localizations.English != original || after.DefinitionDetails["fire_bolt"].Localizations.English != updated {
		t.Fatal("sidecar did not freeze independently")
	}
	if _, ok := after.DefinitionDetails["not-a-definition"]; ok {
		t.Fatal("unknown sidecar ID created a definition")
	}
	if !reflect.DeepEqual(before.DefinitionDetails["fire_bolt"].ForLocale("zh-CN"), after.DefinitionDetails["fire_bolt"].ForLocale("zh-CN")) {
		t.Fatal("English edit changed canonical grade or prose")
	}
	// A localized display copy still must not write a translation map into Pack.
	localized := pack.Localize("en-US")
	raw, _ := json.Marshal(localized)
	if strings.Contains(string(raw), `"localizations"`) {
		t.Fatal("display copy smuggled definition locales into Pack")
	}
}

func TestQualityPackAuthorValidationRejectsInlineDefinitionTranslations(t *testing.T) {
	for _, pack := range builtinPacks() {
		if pack.ID != "fanren-jie" && pack.ID != "lanchuan-qunxia" && pack.ID != "xinghai-689" {
			continue
		}
		if err := pack.Validate(); err != nil {
			t.Fatal(err)
		}
		if pack.Story.Gameplay.AdventureRules != nil {
			pack.Story.Gameplay.AdventureRules.Skills[0].Localizations = &cultivation.DefinitionLocalizations{English: cultivation.DefinitionProse{Description: "inline translation"}}
		} else {
			d := pack.Story.Gameplay.DefinitionDetails["fire_bolt"]
			d.Localizations = &cultivation.DefinitionLocalizations{English: cultivation.DefinitionProse{Description: "inline translation"}}
			pack.Story.Gameplay.DefinitionDetails["fire_bolt"] = d
		}
		if err := pack.Validate(); err == nil {
			t.Fatalf("accepted inline definition English as canonical author content: %s", pack.ID)
		}
	}
}
